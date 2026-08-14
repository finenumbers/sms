package admin

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"finenumbers/sms/internal/audit"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/inventory"
	"finenumbers/sms/internal/runexis"
	"finenumbers/sms/internal/settings"
)

const maxUploadBytes = 4 << 20

type assignRequest struct {
	ClientID uuid.UUID `json:"client_id"`
}

type patchNumberRequest struct {
	Region *string `json:"region"`
	Notes  *string `json:"notes"`
	Status *string `json:"status"`
}

func (h *Handlers) UploadNumbers(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Inventory == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "inventory unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "file required")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "file field required")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "cannot read file")
		return
	}
	if len(raw) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "empty file")
		return
	}
	rep, err := h.Inventory.Upload(r.Context(), raw)
	if err != nil {
		h.Log.Error("upload numbers", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		Action:       "number.upload",
		ResourceType: "def_number",
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata: map[string]any{
			"imported":   rep.Imported,
			"duplicates": rep.Duplicates,
			"invalid":    len(rep.Invalid),
			"encoding":   rep.Encoding,
		},
	})
	httpx.WriteJSON(w, http.StatusOK, rep)
}

func (h *Handlers) SyncNumbers(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Inventory == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "inventory unavailable")
		return
	}
	if h.Limiter == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "rate limiter unavailable")
		return
	}
	const lockKey = "numbers:sync"
	okLock, err := h.Limiter.TryLock(r.Context(), lockKey, 90*time.Second)
	if err != nil {
		h.Log.Error("numbers sync lock", "err", err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "rate limiter unavailable")
		return
	}
	if !okLock {
		httpx.WriteError(w, http.StatusConflict, "sync_in_progress", "number sync already in progress")
		return
	}
	defer func() { _ = h.Limiter.Unlock(r.Context(), lockKey) }()

	rep, err := h.Inventory.SyncFromProvider(r.Context())
	if err != nil {
		code := "runexis_error"
		status := http.StatusBadGateway
		msg := "runexis request failed"
		switch {
		case errors.Is(err, runexis.ErrNotConfigured), errors.Is(err, settings.ErrNotConfigured):
			status = http.StatusConflict
			code = "not_configured"
			msg = "runexis credentials are not configured"
		case errors.Is(err, settings.ErrDecrypt):
			status = http.StatusConflict
			code = "decrypt_failed"
			msg = "не удалось расшифровать секрет; введите его снова после смены ключа"
		default:
			var apiErr *runexis.APIError
			if errors.As(err, &apiErr) && apiErr.Message != "" {
				msg = apiErr.Message
			}
		}
		h.Log.Error("sync numbers", "err", err)
		httpx.WriteError(w, status, code, msg)
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		Action:       "number.sync",
		ResourceType: "def_number",
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata: map[string]any{
			"fetched":         rep.Fetched,
			"sms_ok":          rep.SMSOk,
			"imported":        rep.Imported,
			"updated":         rep.Updated,
			"skipped_no_sms":  rep.SkippedNoSMS,
			"skipped_invalid": rep.SkippedInvalid,
			"truncated":       rep.Truncated,
			"errors":          len(rep.Errors),
		},
	})
	httpx.WriteJSON(w, http.StatusOK, rep)
}

func (h *Handlers) ListNumbers(w http.ResponseWriter, r *http.Request) {
	if h.Inventory == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "inventory unavailable")
		return
	}
	limit, offset := queryPage(r)
	f := inventory.ListFilter{Limit: limit, Offset: offset, Q: strings.TrimSpace(r.URL.Query().Get("q"))}
	if raw := r.URL.Query().Get("status"); raw != "" {
		st := sqlcdb.DefNumberStatus(raw)
		switch st {
		case sqlcdb.DefNumberStatusInventory, sqlcdb.DefNumberStatusAssigned, sqlcdb.DefNumberStatusDisabled:
			f.Status = &st
		default:
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid status")
			return
		}
	}
	if raw := r.URL.Query().Get("client_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid client_id")
			return
		}
		f.ClientID = &id
	}
	items, err := h.Inventory.List(r.Context(), f)
	if err != nil {
		h.Log.Error("list numbers", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, n := range items {
		out = append(out, numberJSON(n.ID, n.Msisdn, n.Status, n.Region, n.Notes, n.SupportsSms, n.CreatedAt, n.UpdatedAt, n.AssignmentID, n.ClientID, n.AssignedAt, n.ClientName))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handlers) GetNumber(w http.ResponseWriter, r *http.Request) {
	if h.Inventory == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "inventory unavailable")
		return
	}
	id, ok := pathUUID(w, r, "numberID")
	if !ok {
		return
	}
	n, err := h.Inventory.Get(r.Context(), id)
	if err != nil {
		writeInventoryErr(w, h, "get number", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, numberJSON(n.ID, n.Msisdn, n.Status, n.Region, n.Notes, n.SupportsSms, n.CreatedAt, n.UpdatedAt, n.AssignmentID, n.ClientID, n.AssignedAt, n.ClientName))
}

func (h *Handlers) PatchNumber(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Inventory == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "inventory unavailable")
		return
	}
	id, ok := pathUUID(w, r, "numberID")
	if !ok {
		return
	}
	var req patchNumberRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	var n sqlcdb.GetDefNumberViewRow
	var err error
	if req.Status != nil {
		st := sqlcdb.DefNumberStatus(strings.TrimSpace(*req.Status))
		n, err = h.Inventory.SetStatus(r.Context(), id, st)
		if err != nil {
			writeInventoryErr(w, h, "patch number status", err)
			return
		}
	}
	if req.Region != nil || req.Notes != nil {
		n, err = h.Inventory.UpdateMeta(r.Context(), id, inventory.UpdateMetaInput{Region: req.Region, Notes: req.Notes})
		if err != nil {
			writeInventoryErr(w, h, "patch number", err)
			return
		}
	}
	if req.Status == nil && req.Region == nil && req.Notes == nil {
		n, err = h.Inventory.Get(r.Context(), id)
		if err != nil {
			writeInventoryErr(w, h, "patch number", err)
			return
		}
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		Action:       "number.update",
		ResourceType: "def_number",
		ResourceID:   &n.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
	})
	httpx.WriteJSON(w, http.StatusOK, numberJSON(n.ID, n.Msisdn, n.Status, n.Region, n.Notes, n.SupportsSms, n.CreatedAt, n.UpdatedAt, n.AssignmentID, n.ClientID, n.AssignedAt, n.ClientName))
}

func (h *Handlers) AssignNumber(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Inventory == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "inventory unavailable")
		return
	}
	id, ok := pathUUID(w, r, "numberID")
	if !ok {
		return
	}
	var req assignRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	if req.ClientID == uuid.Nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "client_id required")
		return
	}
	out, err := h.Inventory.Assign(r.Context(), id, req.ClientID, *p.AdminUserID)
	if err != nil {
		writeInventoryErr(w, h, "assign number", err)
		return
	}
	n := out.Number
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &req.ClientID,
		Action:       "number.assign",
		ResourceType: "def_number",
		ResourceID:   &n.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"msisdn": n.Msisdn, "assignment_id": out.Assignment.ID},
	})
	httpx.WriteJSON(w, http.StatusOK, numberJSON(n.ID, n.Msisdn, n.Status, n.Region, n.Notes, n.SupportsSms, n.CreatedAt, n.UpdatedAt, n.AssignmentID, n.ClientID, n.AssignedAt, n.ClientName))
}

func (h *Handlers) UnassignNumber(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Inventory == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "inventory unavailable")
		return
	}
	id, ok := pathUUID(w, r, "numberID")
	if !ok {
		return
	}
	n, clientID, err := h.Inventory.Unassign(r.Context(), id)
	if err != nil {
		writeInventoryErr(w, h, "unassign number", err)
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &clientID,
		Action:       "number.unassign",
		ResourceType: "def_number",
		ResourceID:   &n.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"msisdn": n.Msisdn},
	})
	httpx.WriteJSON(w, http.StatusOK, numberJSON(n.ID, n.Msisdn, n.Status, n.Region, n.Notes, n.SupportsSms, n.CreatedAt, n.UpdatedAt, n.AssignmentID, n.ClientID, n.AssignedAt, n.ClientName))
}

func writeInventoryErr(w http.ResponseWriter, h *Handlers, op string, err error) {
	switch {
	case errors.Is(err, inventory.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, inventory.ErrValidation):
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
	case errors.Is(err, inventory.ErrAlreadyAssigned), errors.Is(err, inventory.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, inventory.ErrNotAssigned):
		httpx.WriteError(w, http.StatusConflict, "not_assigned", "number is not assigned")
	case errors.Is(err, inventory.ErrNotAssignable):
		httpx.WriteError(w, http.StatusConflict, "not_assignable", err.Error())
	case errors.Is(err, inventory.ErrClientNotActive):
		httpx.WriteError(w, http.StatusConflict, "client_not_active", "client is not active")
	default:
		h.Log.Error(op, "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

func numberJSON(
	id uuid.UUID,
	msisdn string,
	status sqlcdb.DefNumberStatus,
	region, notes *string,
	supportsSMS bool,
	createdAt, updatedAt time.Time,
	assignmentID, clientID *uuid.UUID,
	assignedAt *time.Time,
	clientName *string,
) map[string]any {
	m := map[string]any{
		"id":           id,
		"msisdn":       msisdn,
		"status":       status,
		"region":       region,
		"notes":        notes,
		"supports_sms": supportsSMS,
		"created_at":   createdAt.UTC().Format(time.RFC3339),
		"updated_at":   updatedAt.UTC().Format(time.RFC3339),
	}
	if assignmentID != nil {
		m["assignment_id"] = assignmentID
	}
	if clientID != nil {
		m["client_id"] = clientID
	}
	if assignedAt != nil {
		m["assigned_at"] = assignedAt.UTC().Format(time.RFC3339)
	}
	if clientName != nil {
		m["client_name"] = clientName
	}
	return m
}

package lookup

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/smsc"
)

const callbackNotFoundTTL = time.Hour

type IncomingCallback struct {
	ProviderMessageID string
	PhoneDigits       string
	Normalized        smsc.NormalizedResult
	SkipEnrich        bool
}

type IncomingResult struct {
	Applied   bool
	Duplicate bool
	Reason    string
	Item      sqlcdb.LookupItem
}

func callbackPhonesMatch(itemDigits, callbackDigits string) bool {
	if callbackDigits == "" {
		return true
	}
	return itemDigits == callbackDigits
}

func (w *Worker) ApplyIncoming(ctx context.Context, in IncomingCallback) (IncomingResult, error) {
	if w == nil || w.store == nil || in.ProviderMessageID == "" {
		return IncomingResult{Reason: "not_found"}, nil
	}
	items, err := w.store.Queries.ListLookupItemsByProviderMessage(ctx, sqlcdb.ListLookupItemsByProviderMessageParams{
		ProviderCode:      smsc.ProviderCode,
		ProviderMessageID: &in.ProviderMessageID,
	})
	if err != nil {
		return IncomingResult{}, err
	}
	if len(items) == 0 {
		return IncomingResult{Reason: "not_found"}, nil
	}
	if len(items) > 1 {
		return IncomingResult{Reason: "ambiguous"}, nil
	}
	item := items[0]
	if !callbackPhonesMatch(item.PhoneDigits, in.PhoneDigits) {
		return IncomingResult{Reason: "phone_mismatch", Item: item}, nil
	}
	applied, err := w.ApplyProviderUpdate(ctx, ApplyInput{
		JobItemID:         item.ID,
		ClientID:          item.ClientID,
		ProviderMessageID: in.ProviderMessageID,
		Normalized:        in.Normalized,
		Source:            "callback",
		SkipEnrich:        in.SkipEnrich,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return IncomingResult{Reason: "not_found"}, nil
		}
		return IncomingResult{}, err
	}
	return IncomingResult{
		Applied:   applied.Applied || applied.BecameTerminal,
		Duplicate: applied.Duplicate,
		Item:      applied.Item,
	}, nil
}

func (w *Worker) applyStoredCallbacks(ctx context.Context, deadline time.Time) (int, error) {
	if w.store == nil {
		return 0, nil
	}
	rows, err := w.store.Queries.ListUnprocessedProviderLookupCallbacks(ctx, 20)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range rows {
		if !canStartLookupIO(w.now(), deadline) {
			break
		}
		if w.applyStoredCallback(ctx, rows[i]) {
			n++
		}
	}
	return n, nil
}

func (w *Worker) ConcludeCallback(ctx context.Context, callbackID uuid.UUID, res IncomingResult) error {
	if w == nil || w.store == nil || callbackID == uuid.Nil {
		return nil
	}
	var processErr *string
	if res.Reason != "" && !res.Applied && !res.Duplicate {
		processErr = &res.Reason
	}
	var itemID *uuid.UUID
	var clientID *uuid.UUID
	if res.Item.ID != uuid.Nil {
		itemID = &res.Item.ID
		clientID = &res.Item.ClientID
	}
	return w.store.Queries.MarkProviderLookupCallbackProcessed(ctx, sqlcdb.MarkProviderLookupCallbackProcessedParams{
		ProcessError: processErr,
		JobItemID:    itemID,
		ClientID:     clientID,
		ID:           callbackID,
	})
}

func (w *Worker) applyStoredCallback(ctx context.Context, row sqlcdb.ProviderLookupCallback) bool {
	id := ""
	if row.ProviderMessageID != nil {
		id = *row.ProviderMessageID
	}
	var normalized smsc.NormalizedResult
	if len(row.NormalizedResult) > 0 {
		_ = json.Unmarshal(row.NormalizedResult, &normalized)
	}
	phone := ""
	if obj, ok := rawObject(row.RawPayload); ok {
		phone = smsc.ToPhoneDigits(asCallbackPhone(obj))
		if id == "" {
			id = asStringAny(obj["id"])
		}
	}
	res, err := w.ApplyIncoming(ctx, IncomingCallback{
		ProviderMessageID: id,
		PhoneDigits:       phone,
		Normalized:        normalized,
	})
	if err != nil {
		if w.log != nil {
			w.log.Error("lookup callback apply", "callback_id", row.ID, "err", err)
		}
		return false
	}
	if res.Reason == "not_found" && w.now().Sub(row.CreatedAt) < callbackNotFoundTTL {
		return false
	}
	var processErr *string
	if res.Reason != "" && !res.Applied && !res.Duplicate {
		processErr = &res.Reason
	}
	var itemID *uuid.UUID
	var clientID *uuid.UUID
	if res.Item.ID != uuid.Nil {
		itemID = &res.Item.ID
		clientID = &res.Item.ClientID
	}
	if err := w.store.Queries.MarkProviderLookupCallbackProcessed(ctx, sqlcdb.MarkProviderLookupCallbackProcessedParams{
		ProcessError: processErr,
		JobItemID:    itemID,
		ClientID:     clientID,
		ID:           row.ID,
	}); err != nil && w.log != nil {
		w.log.Error("lookup callback mark processed", "callback_id", row.ID, "err", err)
		return false
	}
	return res.Applied || res.Duplicate
}

func rawObject(raw []byte) (map[string]any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil || m == nil {
		return nil, false
	}
	return m, true
}

func asCallbackPhone(obj map[string]any) string {
	if obj == nil {
		return ""
	}
	if v, ok := obj["phone"]; ok {
		return asStringAny(v)
	}
	return ""
}

func asStringAny(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

package lookup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
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
	a := CallbackPhoneDigits(itemDigits)
	b := CallbackPhoneDigits(callbackDigits)
	if a == b {
		return true
	}
	return phoneTail(a) != "" && phoneTail(a) == phoneTail(b)
}

func phoneTail(digits string) string {
	if len(digits) < 10 {
		return ""
	}
	return digits[len(digits)-10:]
}

func pickCallbackItems(byID, byPhone []sqlcdb.LookupItem) (sqlcdb.LookupItem, string) {
	switch {
	case len(byID) > 1:
		return sqlcdb.LookupItem{}, "ambiguous"
	case len(byID) == 1:
		return byID[0], ""
	case len(byPhone) > 1:
		return sqlcdb.LookupItem{}, "ambiguous"
	case len(byPhone) == 1:
		return byPhone[0], ""
	default:
		return sqlcdb.LookupItem{}, "not_found"
	}
}

// CallbackPhoneDigits normalizes a callback phone the same way create stores
// lookup_items.phone_digits (digits only, RU 8→7 / 9XXXXXXXXX→79).
func CallbackPhoneDigits(raw string) string {
	return smsc.CanonicalPhoneDigits(raw)
}

// ShouldConcludeCallback is true only when the item was actually updated.
// not_found / ambiguous / empty reason must stay unprocessed for worker replay.
func ShouldConcludeCallback(res IncomingResult) bool {
	return res.Applied || res.Duplicate
}

func shouldMarkStoredCallback(res IncomingResult, age time.Duration) bool {
	if ShouldConcludeCallback(res) {
		return true
	}
	if res.Reason == "ambiguous" {
		return false
	}
	if res.Reason == "not_found" || res.Reason == "item_not_found" || res.Reason == "" || res.Reason == "phone_mismatch" || res.Reason == "lifecycle_unmapped" {
		return age >= callbackNotFoundTTL
	}
	return false
}

func (w *Worker) callbackCreatedAfter() time.Time {
	now := time.Now().UTC()
	if w != nil && w.now != nil {
		now = w.now()
	}
	return now.Add(-callbackNotFoundTTL)
}

func (w *Worker) lookupCallbackItems(ctx context.Context, in IncomingCallback) ([]sqlcdb.LookupItem, []sqlcdb.LookupItem, error) {
	var byID []sqlcdb.LookupItem
	if in.ProviderMessageID != "" {
		rows, err := w.store.Queries.ListLookupItemsByProviderMessage(ctx, sqlcdb.ListLookupItemsByProviderMessageParams{
			ProviderCode:      smsc.ProviderCode,
			ProviderMessageID: &in.ProviderMessageID,
		})
		if err != nil {
			return nil, nil, err
		}
		byID = rows
		if len(byID) == 0 {
			rows, err = w.store.Queries.ListOpenLookupItemsForCallbackSendID(ctx, sqlcdb.ListOpenLookupItemsForCallbackSendIDParams{
				CreatedAfter: w.callbackCreatedAfter(),
				CallbackID:   in.ProviderMessageID,
			})
			if err != nil {
				if w.log != nil {
					w.log.Error("lookup callback send-id", "id", in.ProviderMessageID, "err", err)
				}
			} else {
				byID = rows
			}
		}
	}
	var byPhone []sqlcdb.LookupItem
	if len(byID) == 0 && in.PhoneDigits != "" {
		canon := CallbackPhoneDigits(in.PhoneDigits)
		rows, err := w.store.Queries.ListOpenLookupItemsForCallbackPhone(ctx, sqlcdb.ListOpenLookupItemsForCallbackPhoneParams{
			PhoneDigits:       canon,
			CreatedAfter:      w.callbackCreatedAfter(),
			ProviderMessageID: &in.ProviderMessageID,
		})
		if err != nil {
			return nil, nil, err
		}
		byPhone = rows
		if len(byPhone) == 0 {
			if tail := phoneTail(canon); tail != "" {
				rows, err = w.store.Queries.ListOpenLookupItemsForCallbackPhoneTail(ctx, sqlcdb.ListOpenLookupItemsForCallbackPhoneTailParams{
					PhoneTail:         tail,
					CreatedAfter:      w.callbackCreatedAfter(),
					ProviderMessageID: &in.ProviderMessageID,
				})
				if err != nil {
					return nil, nil, err
				}
				byPhone = rows
			}
		}
	}
	return byID, byPhone, nil
}

func (w *Worker) ApplyIncoming(ctx context.Context, in IncomingCallback) (IncomingResult, error) {
	if w == nil || w.store == nil || (in.ProviderMessageID == "" && in.PhoneDigits == "") {
		return IncomingResult{Reason: "not_found"}, nil
	}
	byID, byPhone, err := w.lookupCallbackItems(ctx, in)
	if err != nil {
		return IncomingResult{}, err
	}
	item, reason := pickCallbackItems(byID, byPhone)
	if reason != "" {
		return IncomingResult{Reason: reason}, nil
	}
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
	if !applied.Applied && !applied.Duplicate && !applied.BecameTerminal {
		reason := applied.Reason
		if reason == "" {
			reason = "lifecycle_unmapped"
		}
		return IncomingResult{Reason: reason, Item: applied.Item}, nil
	}
	return IncomingResult{
		Applied:   applied.Applied || applied.BecameTerminal,
		Duplicate: applied.Duplicate,
		Item:      applied.Item,
	}, nil
}

// DrainCallbacks applies stored SMSC callbacks and finalizes jobs that became
// ready. No Tick HTTP budget: this is DB-only (SkipEnrich).
func (w *Worker) DrainCallbacks(ctx context.Context) (int, error) {
	if w == nil || w.store == nil {
		return 0, nil
	}
	n, err := w.applyStoredCallbacks(ctx)
	if err != nil {
		return n, err
	}
	f, err := w.finalizeReady(ctx)
	return n + f, err
}

func (w *Worker) applyStoredCallbacks(ctx context.Context) (int, error) {
	if w.store == nil {
		return 0, nil
	}
	rows, err := w.store.Queries.ListUnprocessedProviderLookupCallbacks(ctx, 20)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range rows {
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

func incomingFromStored(row sqlcdb.ProviderLookupCallback) IncomingCallback {
	id := ""
	if row.ProviderMessageID != nil {
		id = *row.ProviderMessageID
	}
	var normalized smsc.NormalizedResult
	if len(row.NormalizedResult) > 0 {
		_ = json.Unmarshal(row.NormalizedResult, &normalized)
	}
	phone := CallbackPhoneDigits(normalized.PhoneE164)
	if obj, ok := rawObject(row.RawPayload); ok {
		if phone == "" {
			phone = CallbackPhoneDigits(smsc.CallbackPhoneRaw(obj))
		}
		if id == "" {
			id = asStringAny(obj["id"])
		}
	}
	return IncomingCallback{
		ProviderMessageID: id,
		PhoneDigits:       phone,
		Normalized:        normalized,
		SkipEnrich:        true,
	}
}

func (w *Worker) applyStoredCallback(ctx context.Context, row sqlcdb.ProviderLookupCallback) bool {
	res, err := w.ApplyIncoming(ctx, incomingFromStored(row))
	if err != nil {
		if w.log != nil {
			w.log.Error("lookup callback apply", "callback_id", row.ID, "err", err)
		}
		return false
	}
	now := time.Now().UTC()
	if w.now != nil {
		now = w.now()
	}
	if !shouldMarkStoredCallback(res, now.Sub(row.CreatedAt)) {
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

func asStringAny(v any) string {
	if v == nil {
		return ""
	}
	switch n := v.(type) {
	case string:
		return n
	case json.Number:
		return n.String()
	case float64:
		if n == math.Trunc(n) && !math.IsInf(n, 0) && !math.IsNaN(n) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', -1, 64)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	default:
		return fmt.Sprint(v)
	}
}

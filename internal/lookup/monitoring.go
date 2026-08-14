package lookup

import (
	"context"
	"encoding/json"
	"time"

	"finenumbers/sms/internal/smsc"
)

func (s *Service) Monitoring(ctx context.Context, provider *smsc.Provider) (map[string]any, error) {
	since := time.Now().UTC().Add(-24 * time.Hour)
	reqN, err := s.store.Queries.CountProviderLookupRequestsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	cbN, err := s.store.Queries.CountProviderLookupCallbacksSince(ctx, since)
	if err != nil {
		return nil, err
	}
	whN, err := s.store.Queries.CountWebhookDeliveriesSince(ctx, since)
	if err != nil {
		return nil, err
	}
	reqs, err := s.store.Queries.ListRecentProviderLookupRequests(ctx, 20)
	if err != nil {
		return nil, err
	}
	cbs, err := s.store.Queries.ListRecentProviderLookupCallbacks(ctx, 20)
	if err != nil {
		return nil, err
	}
	recentReq := make([]map[string]any, 0, len(reqs))
	for _, row := range reqs {
		recentReq = append(recentReq, map[string]any{
			"id":                  row.ID,
			"provider_code":       row.ProviderCode,
			"kind":                row.Kind,
			"status":              row.Status,
			"provider_message_id": row.ProviderMessageID,
			"http_status":         row.HttpStatus,
			"error_code":          row.ErrorCode,
			"error_message":       row.ErrorMessage,
			"request_payload":     jsonRaw(row.RequestPayload),
			"response_payload":    jsonRaw(row.ResponsePayload),
			"created_at":          row.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	recentCb := make([]map[string]any, 0, len(cbs))
	for _, row := range cbs {
		phone := ""
		if obj, ok := rawObject(row.RawPayload); ok {
			phone = CallbackPhoneDigits(smsc.CallbackPhoneRaw(obj))
		}
		recentCb = append(recentCb, map[string]any{
			"id":                  row.ID,
			"provider_code":       row.ProviderCode,
			"provider_message_id": row.ProviderMessageID,
			"phone":               phone,
			"signature_valid":     row.SignatureValid,
			"processed_at":        formatTimePtr(row.ProcessedAt),
			"process_error":       row.ProcessError,
			"raw_payload":         jsonRaw(row.RawPayload),
			"created_at":          row.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return map[string]any{
		"adapter_configured":         provider != nil && provider.Configured(),
		"callback_secret_configured": provider != nil && provider.CallbackSecretConfigured(),
		"requests_24h":               reqN,
		"callbacks_24h":              cbN,
		"webhooks_24h":               whN,
		"recent_requests":            recentReq,
		"recent_callbacks":           recentCb,
	}, nil
}

func jsonRaw(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return json.RawMessage(b)
}

package webhooks

import (
	"time"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func EndpointJSON(row sqlcdb.WebhookEndpoint) map[string]any {
	events := row.Events
	if events == nil {
		events = []string{}
	}
	return map[string]any{
		"id":                   row.ID,
		"url":                  row.Url,
		"description":          row.Description,
		"enabled":              row.Enabled,
		"events":               events,
		"consecutive_failures": row.ConsecutiveFailures,
		"created_at":           row.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":           row.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func EndpointCreatedJSON(row sqlcdb.WebhookEndpoint, secret string) map[string]any {
	out := EndpointJSON(row)
	out["secret"] = secret
	return out
}

func DeliveryJSON(row sqlcdb.WebhookDelivery) map[string]any {
	return map[string]any{
		"id":                 row.ID,
		"endpoint_id":        row.EndpointID,
		"job_id":             row.JobID,
		"job_item_id":        row.JobItemID,
		"event_type":         row.EventType,
		"status":             string(row.Status),
		"attempt_count":      row.AttemptCount,
		"max_attempts":       row.MaxAttempts,
		"next_attempt_at":    formatTime(row.NextAttemptAt),
		"last_response_code": row.LastResponseCode,
		"last_error":         row.LastError,
		"delivered_at":       formatTime(row.DeliveredAt),
		"created_at":         row.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

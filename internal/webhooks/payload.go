package webhooks

import (
	"encoding/json"
	"strings"
	"time"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func CheckData(item sqlcdb.LookupItem) map[string]any {
	return map[string]any{
		"job_id":        item.JobID,
		"job_item_id":   item.ID,
		"type":          string(item.CheckType),
		"status":        string(item.Status),
		"phone":         item.PhoneE164,
		"result_status": item.ResultStatus,
		"is_reachable":  item.IsReachable,
		"imsi":          item.Imsi,
		"mcc":           item.Mcc,
		"mnc":           item.Mnc,
		"operator_name": item.OperatorName,
		"country_code":  item.CountryCode,
		"ported":        item.Ported,
		"roaming":       item.Roaming,
		"error_code":    item.ErrorCode,
		"error_message": scrubBrand(deref(item.ErrorMessage)),
		"completed_at":  formatTime(item.CompletedAt),
	}
}

func JobData(job sqlcdb.LookupJob) map[string]any {
	return map[string]any{
		"job_id":        job.ID,
		"type":          string(job.CheckType),
		"status":        string(job.Status),
		"item_count":    job.ItemCount,
		"success_count": job.SuccessCount,
		"failure_count": job.FailureCount,
		"completed_at":  formatTime(job.CompletedAt),
	}
}

func Envelope(id, eventType string, createdAt time.Time, data map[string]any) map[string]any {
	return map[string]any{
		"api_version": APIVersion,
		"id":          id,
		"type":        eventType,
		"created_at":  createdAt.UTC().Format(time.RFC3339Nano),
		"data":        data,
	}
}

func Serialize(envelope map[string]any) ([]byte, error) {
	return json.Marshal(envelope)
}

func scrubBrand(s string) any {
	if s == "" {
		return nil
	}
	replacer := strings.NewReplacer("SMSC.ru", "провайдер", "smsc.ru", "провайдер", "SMSC", "провайдер", "smsc", "провайдер")
	return replacer.Replace(s)
}

func formatTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

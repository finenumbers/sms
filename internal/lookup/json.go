package lookup

import (
	"encoding/json"
	"time"

	"finenumbers/sms/internal/billing"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func formatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func extraString(extras map[string]any, keys ...string) string {
	for _, key := range keys {
		if extras == nil {
			continue
		}
		if v, ok := extras[key]; ok && v != nil {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func JobJSON(job sqlcdb.LookupJob) map[string]any {
	return map[string]any{
		"id":                job.ID,
		"client_id":         job.ClientID,
		"type":              string(job.CheckType),
		"source":            string(job.Source),
		"status":            string(job.Status),
		"item_count":        job.ItemCount,
		"success_count":     job.SuccessCount,
		"failure_count":     job.FailureCount,
		"reachable_count":   job.ReachableCount,
		"unreachable_count": job.UnreachableCount,
		"unit_sell_price":   billing.FormatMoneyPtr(job.UnitSellPrice),
		"tariff_plan_code":  job.TariffPlanCode,
		"currency":          job.Currency,
		"estimated_cost":    billing.FormatMoneyPtr(job.EstimatedCost),
		// actual_cost is wallet sell debit (SUM of captured item shares), not SMSC provider cost.
		"actual_cost":       billing.FormatMoneyPtr(job.ActualCost),
		"original_filename": job.OriginalFilename,
		"error_code":        job.ErrorCode,
		"error_message":     job.ErrorMessage,
		"started_at":        formatTimePtr(job.StartedAt),
		"completed_at":      formatTimePtr(job.CompletedAt),
		"created_at":        job.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":        job.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func JobAcceptedJSON(res CreateResult) map[string]any {
	out := JobJSON(res.Job)
	out["deduplicated"] = res.Deduplicated
	out["deduplicated_phone_count"] = res.DeduplicatedPhoneCount
	out["work_units"] = res.WorkUnits
	return out
}

func ItemJSON(item sqlcdb.LookupItem) map[string]any {
	extras := extrasFromItem(item)
	msc := extraString(extras, "msc")
	region := extraString(extras, "region")
	roamingCountry := extraString(extras, "roaming_country", "roamingCountry")
	roamingOperator := extraString(extras, "roaming_operator", "roamingOperator")
	var mscOut, regionOut, rcOut, roOut any
	if msc != "" {
		mscOut = msc
	}
	if region != "" {
		regionOut = region
	}
	if roamingCountry != "" {
		rcOut = roamingCountry
	}
	if roamingOperator != "" {
		roOut = roamingOperator
	}
	cur := any(nil)
	if item.Currency != nil {
		cur = *item.Currency
	}
	return map[string]any{
		"id":               item.ID,
		"job_id":           item.JobID,
		"type":             string(item.CheckType),
		"status":           string(item.Status),
		"phone":            item.PhoneE164,
		"result_status":    item.ResultStatus,
		"is_reachable":     item.IsReachable,
		"imsi":             item.Imsi,
		"mcc":              item.Mcc,
		"mnc":              item.Mnc,
		"operator_name":    item.OperatorName,
		"country_code":     item.CountryCode,
		"region":           regionOut,
		"msc":              mscOut,
		"ported":           item.Ported,
		"roaming":          item.Roaming,
		"roaming_country":  rcOut,
		"roaming_operator": roOut,
		"error_code":       item.ErrorCode,
		"error_message":    itemErrorMessage(item),
		"unit_sell_price":  billing.FormatMoneyPtr(item.UnitSellPrice),
		// actual_cost is the captured sell share, not SMSC provider cost. Null after RELEASE.
		"actual_cost":      billing.FormatMoneyPtr(item.ActualCost),
		"currency":         cur,
		"sent_at":          formatTimePtr(item.SentAt),
		"completed_at":     formatTimePtr(item.CompletedAt),
		"created_at":       item.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func itemErrorMessage(item sqlcdb.LookupItem) any {
	if deref(item.ErrorMessage) != "" {
		return item.ErrorMessage
	}
	label := exportProviderError(item)
	if label == exportDash {
		return nil
	}
	return label
}

func CheckJSON(item sqlcdb.LookupItem) map[string]any {
	out := ItemJSON(item)
	out["check_id"] = item.ID
	out["kind"] = "check"
	return out
}

func PreviewJSON(row sqlcdb.LookupCsvPreview) map[string]any {
	rowCount := int(row.PhoneCount)
	invalidCount := 0
	duplicateCount := 0
	var stats struct {
		RowCount       int `json:"row_count"`
		InvalidCount   int `json:"invalid_count"`
		DuplicateCount int `json:"duplicate_count"`
	}
	if json.Unmarshal(row.PhonesJson, &stats) == nil {
		if stats.RowCount > 0 {
			rowCount = stats.RowCount
		}
		invalidCount = stats.InvalidCount
		duplicateCount = stats.DuplicateCount
	}
	return map[string]any{
		"id":                row.ID,
		"type":              string(row.CheckType),
		"status":            string(row.Status),
		"phone_count":       row.PhoneCount,
		"row_count":         rowCount,
		"invalid_count":     invalidCount,
		"duplicate_count":   duplicateCount,
		"original_filename": row.OriginalFilename,
		"error_message":     row.ErrorMessage,
		"job_id":            row.JobID,
		"expires_at":        row.ExpiresAt.UTC().Format(time.RFC3339Nano),
		"created_at":        row.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func EstimateJSON(est billing.Estimate, workUnits int) map[string]any {
	checkType := "hlr"
	if est.Product == sqlcdb.BillingProductSilentSms {
		checkType = "ping"
	} else if est.Product == sqlcdb.BillingProductHlr {
		checkType = "hlr"
	}
	return map[string]any{
		"type":             checkType,
		"product":          string(est.Product),
		"quantity":         workUnits,
		"unit_sell_price":  billing.FormatMoney(est.UnitSellPrice),
		"estimated_cost":   billing.FormatMoney(est.Total),
		"currency":         est.Currency,
		"tariff_plan_code": est.TariffPlanCode,
	}
}

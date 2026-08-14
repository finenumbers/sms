package lookup

import (
	"encoding/json"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/smsc"
)

func normalizedToJSON(n smsc.NormalizedResult) []byte {
	b, err := json.Marshal(n)
	if err != nil {
		return []byte("{}")
	}
	return b
}

func extrasFromItem(item sqlcdb.LookupItem) map[string]any {
	if len(item.NormalizedResult) == 0 {
		return map[string]any{}
	}
	var raw map[string]any
	if json.Unmarshal(item.NormalizedResult, &raw) != nil {
		return map[string]any{}
	}
	extras, _ := raw["extras"].(map[string]any)
	if extras == nil {
		return map[string]any{}
	}
	return extras
}

func preferString(incoming, existing string) string {
	if incoming != "" {
		return incoming
	}
	return existing
}

func preferBool(incoming, existing *bool) *bool {
	if incoming != nil {
		return incoming
	}
	return existing
}

func preferResultStatus(incoming, existing string) string {
	terminal := map[string]struct{}{
		"reachable": {}, "unreachable": {}, "error": {}, "unknown": {},
	}
	if existing != "" {
		if _, ok := terminal[existing]; ok && (incoming == "" || incoming == "pending") {
			return existing
		}
	}
	if incoming != "" {
		return incoming
	}
	if existing != "" {
		return existing
	}
	return "unknown"
}

func mergeExtras(incoming, existing map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range incoming {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func mergeNormalizedWithItem(n smsc.NormalizedResult, item sqlcdb.LookupItem) smsc.NormalizedResult {
	existing := extrasFromItem(item)
	n.ProviderMessageID = preferString(n.ProviderMessageID, deref(item.ProviderMessageID))
	n.PhoneE164 = preferString(n.PhoneE164, item.PhoneE164)
	n.IMSI = preferString(n.IMSI, deref(item.Imsi))
	n.MCC = preferString(n.MCC, deref(item.Mcc))
	n.MNC = preferString(n.MNC, deref(item.Mnc))
	n.OperatorName = preferString(n.OperatorName, deref(item.OperatorName))
	n.CountryCode = preferString(n.CountryCode, deref(item.CountryCode))
	n.Ported = preferBool(n.Ported, item.Ported)
	n.Roaming = preferBool(n.Roaming, item.Roaming)
	n.IsReachable = preferBool(n.IsReachable, item.IsReachable)
	n.ResultStatus = smsc.ResultStatus(preferString(string(n.ResultStatus), deref(item.ResultStatus)))
	n.Extras = mergeExtras(n.Extras, existing)
	return n
}

func needsHLREnrich(n smsc.NormalizedResult, checkType sqlcdb.LookupCheckType) bool {
	if checkType != sqlcdb.LookupCheckTypeHlr && n.CheckType != smsc.CheckHLR {
		return false
	}
	msc := n.Extras["msc"]
	mscEmpty := msc == nil || msc == ""
	return n.IMSI == "" || mscEmpty
}

func hlrFieldsImproved(n smsc.NormalizedResult, item sqlcdb.LookupItem) bool {
	existing := extrasFromItem(item)
	if n.IMSI != "" && deref(item.Imsi) == "" {
		return true
	}
	if n.MCC != "" && deref(item.Mcc) == "" {
		return true
	}
	if n.MNC != "" && deref(item.Mnc) == "" {
		return true
	}
	if n.OperatorName != "" && deref(item.OperatorName) == "" {
		return true
	}
	if n.CountryCode != "" && deref(item.CountryCode) == "" {
		return true
	}
	if n.Roaming != nil && item.Roaming == nil {
		return true
	}
	if n.IsReachable != nil && (item.IsReachable == nil || *n.IsReachable != *item.IsReachable) {
		return true
	}
	if n.ResultStatus != "" && n.ResultStatus != smsc.ResultPending && string(n.ResultStatus) != deref(item.ResultStatus) {
		return true
	}
	if n.ProviderErrorCode != "" && n.ProviderErrorCode != deref(item.ErrorCode) {
		return true
	}
	for _, key := range []string{"msc", "region", "roamingCountry", "roamingOperator"} {
		next := n.Extras[key]
		prev := existing[key]
		if next != nil && next != "" && (prev == nil || prev == "") {
			return true
		}
	}
	return false
}

func mergeEnrich(merged, rich smsc.NormalizedResult) smsc.NormalizedResult {
	merged.IMSI = preferString(rich.IMSI, merged.IMSI)
	merged.MCC = preferString(rich.MCC, merged.MCC)
	merged.MNC = preferString(rich.MNC, merged.MNC)
	merged.OperatorName = preferString(rich.OperatorName, merged.OperatorName)
	merged.CountryCode = preferString(rich.CountryCode, merged.CountryCode)
	merged.Roaming = preferBool(rich.Roaming, merged.Roaming)
	merged.Ported = preferBool(rich.Ported, merged.Ported)
	merged.IsReachable = preferBool(rich.IsReachable, merged.IsReachable)
	merged.ResultStatus = smsc.ResultStatus(preferResultStatus(string(rich.ResultStatus), string(merged.ResultStatus)))
	merged.ProviderErrorCode = preferString(rich.ProviderErrorCode, merged.ProviderErrorCode)
	merged.ProviderErrorMessage = preferString(rich.ProviderErrorMessage, merged.ProviderErrorMessage)
	merged.ProviderStatusCode = preferString(merged.ProviderStatusCode, rich.ProviderStatusCode)
	merged.Extras = mergeExtras(rich.Extras, merged.Extras)
	merged.Cost = preferString(merged.Cost, rich.Cost)
	return merged
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

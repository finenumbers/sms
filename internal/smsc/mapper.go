package smsc

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func emptyNormalized(checkType CheckType, overrides NormalizedResult) NormalizedResult {
	out := NormalizedResult{
		ProviderCode:    ProviderCode,
		CheckType:       checkType,
		LifecycleStatus: LifecyclePending,
		ResultStatus:    ResultPending,
		Extras:          map[string]any{},
	}
	mergeNormalized(&out, overrides)
	if out.Extras == nil {
		out.Extras = map[string]any{}
	}
	return out
}

func mergeNormalized(dst *NormalizedResult, src NormalizedResult) {
	if src.ProviderCode != "" {
		dst.ProviderCode = src.ProviderCode
	}
	if src.CheckType != "" {
		dst.CheckType = src.CheckType
	}
	if src.ProviderMessageID != "" {
		dst.ProviderMessageID = src.ProviderMessageID
	}
	if src.PhoneE164 != "" {
		dst.PhoneE164 = src.PhoneE164
	}
	if src.LifecycleStatus != "" {
		dst.LifecycleStatus = src.LifecycleStatus
	}
	if src.ResultStatus != "" {
		dst.ResultStatus = src.ResultStatus
	}
	if src.IsReachable != nil {
		dst.IsReachable = src.IsReachable
	}
	if src.IMSI != "" {
		dst.IMSI = src.IMSI
	}
	if src.MCC != "" {
		dst.MCC = src.MCC
	}
	if src.MNC != "" {
		dst.MNC = src.MNC
	}
	if src.OperatorName != "" {
		dst.OperatorName = src.OperatorName
	}
	if src.CountryCode != "" {
		dst.CountryCode = src.CountryCode
	}
	if src.Ported != nil {
		dst.Ported = src.Ported
	}
	if src.Roaming != nil {
		dst.Roaming = src.Roaming
	}
	if src.ProviderErrorCode != "" {
		dst.ProviderErrorCode = src.ProviderErrorCode
	}
	if src.ProviderErrorMessage != "" {
		dst.ProviderErrorMessage = src.ProviderErrorMessage
	}
	if src.ProviderStatusCode != "" {
		dst.ProviderStatusCode = src.ProviderStatusCode
	}
	if src.Cost != "" {
		dst.Cost = src.Cost
	}
	if src.Currency != "" {
		dst.Currency = src.Currency
	}
	if src.Extras != nil {
		dst.Extras = src.Extras
	}
}

type MapStatusInput struct {
	CheckType         CheckType
	StatusCode        any
	ErrorCode         any
	ErrorMessage      string
	PhoneE164         string
	ProviderMessageID string
	Extras            map[string]any
	Cost              string
	Currency          string
}

// MapStatus maps SMSC status / err codes into lifecycle + reachability.
// Conservative rules (HLR mapper canon, not SMSC marketing docs):
//   - status -1,-2,0 → pending
//   - status -3 → failed (not found)
//   - status 1,2 → completed; reachability from err when present
//   - status 3,20+ → completed with unreachable
//   - err 0 with terminal success status → reachable
//   - err non-zero on HLR/Ping often means unavailable / not delivered
func MapStatus(in MapStatusInput) NormalizedResult {
	status, statusOK := asNumber(in.StatusCode)
	errCode, errOK := asNumber(in.ErrorCode)
	extras := cloneMap(in.Extras)

	if !statusOK && !errOK && in.ErrorMessage == "" {
		return emptyNormalized(in.CheckType, NormalizedResult{
			PhoneE164:         in.PhoneE164,
			ProviderMessageID: asString(in.ProviderMessageID),
			LifecycleStatus:   LifecyclePending,
			ResultStatus:      ResultUnknown,
			Extras:            extras,
			Cost:              in.Cost,
			Currency:          in.Currency,
		})
	}

	lifecycle := LifecyclePending
	result := ResultPending
	var reachable *bool

	switch {
	case statusOK && status == -3:
		lifecycle = LifecycleFailed
		result = ResultError
	case statusOK && (status == -1 || status == -2 || status == 0):
		lifecycle = LifecyclePending
		result = ResultPending
	case statusOK && (status == 1 || status == 2):
		if !errOK {
			lifecycle = LifecyclePending
			result = ResultPending
			reachable = nil
		} else if errCode == 0 {
			lifecycle = LifecycleCompleted
			result = ResultReachable
			reachable = boolPtr(true)
		} else {
			lifecycle = LifecycleCompleted
			result = ResultUnreachable
			reachable = boolPtr(false)
		}
	case statusOK && status >= 3:
		lifecycle = LifecycleCompleted
		result = ResultUnreachable
		reachable = boolPtr(false)
	case errOK:
		if errCode == 0 {
			lifecycle = LifecycleCompleted
			result = ResultReachable
			reachable = boolPtr(true)
		} else {
			lifecycle = LifecycleCompleted
			result = ResultUnreachable
			reachable = boolPtr(false)
		}
	case in.ErrorMessage != "":
		lifecycle = LifecycleFailed
		result = ResultError
	}

	out := emptyNormalized(in.CheckType, NormalizedResult{
		PhoneE164:            in.PhoneE164,
		ProviderMessageID:    asString(in.ProviderMessageID),
		LifecycleStatus:      lifecycle,
		ResultStatus:         result,
		IsReachable:          reachable,
		ProviderErrorMessage: in.ErrorMessage,
		Cost:                 in.Cost,
		Currency:             in.Currency,
		Extras:               extras,
	})
	if statusOK {
		out.ProviderStatusCode = formatNumber(status)
	}
	if errOK {
		out.ProviderErrorCode = formatNumber(errCode)
	} else if s := asString(in.ErrorCode); s != "" {
		out.ProviderErrorCode = s
	}
	return out
}

type MapResponseInput struct {
	CheckType         CheckType
	Raw               any
	PhoneE164         string
	ProviderMessageID string
	Currency          string
}

// MapResponse is the unified pipeline for send-ack, status.php, and callbacks.
func MapResponse(in MapResponseInput) NormalizedResult {
	obj, ok := asObject(in.Raw)
	if !ok {
		return emptyNormalized(in.CheckType, NormalizedResult{
			PhoneE164:            in.PhoneE164,
			ProviderMessageID:    in.ProviderMessageID,
			LifecycleStatus:      LifecycleFailed,
			ResultStatus:         ResultError,
			ProviderErrorMessage: "Empty or non-object provider response",
		})
	}

	if nonJSON, _ := obj["_nonJson"].(bool); nonJSON {
		text, _ := obj["text"].(string)
		msg := "Non-JSON provider response"
		if text != "" {
			if len(text) > 200 {
				text = text[:200]
			}
			msg = "Non-JSON provider response: " + text
		}
		return emptyNormalized(in.CheckType, NormalizedResult{
			PhoneE164:            in.PhoneE164,
			ProviderMessageID:    in.ProviderMessageID,
			LifecycleStatus:      LifecycleFailed,
			ResultStatus:         ResultError,
			ProviderErrorMessage: msg,
		})
	}

	if hasNonEmpty(obj, "error_code") {
		return emptyNormalized(in.CheckType, NormalizedResult{
			PhoneE164:            in.PhoneE164,
			ProviderMessageID:    firstNonEmpty(asString(obj["id"]), in.ProviderMessageID),
			LifecycleStatus:      LifecycleFailed,
			ResultStatus:         ResultError,
			ProviderErrorCode:    asString(obj["error_code"]),
			ProviderErrorMessage: asString(obj["error"]),
			Cost:                 asString(obj["cost"]),
			Currency:             in.Currency,
		})
	}

	providerMessageID := firstNonEmpty(asString(obj["id"]), in.ProviderMessageID)
	phoneE164 := firstNonEmpty(normalizePhoneHint(asString(obj["phone"])), in.PhoneE164)

	_, hasStatus := obj["status"]
	_, hasErr := obj["err"]
	if _, hasID := obj["id"]; hasID && !hasStatus && !hasErr {
		extras := map[string]any{}
		if _, ok := obj["cnt"]; ok {
			if n, ok := asNumber(obj["cnt"]); ok {
				extras["cnt"] = n
			} else {
				extras["cnt"] = obj["cnt"]
			}
		}
		if _, ok := obj["balance"]; ok {
			extras["balance"] = asString(obj["balance"])
		}
		return emptyNormalized(in.CheckType, NormalizedResult{
			PhoneE164:         phoneE164,
			ProviderMessageID: providerMessageID,
			LifecycleStatus:   LifecycleAccepted,
			ResultStatus:      ResultPending,
			Cost:              asString(obj["cost"]),
			Currency:          in.Currency,
			Extras:            extras,
		})
	}

	hlr := extractHLRFields(obj)
	mapped := MapStatus(MapStatusInput{
		CheckType:         in.CheckType,
		StatusCode:        obj["status"],
		ErrorCode:         obj["err"],
		ErrorMessage:      asString(obj["error"]),
		PhoneE164:         phoneE164,
		ProviderMessageID: providerMessageID,
		Cost:              asString(obj["cost"]),
		Currency:          in.Currency,
		Extras:            hlr.Extras,
	})
	if hlr.IMSI != "" {
		mapped.IMSI = hlr.IMSI
	}
	if hlr.MCC != "" {
		mapped.MCC = hlr.MCC
	}
	if hlr.MNC != "" {
		mapped.MNC = hlr.MNC
	}
	if hlr.OperatorName != "" {
		mapped.OperatorName = hlr.OperatorName
	}
	if hlr.CountryCode != "" {
		mapped.CountryCode = hlr.CountryCode
	}
	if hlr.Roaming != nil {
		mapped.Roaming = hlr.Roaming
	}
	if hlr.Ported != nil {
		mapped.Ported = hlr.Ported
	}
	mapped.Extras = mergeMaps(mapped.Extras, hlr.Extras)
	return mapped
}

func extractHLRFields(body map[string]any) NormalizedResult {
	mcc := asString(body["mcc"])
	mnc := asString(body["mnc"])
	rcn := asString(body["rcn"])
	rnet := asString(body["rnet"])
	cn := asString(body["cn"])
	net := asString(body["net"])
	country := asString(body["country"])
	operator := asString(body["operator"])
	region := asString(body["region"])

	var roaming *bool
	if rcn != "" || rnet != "" {
		roaming = boolPtr(true)
	} else if cn != "" || net != "" || mcc != "" || mnc != "" || country != "" || operator != "" {
		roaming = boolPtr(false)
	}

	extras := map[string]any{}
	if _, ok := body["msc"]; ok {
		if s := asString(body["msc"]); s != "" {
			extras["msc"] = s
		} else {
			extras["msc"] = nil
		}
	}
	if rcn != "" {
		extras["roamingCountry"] = rcn
	}
	if rnet != "" {
		extras["roamingOperator"] = rnet
	}
	if region != "" {
		extras["region"] = region
	}
	if _, ok := body["type"]; ok {
		if n, ok := asNumber(body["type"]); ok {
			extras["messageType"] = n
		} else {
			extras["messageType"] = body["type"]
		}
	}
	if _, ok := body["flag"]; ok {
		extras["flag"] = body["flag"]
	}

	return NormalizedResult{
		IMSI:         asString(body["imsi"]),
		MCC:          mcc,
		MNC:          mnc,
		OperatorName: firstNonEmpty(net, operator),
		CountryCode:  firstNonEmpty(cn, country),
		Roaming:      roaming,
		Extras:       extras,
	}
}

func CallbackDedupeKey(payload any) string {
	obj, _ := asObject(payload)
	if obj == nil {
		obj = map[string]any{}
	}
	ts := firstNonEmpty(asString(obj["ts"]), asString(obj["time"]))
	material := strings.Join([]string{
		asString(obj["id"]),
		asString(obj["phone"]),
		asString(obj["status"]),
		asString(obj["err"]),
		ts,
	}, "|")
	sum := sha256.Sum256([]byte(material))
	return hex.EncodeToString(sum[:])
}

type VerifyInput struct {
	Payload    map[string]any
	Secret     string
	Signatures CallbackSignatures
}

// VerifyCallbackSignature checks md5/sha1 of `id:phone:status:<secret>`.
// Empty secret → nil (caller fail-closes). Missing both signatures → false.
func VerifyCallbackSignature(in VerifyInput) *bool {
	if in.Secret == "" {
		return nil
	}
	payload := in.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	id := asString(payload["id"])
	phone := asString(payload["phone"])
	status := asString(payload["status"])
	base := id + ":" + phone + ":" + status + ":" + in.Secret

	md5sig := firstNonEmpty(in.Signatures.MD5, asString(payload["md5"]))
	sha1sig := firstNonEmpty(in.Signatures.SHA1, asString(payload["sha1"]))
	if md5sig == "" && sha1sig == "" {
		return boolPtr(false)
	}
	if md5sig != "" {
		sum := md5.Sum([]byte(base))
		if !strings.EqualFold(hex.EncodeToString(sum[:]), md5sig) {
			return boolPtr(false)
		}
	}
	if sha1sig != "" {
		sum := sha1.Sum([]byte(base))
		if !strings.EqualFold(hex.EncodeToString(sum[:]), sha1sig) {
			return boolPtr(false)
		}
	}
	return boolPtr(true)
}

// ClientIDFromKey maps an idempotency key to SMSC numeric id (positive 31-bit).
func ClientIDFromKey(idempotencyKey string) int {
	sum := sha256.Sum256([]byte(idempotencyKey))
	value := binary.BigEndian.Uint32(sum[:4]) & 0x7fffffff
	if value == 0 {
		return 1
	}
	return int(value)
}

// SendIdempotencyKey is SHA256 input for the client id:
// SEND:{hlr|ping}:{item_uuid}
func SendIdempotencyKey(checkType CheckType, itemKey string) string {
	return "SEND:" + string(NormalizeCheckType(checkType)) + ":" + itemKey
}

func NormalizeCheckType(v CheckType) CheckType {
	switch strings.ToLower(strings.TrimSpace(string(v))) {
	case "ping":
		return CheckPing
	default:
		return CheckHLR
	}
}

func InferCheckType(payload map[string]any) CheckType {
	if payload == nil {
		return CheckHLR
	}
	n, ok := asNumber(payload["type"])
	if !ok {
		return CheckHLR
	}
	switch int(n) {
	case 5:
		return CheckPing
	case 4:
		return CheckHLR
	default:
		return CheckHLR
	}
}

func checkTypeFlags(checkType CheckType) map[string]any {
	if NormalizeCheckType(checkType) == CheckPing {
		return map[string]any{"ping": 1}
	}
	return map[string]any{"hlr": 1}
}

func asObject(raw any) (map[string]any, bool) {
	if raw == nil {
		return nil, false
	}
	switch v := raw.(type) {
	case map[string]any:
		return v, true
	case json.RawMessage:
		var m map[string]any
		if json.Unmarshal(v, &m) != nil || m == nil {
			return nil, false
		}
		return m, true
	default:
		return nil, false
	}
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return asString(float64(v))
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case bool:
		return strconv.FormatBool(v)
	default:
		return fmt.Sprint(v)
	}
}

func asNumber(value any) (float64, bool) {
	if value == nil {
		return 0, false
	}
	if s, ok := value.(string); ok && s == "" {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return v, true
	case float32:
		return asNumber(float64(v))
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return n, true
	case string:
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func formatNumber(n float64) string {
	if n == math.Trunc(n) {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}

func normalizePhoneHint(phone string) string {
	if phone == "" {
		return ""
	}
	digits := strings.TrimPrefix(strings.TrimSpace(phone), "+")
	if digits == "" {
		return ""
	}
	if strings.HasPrefix(phone, "+") {
		return phone
	}
	return "+" + digits
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func boolPtr(v bool) *bool { return &v }

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeMaps(a, b map[string]any) map[string]any {
	out := cloneMap(a)
	for k, v := range b {
		out[k] = v
	}
	return out
}

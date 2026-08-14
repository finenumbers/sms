package ops

import (
	"encoding/json"
	"strings"
)

func RedactJSON(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return []byte(`{"_redacted":"invalid_json"}`)
	}
	out, err := json.Marshal(redactValue(v))
	if err != nil {
		return []byte(`{"_redacted":"marshal"}`)
	}
	return out
}

// RawJSON returns redacted bytes as a json.RawMessage suitable for embedding
// in a map that will be marshaled into jsonb. Empty input is omitted by the caller.
func RawJSON(b []byte) json.RawMessage {
	r := RedactJSON(b)
	if len(r) == 0 {
		return nil
	}
	if !json.Valid(r) {
		return json.RawMessage(`{"_redacted":"invalid_json"}`)
	}
	return json.RawMessage(r)
}

func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		smsLike := isSMSContentMap(t)
		out := make(map[string]any, len(t))
		for k, val := range t {
			if redactKey(k) || (smsLike && strings.EqualFold(strings.TrimSpace(k), "message")) {
				out[k] = "[redacted]"
				continue
			}
			out[k] = redactValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactValue(val)
		}
		return out
	default:
		return v
	}
}

func redactKey(k string) bool {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "password", "token", "refresh_token", "secret", "authorization", "cookie", "text", "access",
		"phone", "phone_e164", "phones", "msisdn", "from", "to":
		return true
	default:
		return false
	}
}

func isSMSContentMap(m map[string]any) bool {
	if m == nil {
		return false
	}
	_, sms := m["sms_id"]
	_, sender := m["sender_number"]
	_, recv := m["receiver_number"]
	return sms || sender || recv
}

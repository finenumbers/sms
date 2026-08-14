package smsc

import "strings"

var secretKeys = map[string]struct{}{
	"psw":           {},
	"password":      {},
	"passwd":        {},
	"apikey":        {},
	"api_key":       {},
	"apiKey":        {},
	"login":         {},
	"md5":           {},
	"sha1":          {},
	"crc32":         {},
	"token":         {},
	"secret":        {},
	"authorization": {},
	"signature":     {},
}

// RedactSecrets deep-clones JSON-ish values and replaces credential/signature
// fields. Phones stay intact; log sinks mask them separately.
func RedactSecrets(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = RedactSecrets(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, nested := range v {
			if _, secret := secretKeys[key]; secret {
				out[key] = "[REDACTED]"
			} else {
				out[key] = RedactSecrets(nested)
			}
		}
		return out
	default:
		return value
	}
}

func ToPhoneDigits(phoneE164 string) string {
	return strings.TrimPrefix(strings.TrimSpace(phoneE164), "+")
}

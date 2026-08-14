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

// CanonicalPhoneDigits is the callback/create digit form: digits only,
// RU 8XXXXXXXXXX → 7…, 9XXXXXXXXX → 79…, even when the input had a leading +.
func CanonicalPhoneDigits(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	switch {
	case len(d) == 11 && d[0] == '8':
		return "7" + d[1:]
	case len(d) == 10 && d[0] == '9':
		return "7" + d
	default:
		return d
	}
}

func CanonicalPhoneE164(raw string) string {
	d := CanonicalPhoneDigits(raw)
	if d == "" {
		return ""
	}
	return "+" + d
}

func CallbackPhoneRaw(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if v, ok := payload["phone"]; ok && v != nil {
		return asString(v)
	}
	if v, ok := payload["phones"]; ok && v != nil {
		return asString(v)
	}
	return ""
}

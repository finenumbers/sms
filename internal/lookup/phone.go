package lookup

import (
	"strings"
	"unicode"
)

// RUMobile79RequiredMessage is the HLR pilot gate text. Reject the whole list
// if any number is not +79XXXXXXXXX.
const RUMobile79RequiredMessage = "В списке для проверки есть номера, проверка которых не может быть гарантирована HLR Lookup на мобильных сетях"

type NormalizeResult struct {
	Phones            []string
	DeduplicatedCount int
	Invalid           []InvalidPhone
	Rejected          []string
	HasNon79          bool
}

type InvalidPhone struct {
	Input  string
	Reason string
}

func IsRUMobile79(e164 string) bool {
	digits := digitsOnly(e164)
	return len(digits) == 11 && strings.HasPrefix(digits, "79")
}

func CountNonRUMobile79(phones []string) int {
	n := 0
	for _, p := range phones {
		if !IsRUMobile79(p) {
			n++
		}
	}
	return n
}

// NormalizePhoneE164 converts a single input to +E.164 without msisdn.NormalizeDest
// (that helper accepts 770…). RU national 8XXXXXXXXXX / 9XXXXXXXXX are accepted.
func NormalizePhoneE164(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", wrap(ErrValidation, "validation", "Phone number is empty")
	}
	compact := stripPhoneNoise(trimmed)
	digits := digitsOnly(compact)
	if len(digits) == 0 {
		return "", wrap(ErrValidation, "validation", "Invalid phone number")
	}
	if !strings.HasPrefix(compact, "+") {
		switch {
		case len(digits) == 11 && digits[0] == '8':
			digits = "7" + digits[1:]
		case len(digits) == 10 && digits[0] == '9':
			digits = "7" + digits
		}
		compact = "+" + digits
	} else {
		compact = "+" + digits
	}
	if !validE164(compact) {
		return "", wrap(ErrValidation, "validation", "Invalid phone number")
	}
	return compact, nil
}

func NormalizeAndDeduplicate(inputs []string) NormalizeResult {
	out := NormalizeResult{}
	seenE164 := map[string]struct{}{}
	seenOrig := map[string]struct{}{}
	for _, input := range inputs {
		trimmed := strings.TrimSpace(input)
		e164, err := NormalizePhoneE164(input)
		if err != nil {
			reason := "Invalid phone number"
			if e := AsError(err); e != nil && e.Message != "" {
				reason = e.Message
			}
			out.Invalid = append(out.Invalid, InvalidPhone{Input: input, Reason: reason})
			out.Rejected = appendUniqueToken(out.Rejected, seenOrig, trimmed)
			continue
		}
		if !IsRUMobile79(e164) {
			out.HasNon79 = true
			out.Rejected = appendUniqueToken(out.Rejected, seenOrig, trimmed)
		}
		if _, ok := seenE164[e164]; ok {
			out.DeduplicatedCount++
			continue
		}
		seenE164[e164] = struct{}{}
		out.Phones = append(out.Phones, e164)
	}
	return out
}

func appendUniqueToken(dst []string, seen map[string]struct{}, token string) []string {
	if token == "" {
		return dst
	}
	if _, ok := seen[token]; ok {
		return dst
	}
	seen[token] = struct{}{}
	return append(dst, token)
}

func phoneCapLabel(name string) string {
	if strings.TrimSpace(name) == "" {
		return "max_batch_phones"
	}
	return strings.TrimSpace(name)
}

func PreparePhones(inputs []string, source string, maxPhones int, capName string) (phones []string, deduped int, err error) {
	if len(inputs) == 0 {
		return nil, 0, wrap(ErrValidation, "validation", "phones must be a non-empty array")
	}
	norm := NormalizeAndDeduplicate(inputs)
	if len(norm.Invalid) > 0 || norm.HasNon79 {
		msg := "One or more phone numbers are invalid"
		if norm.HasNon79 {
			msg = RUMobile79RequiredMessage
		}
		return nil, 0, wrapRejected(ErrValidation, "validation", msg, norm.Rejected)
	}
	if len(norm.Phones) == 0 {
		return nil, 0, wrap(ErrValidation, "validation", "No valid phone numbers after normalization")
	}
	if maxPhones > 0 && len(norm.Phones) > maxPhones {
		return nil, 0, wrap(ErrValidation, "validation", "Phone count exceeds "+phoneCapLabel(capName))
	}
	if source == "single" && len(norm.Phones) != 1 {
		return nil, 0, wrap(ErrValidation, "validation", "SINGLE source requires exactly one phone")
	}
	return norm.Phones, norm.DeduplicatedCount, nil
}

func PhoneDigits(e164 string) string {
	return digitsOnly(e164)
}

func Chunk[T any](items []T, size int) [][]T {
	if size <= 0 {
		return nil
	}
	var out [][]T
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		out = append(out, items[i:end])
	}
	return out
}

func stripPhoneNoise(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i, r := range s {
		if r == '+' && i == 0 {
			b.WriteRune(r)
			continue
		}
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func digitsOnly(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func validE164(e164 string) bool {
	if !strings.HasPrefix(e164, "+") {
		return false
	}
	d := e164[1:]
	if len(d) < 8 || len(d) > 15 {
		return false
	}
	if d[0] == '0' {
		return false
	}
	for _, r := range d {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

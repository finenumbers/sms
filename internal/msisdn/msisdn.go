package msisdn

import (
	"fmt"
	"strings"
	"unicode"
)

// NormalizeSender converts a Russian DEF/from number to canonical 7XXXXXXXXXX.
// Accepts 7…, +7…, 8…, and 10-digit 9….
func NormalizeSender(raw string) (string, error) {
	digits := digitsOnly(raw)
	switch {
	case len(digits) == 11 && digits[0] == '7':
		return digits, nil
	case len(digits) == 11 && digits[0] == '8':
		return "7" + digits[1:], nil
	case len(digits) == 10 && digits[0] == '9':
		return "7" + digits, nil
	default:
		return "", fmt.Errorf("want 7XXXXXXXXXX")
	}
}

func IsSender(msisdn string) bool {
	if len(msisdn) != 11 || msisdn[0] != '7' {
		return false
	}
	for i := 0; i < len(msisdn); i++ {
		if msisdn[i] < '0' || msisdn[i] > '9' {
			return false
		}
	}
	return true
}

type Dest struct {
	MSISDN        string
	International bool
}

func NormalizeDest(raw string) (Dest, error) {
	if n, err := NormalizeSender(raw); err == nil {
		return Dest{MSISDN: n, International: false}, nil
	}
	digits := digitsOnly(raw)
	if len(digits) < 8 || len(digits) > 15 {
		return Dest{}, fmt.Errorf("invalid destination")
	}
	if digits[0] == '0' {
		return Dest{}, fmt.Errorf("invalid destination")
	}
	if IsSender(digits) {
		return Dest{MSISDN: digits, International: false}, nil
	}
	return Dest{MSISDN: digits, International: true}, nil
}

func Canonical(raw string) string {
	return digitsOnly(raw)
}

// FromManagement builds a canonical 7XXXXXXXXXX from DIDAPI management
// `code` + `number`. Unlike NormalizeSender, a 10-digit national number
// may start with any digit (DEF 9… or geographic 495…).
// If `number` is already 11 digits starting with 7, `code` is ignored.
func FromManagement(code, number string) (string, error) {
	num := digitsOnly(number)
	if len(num) == 11 && num[0] == '7' {
		return num, nil
	}
	combined := digitsOnly(code) + num
	switch {
	case len(combined) == 11 && combined[0] == '7':
		return combined, nil
	case len(combined) == 10:
		return "7" + combined, nil
	default:
		return "", fmt.Errorf("want 7XXXXXXXXXX")
	}
}

func digitsOnly(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

package billing

import "unicode/utf16"

// GSM-7 default alphabet (3GPP TS 23.038) and extension table (escape = 2 septets).
var gsm7Basic = map[rune]struct{}{}
var gsm7Escape = map[rune]struct{}{}

func init() {
	basic := []rune{
		'@', '£', '$', '¥', 'è', 'é', 'ù', 'ì', 'ò', 'Ç', '\n', 'Ø', 'ø', '\r',
		'Å', 'å', 'Δ', '_', 'Φ', 'Γ', 'Λ', 'Ω', 'Π', 'Ψ', 'Σ', 'Θ', 'Ξ',
		'Æ', 'æ', 'ß', 'É', ' ', '!', '"', '#', '¤', '%', '&', '\'', '(', ')',
		'*', '+', ',', '-', '.', '/',
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
		':', ';', '<', '=', '>', '?', '¡',
		'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', 'K', 'L', 'M',
		'N', 'O', 'P', 'Q', 'R', 'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z',
		'Ä', 'Ö', 'Ñ', 'Ü', '§', '¿',
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm',
		'n', 'o', 'p', 'q', 'r', 's', 't', 'u', 'v', 'w', 'x', 'y', 'z',
		'ä', 'ö', 'ñ', 'ü', 'à',
	}
	for _, r := range basic {
		gsm7Basic[r] = struct{}{}
	}
	for _, r := range []rune{'|', '^', '€', '{', '}', '[', ']', '~', '\\'} {
		gsm7Escape[r] = struct{}{}
	}
}

func isGSM7(text string) bool {
	for _, r := range text {
		if _, ok := gsm7Basic[r]; ok {
			continue
		}
		if _, ok := gsm7Escape[r]; ok {
			continue
		}
		return false
	}
	return true
}

func gsm7Septets(text string) int {
	n := 0
	for _, r := range text {
		if _, ok := gsm7Escape[r]; ok {
			n += 2
			continue
		}
		n++
	}
	return n
}

func utf16Units(text string) int {
	return len(utf16.Encode([]rune(text)))
}

// SegmentCount is the billed PDU count from text (frozen at enqueue).
func SegmentCount(text string) int {
	if text == "" {
		return 1
	}
	if isGSM7(text) {
		n := gsm7Septets(text)
		if n <= 160 {
			return 1
		}
		return (n + 152) / 153
	}
	n := utf16Units(text)
	if n <= 70 {
		return 1
	}
	return (n + 66) / 67
}

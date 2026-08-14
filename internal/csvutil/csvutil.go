package csvutil

import (
	"bytes"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
)

func Decode(raw []byte) (text, encoding string) {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(raw) {
		return string(raw), "utf-8"
	}
	dec := charmap.Windows1251.NewDecoder()
	s, _, err := transform.String(dec, string(raw))
	if err != nil {
		return string(raw), "utf-8"
	}
	return s, "windows-1251"
}

func DetectComma(text string) rune {
	semi, comma := 0, 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		semi += strings.Count(line, ";")
		comma += strings.Count(line, ",")
		break
	}
	if semi > comma {
		return ';'
	}
	return ','
}

func EmptyRecord(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func Comment(rec []string) bool {
	if len(rec) == 0 {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(rec[0]), "#")
}

func Cell(rec []string, idx int) string {
	if idx < 0 || idx >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[idx])
}

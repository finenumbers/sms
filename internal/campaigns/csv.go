package campaigns

import (
	"encoding/csv"
	"io"
	"strings"

	"finenumbers/sms/internal/csvutil"
	"finenumbers/sms/internal/msisdn"
)

type InvalidRow struct {
	Line  int    `json:"line,omitempty"`
	Value string `json:"value"`
	Error string `json:"error"`
}

type ParseResult struct {
	MSISDNs  []string
	Invalid  []InvalidRow
	Encoding string
}

func ParseRecipientFile(raw []byte) ParseResult {
	text, enc := csvutil.Decode(raw)
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = csvutil.DetectComma(text)
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	out := ParseResult{Encoding: enc, MSISDNs: make([]string, 0), Invalid: make([]InvalidRow, 0)}
	header := map[string]int{}
	line := 0
	first := true
	seen := map[string]struct{}{}
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			out.Invalid = append(out.Invalid, InvalidRow{Line: line, Error: "csv parse error"})
			continue
		}
		if csvutil.EmptyRecord(rec) || csvutil.Comment(rec) {
			continue
		}
		if first {
			first = false
			if h, ok := parseRecipientHeader(rec); ok {
				header = h
				continue
			}
		}
		rawNum := recipientCell(rec, header)
		dest, err := msisdn.NormalizeDest(rawNum)
		if err != nil {
			out.Invalid = append(out.Invalid, InvalidRow{Line: line, Value: rawNum, Error: err.Error()})
			continue
		}
		if _, ok := seen[dest.MSISDN]; ok {
			continue
		}
		seen[dest.MSISDN] = struct{}{}
		out.MSISDNs = append(out.MSISDNs, dest.MSISDN)
	}
	return out
}

func parseRecipientHeader(rec []string) (map[string]int, bool) {
	h := map[string]int{}
	found := false
	for i, c := range rec {
		key := recipientHeaderKey(c)
		if key == "" {
			continue
		}
		h[key] = i
		if key == "msisdn" {
			found = true
		}
	}
	if !found {
		return nil, false
	}
	return h, true
}

func recipientHeaderKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "\ufeff")
	switch s {
	case "msisdn", "number", "phone", "phone_number", "to", "номер", "телефон":
		return "msisdn"
	default:
		return ""
	}
}

func recipientCell(rec []string, header map[string]int) string {
	if len(header) > 0 {
		idx, ok := header["msisdn"]
		if !ok {
			return ""
		}
		return csvutil.Cell(rec, idx)
	}
	if len(rec) == 0 {
		return ""
	}
	return strings.TrimSpace(rec[0])
}

func NormalizeRecipientList(raw []string) (msisdns []string, invalid []InvalidRow) {
	seen := map[string]struct{}{}
	for i, v := range raw {
		dest, err := msisdn.NormalizeDest(v)
		if err != nil {
			invalid = append(invalid, InvalidRow{Line: i + 1, Value: v, Error: err.Error()})
			continue
		}
		if _, ok := seen[dest.MSISDN]; ok {
			continue
		}
		seen[dest.MSISDN] = struct{}{}
		msisdns = append(msisdns, dest.MSISDN)
	}
	return msisdns, invalid
}

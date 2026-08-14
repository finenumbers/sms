package inventory

import (
	"encoding/csv"
	"io"
	"strings"

	"finenumbers/sms/internal/csvutil"
	"finenumbers/sms/internal/msisdn"
)

type ParsedRow struct {
	Line   int
	MSISDN string
	Region string
	Notes  string
}

type InvalidRow struct {
	Line  int    `json:"line"`
	Value string `json:"value"`
	Error string `json:"error"`
}

type ParseResult struct {
	Rows     []ParsedRow
	Invalid  []InvalidRow
	Encoding string
}

func ParseNumberFile(raw []byte) ParseResult {
	text, enc := csvutil.Decode(raw)
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = csvutil.DetectComma(text)
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	out := ParseResult{Encoding: enc, Rows: make([]ParsedRow, 0), Invalid: make([]InvalidRow, 0)}
	header := map[string]int{}
	line := 0
	first := true
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			out.Invalid = append(out.Invalid, InvalidRow{Line: line, Value: "", Error: "csv parse error"})
			continue
		}
		if csvutil.EmptyRecord(rec) || csvutil.Comment(rec) {
			continue
		}
		if first {
			first = false
			if h, ok := parseHeader(rec); ok {
				header = h
				continue
			}
		}
		row, inv := rowFromRecord(line, rec, header)
		if inv != nil {
			out.Invalid = append(out.Invalid, *inv)
			continue
		}
		out.Rows = append(out.Rows, row)
	}
	return out
}

func parseHeader(rec []string) (map[string]int, bool) {
	h := map[string]int{}
	found := false
	for i, c := range rec {
		key := headerKey(c)
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

func headerKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "\ufeff")
	switch s {
	case "msisdn", "number", "phone", "phone_number", "номер", "телефон":
		return "msisdn"
	case "region", "регион":
		return "region"
	case "notes", "note", "comment", "заметки", "комментарий":
		return "notes"
	default:
		return ""
	}
}

func rowFromRecord(line int, rec []string, header map[string]int) (ParsedRow, *InvalidRow) {
	raw := ""
	region, notes := "", ""
	if len(header) > 0 {
		raw = cellAt(rec, header, "msisdn")
		region = cellAt(rec, header, "region")
		notes = cellAt(rec, header, "notes")
	} else {
		if len(rec) > 0 {
			raw = strings.TrimSpace(rec[0])
		}
		if len(rec) > 1 {
			region = strings.TrimSpace(rec[1])
		}
		if len(rec) > 2 {
			notes = strings.TrimSpace(rec[2])
		}
	}
	n, err := msisdn.NormalizeSender(raw)
	if err != nil {
		return ParsedRow{}, &InvalidRow{Line: line, Value: raw, Error: err.Error()}
	}
	return ParsedRow{Line: line, MSISDN: n, Region: region, Notes: notes}, nil
}

func cellAt(rec []string, header map[string]int, key string) string {
	idx, ok := header[key]
	if !ok {
		return ""
	}
	return cell(rec, idx)
}

func cell(rec []string, idx int) string {
	if idx < 0 || idx >= len(rec) {
		return ""
	}
	return csvutil.Cell(rec, idx)
}

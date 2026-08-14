package runexis

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// Callback is a normalized DLR or MO payload. Field names follow the statistic
// wire (the only documented SMS shape). Live capture should replace the
// provisional fixtures; this parser stays conservative.
type Callback struct {
	SMSID     string `json:"sms_id,omitempty"`
	From      string `json:"sender_number,omitempty"`
	To        string `json:"receiver_number,omitempty"`
	Text      string `json:"message,omitempty"`
	Incoming  bool   `json:"incoming,omitempty"`
	PDU       int    `json:"pdu,omitempty"`
	Sent      bool   `json:"sent,omitempty"`
	Delivered bool   `json:"delivered,omitempty"`
	Failed    bool   `json:"failed,omitempty"`
	Status    string `json:"status,omitempty"`
}

func ParseCallbacks(query, contentType string, body []byte) []Callback {
	var out []Callback
	if rows := parseJSONCallbacks(body); len(rows) > 0 {
		out = append(out, rows...)
	}
	ct := strings.ToLower(contentType)
	if len(out) == 0 && strings.Contains(ct, "application/x-www-form-urlencoded") {
		if vals, err := url.ParseQuery(string(bytes.TrimSpace(body))); err == nil {
			if row, ok := callbackFromValues(vals); ok {
				out = append(out, row)
			}
		}
	}
	if query != "" {
		if vals, err := url.ParseQuery(query); err == nil {
			if row, ok := callbackFromValues(vals); ok {
				out = mergeCallback(out, row)
			}
		}
	}
	for i := range out {
		out[i] = normalizeCallback(out[i])
	}
	filtered := out[:0]
	for _, row := range out {
		if row.SMSID != "" || row.From != "" || row.To != "" || row.Text != "" || row.Status != "" || row.Sent || row.Delivered || row.Failed {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func parseJSONCallbacks(body []byte) []Callback {
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 {
		return nil
	}
	if rows, ok := callbacksFromRaw(trim); ok {
		return rows
	}
	var env wireEnvelope
	if json.Unmarshal(trim, &env) != nil {
		return nil
	}
	if len(bytes.TrimSpace(env.Data)) == 0 {
		return nil
	}
	rows, _ := callbacksFromRaw(env.Data)
	return rows
}

func callbacksFromRaw(raw json.RawMessage) ([]Callback, bool) {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return nil, false
	}
	if trim[0] == '[' {
		var maps []map[string]any
		if json.Unmarshal(trim, &maps) != nil {
			return nil, false
		}
		out := make([]Callback, 0, len(maps))
		for _, m := range maps {
			if row, ok := callbackFromMap(m); ok {
				out = append(out, row)
			}
		}
		return out, len(out) > 0
	}
	var m map[string]any
	if json.Unmarshal(trim, &m) != nil {
		return nil, false
	}
	if nested, ok := m["data"]; ok {
		if b, err := json.Marshal(nested); err == nil {
			if rows, ok := callbacksFromRaw(b); ok {
				return rows, true
			}
		}
	}
	row, ok := callbackFromMap(m)
	if !ok {
		return nil, false
	}
	return []Callback{row}, true
}

func callbackFromValues(v url.Values) (Callback, bool) {
	m := map[string]any{}
	for k, vals := range v {
		if len(vals) == 0 {
			continue
		}
		m[k] = vals[0]
	}
	return callbackFromMap(m)
}

func callbackFromMap(m map[string]any) (Callback, bool) {
	if len(m) == 0 {
		return Callback{}, false
	}
	row := Callback{
		SMSID:     firstString(m, "sms_id", "id", "message_id", "smsId"),
		From:      firstString(m, "sender_number", "from_number", "from", "from_msisdn"),
		To:        firstString(m, "receiver_number", "to_number", "to", "to_msisdn"),
		Text:      firstString(m, "message", "text", "body"),
		Status:    firstString(m, "status", "state", "dlr_status"),
		Incoming:  firstBool(m, "incoming"),
		Sent:      firstBool(m, "sent"),
		Delivered: firstBool(m, "delivered"),
		Failed:    firstBool(m, "failed"),
		PDU:       firstInt(m, "pdu"),
	}
	if row.SMSID == "" && row.From == "" && row.To == "" && row.Text == "" && row.Status == "" && !row.Sent && !row.Delivered && !row.Failed {
		return Callback{}, false
	}
	return row, true
}

func normalizeCallback(row Callback) Callback {
	st := strings.ToLower(strings.TrimSpace(row.Status))
	row.Status = st
	switch st {
	case "delivered", "delivery", "ok", "success":
		row.Delivered = true
		row.Sent = true
	case "sent", "accepted":
		row.Sent = true
	case "failed", "fail", "undelivered", "not_delivered", "error":
		row.Failed = true
	}
	if row.Delivered {
		row.Sent = true
		row.Failed = false
	}
	return row
}

func mergeCallback(existing []Callback, extra Callback) []Callback {
	if len(existing) == 0 {
		return []Callback{extra}
	}
	dst := existing[0]
	if dst.SMSID == "" {
		dst.SMSID = extra.SMSID
	}
	if dst.From == "" {
		dst.From = extra.From
	}
	if dst.To == "" {
		dst.To = extra.To
	}
	if dst.Text == "" {
		dst.Text = extra.Text
	}
	if dst.Status == "" {
		dst.Status = extra.Status
	}
	dst.Incoming = dst.Incoming || extra.Incoming
	dst.Sent = dst.Sent || extra.Sent
	dst.Delivered = dst.Delivered || extra.Delivered
	dst.Failed = dst.Failed || extra.Failed
	if dst.PDU == 0 {
		dst.PDU = extra.PDU
	}
	existing[0] = dst
	return existing
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if s := strings.TrimSpace(t); s != "" {
					return s
				}
			case float64:
				return strconv.FormatInt(int64(t), 10)
			case json.Number:
				return t.String()
			case json.RawMessage:
				var s string
				if json.Unmarshal(t, &s) == nil && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func firstBool(m map[string]any, keys ...string) bool {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case bool:
				return t
			case string:
				b, err := strconv.ParseBool(strings.TrimSpace(t))
				if err == nil {
					return b
				}
			case float64:
				return t != 0
			}
		}
	}
	return false
}

func firstInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case float64:
				return int(t)
			case json.Number:
				n, _ := t.Int64()
				return int(n)
			case string:
				n, err := strconv.Atoi(strings.TrimSpace(t))
				if err == nil {
					return n
				}
			}
		}
	}
	return 0
}

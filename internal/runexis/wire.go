package runexis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Domain types. Wire field names stay in this package.

type Credentials struct {
	Email    string
	Password string
}

type CredentialSource interface {
	RunexisCredentials(ctx context.Context) (Credentials, error)
}

type Tokens struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
}

type Account struct {
	Name  string
	Email string
}

type SendInput struct {
	From string
	To   string
	Text string
}

type SendResult struct {
	ProviderSMSID string
	Raw           json.RawMessage
}

type StatisticQuery struct {
	From            time.Time
	To              time.Time
	SenderNumbers   []string
	ReceiverNumbers []string
	Incoming        *bool
	Page            int
	Limit           int
}

type StatisticPage struct {
	Items []StatisticRow
	Total int
	Page  int
	Limit int
}

type StatisticRow struct {
	SMSID          string
	Date           string
	SenderNumber   string
	ReceiverNumber string
	Message        string
	Incoming       bool
	PDU            int
	Sent           bool
	Delivered      bool
}

type APIError struct {
	Status    int
	Code      int
	Message   string
	RequestID string
}

func (e *APIError) Error() string {
	if e == nil {
		return "runexis api error"
	}
	msg := e.Message
	if msg == "" {
		if e.Status > 0 {
			msg = fmt.Sprintf("http %d", e.Status)
		} else {
			msg = "api error"
		}
	}
	if e.RequestID != "" {
		return fmt.Sprintf("runexis: %s (request_id=%s)", msg, e.RequestID)
	}
	return fmt.Sprintf("runexis: %s", msg)
}

type wireEnvelope struct {
	Data      json.RawMessage `json:"data"`
	Meta      json.RawMessage `json:"meta"`
	Success   bool            `json:"success"`
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	RequestID string          `json:"request_id"`
}

type wireLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type wireRefreshRequest struct {
	Token string `json:"token"`
}

type wireTokenData struct {
	Token              string `json:"token"`
	RefreshToken       string `json:"refresh_token"`
	TokenExpire        string `json:"token_expire"`
	RefreshTokenExpire string `json:"refresh_token_expire"`
}

type wireMeData struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

type wireSendRequest struct {
	FromNumber string `json:"from_number"`
	ToNumber   string `json:"to_number"`
	Text       string `json:"text"`
}

type wireStatisticRequest struct {
	From            string   `json:"from"`
	To              string   `json:"to"`
	SenderNumbers   []string `json:"sender_numbers,omitempty"`
	ReceiverNumbers []string `json:"receiver_numbers,omitempty"`
	Incoming        *bool    `json:"incoming,omitempty"`
	Page            int      `json:"page,omitempty"`
	Limit           int      `json:"limit,omitempty"`
}

type wireStatisticRow struct {
	SMSID          string `json:"sms_id"`
	Date           string `json:"date"`
	SenderNumber   string `json:"sender_number"`
	ReceiverNumber string `json:"receiver_number"`
	Message        string `json:"message"`
	Incoming       bool   `json:"incoming"`
	PDU            int    `json:"pdu"`
	Sent           bool   `json:"sent"`
	Delivered      bool   `json:"delivered"`
}

type wireStatisticMeta struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

func marshalSend(in SendInput) ([]byte, error) {
	from := strings.TrimSpace(in.From)
	to := strings.TrimSpace(in.To)
	if len(from) != 11 || from[0] != '7' {
		return nil, fmt.Errorf("from_number: want 7XXXXXXXXXX")
	}
	if !validDestMSISDN(to) {
		return nil, fmt.Errorf("to_number: invalid msisdn")
	}
	return json.Marshal(wireSendRequest{
		FromNumber: from,
		ToNumber:   to,
		Text:       in.Text,
	})
}

func validDestMSISDN(s string) bool {
	n := len(s)
	if n < 8 || n > 15 {
		return false
	}
	for i := 0; i < n; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func parseSendResponse(body []byte) (SendResult, error) {
	raw := json.RawMessage(bytes.Clone(body))
	env, err := decodeEnvelope(body)
	if err != nil {
		return SendResult{Raw: raw}, err
	}
	if !env.Success {
		return SendResult{Raw: raw}, envelopeError(0, env)
	}
	return SendResult{ProviderSMSID: extractSMSID(env.Data), Raw: raw}, nil
}

func extractSMSID(data json.RawMessage) string {
	trim := bytes.TrimSpace(data)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) || bytes.Equal(trim, []byte("{}")) || bytes.Equal(trim, []byte("[]")) {
		return ""
	}
	var s string
	if json.Unmarshal(trim, &s) == nil {
		return s
	}
	var obj map[string]any
	if json.Unmarshal(trim, &obj) != nil {
		return ""
	}
	for _, k := range []string{"sms_id", "id", "message_id"} {
		if v, ok := obj[k]; ok {
			switch t := v.(type) {
			case string:
				if t != "" {
					return t
				}
			}
		}
	}
	return ""
}

func decodeEnvelope(body []byte) (wireEnvelope, error) {
	var env wireEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return env, fmt.Errorf("runexis envelope: %w", err)
	}
	return env, nil
}

func envelopeError(status int, env wireEnvelope) error {
	return &APIError{Status: status, Code: env.Code, Message: env.Message, RequestID: env.RequestID}
}

func parseTokens(data json.RawMessage) (Tokens, error) {
	var d wireTokenData
	if err := json.Unmarshal(data, &d); err != nil {
		return Tokens{}, fmt.Errorf("token payload: %w", err)
	}
	if d.Token == "" || d.RefreshToken == "" {
		return Tokens{}, fmt.Errorf("token payload missing token or refresh_token")
	}
	accessExp, err := parseRunexisTime(d.TokenExpire)
	if err != nil {
		return Tokens{}, fmt.Errorf("token_expire: %w", err)
	}
	refreshExp, err := parseRunexisTime(d.RefreshTokenExpire)
	if err != nil {
		return Tokens{}, fmt.Errorf("refresh_token_expire: %w", err)
	}
	return Tokens{
		AccessToken:      d.Token,
		RefreshToken:     d.RefreshToken,
		AccessExpiresAt:  accessExp,
		RefreshExpiresAt: refreshExp,
	}, nil
}

func parseRunexisTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000Z",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q", s)
}

func formatStatisticTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

func marshalStatistic(q StatisticQuery) ([]byte, error) {
	if q.From.IsZero() || q.To.IsZero() {
		return nil, fmt.Errorf("statistic from/to required")
	}
	body := wireStatisticRequest{
		From:            formatStatisticTime(q.From),
		To:              formatStatisticTime(q.To),
		SenderNumbers:   q.SenderNumbers,
		ReceiverNumbers: q.ReceiverNumbers,
		Incoming:        q.Incoming,
		Page:            q.Page,
		Limit:           q.Limit,
	}
	return json.Marshal(body)
}

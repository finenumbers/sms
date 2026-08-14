package runexis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"finenumbers/sms/internal/msisdn"
)

var ErrNoSMS = errors.New("sms account not available")

type SMSDirections struct {
	In     bool
	DomOut bool
	IntOut bool
	InMass bool
}

type wireDirections struct {
	In     bool `json:"in"`
	DomOut bool `json:"dom_out"`
	IntOut bool `json:"int_out"`
	InMass bool `json:"in_mass"`
}

func (c *Client) SetSMSDirections(ctx context.Context, number string, d SMSDirections) error {
	if !msisdn.IsSender(number) {
		return fmt.Errorf("sms directions: invalid msisdn")
	}
	var env wireEnvelope
	_, err := c.doJSON(ctx, http.MethodPatch, "/api/v1/numbers/"+number+"/sms/directions", wireDirections{
		In:     d.In,
		DomOut: d.DomOut,
		IntOut: d.IntOut,
		InMass: d.InMass,
	}, true, &env)
	if err != nil {
		return err
	}
	if !env.Success {
		return envelopeError(0, env)
	}
	return nil
}

type ManagedNumber struct {
	Code     string
	Number   string
	CityName string
	Snapshot json.RawMessage
}

type ManagedNumbersPage struct {
	Items []ManagedNumber
	Total int
	Page  int
	Limit int
}

type SMSAccount struct {
	In     bool
	DomOut bool
	IntOut bool
	InMass bool
}

type wireManagedNumber struct {
	ID       string          `json:"id"`
	Code     string          `json:"code"`
	Number   string          `json:"number"`
	Status   json.RawMessage `json:"status"`
	City     json.RawMessage `json:"city"`
	Tariff   json.RawMessage `json:"tariff"`
	Class    json.RawMessage `json:"class"`
	Operator json.RawMessage `json:"operator"`
}

type wireNamed struct {
	Name string `json:"name"`
}

func (c *Client) ListManagedNumbers(ctx context.Context, page, limit int) (ManagedNumbersPage, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 30
	}
	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))
	path := "/api/v1/numbers/management?" + q.Encode()
	var env wireEnvelope
	if _, err := c.doJSON(ctx, http.MethodGet, path, nil, true, &env); err != nil {
		return ManagedNumbersPage{}, err
	}
	if !env.Success {
		return ManagedNumbersPage{}, envelopeError(0, env)
	}
	return parseManagedNumbersPage(env, page, limit)
}

func parseManagedNumbersPage(env wireEnvelope, page, limit int) (ManagedNumbersPage, error) {
	out := ManagedNumbersPage{Page: page, Limit: limit, Items: []ManagedNumber{}}
	trim := bytes.TrimSpace(env.Data)
	if len(trim) > 0 && !bytes.Equal(trim, []byte("null")) {
		var rows []wireManagedNumber
		if err := json.Unmarshal(env.Data, &rows); err != nil {
			return ManagedNumbersPage{}, fmt.Errorf("management data: %w", err)
		}
		out.Items = make([]ManagedNumber, 0, len(rows))
		for _, r := range rows {
			out.Items = append(out.Items, ManagedNumber{
				Code:     r.Code,
				Number:   r.Number,
				CityName: namedName(r.City),
				Snapshot: managementSnapshot(r),
			})
		}
	}
	if len(env.Meta) > 0 {
		var meta wireStatisticMeta
		if json.Unmarshal(env.Meta, &meta) == nil {
			if meta.Total > 0 {
				out.Total = meta.Total
			}
			if meta.Page > 0 {
				out.Page = meta.Page
			}
			if meta.Limit > 0 {
				out.Limit = meta.Limit
			}
		}
	}
	return out, nil
}

func namedName(raw json.RawMessage) string {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return ""
	}
	var n wireNamed
	if json.Unmarshal(trim, &n) != nil {
		return ""
	}
	return n.Name
}

func managementSnapshot(r wireManagedNumber) json.RawMessage {
	keep := map[string]any{}
	if r.ID != "" {
		keep["id"] = r.ID
	}
	if len(bytes.TrimSpace(r.Status)) > 0 && !bytes.Equal(bytes.TrimSpace(r.Status), []byte("null")) {
		keep["status"] = r.Status
	}
	if len(bytes.TrimSpace(r.City)) > 0 && !bytes.Equal(bytes.TrimSpace(r.City), []byte("null")) {
		keep["city"] = r.City
	}
	if len(bytes.TrimSpace(r.Tariff)) > 0 && !bytes.Equal(bytes.TrimSpace(r.Tariff), []byte("null")) {
		keep["tariff"] = r.Tariff
	}
	if len(bytes.TrimSpace(r.Class)) > 0 && !bytes.Equal(bytes.TrimSpace(r.Class), []byte("null")) {
		keep["class"] = r.Class
	}
	if len(bytes.TrimSpace(r.Operator)) > 0 && !bytes.Equal(bytes.TrimSpace(r.Operator), []byte("null")) {
		keep["operator"] = r.Operator
	}
	b, err := json.Marshal(keep)
	if err != nil {
		return nil
	}
	return b
}

func (c *Client) SMSAccount(ctx context.Context, number string) (SMSAccount, error) {
	if !msisdn.IsSender(number) {
		return SMSAccount{}, fmt.Errorf("sms account: invalid msisdn")
	}
	var env wireEnvelope
	_, err := c.doJSON(ctx, http.MethodGet, "/api/v1/numbers/"+number+"/sms/account", nil, true, &env)
	if err != nil {
		if IsNoSMS(err) {
			return SMSAccount{}, fmt.Errorf("%w: %w", ErrNoSMS, err)
		}
		return SMSAccount{}, err
	}
	if !env.Success {
		return SMSAccount{}, envelopeError(0, env)
	}
	var d wireDirections
	trim := bytes.TrimSpace(env.Data)
	if len(trim) > 0 && !bytes.Equal(trim, []byte("null")) {
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return SMSAccount{}, fmt.Errorf("sms account data: %w", err)
		}
	}
	return SMSAccount{In: d.In, DomOut: d.DomOut, IntOut: d.IntOut, InMass: d.InMass}, nil
}

func IsNoSMS(err error) bool {
	if errors.Is(err, ErrNoSMS) {
		return true
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.Status == http.StatusUnauthorized {
		return false
	}
	return apiErr.Status >= 400 && apiErr.Status < 500
}

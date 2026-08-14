package runexis

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

func (c *Client) Send(ctx context.Context, in SendInput) (SendResult, error) {
	raw, err := marshalSend(in)
	if err != nil {
		return SendResult{}, err
	}
	var wire wireSendRequest
	if err := json.Unmarshal(raw, &wire); err != nil {
		return SendResult{}, err
	}
	var env wireEnvelope
	body, err := c.doJSON(ctx, http.MethodPost, "/api/v1/sms/send", wire, true, &env)
	out := SendResult{Raw: json.RawMessage(bytes.Clone(body))}
	if err != nil {
		return out, err
	}
	parsed, perr := parseSendResponse(body)
	if parsed.ProviderSMSID != "" {
		out.ProviderSMSID = parsed.ProviderSMSID
	}
	if len(parsed.Raw) > 0 {
		out.Raw = parsed.Raw
	}
	return out, perr
}

func (c *Client) Statistic(ctx context.Context, q StatisticQuery) (StatisticPage, error) {
	raw, err := marshalStatistic(q)
	if err != nil {
		return StatisticPage{}, err
	}
	var wire wireStatisticRequest
	if err := json.Unmarshal(raw, &wire); err != nil {
		return StatisticPage{}, err
	}
	var env wireEnvelope
	if _, err := c.doJSON(ctx, http.MethodGet, "/api/v1/sms/statistic", wire, true, &env); err != nil {
		return StatisticPage{}, err
	}
	if !env.Success {
		return StatisticPage{}, envelopeError(0, env)
	}
	var rows []wireStatisticRow
	trim := bytes.TrimSpace(env.Data)
	if len(trim) > 0 && !bytes.Equal(trim, []byte("null")) {
		if err := json.Unmarshal(env.Data, &rows); err != nil {
			return StatisticPage{}, err
		}
	}
	out := StatisticPage{Items: make([]StatisticRow, 0, len(rows)), Total: len(rows), Page: q.Page, Limit: q.Limit}
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
	for _, r := range rows {
		out.Items = append(out.Items, StatisticRow{
			SMSID:          r.SMSID,
			Date:           r.Date,
			SenderNumber:   r.SenderNumber,
			ReceiverNumber: r.ReceiverNumber,
			Message:        r.Message,
			Incoming:       r.Incoming,
			PDU:            r.PDU,
			Sent:           r.Sent,
			Delivered:      r.Delivered,
		})
	}
	return out, nil
}

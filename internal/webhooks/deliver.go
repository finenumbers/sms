package webhooks

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func (s *Service) EnqueueItem(ctx context.Context, item sqlcdb.LookupItem) (int, error) {
	if s == nil || s.store == nil {
		return 0, nil
	}
	return s.enqueue(ctx, item.ClientID, EventForItemStatus(string(item.Status)), &item.JobID, &item.ID, CheckData(item))
}

func (s *Service) EnqueueJob(ctx context.Context, job sqlcdb.LookupJob) (int, error) {
	if s == nil || s.store == nil {
		return 0, nil
	}
	switch job.Status {
	case sqlcdb.LookupJobStatusCompleted, sqlcdb.LookupJobStatusCompletedWithErrors, sqlcdb.LookupJobStatusFailed:
	default:
		return 0, nil
	}
	return s.enqueue(ctx, job.ClientID, EventJobCompleted, &job.ID, nil, JobData(job))
}

func (s *Service) enqueue(ctx context.Context, clientID uuid.UUID, eventType string, jobID, itemID *uuid.UUID, data map[string]any) (int, error) {
	if !KnownEvent(eventType) {
		return 0, nil
	}
	if cl, err := s.store.Queries.GetClientByID(ctx, clientID); err == nil && cl.Status != sqlcdb.ClientStatusActive {
		return 0, nil
	}
	endpoints, err := s.store.Queries.ListEnabledWebhookEndpoints(ctx, clientID)
	if err != nil {
		return 0, err
	}
	maxAttempts := int32(8)
	if s.settings != nil {
		if view, err := s.settings.Get(ctx); err == nil && view.LookupWebhookMaxAttempts > 0 {
			maxAttempts = view.LookupWebhookMaxAttempts
		}
	}
	now := s.now()
	n := 0
	for i := range endpoints {
		ep := endpoints[i]
		if !Subscribes(ep.Events, eventType) {
			continue
		}
		id := uuid.New()
		env := Envelope(id.String(), eventType, now, data)
		payload, err := Serialize(env)
		if err != nil {
			return n, err
		}
		if _, err := s.store.Queries.InsertWebhookDelivery(ctx, sqlcdb.InsertWebhookDeliveryParams{
			ID:            id,
			ClientID:      clientID,
			EndpointID:    ep.ID,
			JobID:         jobID,
			JobItemID:     itemID,
			EventType:     eventType,
			Payload:       payload,
			Status:        sqlcdb.WebhookDeliveryStatusPending,
			MaxAttempts:   maxAttempts,
			NextAttemptAt: &now,
		}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Service) DeliverDue(ctx context.Context, limit int32) (int, error) {
	if s == nil || s.store == nil {
		return 0, nil
	}
	if limit < 1 {
		limit = 10
	}
	rows, err := s.store.Queries.ClaimWebhookDeliveries(ctx, limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range rows {
		if err := s.deliverOne(ctx, rows[i]); err != nil && s.log != nil {
			s.log.Error("webhook deliver", "delivery_id", rows[i].ID, "err", err)
			continue
		}
		n++
	}
	return n, nil
}

func (s *Service) deliverOne(ctx context.Context, delivery sqlcdb.WebhookDelivery) error {
	if delivery.Status == sqlcdb.WebhookDeliveryStatusDelivered || delivery.Status == sqlcdb.WebhookDeliveryStatusDead {
		return nil
	}
	ep, err := s.store.Queries.GetWebhookEndpoint(ctx, delivery.EndpointID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			dead := sqlcdb.WebhookDeliveryStatusDead
			msg := "endpoint missing"
			_, _ = s.store.Queries.MarkWebhookDeliveryAttempt(ctx, sqlcdb.MarkWebhookDeliveryAttemptParams{
				Status:       dead,
				AttemptCount: delivery.AttemptCount,
				LastError:    &msg,
				ID:           delivery.ID,
			})
			return nil
		}
		return err
	}
	if !ep.Enabled {
		dead := sqlcdb.WebhookDeliveryStatusDead
		msg := "endpoint disabled"
		_, _ = s.store.Queries.MarkWebhookDeliveryAttempt(ctx, sqlcdb.MarkWebhookDeliveryAttemptParams{
			Status:       dead,
			AttemptCount: delivery.AttemptCount,
			LastError:    &msg,
			ID:           delivery.ID,
		})
		return nil
	}
	secretPlain, err := s.kr.Decrypt(ep.SecretCiphertext, ep.DekKeyID)
	if err != nil {
		return err
	}
	timeout := 5 * time.Second
	if s.settings != nil {
		if view, err := s.settings.Get(ctx); err == nil && view.LookupWebhookTimeoutMs >= 100 {
			timeout = time.Duration(view.LookupWebhookTimeoutMs) * time.Millisecond
		}
	}
	header, _ := Sign(string(secretPlain), string(delivery.Payload), s.now().Unix())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.Url, bytes.NewReader(delivery.Payload))
	if err != nil {
		return s.failAttempt(ctx, delivery, ep, 0, err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("X-Finenumbers-Signature", header)
	req.Header.Set("X-Finenumbers-Delivery-Id", delivery.ID.String())
	req.Header.Set("X-Finenumbers-Event", delivery.EventType)

	httpClient := s.http
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	if c, ok := httpClient.(*http.Client); ok && c.Timeout == 0 {
		clone := *c
		clone.Timeout = timeout
		httpClient = &clone
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req = req.WithContext(callCtx)

	resp, err := httpClient.Do(req)
	if err != nil {
		return s.failAttempt(ctx, delivery, ep, 0, err.Error())
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	code := int32(resp.StatusCode)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if _, err := s.store.Queries.MarkWebhookDeliveryDelivered(ctx, sqlcdb.MarkWebhookDeliveryDeliveredParams{
			AttemptCount:     delivery.AttemptCount + 1,
			LastResponseCode: &code,
			ID:               delivery.ID,
		}); err != nil {
			return err
		}
		_ = s.store.Queries.ResetWebhookEndpointFailures(ctx, ep.ID)
		return nil
	}
	msg := "HTTP " + resp.Status
	if text := strings.TrimSpace(string(body)); text != "" {
		msg += ": " + truncate(text, 200)
	}
	return s.failAttempt(ctx, delivery, ep, code, msg)
}

func (s *Service) failAttempt(ctx context.Context, delivery sqlcdb.WebhookDelivery, ep sqlcdb.WebhookEndpoint, code int32, message string) error {
	attempt := delivery.AttemptCount + 1
	dead := attempt >= delivery.MaxAttempts
	status := sqlcdb.WebhookDeliveryStatusFailed
	var next *time.Time
	if dead {
		status = sqlcdb.WebhookDeliveryStatusDead
	} else {
		next = NextAttemptAt(attempt, delivery.MaxAttempts, s.now())
	}
	msg := truncate(scrubBrandString(message), 200)
	var codePtr *int32
	if code > 0 {
		codePtr = &code
	}
	if _, err := s.store.Queries.MarkWebhookDeliveryAttempt(ctx, sqlcdb.MarkWebhookDeliveryAttemptParams{
		Status:           status,
		AttemptCount:     attempt,
		LastResponseCode: codePtr,
		LastError:        &msg,
		NextAttemptAt:    next,
		ID:               delivery.ID,
	}); err != nil {
		return err
	}
	updated, err := s.store.Queries.IncrementWebhookEndpointFailures(ctx, sqlcdb.IncrementWebhookEndpointFailuresParams{
		DisableAfter: AutoDisableAfter,
		ID:           ep.ID,
	})
	if err != nil && s.log != nil {
		s.log.Error("webhook increment failures", "endpoint_id", ep.ID, "err", err)
	} else if s.log != nil && !updated.Enabled {
		s.log.Warn("webhook auto-disabled", "endpoint_id", ep.ID, "consecutive_failures", updated.ConsecutiveFailures)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func scrubBrandString(s string) string {
	if v, ok := scrubBrand(s).(string); ok {
		return v
	}
	return s
}

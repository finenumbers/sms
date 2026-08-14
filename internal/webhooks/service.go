package webhooks

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/secret"
	"finenumbers/sms/internal/settings"
)

const maxEndpointsPerClient = 20

type SettingsView interface {
	Get(ctx context.Context) (settings.Public, error)
}

type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type Service struct {
	store    *db.Store
	kr       *secret.Keyring
	settings SettingsView
	http     HTTPDoer
	log      *slog.Logger
	now      func() time.Time
}

func New(store *db.Store, kr *secret.Keyring, settings SettingsView, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store:    store,
		kr:       kr,
		settings: settings,
		http:     &http.Client{},
		log:      log,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) SetHTTP(d HTTPDoer) {
	if s != nil && d != nil {
		s.http = d
	}
}

type CreateInput struct {
	ClientID    uuid.UUID
	URL         string
	Description *string
	Events      []string
	Enabled     *bool
}

type PatchInput struct {
	URL         *string
	Description *string
	Events      *[]string
	Enabled     *bool
}

func (s *Service) Create(ctx context.Context, in CreateInput) (sqlcdb.WebhookEndpoint, string, error) {
	if err := s.ensureReady(); err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	u, err := normalizeURL(in.URL)
	if err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	events, err := normalizeEvents(in.Events)
	if err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	n, err := s.store.Queries.CountWebhookEndpointsForClient(ctx, in.ClientID)
	if err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	if n >= maxEndpointsPerClient {
		return sqlcdb.WebhookEndpoint{}, "", wrap(ErrValidation, "validation", "webhook endpoint limit reached")
	}
	secretPlain, err := generateSecret()
	if err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	ct, kid, err := s.kr.Encrypt([]byte(secretPlain))
	if err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	row, err := s.store.Queries.InsertWebhookEndpoint(ctx, sqlcdb.InsertWebhookEndpointParams{
		ClientID:         in.ClientID,
		Url:              u,
		SecretCiphertext: ct,
		DekKeyID:         kid,
		Description:      trimPtr(in.Description),
		Enabled:          enabled,
		Events:           events,
	})
	if err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	return row, secretPlain, nil
}

func (s *Service) GetForClient(ctx context.Context, clientID, id uuid.UUID) (sqlcdb.WebhookEndpoint, error) {
	row, err := s.store.Queries.GetWebhookEndpointForClient(ctx, sqlcdb.GetWebhookEndpointForClientParams{
		ID:       id,
		ClientID: clientID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.WebhookEndpoint{}, wrap(ErrNotFound, "not_found", "webhook not found")
		}
		return sqlcdb.WebhookEndpoint{}, err
	}
	return row, nil
}

func (s *Service) List(ctx context.Context, clientID uuid.UUID, limit, offset int32) ([]sqlcdb.WebhookEndpoint, int64, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.store.Queries.ListWebhookEndpointsForClient(ctx, sqlcdb.ListWebhookEndpointsForClientParams{
		ClientID:   clientID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	n, err := s.store.Queries.CountWebhookEndpointsForClient(ctx, clientID)
	if err != nil {
		return nil, 0, err
	}
	return rows, n, nil
}

func (s *Service) Patch(ctx context.Context, clientID, id uuid.UUID, in PatchInput) (sqlcdb.WebhookEndpoint, error) {
	if _, err := s.GetForClient(ctx, clientID, id); err != nil {
		return sqlcdb.WebhookEndpoint{}, err
	}
	arg := sqlcdb.UpdateWebhookEndpointParams{ID: id, ClientID: clientID}
	if in.URL != nil {
		u, err := normalizeURL(*in.URL)
		if err != nil {
			return sqlcdb.WebhookEndpoint{}, err
		}
		arg.Url = &u
	}
	if in.Description != nil {
		arg.Description = trimPtr(in.Description)
	}
	if in.Events != nil {
		events, err := normalizeEvents(*in.Events)
		if err != nil {
			return sqlcdb.WebhookEndpoint{}, err
		}
		arg.Events = events
	}
	if in.Enabled != nil {
		arg.Enabled = in.Enabled
		if *in.Enabled {
			zero := int32(0)
			arg.ConsecutiveFailures = &zero
		}
	}
	return s.store.Queries.UpdateWebhookEndpoint(ctx, arg)
}

func (s *Service) RotateSecret(ctx context.Context, clientID, id uuid.UUID) (sqlcdb.WebhookEndpoint, string, error) {
	if err := s.ensureReady(); err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	if _, err := s.GetForClient(ctx, clientID, id); err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	secretPlain, err := generateSecret()
	if err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	ct, kid, err := s.kr.Encrypt([]byte(secretPlain))
	if err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	zero := int32(0)
	row, err := s.store.Queries.UpdateWebhookEndpoint(ctx, sqlcdb.UpdateWebhookEndpointParams{
		SecretCiphertext:    ct,
		DekKeyID:            &kid,
		ConsecutiveFailures: &zero,
		ID:                  id,
		ClientID:            clientID,
	})
	if err != nil {
		return sqlcdb.WebhookEndpoint{}, "", err
	}
	return row, secretPlain, nil
}

func (s *Service) Delete(ctx context.Context, clientID, id uuid.UUID) error {
	n, err := s.store.Queries.DeleteWebhookEndpoint(ctx, sqlcdb.DeleteWebhookEndpointParams{
		ID:       id,
		ClientID: clientID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return wrap(ErrNotFound, "not_found", "webhook not found")
	}
	return nil
}

func (s *Service) ListDeliveries(ctx context.Context, clientID uuid.UUID, endpointID *uuid.UUID, status *sqlcdb.WebhookDeliveryStatus, limit, offset int32) ([]sqlcdb.WebhookDelivery, int64, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	arg := sqlcdb.ListWebhookDeliveriesForClientParams{
		ClientID:     clientID,
		EndpointID:   endpointID,
		FilterStatus: nullDeliveryStatus(status),
		PageLimit:    limit,
		PageOffset:   offset,
	}
	rows, err := s.store.Queries.ListWebhookDeliveriesForClient(ctx, arg)
	if err != nil {
		return nil, 0, err
	}
	n, err := s.store.Queries.CountWebhookDeliveriesForClient(ctx, sqlcdb.CountWebhookDeliveriesForClientParams{
		ClientID:     clientID,
		EndpointID:   endpointID,
		FilterStatus: nullDeliveryStatus(status),
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, n, nil
}

func (s *Service) ensureReady() error {
	if s == nil || s.store == nil {
		return errors.New("webhooks not configured")
	}
	if s.kr == nil {
		return errors.New("webhook keyring not configured")
	}
	return nil
}

func generateSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", wrap(ErrValidation, "validation", "url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", wrap(ErrValidation, "validation", "url must be http or https")
	}
	return raw, nil
}

func normalizeEvents(events []string) ([]string, error) {
	if len(events) == 0 {
		return []string{}, nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, e := range events {
		e = strings.TrimSpace(e)
		if !KnownEvent(e) {
			return nil, wrap(ErrValidation, "validation", "unknown webhook event: "+e)
		}
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		out = append(out, e)
	}
	return out, nil
}

func trimPtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func nullDeliveryStatus(v *sqlcdb.WebhookDeliveryStatus) sqlcdb.NullWebhookDeliveryStatus {
	if v == nil {
		return sqlcdb.NullWebhookDeliveryStatus{}
	}
	return sqlcdb.NullWebhookDeliveryStatus{WebhookDeliveryStatus: *v, Valid: true}
}

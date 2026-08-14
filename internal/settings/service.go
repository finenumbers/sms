package settings

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/ingress"
	"finenumbers/sms/internal/secret"
)

var (
	ErrValidation    = errors.New("validation")
	ErrNotConfigured = errors.New("runexis credentials not configured")
	ErrDecrypt       = errors.New("settings secret cannot be decrypted")
)

type Store interface {
	GetSystemSettings(ctx context.Context) (sqlcdb.SystemSetting, error)
	UpdateSystemSettings(ctx context.Context, arg sqlcdb.UpdateSystemSettingsParams) (sqlcdb.SystemSetting, error)
}

type Service struct {
	q  Store
	kr *secret.Keyring
}

func New(q Store, kr *secret.Keyring) *Service {
	return &Service{q: q, kr: kr}
}

func (s *Service) Keyring() *secret.Keyring {
	if s == nil {
		return nil
	}
	return s.kr
}

type Directions struct {
	In     bool `json:"in"`
	DomOut bool `json:"dom_out"`
	IntOut bool `json:"int_out"`
	InMass bool `json:"in_mass"`
}

type Public struct {
	RunexisEmail             string     `json:"runexis_email,omitempty"`
	RunexisPasswordSet       bool       `json:"runexis_password_set"`
	DekKeyID                 string     `json:"dek_key_id,omitempty"`
	CallbackBaseURL          string     `json:"callback_base_url,omitempty"`
	SMSDirections            Directions `json:"sms_directions"`
	ProviderRPS              float64    `json:"provider_rps"`
	ClientRPSDefault         float64    `json:"client_rps_default"`
	RetentionDays            int32      `json:"retention_days"`
	AuditRetentionDays       int32      `json:"audit_retention_days"`
	OpsRetentionDays         int32      `json:"ops_retention_days"`
	BillingEnforced          bool       `json:"billing_enforced"`
	LowBalanceThreshold      string     `json:"low_balance_threshold"`
	IngressTokenSet          bool       `json:"ingress_token_set"`
	IngressToken             string     `json:"ingress_token,omitempty"`
	UpdatedAt                string     `json:"updated_at"`
	LookupEnabled            bool       `json:"lookup_enabled"`
	LookupCheckTimeoutSec    int32      `json:"lookup_check_timeout_sec"`
	LookupPollIntervalSec    int32      `json:"lookup_poll_interval_sec"`
	LookupMaxCSVRows         int32      `json:"lookup_max_csv_rows"`
	LookupMaxCSVBytes        int32      `json:"lookup_max_csv_bytes"`
	LookupMaxBatchPhones     int32      `json:"lookup_max_batch_phones"`
	LookupWebhookMaxAttempts int32      `json:"lookup_webhook_max_attempts"`
	LookupWebhookTimeoutMs   int32      `json:"lookup_webhook_timeout_ms"`
	LookupRetentionDays      int32      `json:"lookup_retention_days"`
	SMSCBaseURL              string     `json:"smsc_base_url,omitempty"`
	SMSCLogin                string     `json:"smsc_login,omitempty"`
	SMSCPasswordSet          bool       `json:"smsc_password_set"`
	SMSCAPIKeySet            bool       `json:"smsc_apikey_set"`
	SMSCCallbackSecretSet    bool       `json:"smsc_callback_secret_set"`
	SMSCCurrency             string     `json:"smsc_currency,omitempty"`
	SMSCCallbackURL          string     `json:"smsc_callback_url,omitempty"`
}

type Patch struct {
	RunexisEmail        *string
	RunexisPassword     *string
	CallbackBaseURL     *string
	SMSDirections       *Directions
	ProviderRPS         *float64
	ClientRPSDefault    *float64
	RetentionDays       *int32
	AuditRetentionDays  *int32
	OpsRetentionDays    *int32
	BillingEnforced     *bool
	LowBalanceThreshold *string
	RotateIngressToken         bool
	LookupEnabled              *bool
	LookupCheckTimeoutSec      *int32
	LookupPollIntervalSec      *int32
	LookupMaxCSVRows           *int32
	LookupMaxCSVBytes          *int32
	LookupMaxBatchPhones       *int32
	LookupWebhookMaxAttempts   *int32
	LookupWebhookTimeoutMs     *int32
	LookupRetentionDays        *int32
	SMSCBaseURL                *string
	SMSCLogin                  *string
	SMSCPassword               *string
	SMSCAPIKey                 *string
	SMSCCallbackSecret         *string
	SMSCCurrency               *string
	UpdatedBy                  uuid.UUID
}

type Credentials struct {
	Email    string
	Password string
}

func (s *Service) Get(ctx context.Context) (Public, error) {
	row, err := s.q.GetSystemSettings(ctx)
	if err != nil {
		return Public{}, err
	}
	return PublicView(row), nil
}

func (s *Service) Update(ctx context.Context, p Patch) (Public, bool, error) {
	row, err := s.q.GetSystemSettings(ctx)
	if err != nil {
		return Public{}, false, err
	}
	credsChanged := false

	if p.RunexisEmail != nil {
		email := strings.ToLower(strings.TrimSpace(*p.RunexisEmail))
		if email == "" {
			row.RunexisEmail = nil
			row.RunexisPasswordCiphertext = nil
			if !rowHasEncryptedSecrets(row) {
				row.DekKeyID = nil
			}
			credsChanged = true
		} else {
			if _, err := mail.ParseAddress(email); err != nil {
				return Public{}, false, fmt.Errorf("%w: invalid runexis_email", ErrValidation)
			}
			if row.RunexisEmail == nil || *row.RunexisEmail != email {
				credsChanged = true
			}
			row.RunexisEmail = &email
		}
	}
	if p.RunexisPassword != nil {
		pw := *p.RunexisPassword
		if strings.TrimSpace(pw) == "" {
			return Public{}, false, fmt.Errorf("%w: runexis_password is empty", ErrValidation)
		}
		ct, kid, err := s.kr.Encrypt([]byte(pw))
		if err != nil {
			return Public{}, false, err
		}
		row.RunexisPasswordCiphertext = ct
		row.DekKeyID = &kid
		credsChanged = true
	}
	if p.CallbackBaseURL != nil {
		u := strings.TrimSpace(*p.CallbackBaseURL)
		if u == "" {
			row.CallbackBaseUrl = nil
		} else {
			if err := validateCallbackURL(u); err != nil {
				return Public{}, false, err
			}
			row.CallbackBaseUrl = &u
		}
	}
	if p.SMSDirections != nil {
		row.SmsDirIn = p.SMSDirections.In
		row.SmsDirDomOut = p.SMSDirections.DomOut
		row.SmsDirIntOut = p.SMSDirections.IntOut
		row.SmsDirInMass = p.SMSDirections.InMass
	}
	if p.ProviderRPS != nil {
		if err := validateRPS(*p.ProviderRPS); err != nil {
			return Public{}, false, fmt.Errorf("%w: provider_rps: %s", ErrValidation, err.Error())
		}
		n, err := numericFromFloat(*p.ProviderRPS)
		if err != nil {
			return Public{}, false, err
		}
		row.ProviderRps = n
	}
	if p.ClientRPSDefault != nil {
		if err := validateRPS(*p.ClientRPSDefault); err != nil {
			return Public{}, false, fmt.Errorf("%w: client_rps_default: %s", ErrValidation, err.Error())
		}
		n, err := numericFromFloat(*p.ClientRPSDefault)
		if err != nil {
			return Public{}, false, err
		}
		row.ClientRpsDefault = n
	}
	if p.RetentionDays != nil {
		if *p.RetentionDays < 1 || *p.RetentionDays > 3650 {
			return Public{}, false, fmt.Errorf("%w: retention_days must be 1..3650", ErrValidation)
		}
		row.RetentionDays = *p.RetentionDays
	}
	if p.AuditRetentionDays != nil {
		if *p.AuditRetentionDays < 1 || *p.AuditRetentionDays > 3650 {
			return Public{}, false, fmt.Errorf("%w: audit_retention_days must be 1..3650", ErrValidation)
		}
		row.AuditRetentionDays = *p.AuditRetentionDays
	}
	if p.OpsRetentionDays != nil {
		if *p.OpsRetentionDays < 1 || *p.OpsRetentionDays > 90 {
			return Public{}, false, fmt.Errorf("%w: ops_retention_days must be 1..90", ErrValidation)
		}
		row.OpsRetentionDays = *p.OpsRetentionDays
	}
	row.BillingEnforced = true
	if p.LowBalanceThreshold != nil {
		d, err := decimal.NewFromString(strings.TrimSpace(*p.LowBalanceThreshold))
		if err != nil || d.IsNegative() {
			return Public{}, false, fmt.Errorf("%w: invalid low_balance_threshold", ErrValidation)
		}
		row.LowBalanceThreshold = d
	}
	if err := applyLookupPatch(&row, p); err != nil {
		return Public{}, false, err
	}
	if err := applySMSCPatch(&row, p, s); err != nil {
		return Public{}, false, err
	}
	if !rowHasEncryptedSecrets(row) {
		row.DekKeyID = nil
	}

	var issued string
	if p.RotateIngressToken {
		tok, err := newIngressToken()
		if err != nil {
			return Public{}, false, err
		}
		h := ingress.HashToken(tok)
		row.IngressTokenHash = &h
		issued = tok
	}

	updated, err := s.q.UpdateSystemSettings(ctx, sqlcdb.UpdateSystemSettingsParams{
		RunexisEmail:                 row.RunexisEmail,
		RunexisPasswordCiphertext:    row.RunexisPasswordCiphertext,
		DekKeyID:                     row.DekKeyID,
		CallbackBaseUrl:              row.CallbackBaseUrl,
		SmsDirIn:                     row.SmsDirIn,
		SmsDirDomOut:                 row.SmsDirDomOut,
		SmsDirIntOut:                 row.SmsDirIntOut,
		SmsDirInMass:                 row.SmsDirInMass,
		ProviderRps:                  row.ProviderRps,
		ClientRpsDefault:             row.ClientRpsDefault,
		RetentionDays:                row.RetentionDays,
		AuditRetentionDays:           row.AuditRetentionDays,
		OpsRetentionDays:             row.OpsRetentionDays,
		IngressTokenHash:             row.IngressTokenHash,
		BillingEnforced:              row.BillingEnforced,
		LowBalanceThreshold:          row.LowBalanceThreshold,
		LookupEnabled:                row.LookupEnabled,
		LookupCheckTimeoutSec:        row.LookupCheckTimeoutSec,
		LookupPollIntervalSec:        row.LookupPollIntervalSec,
		LookupMaxCsvRows:             row.LookupMaxCsvRows,
		LookupMaxCsvBytes:            row.LookupMaxCsvBytes,
		LookupMaxBatchPhones:         row.LookupMaxBatchPhones,
		LookupWebhookMaxAttempts:     row.LookupWebhookMaxAttempts,
		LookupWebhookTimeoutMs:       row.LookupWebhookTimeoutMs,
		LookupRetentionDays:          row.LookupRetentionDays,
		SmscBaseUrl:                  row.SmscBaseUrl,
		SmscLogin:                    row.SmscLogin,
		SmscPasswordCiphertext:       row.SmscPasswordCiphertext,
		SmscApikeyCiphertext:         row.SmscApikeyCiphertext,
		SmscCallbackSecretCiphertext: row.SmscCallbackSecretCiphertext,
		SmscCurrency:                 row.SmscCurrency,
		UpdatedBy:                    &p.UpdatedBy,
	})
	if err != nil {
		return Public{}, false, err
	}
	view := PublicView(updated)
	view.IngressToken = issued
	return view, credsChanged, nil
}

func (s *Service) RunexisCredentials(ctx context.Context) (Credentials, error) {
	row, err := s.q.GetSystemSettings(ctx)
	if err != nil {
		return Credentials{}, err
	}
	if row.RunexisEmail == nil || strings.TrimSpace(*row.RunexisEmail) == "" {
		return Credentials{}, ErrNotConfigured
	}
	if len(row.RunexisPasswordCiphertext) == 0 || row.DekKeyID == nil {
		return Credentials{}, ErrNotConfigured
	}
	plain, err := s.kr.Decrypt(row.RunexisPasswordCiphertext, *row.DekKeyID)
	if err != nil {
		return Credentials{}, fmt.Errorf("%w: %v", ErrDecrypt, err)
	}
	return Credentials{Email: *row.RunexisEmail, Password: string(plain)}, nil
}

func PublicView(row sqlcdb.SystemSetting) Public {
	v := Public{
		RunexisPasswordSet: len(row.RunexisPasswordCiphertext) > 0,
		SMSDirections: Directions{
			In:     row.SmsDirIn,
			DomOut: row.SmsDirDomOut,
			IntOut: row.SmsDirIntOut,
			InMass: row.SmsDirInMass,
		},
		ProviderRPS:              numericToFloat(row.ProviderRps),
		ClientRPSDefault:         numericToFloat(row.ClientRpsDefault),
		RetentionDays:            row.RetentionDays,
		AuditRetentionDays:       row.AuditRetentionDays,
		OpsRetentionDays:         row.OpsRetentionDays,
		BillingEnforced:          true,
		LowBalanceThreshold:      row.LowBalanceThreshold.StringFixed(6),
		IngressTokenSet:          row.IngressTokenHash != nil && *row.IngressTokenHash != "",
		UpdatedAt:                row.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		LookupEnabled:            row.LookupEnabled,
		LookupCheckTimeoutSec:    row.LookupCheckTimeoutSec,
		LookupPollIntervalSec:    row.LookupPollIntervalSec,
		LookupMaxCSVRows:         row.LookupMaxCsvRows,
		LookupMaxCSVBytes:        row.LookupMaxCsvBytes,
		LookupMaxBatchPhones:     row.LookupMaxBatchPhones,
		LookupWebhookMaxAttempts: row.LookupWebhookMaxAttempts,
		LookupWebhookTimeoutMs:   row.LookupWebhookTimeoutMs,
		LookupRetentionDays:      row.LookupRetentionDays,
		SMSCPasswordSet:          len(row.SmscPasswordCiphertext) > 0,
		SMSCAPIKeySet:            len(row.SmscApikeyCiphertext) > 0,
		SMSCCallbackSecretSet:    len(row.SmscCallbackSecretCiphertext) > 0,
		SMSCCurrency:             row.SmscCurrency,
	}
	if row.RunexisEmail != nil {
		v.RunexisEmail = *row.RunexisEmail
	}
	if row.DekKeyID != nil {
		v.DekKeyID = *row.DekKeyID
	}
	if row.CallbackBaseUrl != nil {
		v.CallbackBaseURL = *row.CallbackBaseUrl
	}
	if row.SmscBaseUrl != nil {
		v.SMSCBaseURL = *row.SmscBaseUrl
	}
	if row.SmscLogin != nil {
		v.SMSCLogin = *row.SmscLogin
	}
	v.SMSCCallbackURL = SMSCCallbackURL(v.CallbackBaseURL)
	return v
}

func validateCallbackURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%w: invalid callback_base_url", ErrValidation)
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "http":
	default:
		return fmt.Errorf("%w: callback_base_url must be http or https", ErrValidation)
	}
	return nil
}

func validateRPS(v float64) error {
	if v < 0.1 || v > 100 {
		return fmt.Errorf("must be between 0.1 and 100")
	}
	return nil
}

func numericFromFloat(f float64) (pgtype.Numeric, error) {
	var n pgtype.Numeric
	if err := n.Scan(strconv.FormatFloat(f, 'f', 2, 64)); err != nil {
		return n, err
	}
	return n, nil
}

func numericToFloat(n pgtype.Numeric) float64 {
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}

func (s *Service) IngressHash(ctx context.Context) (string, error) {
	row, err := s.q.GetSystemSettings(ctx)
	if err != nil {
		return "", err
	}
	if row.IngressTokenHash == nil {
		return "", nil
	}
	return *row.IngressTokenHash, nil
}

func CallbackURLs(base, token string) (dlr, mo string) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	return base + "/internal/runexis/dlr/" + token, base + "/internal/runexis/mo/" + token
}

func newIngressToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

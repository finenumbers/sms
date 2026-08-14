package settings

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/ingress"
	"finenumbers/sms/internal/secret"
)

type memStore struct {
	row sqlcdb.SystemSetting
}

func (m *memStore) GetSystemSettings(context.Context) (sqlcdb.SystemSetting, error) {
	return m.row, nil
}

func (m *memStore) UpdateSystemSettings(_ context.Context, arg sqlcdb.UpdateSystemSettingsParams) (sqlcdb.SystemSetting, error) {
	m.row.RunexisEmail = arg.RunexisEmail
	m.row.RunexisPasswordCiphertext = arg.RunexisPasswordCiphertext
	m.row.DekKeyID = arg.DekKeyID
	m.row.CallbackBaseUrl = arg.CallbackBaseUrl
	m.row.SmsDirIn = arg.SmsDirIn
	m.row.SmsDirDomOut = arg.SmsDirDomOut
	m.row.SmsDirIntOut = arg.SmsDirIntOut
	m.row.SmsDirInMass = arg.SmsDirInMass
	m.row.ProviderRps = arg.ProviderRps
	m.row.ClientRpsDefault = arg.ClientRpsDefault
	m.row.RetentionDays = arg.RetentionDays
	m.row.AuditRetentionDays = arg.AuditRetentionDays
	m.row.OpsRetentionDays = arg.OpsRetentionDays
	m.row.IngressTokenHash = arg.IngressTokenHash
	m.row.BillingEnforced = arg.BillingEnforced
	m.row.LowBalanceThreshold = arg.LowBalanceThreshold
	m.row.LookupEnabled = arg.LookupEnabled
	m.row.LookupCheckTimeoutSec = arg.LookupCheckTimeoutSec
	m.row.LookupPollIntervalSec = arg.LookupPollIntervalSec
	m.row.LookupMaxCsvRows = arg.LookupMaxCsvRows
	m.row.LookupMaxCsvBytes = arg.LookupMaxCsvBytes
	m.row.LookupMaxBatchPhones = arg.LookupMaxBatchPhones
	m.row.LookupWebhookMaxAttempts = arg.LookupWebhookMaxAttempts
	m.row.LookupWebhookTimeoutMs = arg.LookupWebhookTimeoutMs
	m.row.LookupRetentionDays = arg.LookupRetentionDays
	m.row.SmscBaseUrl = arg.SmscBaseUrl
	m.row.SmscLogin = arg.SmscLogin
	m.row.SmscPasswordCiphertext = arg.SmscPasswordCiphertext
	m.row.SmscApikeyCiphertext = arg.SmscApikeyCiphertext
	m.row.SmscCallbackSecretCiphertext = arg.SmscCallbackSecretCiphertext
	m.row.SmscCurrency = arg.SmscCurrency
	m.row.UpdatedBy = arg.UpdatedBy
	m.row.UpdatedAt = time.Now().UTC()
	return m.row, nil
}

func TestPublicViewOmitsSecrets(t *testing.T) {
	email := "agent@example.com"
	kid := "abcd1234abcd1234"
	row := sqlcdb.SystemSetting{
		RunexisEmail:              &email,
		RunexisPasswordCiphertext: []byte("ciphertext-must-not-leak"),
		DekKeyID:                  &kid,
		SmsDirIn:                  true,
		SmsDirDomOut:              true,
		ProviderRps:               mustNumeric(t, 20),
		ClientRpsDefault:          mustNumeric(t, 5),
		RetentionDays:             365,
		AuditRetentionDays:        730,
		OpsRetentionDays:          14,
		UpdatedAt:                 time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC),
	}
	b, err := json.Marshal(PublicView(row))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "ciphertext") || strings.Contains(s, "runexis_password\"") {
		t.Fatalf("leaked secret: %s", s)
	}
	if !strings.Contains(s, `"runexis_password_set":true`) {
		t.Fatalf("missing password_set: %s", s)
	}
	if !strings.Contains(s, email) {
		t.Fatalf("missing email: %s", s)
	}
}

func mustNumeric(t *testing.T, f float64) pgtype.Numeric {
	t.Helper()
	n, err := numericFromFloat(f)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestClearEmailClearsPassword(t *testing.T) {
	email := "agent@example.com"
	kid := "abcd1234abcd1234"
	empty := ""
	st := &memStore{row: sqlcdb.SystemSetting{
		RunexisEmail:              &email,
		RunexisPasswordCiphertext: []byte("ciphertext"),
		DekKeyID:                  &kid,
		SmsDirIn:                  true,
		SmsDirDomOut:              true,
		ProviderRps:               mustNumeric(t, 20),
		ClientRpsDefault:          mustNumeric(t, 5),
		RetentionDays:             365,
		AuditRetentionDays:        730,
		OpsRetentionDays:          14,
		UpdatedAt:                 time.Now().UTC(),
	}}
	kr, err := secret.NewKeyring("change-me-to-a-random-32-byte-string!!")
	if err != nil {
		t.Fatal(err)
	}
	svc := New(st, kr)
	view, changed, err := svc.Update(context.Background(), Patch{
		RunexisEmail: &empty,
		UpdatedBy:    uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected credsChanged")
	}
	if view.RunexisPasswordSet || view.RunexisEmail != "" || view.DekKeyID != "" {
		t.Fatalf("view=%+v", view)
	}
}

func TestRotateIngressTokenReturnedOnce(t *testing.T) {
	st := &memStore{row: sqlcdb.SystemSetting{
		SmsDirIn:           true,
		SmsDirDomOut:       true,
		ProviderRps:        mustNumeric(t, 20),
		ClientRpsDefault:   mustNumeric(t, 5),
		RetentionDays:      365,
		AuditRetentionDays: 730,
		OpsRetentionDays:   14,
		UpdatedAt:          time.Now().UTC(),
	}}
	kr, err := secret.NewKeyring("change-me-to-a-random-32-byte-string!!")
	if err != nil {
		t.Fatal(err)
	}
	svc := New(st, kr)
	view, _, err := svc.Update(context.Background(), Patch{
		RotateIngressToken: true,
		UpdatedBy:          uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.IngressToken == "" || !view.IngressTokenSet {
		t.Fatalf("expected token once, view=%+v", view)
	}
	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.IngressToken != "" {
		t.Fatal("Get must not return plaintext token")
	}
	if !got.IngressTokenSet {
		t.Fatal("hash should remain set")
	}
	h, err := svc.IngressHash(context.Background())
	if err != nil || h == "" {
		t.Fatal(err)
	}
	if h != ingress.HashToken(view.IngressToken) {
		t.Fatal("stored hash does not match issued token")
	}
}

func TestOpsRetentionDaysValidation(t *testing.T) {
	st := &memStore{row: sqlcdb.SystemSetting{
		SmsDirIn:           true,
		SmsDirDomOut:       true,
		ProviderRps:        mustNumeric(t, 20),
		ClientRpsDefault:   mustNumeric(t, 5),
		RetentionDays:      365,
		AuditRetentionDays: 730,
		OpsRetentionDays:   14,
		UpdatedAt:          time.Now().UTC(),
	}}
	kr, err := secret.NewKeyring("change-me-to-a-random-32-byte-string!!")
	if err != nil {
		t.Fatal(err)
	}
	svc := New(st, kr)
	bad := int32(91)
	_, _, err = svc.Update(context.Background(), Patch{
		OpsRetentionDays: &bad,
		UpdatedBy:        uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
	})
	if err == nil || !strings.Contains(err.Error(), "ops_retention_days") {
		t.Fatalf("err=%v", err)
	}
	ok := int32(30)
	view, _, err := svc.Update(context.Background(), Patch{
		OpsRetentionDays: &ok,
		UpdatedBy:        uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.OpsRetentionDays != 30 {
		t.Fatalf("got %d", view.OpsRetentionDays)
	}
}

func TestSMSCSecretsStayInSettings(t *testing.T) {
	st := &memStore{row: sqlcdb.SystemSetting{
		SmsDirIn:           true,
		SmsDirDomOut:       true,
		ProviderRps:        mustNumeric(t, 20),
		ClientRpsDefault:   mustNumeric(t, 5),
		RetentionDays:      365,
		AuditRetentionDays: 730,
		OpsRetentionDays:   14,
		UpdatedAt:          time.Now().UTC(),
	}}
	kr, err := secret.NewKeyring("change-me-to-a-random-32-byte-string!!")
	if err != nil {
		t.Fatal(err)
	}
	svc := New(st, kr)
	login := "hlr-login"
	pw := "hlr-password"
	secretVal := "callback-secret"
	base := "https://smsc.ru"
	on := true
	view, _, err := svc.Update(context.Background(), Patch{
		SMSCBaseURL:        &base,
		SMSCLogin:          &login,
		SMSCPassword:       &pw,
		SMSCCallbackSecret: &secretVal,
		LookupEnabled:      &on,
		UpdatedBy:          uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, pw) || strings.Contains(s, secretVal) || strings.Contains(s, "ciphertext") {
		t.Fatalf("leaked smsc secret: %s", s)
	}
	if !view.SMSCPasswordSet || !view.SMSCCallbackSecretSet || view.SMSCLogin != login || !view.LookupEnabled {
		t.Fatalf("view=%+v", view)
	}
	got, err := svc.SMSCSecrets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Login != login || got.Password != pw || got.CallbackSecret != secretVal {
		t.Fatalf("secrets=%+v", got)
	}
}

func TestClearRunexisKeepsSMSCDek(t *testing.T) {
	st := &memStore{row: sqlcdb.SystemSetting{
		SmsDirIn:           true,
		SmsDirDomOut:       true,
		ProviderRps:        mustNumeric(t, 20),
		ClientRpsDefault:   mustNumeric(t, 5),
		RetentionDays:      365,
		AuditRetentionDays: 730,
		OpsRetentionDays:   14,
		UpdatedAt:          time.Now().UTC(),
	}}
	kr, err := secret.NewKeyring("change-me-to-a-random-32-byte-string!!")
	if err != nil {
		t.Fatal(err)
	}
	svc := New(st, kr)
	email := "agent@example.com"
	rpw := "runexis-pass"
	login := "smsc"
	spw := "smsc-pass"
	if _, _, err := svc.Update(context.Background(), Patch{
		RunexisEmail:    &email,
		RunexisPassword: &rpw,
		SMSCLogin:       &login,
		SMSCPassword:    &spw,
		UpdatedBy:       uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
	}); err != nil {
		t.Fatal(err)
	}
	empty := ""
	view, _, err := svc.Update(context.Background(), Patch{
		RunexisEmail: &empty,
		UpdatedBy:    uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.RunexisPasswordSet || view.DekKeyID == "" || !view.SMSCPasswordSet {
		t.Fatalf("view=%+v", view)
	}
	got, err := svc.SMSCSecrets(context.Background())
	if err != nil || got.Password != spw {
		t.Fatalf("smsc after runexis clear: %+v %v", got, err)
	}
}

func TestErrDecryptIsProviderNeutral(t *testing.T) {
	if strings.Contains(ErrDecrypt.Error(), "runexis") || strings.Contains(ErrDecrypt.Error(), "smsc") {
		t.Fatalf("ErrDecrypt must not name a provider: %q", ErrDecrypt)
	}
}

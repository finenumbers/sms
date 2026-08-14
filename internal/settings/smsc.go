package settings

import (
	"context"
	"fmt"
	"strings"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

type SMSCSecrets struct {
	BaseURL        string
	Login          string
	Password       string
	APIKey         string
	Currency       string
	CallbackSecret string
}

func (s *Service) SMSCSecrets(ctx context.Context) (SMSCSecrets, error) {
	if s == nil {
		return SMSCSecrets{}, ErrNotConfigured
	}
	row, err := s.q.GetSystemSettings(ctx)
	if err != nil {
		return SMSCSecrets{}, err
	}
	out := SMSCSecrets{
		Currency: strings.TrimSpace(row.SmscCurrency),
	}
	if row.SmscBaseUrl != nil {
		out.BaseURL = strings.TrimSpace(*row.SmscBaseUrl)
	}
	if row.SmscLogin != nil {
		out.Login = strings.TrimSpace(*row.SmscLogin)
	}
	if len(row.SmscPasswordCiphertext) > 0 {
		plain, err := s.decryptSetting(row.SmscPasswordCiphertext, row.DekKeyID)
		if err != nil {
			return SMSCSecrets{}, err
		}
		out.Password = string(plain)
	}
	if len(row.SmscApikeyCiphertext) > 0 {
		plain, err := s.decryptSetting(row.SmscApikeyCiphertext, row.DekKeyID)
		if err != nil {
			return SMSCSecrets{}, err
		}
		out.APIKey = string(plain)
	}
	if len(row.SmscCallbackSecretCiphertext) > 0 {
		plain, err := s.decryptSetting(row.SmscCallbackSecretCiphertext, row.DekKeyID)
		if err != nil {
			return SMSCSecrets{}, err
		}
		out.CallbackSecret = string(plain)
	}
	return out, nil
}

func (s *Service) decryptSetting(ct []byte, kid *string) ([]byte, error) {
	if kid == nil || strings.TrimSpace(*kid) == "" {
		return nil, fmt.Errorf("%w: missing dek_key_id", ErrDecrypt)
	}
	plain, err := s.kr.Decrypt(ct, *kid)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDecrypt, err)
	}
	return plain, nil
}

func SMSCCallbackURL(callbackBaseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(callbackBaseURL), "/")
	if base == "" {
		return ""
	}
	return base + "/internal/smsc/callback"
}

func applyLookupPatch(row *sqlcdb.SystemSetting, p Patch) error {
	if p.LookupEnabled != nil {
		row.LookupEnabled = *p.LookupEnabled
	}
	if p.LookupCheckTimeoutSec != nil {
		if *p.LookupCheckTimeoutSec < 30 || *p.LookupCheckTimeoutSec > 86400 {
			return fmt.Errorf("%w: lookup_check_timeout_sec must be 30..86400", ErrValidation)
		}
		row.LookupCheckTimeoutSec = *p.LookupCheckTimeoutSec
	}
	if p.LookupPollIntervalSec != nil {
		if *p.LookupPollIntervalSec < 1 || *p.LookupPollIntervalSec > 3600 {
			return fmt.Errorf("%w: lookup_poll_interval_sec must be 1..3600", ErrValidation)
		}
		row.LookupPollIntervalSec = *p.LookupPollIntervalSec
	}
	if p.LookupMaxCSVRows != nil {
		if *p.LookupMaxCSVRows < 1 || *p.LookupMaxCSVRows > 100000 {
			return fmt.Errorf("%w: lookup_max_csv_rows must be 1..100000", ErrValidation)
		}
		row.LookupMaxCsvRows = *p.LookupMaxCSVRows
	}
	if p.LookupMaxCSVBytes != nil {
		if *p.LookupMaxCSVBytes < 1024 || *p.LookupMaxCSVBytes > 52428800 {
			return fmt.Errorf("%w: lookup_max_csv_bytes must be 1024..52428800", ErrValidation)
		}
		row.LookupMaxCsvBytes = *p.LookupMaxCSVBytes
	}
	if p.LookupMaxBatchPhones != nil {
		if *p.LookupMaxBatchPhones < 1 || *p.LookupMaxBatchPhones > 10000 {
			return fmt.Errorf("%w: lookup_max_batch_phones must be 1..10000", ErrValidation)
		}
		row.LookupMaxBatchPhones = *p.LookupMaxBatchPhones
	}
	if p.LookupWebhookMaxAttempts != nil {
		if *p.LookupWebhookMaxAttempts < 1 || *p.LookupWebhookMaxAttempts > 20 {
			return fmt.Errorf("%w: lookup_webhook_max_attempts must be 1..20", ErrValidation)
		}
		row.LookupWebhookMaxAttempts = *p.LookupWebhookMaxAttempts
	}
	if p.LookupWebhookTimeoutMs != nil {
		if *p.LookupWebhookTimeoutMs < 100 || *p.LookupWebhookTimeoutMs > 30000 {
			return fmt.Errorf("%w: lookup_webhook_timeout_ms must be 100..30000", ErrValidation)
		}
		row.LookupWebhookTimeoutMs = *p.LookupWebhookTimeoutMs
	}
	if p.LookupRetentionDays != nil {
		if *p.LookupRetentionDays < 1 || *p.LookupRetentionDays > 3650 {
			return fmt.Errorf("%w: lookup_retention_days must be 1..3650", ErrValidation)
		}
		row.LookupRetentionDays = *p.LookupRetentionDays
	}
	return nil
}

func applySMSCPatch(row *sqlcdb.SystemSetting, p Patch, s *Service) error {
	if p.SMSCBaseURL != nil {
		u := strings.TrimSpace(*p.SMSCBaseURL)
		if u == "" {
			row.SmscBaseUrl = nil
		} else {
			if err := validateCallbackURL(u); err != nil {
				return fmt.Errorf("%w: invalid smsc_base_url", ErrValidation)
			}
			row.SmscBaseUrl = &u
		}
	}
	if p.SMSCLogin != nil {
		login := strings.TrimSpace(*p.SMSCLogin)
		if login == "" {
			row.SmscLogin = nil
			row.SmscPasswordCiphertext = nil
		} else {
			row.SmscLogin = &login
		}
	}
	if p.SMSCPassword != nil {
		pw := *p.SMSCPassword
		if strings.TrimSpace(pw) == "" {
			return fmt.Errorf("%w: smsc_password is empty", ErrValidation)
		}
		ct, kid, err := s.kr.Encrypt([]byte(pw))
		if err != nil {
			return err
		}
		row.SmscPasswordCiphertext = ct
		row.DekKeyID = &kid
	}
	if p.SMSCAPIKey != nil {
		key := strings.TrimSpace(*p.SMSCAPIKey)
		if key == "" {
			row.SmscApikeyCiphertext = nil
		} else {
			ct, kid, err := s.kr.Encrypt([]byte(key))
			if err != nil {
				return err
			}
			row.SmscApikeyCiphertext = ct
			row.DekKeyID = &kid
		}
	}
	if p.SMSCCallbackSecret != nil {
		sec := *p.SMSCCallbackSecret
		if strings.TrimSpace(sec) == "" {
			row.SmscCallbackSecretCiphertext = nil
		} else {
			ct, kid, err := s.kr.Encrypt([]byte(sec))
			if err != nil {
				return err
			}
			row.SmscCallbackSecretCiphertext = ct
			row.DekKeyID = &kid
		}
	}
	if p.SMSCCurrency != nil {
		cur := strings.ToUpper(strings.TrimSpace(*p.SMSCCurrency))
		if cur == "" {
			cur = "RUB"
		}
		if len(cur) != 3 {
			return fmt.Errorf("%w: smsc_currency must be a 3-letter code", ErrValidation)
		}
		row.SmscCurrency = cur
	}
	if row.SmscCurrency == "" {
		row.SmscCurrency = "RUB"
	}
	return nil
}

func rowHasEncryptedSecrets(row sqlcdb.SystemSetting) bool {
	return len(row.RunexisPasswordCiphertext) > 0 ||
		len(row.SmscPasswordCiphertext) > 0 ||
		len(row.SmscApikeyCiphertext) > 0 ||
		len(row.SmscCallbackSecretCiphertext) > 0
}

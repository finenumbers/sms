package smsc

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultBaseURL        = "https://smsc.ru"
	defaultCurrency       = "RUB"
	defaultTimeout        = 15 * time.Second
	defaultRetryMax       = 2
	defaultRetryBaseDelay = 200 * time.Millisecond
)

type AuthMode string

const (
	AuthNone   AuthMode = ""
	AuthLogin  AuthMode = "login"
	AuthAPIKey AuthMode = "apikey"
)

type Auth struct {
	Mode     AuthMode
	Login    string
	Password string
	APIKey   string
}

type Config struct {
	BaseURL          string
	Auth             Auth
	Currency         string
	Timeout          time.Duration
	RetryMaxAttempts int
	RetryBaseDelay   time.Duration
	CallbackSecret   string
}

func (c Config) Configured() bool {
	return c.Auth.Mode != AuthNone
}

type Input struct {
	BaseURL          string
	Login            string
	Password         string
	APIKey           string
	Currency         string
	Timeout          time.Duration
	RetryMaxAttempts int
	RetryBaseDelay   time.Duration
	CallbackSecret   string
}

func Resolve(in Input) (Config, error) {
	return parseConfig(in, true)
}

// Load builds a config without requiring credentials. Used for Settings-backed runtime.
func Load(in Input) Config {
	cfg, _ := parseConfig(in, false)
	return cfg
}

func parseConfig(in Input, requireAuth bool) (Config, error) {
	base := strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	if base == "" {
		base = DefaultBaseURL
	}
	apiKey := strings.TrimSpace(in.APIKey)
	login := strings.TrimSpace(in.Login)
	password := in.Password

	var auth Auth
	switch {
	case apiKey != "":
		auth = Auth{Mode: AuthAPIKey, APIKey: apiKey}
	case login != "" && password != "":
		auth = Auth{Mode: AuthLogin, Login: login, Password: password}
	case requireAuth:
		return Config{}, fmt.Errorf("SMSC credentials missing: set login+password or API key in Settings")
	}

	timeout := in.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	retryMax := in.RetryMaxAttempts
	if retryMax < 0 {
		retryMax = defaultRetryMax
	}
	delay := in.RetryBaseDelay
	if delay <= 0 {
		delay = defaultRetryBaseDelay
	}
	currency := strings.TrimSpace(in.Currency)
	if currency == "" {
		currency = defaultCurrency
	}

	return Config{
		BaseURL:          base,
		Auth:             auth,
		Currency:         currency,
		Timeout:          timeout,
		RetryMaxAttempts: retryMax,
		RetryBaseDelay:   delay,
		CallbackSecret:   in.CallbackSecret,
	}, nil
}

func (a Auth) params() map[string]string {
	switch a.Mode {
	case AuthAPIKey:
		return map[string]string{"apikey": a.APIKey}
	case AuthLogin:
		return map[string]string{"login": a.Login, "psw": a.Password}
	default:
		return nil
	}
}

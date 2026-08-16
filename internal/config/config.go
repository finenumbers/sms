package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

)

type Mode string

const (
	ModeAPI     Mode = "api"
	ModeWorker  Mode = "worker"
	ModeAll     Mode = "all"
	ModeMigrate Mode = "migrate"
)

type Config struct {
	Mode              Mode
	HTTPAddr          string
	DatabaseURL       string
	RedisURL          string
	AppMasterKey      string
	SeedAdminEmail    string
	SeedAdminPassword string
	SeedAdminName     string
	LogLevel          string
	ShutdownTimeout   time.Duration
	SessionTTL        time.Duration
	CookieSecure      bool
	AdminCookieName   string
	ClientCookieName  string
	RunexisBaseURL    string
	AppMasterKeyPrev  string
	AdminHost         string
	ClientHost        string
	APIHost           string
	CORSAllowOrigins  []string
	APIKeyPepper      string
	MetricsAddr       string
	AdminSPADir       string
	ClientSPADir      string
}

func (c Config) APIKeyPepperValue() string {
	if c.APIKeyPepper != "" {
		return c.APIKeyPepper
	}
	return c.AppMasterKey
}

func Load() (Config, error) {
	mode := Mode(strings.ToLower(env("APP_MODE", "api")))
	switch mode {
	case ModeAPI, ModeWorker, ModeAll, ModeMigrate:
	default:
		return Config{}, fmt.Errorf("invalid APP_MODE %q (api|worker|all|migrate)", mode)
	}

	cfg := Config{
		Mode:              mode,
		HTTPAddr:          env("HTTP_ADDR", ":8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		RedisURL:          env("REDIS_URL", "redis://127.0.0.1:6379/0"),
		AppMasterKey:      os.Getenv("APP_MASTER_KEY"),
		SeedAdminEmail:    os.Getenv("SEED_ADMIN_EMAIL"),
		SeedAdminPassword: os.Getenv("SEED_ADMIN_PASSWORD"),
		SeedAdminName:     env("SEED_ADMIN_NAME", "Administrator"),
		LogLevel:          env("LOG_LEVEL", "info"),
		ShutdownTimeout:   15 * time.Second,
		SessionTTL:        12 * time.Hour,
		CookieSecure:      envBool("COOKIE_SECURE", true),
		AdminCookieName:   env("ADMIN_COOKIE_NAME", "__Host-fn_admin_sid"),
		ClientCookieName:  env("CLIENT_COOKIE_NAME", "__Host-fn_client_sid"),
		RunexisBaseURL:    env("RUNEXIS_BASE_URL", "https://didapi.runexis.ru"),
		AppMasterKeyPrev:  os.Getenv("APP_MASTER_KEY_PREVIOUS"),
		AdminHost:         env("ADMIN_HOST", ""),
		ClientHost:        env("CLIENT_HOST", ""),
		APIHost:           env("API_HOST", ""),
		CORSAllowOrigins:  envCSV("CORS_ALLOW_ORIGINS"),
		APIKeyPepper:      os.Getenv("API_KEY_PEPPER"),
		MetricsAddr:       env("METRICS_ADDR", ""),
		AdminSPADir:       env("ADMIN_SPA_DIR", "/srv/admin"),
		ClientSPADir:      env("CLIENT_SPA_DIR", "/srv/client"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if mode != ModeMigrate && len(cfg.AppMasterKey) < 32 {
		return Config{}, fmt.Errorf("APP_MASTER_KEY must be at least 32 characters")
	}
	if err := validateCookie(cfg.AdminCookieName, cfg.CookieSecure); err != nil {
		return Config{}, err
	}
	if err := validateCookie(cfg.ClientCookieName, cfg.CookieSecure); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func validateCookie(name string, secure bool) error {
	if strings.HasPrefix(name, "__Host-") && !secure {
		return fmt.Errorf("cookie %q uses __Host- prefix and requires COOKIE_SECURE=true", name)
	}
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envCSV(key string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

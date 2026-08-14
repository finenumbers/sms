package admin

import (
	"testing"

	"finenumbers/sms/internal/config"
	"finenumbers/sms/internal/settings"
	"finenumbers/sms/internal/smsc"
)

func TestPublicAPIBaseURL(t *testing.T) {
	if got := publicAPIBaseURL(config.Config{APIHost: "api.sms.localhost"}); got != "http://api.sms.localhost" {
		t.Fatalf("localhost: %s", got)
	}
	if got := publicAPIBaseURL(config.Config{APIHost: "api.example.ru"}); got != "https://api.example.ru" {
		t.Fatalf("public: %s", got)
	}
	if got := publicAPIBaseURL(config.Config{}); got != "" {
		t.Fatalf("empty: %s", got)
	}
}

func TestApplySettingsURLDefaults(t *testing.T) {
	view := settings.Public{}
	applySettingsURLDefaults(&view, config.Config{APIHost: "api.sms.localhost"})
	if view.SMSCBaseURL != smsc.DefaultBaseURL {
		t.Fatalf("smsc %s", view.SMSCBaseURL)
	}
	if view.CallbackBaseURL != "http://api.sms.localhost" {
		t.Fatalf("callback %s", view.CallbackBaseURL)
	}
	if view.SMSCCallbackURL != "http://api.sms.localhost/internal/smsc/callback" {
		t.Fatalf("smsc callback %s", view.SMSCCallbackURL)
	}
	kept := settings.Public{SMSCBaseURL: "https://custom.smsc", CallbackBaseURL: "https://api.prod.ru"}
	applySettingsURLDefaults(&kept, config.Config{APIHost: "api.sms.localhost"})
	if kept.SMSCBaseURL != "https://custom.smsc" || kept.CallbackBaseURL != "https://api.prod.ru" {
		t.Fatalf("must keep saved URLs: %+v", kept)
	}
}

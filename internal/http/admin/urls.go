package admin

import (
	"strings"

	"finenumbers/sms/internal/config"
	"finenumbers/sms/internal/settings"
	"finenumbers/sms/internal/smsc"
)

func applySettingsURLDefaults(view *settings.Public, cfg config.Config) {
	if view == nil {
		return
	}
	if strings.TrimSpace(view.SMSCBaseURL) == "" {
		view.SMSCBaseURL = smsc.DefaultBaseURL
	}
	if strings.TrimSpace(view.CallbackBaseURL) == "" {
		view.CallbackBaseURL = publicAPIBaseURL(cfg)
	}
	view.SMSCCallbackURL = settings.SMSCCallbackURL(view.CallbackBaseURL)
}

func publicAPIBaseURL(cfg config.Config) string {
	host := strings.TrimSpace(cfg.APIHost)
	if host == "" {
		return ""
	}
	scheme := "https"
	if strings.Contains(host, "localhost") || strings.HasPrefix(host, "127.") {
		scheme = "http"
	}
	return scheme + "://" + host
}

package smsc

import "testing"

func TestResolveRequiresCredentials(t *testing.T) {
	if _, err := Resolve(Input{}); err == nil {
		t.Fatal("expected missing credentials")
	}
	cfg, err := Resolve(Input{Login: "u", Password: "p"})
	if err != nil || cfg.Auth.Mode != AuthLogin || cfg.BaseURL != DefaultBaseURL {
		t.Fatalf("login auth: %#v %v", cfg, err)
	}
	cfg, err = Resolve(Input{APIKey: "k", Login: "u", Password: "p"})
	if err != nil || cfg.Auth.Mode != AuthAPIKey || cfg.Auth.APIKey != "k" {
		t.Fatalf("apikey wins: %#v %v", cfg, err)
	}
}

func TestLoadAllowsMissingCreds(t *testing.T) {
	cfg := Load(Input{})
	if cfg.Configured() {
		t.Fatal("expected unconfigured")
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Fatalf("base %s", cfg.BaseURL)
	}
}

package apikeys

import (
	"net/netip"
	"testing"
)

func TestParseAndVerify(t *testing.T) {
	pepper := "test-pepper-at-least-32-characters!!"
	prefix, secret, err := generate()
	if err != nil {
		t.Fatal(err)
	}
	token := Format(prefix, secret)
	parsed, err := Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Prefix != prefix || parsed.Secret != secret {
		t.Fatalf("parsed %+v", parsed)
	}
	hash := HashSecret(pepper, secret)
	if !Verify(pepper, secret, hash) {
		t.Fatal("verify failed")
	}
	if Verify(pepper, "other", hash) {
		t.Fatal("wrong secret accepted")
	}
	if Verify("other-pepper-at-least-32-characters!!", secret, hash) {
		t.Fatal("wrong pepper accepted")
	}
}

func TestParseRejects(t *testing.T) {
	for _, tok := range []string{
		"",
		"fnk_test_ab_cd",
		"fnk_live_short_secret",
		"fnk_live_nothexxxxxxx_secret",
		"fnk_live_0123456789abcdef",
		"Bearer fnk_live_0123456789abcdef_x",
	} {
		if _, err := Parse(tok); err == nil {
			t.Fatalf("accepted %q", tok)
		}
	}
}

func TestIPAllowed(t *testing.T) {
	ip := netip.MustParseAddr("203.0.113.10")
	if !IPAllowed(nil, &ip) {
		t.Fatal("empty cidrs should allow")
	}
	if !IPAllowed([]string{}, &ip) {
		t.Fatal("empty slice should allow")
	}
	cidrs, err := ParseCIDRs([]string{"203.0.113.0/24", "10.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if !IPAllowed(cidrs, &ip) {
		t.Fatal("in range")
	}
	other := netip.MustParseAddr("198.51.100.1")
	if IPAllowed(cidrs, &other) {
		t.Fatal("out of range")
	}
	if IPAllowed(cidrs, nil) {
		t.Fatal("nil ip")
	}
	host := netip.MustParseAddr("10.0.0.1")
	if !IPAllowed(cidrs, &host) {
		t.Fatal("exact host")
	}
}

func TestParseCIDRsInvalid(t *testing.T) {
	if _, err := ParseCIDRs([]string{"not-a-cidr"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeScopes(t *testing.T) {
	got, err := normalizeScopes(nil)
	if err != nil || len(got) != 5 {
		t.Fatalf("default: %v %v", got, err)
	}
	got, err = normalizeScopes([]string{"sms:send", "sms:send", "sms:read"})
	if err != nil || len(got) != 2 {
		t.Fatalf("dedupe: %v %v", got, err)
	}
	if _, err := normalizeScopes([]string{"sms:admin"}); err == nil {
		t.Fatal("unknown scope")
	}
}

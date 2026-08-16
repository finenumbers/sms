package identity

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateEmailRejectsDisplayName(t *testing.T) {
	if err := validateEmail("user@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := validateEmail("Name <user@example.com>"); err == nil {
		t.Fatal("expected reject display-name form")
	}
	if err := validateEmail(""); err == nil || !errors.Is(err, ErrValidation) {
		t.Fatalf("empty: %v", err)
	}
}

func TestValidateUserName(t *testing.T) {
	got, err := validateUserName("  Иванов  ")
	if err != nil || got != "Иванов" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := validateUserName("  "); err == nil {
		t.Fatal("expected name required")
	}
	if _, err := validateUserName(strings.Repeat("я", 121)); err == nil {
		t.Fatal("expected name too long")
	}
}

func TestCanDisableActiveOwner(t *testing.T) {
	if CanDisableActiveOwner(1) {
		t.Fatal("last owner must stay")
	}
	if !CanDisableActiveOwner(2) {
		t.Fatal("two owners can disable one")
	}
}

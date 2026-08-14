package password_test

import (
	"testing"

	"finenumbers/sms/internal/password"
)

func TestHashVerify(t *testing.T) {
	hash, err := password.Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := password.Verify("correct horse battery", hash)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected match")
	}
	ok, err = password.Verify("wrong", hash)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected mismatch")
	}
}

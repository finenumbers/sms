package secret

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	kr, err := NewKeyring("change-me-to-a-random-32-byte-string!!")
	if err != nil {
		t.Fatal(err)
	}
	ct, id, err := kr.Encrypt([]byte("runexis-password"))
	if err != nil {
		t.Fatal(err)
	}
	if id != kr.CurrentID() || id == "" {
		t.Fatalf("key id %q", id)
	}
	got, err := kr.Decrypt(ct, id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("runexis-password")) {
		t.Fatalf("got %q", got)
	}
}

func TestEncryptRejectsEmpty(t *testing.T) {
	kr, err := NewKeyring("change-me-to-a-random-32-byte-string!!")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := kr.Encrypt(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecryptUnknownKeyID(t *testing.T) {
	kr, err := NewKeyring("change-me-to-a-random-32-byte-string!!")
	if err != nil {
		t.Fatal(err)
	}
	ct, _, err := kr.Encrypt([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kr.Decrypt(ct, "deadbeefdeadbeef"); err == nil {
		t.Fatal("expected unknown key id")
	}
}

func TestPreviousKeyDecrypt(t *testing.T) {
	old := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cur := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	oldKR, err := NewKeyring(old)
	if err != nil {
		t.Fatal(err)
	}
	ct, oldID, err := oldKR.Encrypt([]byte("rotated"))
	if err != nil {
		t.Fatal(err)
	}
	kr, err := NewKeyring(cur, old)
	if err != nil {
		t.Fatal(err)
	}
	got, err := kr.Decrypt(ct, oldID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "rotated" {
		t.Fatalf("got %q", got)
	}
	if kr.CurrentID() == oldID {
		t.Fatal("current id should change after rotation")
	}
}

func TestHexMasterKey(t *testing.T) {
	hexKey := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	kr, err := NewKeyring(hexKey)
	if err != nil {
		t.Fatal(err)
	}
	ct, id, err := kr.Encrypt([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := kr.Decrypt(ct, id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "x" {
		t.Fatalf("got %q", got)
	}
}

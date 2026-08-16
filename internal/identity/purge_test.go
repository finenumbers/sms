package identity

import (
	"testing"

	"github.com/google/uuid"
)

func TestScrambledUserEmail(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	got := ScrambledUserEmail(id)
	want := "deleted-11111111-1111-1111-1111-111111111111@invalid"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if !EmailAlreadyScrambled(got) {
		t.Fatal("expected scrambled")
	}
}

func TestEmailAlreadyScrambled(t *testing.T) {
	if EmailAlreadyScrambled("owner@example.com") {
		t.Fatal("normal email")
	}
	if EmailAlreadyScrambled("deleted-not-a-uuid@example.com") {
		t.Fatal("wrong domain")
	}
	if !EmailAlreadyScrambled("DELETED-aaaa@invalid") {
		t.Fatal("expected case-insensitive match")
	}
}

func TestPurgeLockKeysStable(t *testing.T) {
	id := uuid.MustParse("01020304-0506-0708-090a-0b0c0d0e0f10")
	a := purgeLockKeys(id)
	b := purgeLockKeys(id)
	if a != b {
		t.Fatalf("%v != %v", a, b)
	}
	other := purgeLockKeys(uuid.MustParse("11020304-0506-0708-090a-0b0c0d0e0f10"))
	if a == other {
		t.Fatal("different UUID must not share lock keys")
	}
}

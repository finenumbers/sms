package lookup

import "testing"

func TestNormalizePhoneE164(t *testing.T) {
	got, err := NormalizePhoneE164("79991234567")
	if err != nil || got != "+79991234567" {
		t.Fatalf("got %s %v", got, err)
	}
	got, err = NormalizePhoneE164("+7 999 123-45-67")
	if err != nil || got != "+79991234567" {
		t.Fatalf("spaces: %s %v", got, err)
	}
	if _, err := NormalizePhoneE164("123"); err == nil {
		t.Fatal("expected invalid")
	}
}

func TestNormalizeAndDeduplicate(t *testing.T) {
	result := NormalizeAndDeduplicate([]string{
		"+79991234567",
		"79991234567",
		"+7 999 123-45-67",
		"+79997654321",
	})
	if len(result.Phones) != 2 || result.DeduplicatedCount != 2 || len(result.Invalid) != 0 {
		t.Fatalf("%#v", result)
	}
	if result.Phones[0] != "+79991234567" || result.Phones[1] != "+79997654321" {
		t.Fatalf("order %#v", result.Phones)
	}
	bad := NormalizeAndDeduplicate([]string{"+79991234567", "not-a-phone"})
	if len(bad.Phones) != 1 || len(bad.Invalid) != 1 {
		t.Fatalf("%#v", bad)
	}
}

func TestIsRUMobile79(t *testing.T) {
	if !IsRUMobile79("+79001234567") || !IsRUMobile79("79001234567") {
		t.Fatal("expected 79 mobile")
	}
	if IsRUMobile79("+74951234567") || IsRUMobile79("+380501234567") || IsRUMobile79("+7900123456") {
		t.Fatal("rejected numbers accepted")
	}
	if CountNonRUMobile79([]string{"+79001234567", "+74951234567"}) != 1 {
		t.Fatal("count")
	}
}

func TestPreparePhonesRejectsNon79(t *testing.T) {
	_, _, err := PreparePhones([]string{"+74951234567"}, "bulk", 1000, "")
	if err == nil || AsError(err) == nil || AsError(err).Message != RUMobile79RequiredMessage {
		t.Fatalf("landline: %v", err)
	}
	if got := AsError(err).RejectedPhones; len(got) != 1 || got[0] != "+74951234567" {
		t.Fatalf("landline rejected %#v", got)
	}
	_, _, err = PreparePhones([]string{"+79001234567", "74951234567"}, "bulk", 1000, "")
	if err == nil {
		t.Fatal("mixed list must fail entirely")
	}
	le := AsError(err)
	if le == nil || le.Message != RUMobile79RequiredMessage {
		t.Fatalf("mixed message: %v", err)
	}
	if len(le.RejectedPhones) != 1 || le.RejectedPhones[0] != "74951234567" {
		t.Fatalf("mixed rejected %#v", le.RejectedPhones)
	}
	_, _, err = PreparePhones([]string{"+77001234567"}, "bulk", 1000, "")
	if err == nil {
		t.Fatal("KZ 770 must fail")
	}
	phones, deduped, err := PreparePhones([]string{"+79001234567", "79001234567"}, "bulk", 1000, "")
	if err != nil || len(phones) != 1 || deduped != 1 {
		t.Fatalf("%v %v %v", phones, deduped, err)
	}
	_, _, err = PreparePhones([]string{"+79001234567", "+79007654321"}, "single", 1000, "")
	if err == nil {
		t.Fatal("single requires one")
	}
}

func TestPreparePhonesRejectedOriginalsAndInvalid(t *testing.T) {
	_, _, err := PreparePhones([]string{"74951234567", "+74951234567", "74951234567"}, "bulk", 1000, "")
	le := AsError(err)
	if le == nil || le.Message != RUMobile79RequiredMessage {
		t.Fatalf("message: %v", err)
	}
	if len(le.RejectedPhones) != 2 || le.RejectedPhones[0] != "74951234567" || le.RejectedPhones[1] != "+74951234567" {
		t.Fatalf("originals %#v", le.RejectedPhones)
	}
	_, _, err = PreparePhones([]string{"+79001234567", "not-a-phone", "74951234567"}, "bulk", 1000, "")
	le = AsError(err)
	if le == nil || le.Message != RUMobile79RequiredMessage {
		t.Fatalf("mixed invalid+non79 message: %v", err)
	}
	if len(le.RejectedPhones) != 2 || le.RejectedPhones[0] != "not-a-phone" || le.RejectedPhones[1] != "74951234567" {
		t.Fatalf("mixed invalid+non79 %#v", le.RejectedPhones)
	}
	_, _, err = PreparePhones([]string{"+79001234567", "abc"}, "bulk", 1000, "")
	le = AsError(err)
	if le == nil || le.Message != "One or more phone numbers are invalid" {
		t.Fatalf("invalid only: %v", err)
	}
	if len(le.RejectedPhones) != 1 || le.RejectedPhones[0] != "abc" {
		t.Fatalf("invalid rejected %#v", le.RejectedPhones)
	}
}

func TestPreparePhonesCapMessage(t *testing.T) {
	phones := make([]string, 3)
	for i := range phones {
		phones[i] = "+7900123456" + string(rune('0'+i))
	}
	_, _, err := PreparePhones(phones, "bulk", 2, "max_csv_rows")
	if err == nil || AsError(err) == nil || AsError(err).Message != "Phone count exceeds max_csv_rows" {
		t.Fatalf("csv cap: %v", err)
	}
	_, _, err = PreparePhones(phones, "bulk", 2, "")
	if err == nil || AsError(err) == nil || AsError(err).Message != "Phone count exceeds max_batch_phones" {
		t.Fatalf("batch cap: %v", err)
	}
}

func TestChunk(t *testing.T) {
	got := Chunk([]int{1, 2, 3, 4, 5}, 2)
	if len(got) != 3 || len(got[2]) != 1 {
		t.Fatalf("%#v", got)
	}
}

package msisdn

import "testing"

func TestNormalizeSender(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"79991112233", "79991112233"},
		{"+7 (999) 111-22-33", "79991112233"},
		{"8 999 111 22 33", "79991112233"},
		{"9991112233", "79991112233"},
	}
	for _, tc := range cases {
		got, err := NormalizeSender(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %s want %s", tc.in, got, tc.want)
		}
		if !IsSender(got) {
			t.Fatalf("%q: not sender", got)
		}
	}
}

func TestNormalizeSenderRejects(t *testing.T) {
	for _, in := range []string{"", "123", "123456789012", "19991112233", "abc"} {
		if _, err := NormalizeSender(in); err == nil {
			t.Fatalf("expected error for %q", in)
		}
	}
}

func TestFromManagement(t *testing.T) {
	cases := []struct {
		code, number, want string
	}{
		{"999", "1234567", "79991234567"},
		{"495", "1234567", "74951234567"},
		{"999", "79991234567", "79991234567"},
		{"", "79991234567", "79991234567"},
		{"", "9991234567", "79991234567"},
		{"7", "9991234567", "79991234567"},
	}
	for _, tc := range cases {
		got, err := FromManagement(tc.code, tc.number)
		if err != nil {
			t.Fatalf("code=%q number=%q: %v", tc.code, tc.number, err)
		}
		if got != tc.want {
			t.Fatalf("code=%q number=%q: got %s want %s", tc.code, tc.number, got, tc.want)
		}
		if !IsSender(got) {
			t.Fatalf("not sender: %s", got)
		}
	}
}

func TestFromManagementRejects(t *testing.T) {
	for _, tc := range []struct{ code, number string }{
		{"", ""},
		{"99", "123"},
		{"999", "12345678"},
		{"8", "9991112233"},
	} {
		if _, err := FromManagement(tc.code, tc.number); err == nil {
			t.Fatalf("expected error for code=%q number=%q", tc.code, tc.number)
		}
	}
	if _, err := NormalizeSender("4951234567"); err == nil {
		t.Fatal("NormalizeSender must still reject geographic 10-digit")
	}
}

func TestNormalizeDest(t *testing.T) {
	d, err := NormalizeDest("+7 999 111-22-33")
	if err != nil || d.International || d.MSISDN != "79991112233" {
		t.Fatalf("%+v %v", d, err)
	}
	d, err = NormalizeDest("14155551234")
	if err != nil || !d.International || d.MSISDN != "14155551234" {
		t.Fatalf("%+v %v", d, err)
	}
	if _, err := NormalizeDest("0123"); err == nil {
		t.Fatal("expected error")
	}
}

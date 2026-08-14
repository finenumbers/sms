package runexis

import (
	"testing"
)

func TestParseDLRProvisionalFixture(t *testing.T) {
	rows := ParseCallbacks("", "application/json", fixture(t, "dlr_callback.provisional.json"))
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	row := rows[0]
	if row.SMSID != "8ace264b-a78e-488e-b411-af2581bd3f23" || !row.Delivered || !row.Sent || row.Failed {
		t.Fatalf("%+v", row)
	}
}

func TestParseLiveDLRCallback(t *testing.T) {
	rows := ParseCallbacks("", "application/json", fixture(t, "dlr_callback.json"))
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	row := rows[0]
	if row.SMSID != "0ca3c3ed-97f1-11f1-a65b-000c296c1599" {
		t.Fatalf("sms_id %+v", row)
	}
	if !row.Sent || row.Delivered || row.Failed {
		t.Fatalf("status flags %+v", row)
	}
	if row.Status != "2" {
		t.Fatalf("status %+v", row)
	}
}

func TestParseMessageStatusUnknownIsNotSent(t *testing.T) {
	rows := ParseCallbacks("", "application/json", []byte(`{"id":"abc","message_status":7}`))
	if len(rows) != 1 || rows[0].Sent || rows[0].Delivered || rows[0].Failed || rows[0].Status != "7" {
		t.Fatalf("%+v", rows)
	}
}

func TestParseDLRFailedFixture(t *testing.T) {
	rows := ParseCallbacks("", "application/json", fixture(t, "dlr_callback.failed.provisional.json"))
	if len(rows) != 1 || !rows[0].Failed || rows[0].Delivered {
		t.Fatalf("%+v", rows)
	}
}

func TestParseMOProvisionalFixture(t *testing.T) {
	rows := ParseCallbacks("", "application/json", fixture(t, "mo_callback.provisional.json"))
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	row := rows[0]
	if !row.Incoming || row.From != "79876543210" || row.To != "79991234567" || row.Text != "Lorem ipsum." {
		t.Fatalf("%+v", row)
	}
}

func TestParseCallbackQueryAndForm(t *testing.T) {
	rows := ParseCallbacks("sms_id=abc&status=delivered", "", nil)
	if len(rows) != 1 || rows[0].SMSID != "abc" || !rows[0].Delivered {
		t.Fatalf("%+v", rows)
	}
	form := ParseCallbacks("", "application/x-www-form-urlencoded", []byte("sms_id=xyz&from=79991112233&to=79876543210&text=hi&incoming=true"))
	if len(form) != 1 || form[0].SMSID != "xyz" || form[0].From != "79991112233" || !form[0].Incoming {
		t.Fatalf("%+v", form)
	}
}

func TestParseCallbackEnvelopeAndArray(t *testing.T) {
	body := []byte(`{"success":true,"data":[{"sms_id":"1","delivered":true},{"sms_id":"2","status":"failed"}]}`)
	rows := ParseCallbacks("", "application/json", body)
	if len(rows) != 2 || rows[0].SMSID != "1" || !rows[0].Delivered || !rows[1].Failed {
		t.Fatalf("%+v", rows)
	}
}

func TestParseCallbackEmpty(t *testing.T) {
	if rows := ParseCallbacks("", "application/json", []byte(`{"foo":1}`)); len(rows) != 0 {
		t.Fatalf("%+v", rows)
	}
}

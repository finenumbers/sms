package messaging

import (
	"testing"

	"finenumbers/sms/internal/runexis"
)

func TestMatchStatistic(t *testing.T) {
	from, to := "79991112233", "79993332211"
	hello := runexis.StatisticRow{
		SMSID: "a", SenderNumber: from, ReceiverNumber: to, Message: "hello", Incoming: false,
	}
	incoming := runexis.StatisticRow{
		SMSID: "b", SenderNumber: from, ReceiverNumber: to, Message: "hello", Incoming: true,
	}
	other := runexis.StatisticRow{
		SMSID: "c", SenderNumber: from, ReceiverNumber: to, Message: "other", Incoming: false,
	}
	emptyOne := runexis.StatisticRow{
		SMSID: "d", SenderNumber: from, ReceiverNumber: to, Message: "", Incoming: false,
	}
	emptyTwo := runexis.StatisticRow{
		SMSID: "e", SenderNumber: from, ReceiverNumber: to, Message: "  ", Incoming: false,
	}
	emptyNoID := runexis.StatisticRow{
		SMSID: "", SenderNumber: from, ReceiverNumber: to, Message: "", Incoming: false,
	}

	tests := []struct {
		name   string
		text   string
		rows   []runexis.StatisticRow
		used   map[string]struct{}
		wantOK bool
		wantID string
	}{
		{name: "exact text", text: "hello", rows: []runexis.StatisticRow{hello, incoming}, wantOK: true, wantID: "a"},
		{name: "used sms_id skipped", text: "hello", rows: []runexis.StatisticRow{hello, incoming}, used: map[string]struct{}{"a": {}}, wantOK: false},
		{name: "text mismatch", text: "other", rows: []runexis.StatisticRow{hello}, wantOK: false},
		{name: "incoming ignored", text: "hello", rows: []runexis.StatisticRow{incoming}, wantOK: false},
		{name: "exact preferred over empty", text: "hello", rows: []runexis.StatisticRow{emptyOne, hello}, wantOK: true, wantID: "a"},
		{name: "empty unique with sms_id", text: "hello", rows: []runexis.StatisticRow{emptyOne}, wantOK: true, wantID: "d"},
		{name: "empty two candidates miss", text: "hello", rows: []runexis.StatisticRow{emptyOne, emptyTwo}, wantOK: false},
		{name: "empty without sms_id miss", text: "hello", rows: []runexis.StatisticRow{emptyNoID}, wantOK: false},
		{name: "wrong text plus unique empty", text: "hello", rows: []runexis.StatisticRow{other, emptyOne}, wantOK: true, wantID: "d"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := MatchStatistic(from, to, tc.text, tc.rows, tc.used)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v got=%+v", ok, tc.wantOK, got)
			}
			if tc.wantOK && got.SMSID != tc.wantID {
				t.Fatalf("sms_id=%s want %s", got.SMSID, tc.wantID)
			}
		})
	}
}

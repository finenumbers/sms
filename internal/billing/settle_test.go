package billing

import (
	"testing"
	"time"

	"github.com/google/uuid"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/msisdn"
)

func TestSettleAction(t *testing.T) {
	id := uuid.New()
	accepted := time.Now()
	cases := []struct {
		name string
		row  sqlcdb.ListOpenHoldMessagesRow
		want string
	}{
		{
			name: "accepted_at captures even if job uncertain",
			row: sqlcdb.ListOpenHoldMessagesRow{
				SmsMessageID:  &id,
				AcceptedAt:    &accepted,
				MessageStatus: sqlcdb.SmsStatusAccepted,
				JobStatus:     sqlcdb.NullSendJobStatus{SendJobStatus: sqlcdb.SendJobStatusUncertain, Valid: true},
			},
			want: "capture",
		},
		{
			name: "failed without accept and dead job releases",
			row: sqlcdb.ListOpenHoldMessagesRow{
				SmsMessageID:  &id,
				MessageStatus: sqlcdb.SmsStatusFailed,
				JobStatus:     sqlcdb.NullSendJobStatus{SendJobStatus: sqlcdb.SendJobStatusDead, Valid: true},
			},
			want: "release",
		},
		{
			name: "queued uncertain is skipped",
			row: sqlcdb.ListOpenHoldMessagesRow{
				SmsMessageID:  &id,
				MessageStatus: sqlcdb.SmsStatusQueued,
				JobStatus:     sqlcdb.NullSendJobStatus{SendJobStatus: sqlcdb.SendJobStatusUncertain, Valid: true},
			},
			want: "",
		},
		{
			name: "billing_action capture wins",
			row: sqlcdb.ListOpenHoldMessagesRow{
				SmsMessageID:  &id,
				MessageStatus: sqlcdb.SmsStatusFailed,
				BillingAction: sqlcdb.NullBillingAction{BillingAction: sqlcdb.BillingActionCapture, Valid: true},
			},
			want: "capture",
		},
	}
	for _, tc := range cases {
		if got := settleAction(tc.row); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestProductForDestDomestic7(t *testing.T) {
	for _, raw := range []string{"79001234567", "77001234567"} {
		d, err := msisdn.NormalizeDest(raw)
		if err != nil {
			t.Fatal(err)
		}
		if ProductForDest(d) != sqlcdb.BillingProductSmsDomestic {
			t.Fatalf("%s want domestic international=%v", raw, d.International)
		}
	}
	d, err := msisdn.NormalizeDest("14155551234")
	if err != nil {
		t.Fatal(err)
	}
	if ProductForDest(d) != sqlcdb.BillingProductSmsInternational {
		t.Fatal("uk should be international")
	}
}

func TestCodeOf(t *testing.T) {
	if CodeOf(ErrInsufficientFunds) != "insufficient_funds" {
		t.Fatal(CodeOf(ErrInsufficientFunds))
	}
	if CodeOf(ErrTariffNotConfigured) != "tariff_not_configured" {
		t.Fatal(CodeOf(ErrTariffNotConfigured))
	}
}

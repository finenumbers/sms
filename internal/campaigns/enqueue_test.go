package campaigns

import (
	"testing"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func TestCanEnqueue(t *testing.T) {
	if !CanEnqueue(sqlcdb.CampaignStatusRunning) {
		t.Fatal("running should enqueue")
	}
	for _, st := range []sqlcdb.CampaignStatus{
		sqlcdb.CampaignStatusDraft,
		sqlcdb.CampaignStatusQueued,
		sqlcdb.CampaignStatusCancelled,
		sqlcdb.CampaignStatusCompleted,
		sqlcdb.CampaignStatusFailed,
	} {
		if CanEnqueue(st) {
			t.Fatalf("%s must not enqueue", st)
		}
	}
}

package campaigns

import sqlcdb "finenumbers/sms/internal/db/sqlc"

// CanEnqueue is true only while the campaign is running. Cancelled/queued/draft
// must not create new send_jobs; already-created jobs are drained by the outbox.
func CanEnqueue(st sqlcdb.CampaignStatus) bool {
	return st == sqlcdb.CampaignStatusRunning
}

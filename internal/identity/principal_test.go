package identity

import (
	"testing"

	"github.com/google/uuid"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func TestPrincipalAuditActorAndScope(t *testing.T) {
	keyID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	p := Principal{APIKeyID: &keyID, Scopes: []string{"sms:send"}}
	at, id := p.AuditActor()
	if at != sqlcdb.ActorTypeApiKey || id == nil || *id != keyID {
		t.Fatalf("api key actor %s %v", at, id)
	}
	if !p.HasScope("sms:send") || p.HasScope("sms:read") {
		t.Fatal("scopes")
	}

	adminID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	p = Principal{AdminUserID: &adminID}
	at, id = p.AuditActor()
	if at != sqlcdb.ActorTypeAdmin || id == nil || *id != adminID {
		t.Fatalf("admin actor %s %v", at, id)
	}

	userID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	p = Principal{ClientUserID: &userID}
	at, id = p.AuditActor()
	if at != sqlcdb.ActorTypeClientUser || id == nil || *id != userID {
		t.Fatalf("client actor %s %v", at, id)
	}
}

package lookup

import (
	"context"

	"github.com/google/uuid"

	"finenumbers/sms/internal/billing"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func (s *Service) Estimate(ctx context.Context, clientID uuid.UUID, checkType sqlcdb.LookupCheckType, phones []string, maxPhones int, capName string) (billing.Estimate, int, error) {
	if s == nil || s.store == nil || s.billing == nil {
		return billing.Estimate{}, 0, wrap(ErrValidation, "validation", "lookup service not configured")
	}
	view, err := s.settings.Get(ctx)
	if err != nil {
		return billing.Estimate{}, 0, err
	}
	if !view.LookupEnabled {
		return billing.Estimate{}, 0, wrap(ErrLookupDisabled, "lookup_disabled", "lookup is disabled")
	}
	if err := s.ensureClient(ctx, clientID); err != nil {
		return billing.Estimate{}, 0, err
	}
	if maxPhones <= 0 {
		maxPhones = int(view.LookupMaxBatchPhones)
	}
	prepared, _, err := PreparePhones(phones, "bulk", maxPhones, capName)
	if err != nil {
		return billing.Estimate{}, 0, err
	}
	est, err := s.billing.EstimateLookup(ctx, clientID, checkType, int64(len(prepared)))
	if err != nil {
		return billing.Estimate{}, 0, mapBillingErr(err)
	}
	return est, len(prepared), nil
}

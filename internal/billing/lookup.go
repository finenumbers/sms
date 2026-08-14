package billing

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func ProductForCheckType(checkType sqlcdb.LookupCheckType) (sqlcdb.BillingProduct, error) {
	switch checkType {
	case sqlcdb.LookupCheckTypeHlr:
		return sqlcdb.BillingProductHlr, nil
	case sqlcdb.LookupCheckTypePing:
		return sqlcdb.BillingProductSilentSms, nil
	default:
		return "", wrap(ErrValidation, "validation", "invalid check type")
	}
}

func KnownProduct(p sqlcdb.BillingProduct) bool {
	switch p {
	case sqlcdb.BillingProductSmsDomestic,
		sqlcdb.BillingProductSmsInternational,
		sqlcdb.BillingProductHlr,
		sqlcdb.BillingProductSilentSms:
		return true
	default:
		return false
	}
}

func LookupHoldKey(jobID uuid.UUID) string {
	return "hold:lookup_job:" + jobID.String()
}

func LookupDebitKey(itemID uuid.UUID) string {
	return "debit:lookup_item:" + itemID.String()
}

func LookupReleaseKey(itemID uuid.UUID) string {
	return "release:lookup_item:" + itemID.String()
}

func LookupReleaseRemainderKey(jobID uuid.UUID) string {
	return "release-remainder:lookup_job:" + jobID.String()
}

func RemainingOnHold(holdAmount, settledAmount decimal.Decimal) (decimal.Decimal, error) {
	left := holdAmount.Sub(settledAmount)
	if left.IsNegative() {
		return decimal.Zero, wrap(ErrInvalidAmount, "invalid_amount", "settled exceeds hold")
	}
	return left, nil
}

func HoldIsOpen(remaining decimal.Decimal) bool {
	return remaining.GreaterThan(decimal.Zero)
}

// LookupRemainderBlocked is true while any item is non-terminal or terminal
// without a posted billing_action. Remainder must not run in that state.
func LookupRemainderBlocked(blockingItems int64) bool {
	return blockingItems > 0
}

// LookupItemSettleAction chooses capture vs release when billing_action is empty.
// Policy B: provider-final result (reachable / unreachable / error) → capture.
func LookupItemSettleAction(item sqlcdb.LookupItem) string {
	if item.BillingAction.Valid {
		switch item.BillingAction.BillingAction {
		case sqlcdb.BillingActionCapture:
			return "capture"
		case sqlcdb.BillingActionRelease:
			return "release"
		}
	}
	if item.Status == sqlcdb.LookupItemStatusCompleted {
		return "capture"
	}
	switch item.ResultStatus {
	case nil:
	default:
		switch *item.ResultStatus {
		case "reachable", "unreachable", "error":
			return "capture"
		}
	}
	if item.Status == sqlcdb.LookupItemStatusFailed {
		return "release"
	}
	return ""
}

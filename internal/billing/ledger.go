package billing

import (
	"github.com/shopspring/decimal"
)

func applyHold(avail, held, amt decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	nextAvail := avail.Sub(amt)
	if nextAvail.IsNegative() {
		return avail, held, wrap(ErrInsufficientFunds, "insufficient_funds", "insufficient funds")
	}
	return nextAvail, held.Add(amt), nil
}

func applyRelease(avail, held, amt decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	nextHeld := held.Sub(amt)
	if nextHeld.IsNegative() {
		return avail, held, wrap(ErrInvalidAmount, "invalid_amount", "release exceeds held")
	}
	return avail.Add(amt), nextHeld, nil
}

func applyDebitFromHold(avail, held, amt decimal.Decimal) (decimal.Decimal, decimal.Decimal, error) {
	nextHeld := held.Sub(amt)
	if nextHeld.IsNegative() {
		return avail, held, wrap(ErrInvalidAmount, "invalid_amount", "debit exceeds held")
	}
	return avail, nextHeld, nil
}

func applyAdjustment(avail, amt decimal.Decimal, debit bool, allowNeg bool) (decimal.Decimal, error) {
	next := avail.Add(amt)
	if debit {
		next = avail.Sub(amt)
	}
	if next.IsNegative() && !allowNeg {
		return avail, wrap(ErrNegativeBalance, "negative_balance_forbidden", "negative balance forbidden")
	}
	return next, nil
}

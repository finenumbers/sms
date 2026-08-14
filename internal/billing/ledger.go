package billing

import (
	"encoding/json"

	"github.com/shopspring/decimal"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

type projected struct {
	Available decimal.Decimal
	Held      decimal.Decimal
}

func foldLedger(rows []sqlcdb.WalletTransaction) (projected, error) {
	var p projected
	for _, row := range rows {
		amt := row.Amount
		switch row.Type {
		case sqlcdb.WalletTxTypeCREDIT:
			p.Available = p.Available.Add(amt)
		case sqlcdb.WalletTxTypeHOLD:
			p.Available = p.Available.Sub(amt)
			p.Held = p.Held.Add(amt)
		case sqlcdb.WalletTxTypeRELEASE:
			p.Held = p.Held.Sub(amt)
			p.Available = p.Available.Add(amt)
		case sqlcdb.WalletTxTypeDEBIT:
			p.Held = p.Held.Sub(amt)
		case sqlcdb.WalletTxTypeADJUSTMENT:
			dir := adjustmentDirection(row.Metadata)
			if dir == "debit" {
				p.Available = p.Available.Sub(amt)
			} else {
				p.Available = p.Available.Add(amt)
			}
		}
	}
	return p, nil
}

func adjustmentDirection(meta []byte) string {
	if len(meta) == 0 {
		return "credit"
	}
	var m map[string]any
	if err := json.Unmarshal(meta, &m); err != nil {
		return "credit"
	}
	s, _ := m["direction"].(string)
	if s == "debit" {
		return "debit"
	}
	return "credit"
}

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

package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/msisdn"
)

type ResolvedTariff struct {
	PlanID     uuid.UUID
	PlanCode   string
	Product    sqlcdb.BillingProduct
	SellPrice  decimal.Decimal
	Currency   string
	Assignment uuid.UUID
}

func ProductForDest(dest msisdn.Dest) sqlcdb.BillingProduct {
	if dest.International {
		return sqlcdb.BillingProductSmsInternational
	}
	return sqlcdb.BillingProductSmsDomestic
}

func (s *Service) ResolveTariff(ctx context.Context, q *sqlcdb.Queries, clientID uuid.UUID, product sqlcdb.BillingProduct) (ResolvedTariff, error) {
	if q == nil {
		q = s.queries()
	}
	row, err := q.GetClientTariff(ctx, sqlcdb.GetClientTariffParams{ClientID: clientID, Product: product})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedTariff{}, wrap(ErrTariffNotConfigured, "tariff_not_configured", "tariff not configured")
		}
		return ResolvedTariff{}, err
	}
	if !row.PlanIsActive {
		return ResolvedTariff{}, wrap(ErrInvalidTariff, "invalid_tariff", "tariff plan is inactive")
	}
	now := time.Now().UTC()
	if row.EffectiveFrom.After(now) {
		return ResolvedTariff{}, wrap(ErrInvalidTariff, "invalid_tariff", "tariff not yet effective")
	}
	if row.EffectiveTo != nil && !row.EffectiveTo.After(now) {
		return ResolvedTariff{}, wrap(ErrInvalidTariff, "invalid_tariff", "tariff expired")
	}
	sell := row.PlanSellPrice
	if row.PriceOverride != nil {
		sell = *row.PriceOverride
	}
	if err := mustPositive(sell, "sell_price"); err != nil {
		return ResolvedTariff{}, err
	}
	wallet, err := q.GetWalletByClientID(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResolvedTariff{}, wrap(ErrWalletNotFound, "wallet_not_found", "wallet not found")
		}
		return ResolvedTariff{}, err
	}
	if trimCurrency(wallet.Currency) != trimCurrency(row.Currency) {
		return ResolvedTariff{}, wrap(ErrInvalidTariff, "invalid_tariff", "wallet and tariff currency mismatch")
	}
	return ResolvedTariff{
		PlanID:     row.TariffPlanID,
		PlanCode:   row.PlanCode,
		Product:    row.Product,
		SellPrice:  sell,
		Currency:   trimCurrency(row.Currency),
		Assignment: row.ID,
	}, nil
}

func trimCurrency(s string) string {
	return strings.TrimSpace(s)
}

// AssertPlanAssignable rejects a catalog plan that cannot be assigned to product.
func AssertPlanAssignable(plan sqlcdb.TariffPlan, product sqlcdb.BillingProduct) error {
	if plan.Product != product {
		return wrap(ErrValidation, "validation", "tariff plan product mismatch")
	}
	if !plan.IsActive {
		return wrap(ErrInvalidTariff, "invalid_tariff", "tariff plan is inactive")
	}
	return nil
}

func (s *Service) Estimate(ctx context.Context, clientID uuid.UUID, dest msisdn.Dest, text string) (Estimate, error) {
	product := ProductForDest(dest)
	tariff, err := s.ResolveTariff(ctx, nil, clientID, product)
	if err != nil {
		return Estimate{}, err
	}
	seg := SegmentCount(text)
	total := tariff.SellPrice.Mul(decimal.NewFromInt(int64(seg)))
	return Estimate{
		Product:        product,
		Segments:       seg,
		UnitSellPrice:  tariff.SellPrice,
		Total:          total,
		Currency:       tariff.Currency,
		TariffPlanID:   tariff.PlanID,
		TariffPlanCode: tariff.PlanCode,
	}, nil
}

type Estimate struct {
	Product        sqlcdb.BillingProduct
	Segments       int
	UnitSellPrice  decimal.Decimal
	Total          decimal.Decimal
	Currency       string
	TariffPlanID   uuid.UUID
	TariffPlanCode string
}

func (s *Service) AssertCanAfford(ctx context.Context, clientID uuid.UUID, required decimal.Decimal) error {
	w, err := s.queries().GetWalletByClientID(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wrap(ErrWalletNotFound, "wallet_not_found", "wallet not found")
		}
		return err
	}
	if w.AvailableBalance.LessThan(required) {
		return wrap(ErrInsufficientFunds, "insufficient_funds",
			fmt.Sprintf("insufficient funds: need %s have %s", moneyString(required), moneyString(w.AvailableBalance)))
	}
	return nil
}

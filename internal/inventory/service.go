package inventory

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/msisdn"
	"finenumbers/sms/internal/runexis"
	"finenumbers/sms/internal/settings"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrValidation      = errors.New("validation")
	ErrNotAssignable   = errors.New("number is not assignable")
	ErrAlreadyAssigned = errors.New("number already assigned")
	ErrNotAssigned     = errors.New("number is not assigned")
	ErrClientNotActive = errors.New("client is not active")
	ErrConflict        = errors.New("conflict")
)

type DirectionSource interface {
	Get(ctx context.Context) (settings.Public, error)
}

type Service struct {
	store *db.Store
	dirs  DirectionSource
	rx    *runexis.Client
}

func New(store *db.Store, dirs DirectionSource, rx *runexis.Client) *Service {
	return &Service{store: store, dirs: dirs, rx: rx}
}

type UploadReport struct {
	Imported   int          `json:"imported"`
	Duplicates int          `json:"duplicates"`
	Invalid    []InvalidRow `json:"invalid"`
	Encoding   string       `json:"encoding"`
}

func (s *Service) Upload(ctx context.Context, raw []byte) (UploadReport, error) {
	parsed := ParseNumberFile(raw)
	rep := UploadReport{Invalid: parsed.Invalid, Encoding: parsed.Encoding}
	if len(parsed.Rows) == 0 {
		return rep, nil
	}

	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return UploadReport{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	seen := map[string]struct{}{}
	for _, row := range parsed.Rows {
		if _, ok := seen[row.MSISDN]; ok {
			rep.Duplicates++
			continue
		}
		seen[row.MSISDN] = struct{}{}
		_, err := q.InsertDefNumber(ctx, sqlcdb.InsertDefNumberParams{
			Msisdn: row.MSISDN,
			Region: optString(row.Region),
			Notes:  optString(row.Notes),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				rep.Duplicates++
				continue
			}
			return UploadReport{}, err
		}
		rep.Imported++
	}
	if err := tx.Commit(ctx); err != nil {
		return UploadReport{}, err
	}
	return rep, nil
}

type ListFilter struct {
	Status   *sqlcdb.DefNumberStatus
	ClientID *uuid.UUID
	Q        string
	Limit    int32
	Offset   int32
}

func (s *Service) List(ctx context.Context, f ListFilter) ([]sqlcdb.ListDefNumbersRow, error) {
	limit, offset := page(f.Limit, f.Offset)
	arg := sqlcdb.ListDefNumbersParams{
		ClientID:   f.ClientID,
		PageOffset: offset,
		PageLimit:  limit,
	}
	if f.Status != nil {
		arg.Status = sqlcdb.NullDefNumberStatus{DefNumberStatus: *f.Status, Valid: true}
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		arg.Q = &q
	}
	return s.store.Queries.ListDefNumbers(ctx, arg)
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (sqlcdb.GetDefNumberViewRow, error) {
	row, err := s.store.Queries.GetDefNumberView(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.GetDefNumberViewRow{}, ErrNotFound
		}
		return sqlcdb.GetDefNumberViewRow{}, err
	}
	return row, nil
}

type UpdateMetaInput struct {
	Region *string
	Notes  *string
}

func (s *Service) UpdateMeta(ctx context.Context, id uuid.UUID, in UpdateMetaInput) (sqlcdb.GetDefNumberViewRow, error) {
	cur, err := s.store.Queries.GetDefNumberByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.GetDefNumberViewRow{}, ErrNotFound
		}
		return sqlcdb.GetDefNumberViewRow{}, err
	}
	region, notes := cur.Region, cur.Notes
	if in.Region != nil {
		region = optString(strings.TrimSpace(*in.Region))
	}
	if in.Notes != nil {
		notes = optString(strings.TrimSpace(*in.Notes))
	}
	if _, err := s.store.Queries.UpdateDefNumberMeta(ctx, sqlcdb.UpdateDefNumberMetaParams{
		Region: region,
		Notes:  notes,
		ID:     id,
	}); err != nil {
		return sqlcdb.GetDefNumberViewRow{}, err
	}
	return s.Get(ctx, id)
}

type AssignResult struct {
	Number     sqlcdb.GetDefNumberViewRow
	Assignment sqlcdb.NumberAssignment
}

func (s *Service) Assign(ctx context.Context, numberID, clientID, adminID uuid.UUID) (AssignResult, error) {
	dirs := defaultDirections()
	if s.dirs != nil {
		if v, err := s.dirs.Get(ctx); err == nil {
			dirs = v.SMSDirections
		}
	}

	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return AssignResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	n, err := q.GetDefNumberByIDForUpdate(ctx, numberID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AssignResult{}, ErrNotFound
		}
		return AssignResult{}, err
	}
	if n.Status != sqlcdb.DefNumberStatusInventory {
		if n.Status == sqlcdb.DefNumberStatusAssigned {
			return AssignResult{}, ErrAlreadyAssigned
		}
		return AssignResult{}, fmt.Errorf("%w: status %s", ErrNotAssignable, n.Status)
	}
	if !msisdn.IsSender(n.Msisdn) {
		return AssignResult{}, fmt.Errorf("%w: invalid msisdn", ErrValidation)
	}

	cl, err := q.GetClientByIDForUpdate(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AssignResult{}, ErrNotFound
		}
		return AssignResult{}, err
	}
	if cl.Status != sqlcdb.ClientStatusActive {
		return AssignResult{}, ErrClientNotActive
	}

	asg, err := q.InsertAssignment(ctx, sqlcdb.InsertAssignmentParams{
		DefNumberID: n.ID,
		ClientID:    cl.ID,
		AssignedBy:  &adminID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return AssignResult{}, ErrAlreadyAssigned
		}
		return AssignResult{}, err
	}
	if _, err := q.UpdateDefNumberStatus(ctx, sqlcdb.UpdateDefNumberStatusParams{
		Status: sqlcdb.DefNumberStatusAssigned,
		ID:     n.ID,
	}); err != nil {
		return AssignResult{}, err
	}
	if _, err := q.InsertDirectionJob(ctx, sqlcdb.InsertDirectionJobParams{
		DefNumberID:  n.ID,
		AssignmentID: &asg.ID,
		Msisdn:       n.Msisdn,
		DirIn:        dirs.In,
		DirDomOut:    dirs.DomOut,
		DirIntOut:    dirs.IntOut,
		DirInMass:    dirs.InMass,
	}); err != nil {
		return AssignResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AssignResult{}, err
	}
	view, err := s.Get(ctx, n.ID)
	if err != nil {
		return AssignResult{}, err
	}
	return AssignResult{Number: view, Assignment: asg}, nil
}

func (s *Service) Unassign(ctx context.Context, numberID uuid.UUID) (sqlcdb.GetDefNumberViewRow, uuid.UUID, error) {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return sqlcdb.GetDefNumberViewRow{}, uuid.Nil, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	n, err := q.GetDefNumberByIDForUpdate(ctx, numberID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.GetDefNumberViewRow{}, uuid.Nil, ErrNotFound
		}
		return sqlcdb.GetDefNumberViewRow{}, uuid.Nil, err
	}
	asg, err := q.GetOpenAssignmentByNumber(ctx, n.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.GetDefNumberViewRow{}, uuid.Nil, ErrNotAssigned
		}
		return sqlcdb.GetDefNumberViewRow{}, uuid.Nil, err
	}
	if _, err := q.CloseAssignment(ctx, asg.ID); err != nil {
		return sqlcdb.GetDefNumberViewRow{}, uuid.Nil, err
	}
	if _, err := q.UpdateDefNumberStatus(ctx, sqlcdb.UpdateDefNumberStatusParams{
		Status: sqlcdb.DefNumberStatusInventory,
		ID:     n.ID,
	}); err != nil {
		return sqlcdb.GetDefNumberViewRow{}, uuid.Nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sqlcdb.GetDefNumberViewRow{}, uuid.Nil, err
	}
	view, err := s.Get(ctx, n.ID)
	if err != nil {
		return sqlcdb.GetDefNumberViewRow{}, uuid.Nil, err
	}
	return view, asg.ClientID, nil
}

func UnassignAll(ctx context.Context, q *sqlcdb.Queries, clientID uuid.UUID) (int, error) {
	rows, err := q.ListOpenAssignmentsByClient(ctx, clientID)
	if err != nil {
		return 0, err
	}
	for _, asg := range rows {
		if _, err := q.GetDefNumberByIDForUpdate(ctx, asg.DefNumberID); err != nil {
			return 0, err
		}
		if _, err := q.CloseAssignment(ctx, asg.ID); err != nil {
			return 0, err
		}
		if _, err := q.UpdateDefNumberStatus(ctx, sqlcdb.UpdateDefNumberStatusParams{
			Status: sqlcdb.DefNumberStatusInventory,
			ID:     asg.DefNumberID,
		}); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status sqlcdb.DefNumberStatus) (sqlcdb.GetDefNumberViewRow, error) {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return sqlcdb.GetDefNumberViewRow{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	n, err := q.GetDefNumberByIDForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.GetDefNumberViewRow{}, ErrNotFound
		}
		return sqlcdb.GetDefNumberViewRow{}, err
	}
	switch status {
	case sqlcdb.DefNumberStatusDisabled:
		if n.Status != sqlcdb.DefNumberStatusInventory {
			if n.Status == sqlcdb.DefNumberStatusAssigned {
				return sqlcdb.GetDefNumberViewRow{}, fmt.Errorf("%w: unassign first", ErrConflict)
			}
			return sqlcdb.GetDefNumberViewRow{}, fmt.Errorf("%w: status %s", ErrConflict, n.Status)
		}
	case sqlcdb.DefNumberStatusInventory:
		if n.Status != sqlcdb.DefNumberStatusDisabled {
			return sqlcdb.GetDefNumberViewRow{}, fmt.Errorf("%w: status %s", ErrConflict, n.Status)
		}
	default:
		return sqlcdb.GetDefNumberViewRow{}, fmt.Errorf("%w: status must be inventory or disabled", ErrValidation)
	}
	if _, err := q.UpdateDefNumberStatus(ctx, sqlcdb.UpdateDefNumberStatusParams{
		Status: status,
		ID:     n.ID,
	}); err != nil {
		return sqlcdb.GetDefNumberViewRow{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sqlcdb.GetDefNumberViewRow{}, err
	}
	return s.Get(ctx, n.ID)
}

func defaultDirections() settings.Directions {
	return settings.Directions{In: true, DomOut: true, IntOut: false, InMass: false}
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func page(limit, offset int32) (int32, int32) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

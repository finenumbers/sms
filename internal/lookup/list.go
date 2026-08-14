package lookup

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

type ListJobsFilter struct {
	ClientID  *uuid.UUID
	Status    *sqlcdb.LookupJobStatus
	CheckType *sqlcdb.LookupCheckType
	Limit     int32
	Offset    int32
}

func (s *Service) GetJob(ctx context.Context, id uuid.UUID) (sqlcdb.LookupJob, error) {
	job, err := s.store.Queries.GetLookupJob(ctx, id)
	if err != nil {
		if errorsIsNoRows(err) {
			return sqlcdb.LookupJob{}, wrap(ErrNotFound, "not_found", "job not found")
		}
		return sqlcdb.LookupJob{}, err
	}
	return job, nil
}

func (s *Service) GetJobForClient(ctx context.Context, clientID, jobID uuid.UUID) (sqlcdb.LookupJob, error) {
	job, err := s.store.Queries.GetLookupJobForClient(ctx, sqlcdb.GetLookupJobForClientParams{
		ID:       jobID,
		ClientID: clientID,
	})
	if err != nil {
		if errorsIsNoRows(err) {
			return sqlcdb.LookupJob{}, wrap(ErrNotFound, "not_found", "job not found")
		}
		return sqlcdb.LookupJob{}, err
	}
	return job, nil
}

func (s *Service) GetItemForClient(ctx context.Context, clientID, itemID uuid.UUID) (sqlcdb.LookupItem, error) {
	item, err := s.store.Queries.GetLookupItemForClient(ctx, sqlcdb.GetLookupItemForClientParams{
		ID:       itemID,
		ClientID: clientID,
	})
	if err != nil {
		if errorsIsNoRows(err) {
			return sqlcdb.LookupItem{}, wrap(ErrNotFound, "not_found", "check not found")
		}
		return sqlcdb.LookupItem{}, err
	}
	return item, nil
}

func (s *Service) GetCheckOrJob(ctx context.Context, clientID, id uuid.UUID) (item *sqlcdb.LookupItem, job *sqlcdb.LookupJob, err error) {
	it, err := s.GetItemForClient(ctx, clientID, id)
	if err == nil {
		return &it, nil, nil
	}
	if AsError(err) == nil || AsError(err).Code != "not_found" {
		return nil, nil, err
	}
	jb, err := s.GetJobForClient(ctx, clientID, id)
	if err != nil {
		return nil, nil, err
	}
	return nil, &jb, nil
}

func (s *Service) ListJobs(ctx context.Context, f ListJobsFilter) ([]sqlcdb.LookupJob, int64, error) {
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	arg := sqlcdb.ListLookupJobsParams{
		ClientID:        f.ClientID,
		FilterStatus:    nullJobStatus(f.Status),
		FilterCheckType: nullCheckType(f.CheckType),
		PageLimit:       f.Limit,
		PageOffset:      f.Offset,
	}
	rows, err := s.store.Queries.ListLookupJobs(ctx, arg)
	if err != nil {
		return nil, 0, err
	}
	n, err := s.store.Queries.CountLookupJobs(ctx, sqlcdb.CountLookupJobsParams{
		ClientID:        f.ClientID,
		FilterStatus:    nullJobStatus(f.Status),
		FilterCheckType: nullCheckType(f.CheckType),
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, n, nil
}

func (s *Service) ListItemsForClient(ctx context.Context, clientID uuid.UUID, status *sqlcdb.LookupItemStatus, checkType *sqlcdb.LookupCheckType, limit, offset int32) ([]sqlcdb.LookupItem, int64, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.store.Queries.ListLookupItemsForClient(ctx, sqlcdb.ListLookupItemsForClientParams{
		ClientID:        clientID,
		FilterStatus:    nullItemStatus(status),
		FilterCheckType: nullCheckType(checkType),
		PageLimit:       limit,
		PageOffset:      offset,
	})
	if err != nil {
		return nil, 0, err
	}
	n, err := s.store.Queries.CountLookupItemsForClient(ctx, sqlcdb.CountLookupItemsForClientParams{
		ClientID:        clientID,
		FilterStatus:    nullItemStatus(status),
		FilterCheckType: nullCheckType(checkType),
	})
	if err != nil {
		return nil, 0, err
	}
	return rows, n, nil
}

func (s *Service) ExportJobItems(ctx context.Context, job sqlcdb.LookupJob) ([]byte, error) {
	n, err := s.store.Queries.CountLookupItemsByJob(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	if n > ExportMaxRows {
		return nil, wrap(ErrValidation, "validation", "export exceeds 50000 rows")
	}
	items, err := s.store.Queries.ListLookupItemsByJob(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	return BuildXLSX(job.CheckType, items)
}

func (s *Service) ListItems(ctx context.Context, jobID uuid.UUID, limit, offset int32) ([]sqlcdb.LookupItem, int64, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.store.Queries.ListLookupItemsByJobPage(ctx, sqlcdb.ListLookupItemsByJobPageParams{
		JobID:      jobID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, 0, err
	}
	n, err := s.store.Queries.CountLookupItemsByJob(ctx, jobID)
	if err != nil {
		return nil, 0, err
	}
	return rows, n, nil
}

func errorsIsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

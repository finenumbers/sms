package lookup

import (
	"strings"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func ParseCheckType(raw string) (sqlcdb.LookupCheckType, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "hlr":
		return sqlcdb.LookupCheckTypeHlr, nil
	case "ping":
		return sqlcdb.LookupCheckTypePing, nil
	default:
		return "", wrap(ErrValidation, "validation", "type must be hlr or ping")
	}
}

func ParseJobStatus(raw string) (sqlcdb.LookupJobStatus, error) {
	st := sqlcdb.LookupJobStatus(strings.ToLower(strings.TrimSpace(raw)))
	switch st {
	case sqlcdb.LookupJobStatusQueued, sqlcdb.LookupJobStatusProcessing,
		sqlcdb.LookupJobStatusCompleted, sqlcdb.LookupJobStatusCompletedWithErrors,
		sqlcdb.LookupJobStatusFailed:
		return st, nil
	default:
		return "", wrap(ErrValidation, "validation", "invalid status")
	}
}

func nullJobStatus(v *sqlcdb.LookupJobStatus) sqlcdb.NullLookupJobStatus {
	if v == nil {
		return sqlcdb.NullLookupJobStatus{}
	}
	return sqlcdb.NullLookupJobStatus{LookupJobStatus: *v, Valid: true}
}

func nullCheckType(v *sqlcdb.LookupCheckType) sqlcdb.NullLookupCheckType {
	if v == nil {
		return sqlcdb.NullLookupCheckType{}
	}
	return sqlcdb.NullLookupCheckType{LookupCheckType: *v, Valid: true}
}

func nullItemStatus(v *sqlcdb.LookupItemStatus) sqlcdb.NullLookupItemStatus {
	if v == nil {
		return sqlcdb.NullLookupItemStatus{}
	}
	return sqlcdb.NullLookupItemStatus{LookupItemStatus: *v, Valid: true}
}

func nullPreviewStatus(v *sqlcdb.LookupCsvPreviewStatus) sqlcdb.NullLookupCsvPreviewStatus {
	if v == nil {
		return sqlcdb.NullLookupCsvPreviewStatus{}
	}
	return sqlcdb.NullLookupCsvPreviewStatus{LookupCsvPreviewStatus: *v, Valid: true}
}

func ParseItemStatus(raw string) (sqlcdb.LookupItemStatus, error) {
	st := sqlcdb.LookupItemStatus(strings.ToLower(strings.TrimSpace(raw)))
	switch st {
	case sqlcdb.LookupItemStatusQueued, sqlcdb.LookupItemStatusReserved,
		sqlcdb.LookupItemStatusPending, sqlcdb.LookupItemStatusCompleted,
		sqlcdb.LookupItemStatusFailed:
		return st, nil
	default:
		return "", wrap(ErrValidation, "validation", "invalid status")
	}
}

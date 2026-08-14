package smsc

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Persistence interface {
	SaveRequest(ctx context.Context, record RequestRecord) (SaveResult, error)
	UpdateRequest(ctx context.Context, id string, patch RequestPatch) error
	FindSucceededSend(ctx context.Context, providerCode, tenantID, idempotencyKey string) (*RequestRecord, error)
	FindLatestSend(ctx context.Context, providerCode, tenantID, idempotencyKey string) (*RequestRecord, error)
	SaveCallback(ctx context.Context, record CallbackRecord) (SaveResult, error)
}

const persistTimeout = 5 * time.Second

// persistContext keeps a write alive after Tick/HTTP cancel so a finished
// SMSC SEND is still recorded (otherwise the next poll may submit twice).
func persistContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
}

type IdempotencyConflictError struct {
	IdempotencyKey string
}

func (e *IdempotencyConflictError) Error() string {
	return fmt.Sprintf("SEND already in flight for idempotencyKey=%s", e.IdempotencyKey)
}

// Memory is an in-memory Persistence for unit tests.
type Memory struct {
	mu        sync.Mutex
	requests  map[string]RequestRecord
	callbacks map[string]CallbackRecord
	seq       int
}

func NewMemory() *Memory {
	return &Memory{
		requests:  map[string]RequestRecord{},
		callbacks: map[string]CallbackRecord{},
	}
}

func (m *Memory) Requests() []RequestRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RequestRecord, 0, len(m.requests))
	for _, row := range m.requests {
		out = append(out, row)
	}
	return out
}

func (m *Memory) Callbacks() []CallbackRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CallbackRecord, 0, len(m.callbacks))
	for _, row := range m.callbacks {
		out = append(out, row)
	}
	return out
}

func (m *Memory) SaveRequest(_ context.Context, record RequestRecord) (SaveResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if record.IdempotencyKey != "" && record.Kind == KindSend {
		if record.TenantID == "" {
			return SaveResult{}, fmt.Errorf("SEND provider requests require tenantId")
		}
		existing := m.latestSendLocked(record.ProviderCode, record.TenantID, record.IdempotencyKey)
		if existing != nil && existing.Status == RequestSucceeded && existing.ID != "" {
			return SaveResult{ID: existing.ID, Deduplicated: true}, nil
		}
		if existing != nil && existing.Status == RequestPending {
			return SaveResult{}, &IdempotencyConflictError{IdempotencyKey: record.IdempotencyKey}
		}
	}
	m.seq++
	id := record.ID
	if id == "" {
		id = fmt.Sprintf("req_%d", m.seq)
	}
	record.ID = id
	record.seq = m.seq
	m.requests[id] = record
	return SaveResult{ID: id, Deduplicated: false}, nil
}

func (m *Memory) UpdateRequest(_ context.Context, id string, patch RequestPatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.requests[id]
	if !ok {
		return nil
	}
	if patch.Status != "" {
		current.Status = patch.Status
	}
	if patch.ProviderMessageID != "" {
		current.ProviderMessageID = patch.ProviderMessageID
	}
	if patch.HTTPStatus != 0 {
		current.HTTPStatus = patch.HTTPStatus
	}
	if patch.ErrorCode != "" {
		current.ErrorCode = patch.ErrorCode
	}
	if patch.ErrorMessage != "" {
		current.ErrorMessage = patch.ErrorMessage
	}
	if patch.ResponsePayload != nil {
		current.ResponsePayload = patch.ResponsePayload
	}
	if patch.Normalized != nil {
		current.Normalized = patch.Normalized
	}
	if !patch.FinishedAt.IsZero() {
		current.FinishedAt = patch.FinishedAt
	}
	m.requests[id] = current
	return nil
}

func (m *Memory) FindSucceededSend(_ context.Context, providerCode, tenantID, idempotencyKey string) (*RequestRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	latest := m.latestSendLocked(providerCode, tenantID, idempotencyKey)
	if latest == nil || latest.Status != RequestSucceeded {
		return nil, nil
	}
	cp := *latest
	return &cp, nil
}

func (m *Memory) FindLatestSend(_ context.Context, providerCode, tenantID, idempotencyKey string) (*RequestRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	latest := m.latestSendLocked(providerCode, tenantID, idempotencyKey)
	if latest == nil {
		return nil, nil
	}
	cp := *latest
	return &cp, nil
}

func (m *Memory) latestSendLocked(providerCode, tenantID, idempotencyKey string) *RequestRecord {
	var best *RequestRecord
	for _, row := range m.requests {
		if row.ProviderCode != providerCode || row.TenantID != tenantID || row.IdempotencyKey != idempotencyKey || row.Kind != KindSend {
			continue
		}
		if best == nil || row.seq >= best.seq {
			cp := row
			best = &cp
		}
	}
	return best
}

func (m *Memory) SaveCallback(_ context.Context, record CallbackRecord) (SaveResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if record.DedupeKey != "" {
		for _, row := range m.callbacks {
			if row.DedupeKey == record.DedupeKey && row.ProviderCode == record.ProviderCode {
				return SaveResult{ID: row.ID, Deduplicated: true}, nil
			}
		}
	}
	m.seq++
	id := record.ID
	if id == "" {
		id = fmt.Sprintf("cb_%d", m.seq)
	}
	record.ID = id
	m.callbacks[id] = record
	return SaveResult{ID: id, Deduplicated: false}, nil
}

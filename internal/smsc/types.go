package smsc

import "time"

const ProviderCode = "smsc"

type CheckType string

const (
	CheckHLR  CheckType = "hlr"
	CheckPing CheckType = "ping"
)

type Lifecycle string

const (
	LifecycleAccepted  Lifecycle = "accepted"
	LifecyclePending   Lifecycle = "pending"
	LifecycleCompleted Lifecycle = "completed"
	LifecycleFailed    Lifecycle = "failed"
)

type ResultStatus string

const (
	ResultReachable   ResultStatus = "reachable"
	ResultUnreachable ResultStatus = "unreachable"
	ResultPending     ResultStatus = "pending"
	ResultError       ResultStatus = "error"
	ResultUnknown     ResultStatus = "unknown"
)

type RequestKind string

const (
	KindSend    RequestKind = "SEND"
	KindStatus  RequestKind = "STATUS"
	KindCost    RequestKind = "COST"
	KindBalance RequestKind = "BALANCE"
)

type RequestStatus string

const (
	RequestPending   RequestStatus = "PENDING"
	RequestSucceeded RequestStatus = "SUCCEEDED"
	RequestFailed    RequestStatus = "FAILED"
)

// NormalizedResult is the provider-agnostic snapshot. Raw SMSC bodies stay
// in request/callback rows, never inside this struct.
type NormalizedResult struct {
	ProviderCode         string         `json:"provider_code"`
	CheckType            CheckType      `json:"check_type"`
	ProviderMessageID    string         `json:"provider_message_id"`
	PhoneE164            string         `json:"phone_e164"`
	LifecycleStatus      Lifecycle      `json:"lifecycle_status"`
	ResultStatus         ResultStatus   `json:"result_status"`
	IsReachable          *bool          `json:"is_reachable"`
	IMSI                 string         `json:"imsi"`
	MCC                  string         `json:"mcc"`
	MNC                  string         `json:"mnc"`
	OperatorName         string         `json:"operator_name"`
	CountryCode          string         `json:"country_code"`
	Ported               *bool          `json:"ported"`
	Roaming              *bool          `json:"roaming"`
	ProviderErrorCode    string         `json:"provider_error_code"`
	ProviderErrorMessage string         `json:"provider_error_message"`
	ProviderStatusCode   string         `json:"provider_status_code"`
	Cost                 string         `json:"cost"`
	Currency             string         `json:"currency"`
	Extras               map[string]any `json:"extras"`
}

type CostEstimate struct {
	ProviderCode string
	CheckType    CheckType
	PhoneE164    string
	Cost         string
	Currency     string
	Parts        *int
	RawResponse  any
}

type Balance struct {
	ProviderCode string
	Balance      string
	Currency     string
	RawResponse  any
}

type SubmitInput struct {
	PhoneE164      string
	IdempotencyKey string
	TenantID       string
	JobItemID      string
	CorrelationID  string
}

type SubmitResult struct {
	ProviderCode      string
	CheckType         CheckType
	ProviderMessageID string
	Accepted          bool
	Deduplicated      bool
	Cost              string
	Balance           string
	Normalized        NormalizedResult
	RawRequest        any
	RawResponse       any
	ProviderRequestID string
}

type FetchStatusInput struct {
	ProviderMessageID string
	PhoneE164         string
	CheckType         CheckType
	TenantID          string
	JobItemID         string
	CorrelationID     string
	IncludeDetails    *bool
}

type FetchStatusResult struct {
	ProviderCode      string
	ProviderMessageID string
	Normalized        NormalizedResult
	RawRequest        any
	RawResponse       any
	ProviderRequestID string
}

type CallbackSignatures struct {
	MD5   string
	SHA1  string
	CRC32 string
}

type CallbackInput struct {
	RawPayload    any
	Signatures    CallbackSignatures
	TenantID      string
	JobItemID     string
	CorrelationID string
}

type CallbackResult struct {
	ProviderCode       string
	ProviderMessageID  string
	SignatureValid     *bool
	Deduplicated       bool
	Normalized         NormalizedResult
	RawPayload         any
	ProviderCallbackID string
}

type HTTPResult struct {
	OK         bool
	HTTPStatus int
	Body       any
	Duration   time.Duration
	Attempts   int
	URLPath    string
}

type RequestRecord struct {
	ID                string
	TenantID          string
	JobItemID         string
	ProviderCode      string
	Kind              RequestKind
	Status            RequestStatus
	ProviderMessageID string
	HTTPStatus        int
	ErrorCode         string
	ErrorMessage      string
	RequestPayload    any
	ResponsePayload   any
	Normalized        *NormalizedResult
	IdempotencyKey    string
	StartedAt         time.Time
	FinishedAt        time.Time
	seq               int
}

type RequestPatch struct {
	Status            RequestStatus
	ProviderMessageID string
	HTTPStatus        int
	ErrorCode         string
	ErrorMessage      string
	ResponsePayload   any
	Normalized        *NormalizedResult
	FinishedAt        time.Time
}

type CallbackRecord struct {
	ID                string
	TenantID          string
	JobItemID         string
	ProviderCode      string
	ProviderMessageID string
	RawPayload        any
	Normalized        *NormalizedResult
	SignatureValid    *bool
	DedupeKey         string
	ProcessError      string
	ProcessedAt       time.Time
}

type SaveResult struct {
	ID           string
	Deduplicated bool
}

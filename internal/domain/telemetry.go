package domain

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

var ErrUnknownRuntimeConnection = errors.New("unknown runtime connection")

// UsageSource identifies the connector that produced an observation.
type UsageSource string

const (
	UsageSourceGantryNative UsageSource = "gantry_native"
	UsageSourceLangSmith    UsageSource = "langsmith"
	UsageSourceOTEL         UsageSource = "otel"
)

// Decimal is an exact base-10 value represented without binary floating-point
// rounding. Repositories persist it to PostgreSQL numeric columns.
type Decimal string

func (d Decimal) MarshalJSON() ([]byte, error) {
	if _, err := strconv.ParseFloat(string(d), 64); err != nil {
		return nil, fmt.Errorf("marshal decimal %q: %w", d, err)
	}
	return []byte(d), nil
}

// UsageObservation is a runtime-neutral, content-free usage fact. Prompt and
// response bodies are deliberately excluded from the contract.
type UsageObservation struct {
	Source              UsageSource
	SourceObservationID string

	RuntimeConnectionID string
	RuntimeAgentID      string
	RuntimeExecutionID  string
	ParentExecutionID   string
	ThreadID            string

	Model    string
	Provider string

	RequestCount        int64
	InputTokens         int64
	OutputTokens        int64
	CachedInputTokens   int64
	CacheCreationTokens int64
	ReasoningTokens     int64
	TotalTokens         int64

	ContextWindowTokens *int64
	ContextUtilization  *float64

	InputCostUSD  *Decimal
	OutputCostUSD *Decimal
	TotalCostUSD  *Decimal

	DurationMS         *int64
	TimeToFirstTokenMS *int64
	ErrorCount         int64

	StartedAt  *time.Time
	EndedAt    *time.Time
	ObservedAt time.Time

	ModelMetadataVersion string
	Attributes           map[string]any
}

// UsageQuery describes a bounded connector or metrics query.
type UsageQuery struct {
	From                time.Time
	To                  time.Time
	Interval            time.Duration
	RuntimeConnectionID string
	AgentID             string
	RuntimeAgentID      string
	RuntimeExecutionID  string
	RuntimeJobID        string
	GroupingDimension   string
	Model               string
	Source              UsageSource
}

// UsageSummary is an aggregate over one preferred telemetry source.
type UsageSummary struct {
	RuntimeConnectionID string
	AgentID             string
	From                time.Time
	To                  time.Time
	Source              UsageSource
	Available           bool
	Configured          bool
	LastObservedAt      *time.Time
	RequestCount        int64
	InputTokens         int64
	OutputTokens        int64
	CachedInputTokens   int64
	CacheCreationTokens int64
	ReasoningTokens     int64
	TotalTokens         int64
	EstimatedCostUSD    *Decimal
	AverageDurationMS   *float64
	P95DurationMS       *float64
	ErrorRate           *float64
	ContextUtilization  *float64
	ModelBreakdown      []ModelUsageSummary
	TimeSeries          []UsageTimeBucket
}

type ModelUsageSummary struct {
	Model        string `json:"model"`
	RequestCount int64  `json:"request_count"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
}

type UsageTimeBucket struct {
	StartedAt    time.Time `json:"started_at"`
	RequestCount int64     `json:"request_count"`
	InputTokens  int64     `json:"input_tokens"`
	OutputTokens int64     `json:"output_tokens"`
	TotalTokens  int64     `json:"total_tokens"`
}

type TelemetryRunStatus string

const (
	TelemetryRunRunning   TelemetryRunStatus = "running"
	TelemetryRunSucceeded TelemetryRunStatus = "succeeded"
	TelemetryRunFailed    TelemetryRunStatus = "failed"
)

// TelemetryIngestionRun records connector health without replacing previously
// accepted observations when a connector is unavailable.
type TelemetryIngestionRun struct {
	ID                  string
	RuntimeConnectionID string
	Source              UsageSource
	Status              TelemetryRunStatus
	Cursor              string
	SchemaVersion       string
	StartedAt           time.Time
	FinishedAt          *time.Time
	LastSuccessfulAt    *time.Time
	Accepted            int64
	Rejected            int64
	Deduplicated        int64
	LastError           string
}

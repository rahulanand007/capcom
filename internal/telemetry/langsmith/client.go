// Package langsmith imports LangSmith LLM child runs into Capcom's normalized
// telemetry plane.
package langsmith

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
)

const (
	defaultBaseURL = "https://api.smith.langchain.com"
	defaultTimeout = 30 * time.Second
)

type Client struct {
	httpClient  *http.Client
	credentials runtimeadapter.CredentialResolver
	timeout     time.Duration
}

func NewClient(httpClient *http.Client, credentials runtimeadapter.CredentialResolver) Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return Client{httpClient: httpClient, credentials: credentials, timeout: defaultTimeout}
}

func (c Client) TelemetryConfigured(conn domain.RuntimeConnection) bool {
	return connectionSetting(conn, "langsmith_project") != "" &&
		connectionSetting(conn, "langsmith_auth_ref") != ""
}

func (c Client) QueryUsage(ctx context.Context, conn domain.RuntimeConnection, query domain.UsageQuery) ([]domain.UsageObservation, error) {
	if !c.TelemetryConfigured(conn) {
		return nil, fmt.Errorf("langsmith telemetry is not configured")
	}
	baseURL := connectionSetting(conn, "langsmith_api_url")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid langsmith API URL")
	}
	requestBody := map[string]any{
		"project_name": connectionSetting(conn, "langsmith_project"),
		"start_time":   query.From.UTC().Format(time.RFC3339Nano),
		"end_time":     query.To.UTC().Format(time.RFC3339Nano),
		"filter":       `eq(run_type, "llm")`,
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal langsmith query: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, parsed.String()+"/runs/query", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build langsmith query: %w", err)
	}
	if c.credentials == nil {
		return nil, fmt.Errorf("langsmith credential resolver is required")
	}
	token, err := c.credentials.Resolve(requestCtx, connectionSetting(conn, "langsmith_auth_ref"))
	if err != nil {
		return nil, fmt.Errorf("resolve langsmith credential: %w", err)
	}
	req.Header.Set("x-api-key", token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query langsmith runs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("langsmith runs query returned %d: %s", resp.StatusCode, strings.TrimSpace(string(limited)))
	}
	var payload json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode langsmith runs: %w", err)
	}
	runs, err := decodeRuns(payload)
	if err != nil {
		return nil, err
	}
	observations := make([]domain.UsageObservation, 0, len(runs))
	for _, run := range runs {
		observation, ok := normalizeRun(conn.ID, run)
		if ok {
			observations = append(observations, observation)
		}
	}
	return observations, nil
}

type runEnvelope struct {
	Runs []traceRun `json:"runs"`
}

type traceRun struct {
	ID          string         `json:"id"`
	ParentRunID string         `json:"parent_run_id"`
	TraceID     string         `json:"trace_id"`
	RunType     string         `json:"run_type"`
	StartTime   string         `json:"start_time"`
	EndTime     string         `json:"end_time"`
	Error       any            `json:"error"`
	Usage       usageMetadata  `json:"usage_metadata"`
	Extra       map[string]any `json:"extra"`
	Outputs     map[string]any `json:"outputs"`
}

type usageMetadata struct {
	InputTokens         int64        `json:"input_tokens"`
	OutputTokens        int64        `json:"output_tokens"`
	TotalTokens         int64        `json:"total_tokens"`
	CachedInputTokens   int64        `json:"cached_input_tokens"`
	CacheCreationTokens int64        `json:"cache_creation_tokens"`
	ReasoningTokens     int64        `json:"reasoning_tokens"`
	InputCost           decimalValue `json:"input_cost"`
	OutputCost          decimalValue `json:"output_cost"`
	TotalCost           decimalValue `json:"total_cost"`
	Model               string       `json:"model"`
	ModelProvider       string       `json:"model_provider"`
	InputTokenDetails   struct {
		Cached        int64 `json:"cached"`
		CacheRead     int64 `json:"cache_read"`
		CacheCreation int64 `json:"cache_creation"`
	} `json:"input_token_details"`
	OutputTokenDetails struct {
		Reasoning int64 `json:"reasoning"`
	} `json:"output_token_details"`
}

type decimalValue string

func (d *decimalValue) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*d = ""
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*d = decimalValue(strings.TrimSpace(text))
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("decode decimal value: %w", err)
	}
	*d = decimalValue(number.String())
	return nil
}

func decodeRuns(payload []byte) ([]traceRun, error) {
	var runs []traceRun
	if err := json.Unmarshal(payload, &runs); err == nil {
		return runs, nil
	}
	var envelope runEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, fmt.Errorf("decode langsmith run list: %w", err)
	}
	return envelope.Runs, nil
}

func normalizeRun(runtimeID string, run traceRun) (domain.UsageObservation, bool) {
	if run.ID == "" || (run.RunType != "" && run.RunType != "llm") {
		return domain.UsageObservation{}, false
	}
	metadata := nestedMap(run.Extra, "metadata")
	startedAt := parseTime(run.StartTime)
	endedAt := parseTime(run.EndTime)
	observedAt := time.Now().UTC()
	if endedAt != nil {
		observedAt = *endedAt
	} else if startedAt != nil {
		observedAt = *startedAt
	}
	var duration *int64
	if startedAt != nil && endedAt != nil {
		value := endedAt.Sub(*startedAt).Milliseconds()
		duration = &value
	}
	total := run.Usage.TotalTokens
	if total == 0 {
		total = run.Usage.InputTokens + run.Usage.OutputTokens
	}
	errorCount := int64(0)
	if run.Error != nil {
		errorCount = 1
	}
	return domain.UsageObservation{
		Source: domain.UsageSourceLangSmith, SourceObservationID: run.ID,
		RuntimeConnectionID: runtimeID,
		RuntimeAgentID:      firstMetadataString(metadata, "langgraph_assistant_id", "capcom_runtime_agent_id"),
		RuntimeExecutionID:  firstMetadataString(metadata, "langgraph_run_id", "capcom_runtime_execution_id"),
		ParentExecutionID:   run.ParentRunID,
		ThreadID:            firstMetadataString(metadata, "langgraph_thread_id", "thread_id"),
		Model:               run.Usage.Model, Provider: run.Usage.ModelProvider,
		RequestCount: 1, InputTokens: run.Usage.InputTokens, OutputTokens: run.Usage.OutputTokens,
		CachedInputTokens: firstNonzero(
			run.Usage.CachedInputTokens,
			run.Usage.InputTokenDetails.Cached,
			run.Usage.InputTokenDetails.CacheRead,
		),
		CacheCreationTokens: firstNonzero(run.Usage.CacheCreationTokens, run.Usage.InputTokenDetails.CacheCreation),
		ReasoningTokens:     firstNonzero(run.Usage.ReasoningTokens, run.Usage.OutputTokenDetails.Reasoning),
		TotalTokens:         total,
		InputCostUSD:        decimalPointer(run.Usage.InputCost),
		OutputCostUSD:       decimalPointer(run.Usage.OutputCost),
		TotalCostUSD:        decimalPointer(run.Usage.TotalCost),
		DurationMS:          duration, ErrorCount: errorCount, StartedAt: startedAt,
		EndedAt: endedAt, ObservedAt: observedAt,
		Attributes: map[string]any{"trace_id": run.TraceID, "quality": "trace"},
	}, true
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return strings.TrimSpace(value)
}

func connectionSetting(conn domain.RuntimeConnection, key string) string {
	if value := metadataString(conn.Metadata, key); value != "" {
		return value
	}
	return strings.TrimSpace(conn.Labels[key])
}

func nestedMap(value map[string]any, key string) map[string]any {
	nested, _ := value[key].(map[string]any)
	return nested
}

func firstMetadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := metadataString(metadata, key); value != "" {
			return value
		}
	}
	return ""
}

func parseTime(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func decimalPointer(value decimalValue) *domain.Decimal {
	if strings.TrimSpace(string(value)) == "" {
		return nil
	}
	decimal := domain.Decimal(strings.TrimSpace(string(value)))
	return &decimal
}

func firstNonzero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

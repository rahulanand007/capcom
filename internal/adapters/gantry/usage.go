package gantry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"capcom/internal/domain"
)

type gantryUsageEnvelope struct {
	Usage []gantryUsageRecord `json:"usage"`
	Data  []gantryUsageRecord `json:"data"`
}

type gantryUsageRecord struct {
	ID           string          `json:"id"`
	AgentID      string          `json:"agentId"`
	RunID        string          `json:"runId"`
	JobID        string          `json:"jobId"`
	Model        string          `json:"model"`
	Provider     string          `json:"provider"`
	RequestCount int64           `json:"requestCount"`
	InputTokens  int64           `json:"inputTokens"`
	OutputTokens int64           `json:"outputTokens"`
	StartedAt    string          `json:"startedAt"`
	EndedAt      string          `json:"endedAt"`
	Timestamp    string          `json:"timestamp"`
	Raw          json.RawMessage `json:"-"`
}

func (c Client) QueryUsage(ctx context.Context, conn domain.RuntimeConnection, query domain.UsageQuery) ([]domain.UsageObservation, error) {
	values := url.Values{}
	if !query.From.IsZero() {
		values.Set("from", query.From.UTC().Format(time.RFC3339))
	}
	if !query.To.IsZero() {
		values.Set("to", query.To.UTC().Format(time.RFC3339))
	}
	if query.RuntimeAgentID != "" {
		values.Set("agent", query.RuntimeAgentID)
	}
	if query.RuntimeExecutionID != "" {
		values.Set("run", query.RuntimeExecutionID)
	}
	if query.RuntimeJobID != "" {
		values.Set("job", query.RuntimeJobID)
	}
	if query.Model != "" {
		values.Set("model", query.Model)
	}
	if query.Interval > 0 {
		values.Set("interval", strconv.FormatInt(int64(query.Interval/time.Second), 10)+"s")
	}
	if query.GroupingDimension != "" {
		values.Set("groupBy", query.GroupingDimension)
	}
	path := "/v1/usage"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var payload json.RawMessage
	if err := c.doJSON(ctx, conn, http.MethodGet, path, nil, &payload); err != nil {
		return nil, err
	}
	records, err := decodeGantryUsage(payload)
	if err != nil {
		return nil, fmt.Errorf("decode gantry usage: %w", err)
	}
	observations := make([]domain.UsageObservation, 0, len(records))
	for _, record := range records {
		observedAt := firstUsageTime(record.Timestamp, record.EndedAt, record.StartedAt, query.To)
		startedAt := parseUsageTime(record.StartedAt)
		endedAt := parseUsageTime(record.EndedAt)
		sourceID := strings.TrimSpace(record.ID)
		if sourceID == "" {
			sourceID = stableUsageID(record, observedAt)
		}
		executionID := record.RunID
		if executionID == "" {
			executionID = record.JobID
		}
		observations = append(observations, domain.UsageObservation{
			Source: domain.UsageSourceGantryNative, SourceObservationID: sourceID,
			RuntimeConnectionID: conn.ID, RuntimeAgentID: record.AgentID,
			RuntimeExecutionID: executionID, Model: record.Model, Provider: record.Provider,
			RequestCount: record.RequestCount, InputTokens: record.InputTokens,
			OutputTokens: record.OutputTokens, TotalTokens: record.InputTokens + record.OutputTokens,
			StartedAt: startedAt, EndedAt: endedAt, ObservedAt: observedAt,
			Attributes: map[string]any{"job_id": record.JobID, "quality": "native"},
		})
	}
	return observations, nil
}

func decodeGantryUsage(payload []byte) ([]gantryUsageRecord, error) {
	var records []gantryUsageRecord
	if err := json.Unmarshal(payload, &records); err == nil {
		return records, nil
	}
	var envelope gantryUsageEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, err
	}
	if envelope.Usage != nil {
		return envelope.Usage, nil
	}
	return envelope.Data, nil
}

func firstUsageTime(values ...any) time.Time {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if parsed := parseUsageTime(typed); parsed != nil {
				return *parsed
			}
		case time.Time:
			if !typed.IsZero() {
				return typed.UTC()
			}
		}
	}
	return time.Now().UTC()
}

func parseUsageTime(value string) *time.Time {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func stableUsageID(record gantryUsageRecord, observedAt time.Time) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		record.AgentID, record.RunID, record.JobID, record.Model,
		observedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

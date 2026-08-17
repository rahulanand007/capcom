package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"capcom/internal/domain"
	"capcom/internal/services"

	collecttracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

const maxOTLPRequestBytes = 4 << 20

func handleMetricsSummary(cfg RouterConfig) http.HandlerFunc {
	return metricsHandler(cfg, func(*http.Request) (string, string) { return "", "" })
}

func handleAgentMetrics(cfg RouterConfig) http.HandlerFunc {
	return metricsHandler(cfg, func(r *http.Request) (string, string) { return "", r.PathValue("id") })
}

func handleRuntimeMetrics(cfg RouterConfig) http.HandlerFunc {
	return metricsHandler(cfg, func(r *http.Request) (string, string) { return r.PathValue("id"), "" })
}

func metricsHandler(cfg RouterConfig, scope func(*http.Request) (runtimeID, agentID string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Telemetry == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "telemetry_not_configured"})
			return
		}
		query, err := usageQueryFromRequest(r)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		query.RuntimeConnectionID, query.AgentID = scope(r)
		summary, err := cfg.Telemetry.Summary(r.Context(), query)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, services.ErrInvalidUsageQuery) {
				status = http.StatusBadRequest
			}
			writeAPIError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, metricsResponseFromDomain(summary))
	}
}

func handleTelemetryHealth(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Telemetry == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "telemetry_not_configured"})
			return
		}
		run, err := cfg.Telemetry.Health(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		if run.ID == "" {
			writeJSON(w, http.StatusOK, map[string]any{
				"configured": false,
				"status":     "not_configured",
				"message":    "Token telemetry is not configured",
			})
			return
		}
		writeJSON(w, http.StatusOK, telemetryHealthResponseFromDomain(run))
	}
}

func usageQueryFromRequest(r *http.Request) (domain.UsageQuery, error) {
	now := time.Now().UTC()
	query := domain.UsageQuery{From: now.Add(-24 * time.Hour), To: now}
	var err error
	if value := r.URL.Query().Get("from"); value != "" {
		query.From, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return query, fmt.Errorf("from must be RFC3339")
		}
	}
	if value := r.URL.Query().Get("to"); value != "" {
		query.To, err = time.Parse(time.RFC3339, value)
		if err != nil {
			return query, fmt.Errorf("to must be RFC3339")
		}
	}
	if value := r.URL.Query().Get("interval"); value != "" {
		query.Interval, err = parseMetricsInterval(value)
		if err != nil {
			return query, err
		}
	}
	query.Model = strings.TrimSpace(r.URL.Query().Get("model"))
	query.Source = domain.UsageSource(strings.TrimSpace(r.URL.Query().Get("source")))
	query.RuntimeExecutionID = strings.TrimSpace(r.URL.Query().Get("runtime_execution_id"))
	switch query.Source {
	case "", domain.UsageSourceGantryNative, domain.UsageSourceLangSmith, domain.UsageSourceOTEL:
	default:
		return query, fmt.Errorf("source must be gantry_native, langsmith, or otel")
	}
	return query, nil
}

func parseMetricsInterval(value string) (time.Duration, error) {
	aliases := map[string]time.Duration{
		"5m": 5 * time.Minute, "15m": 15 * time.Minute, "1h": time.Hour,
		"6h": 6 * time.Hour, "1d": 24 * time.Hour,
	}
	if interval, ok := aliases[value]; ok {
		return interval, nil
	}
	return 0, fmt.Errorf("interval must be one of 5m, 15m, 1h, 6h, or 1d")
}

type metricsResponse struct {
	AgentID             string                     `json:"agent_id,omitempty"`
	RuntimeConnectionID string                     `json:"runtime_connection_id,omitempty"`
	Period              metricsPeriod              `json:"period"`
	Available           bool                       `json:"available"`
	Configured          bool                       `json:"configured"`
	Status              string                     `json:"status"`
	Source              domain.UsageSource         `json:"source,omitempty"`
	LastObservedAt      *time.Time                 `json:"last_observed_at,omitempty"`
	Usage               metricsUsage               `json:"usage,omitempty"`
	Performance         metricsPerformance         `json:"performance,omitempty"`
	Context             metricsContext             `json:"context,omitempty"`
	Models              []domain.ModelUsageSummary `json:"models,omitempty"`
	TimeSeries          []domain.UsageTimeBucket   `json:"time_series,omitempty"`
}

type metricsPeriod struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type metricsUsage struct {
	Requests            int64           `json:"requests"`
	InputTokens         int64           `json:"input_tokens"`
	OutputTokens        int64           `json:"output_tokens"`
	CachedInputTokens   int64           `json:"cached_input_tokens"`
	CacheCreationTokens int64           `json:"cache_creation_tokens"`
	ReasoningTokens     int64           `json:"reasoning_tokens"`
	TotalTokens         int64           `json:"total_tokens"`
	EstimatedCostUSD    *domain.Decimal `json:"estimated_cost_usd"`
}

type metricsPerformance struct {
	AverageDurationMS *float64 `json:"average_duration_ms"`
	P95DurationMS     *float64 `json:"p95_duration_ms"`
	ErrorRate         *float64 `json:"error_rate"`
}

type metricsContext struct {
	Utilization *float64 `json:"utilization"`
	Quality     string   `json:"quality,omitempty"`
}

func metricsResponseFromDomain(summary domain.UsageSummary) metricsResponse {
	status := "available"
	if !summary.Available {
		status = "not_configured"
		if summary.Configured {
			status = "unavailable"
		}
	}
	quality := ""
	if summary.ContextUtilization != nil {
		quality = "estimated"
	}
	return metricsResponse{
		AgentID: summary.AgentID, RuntimeConnectionID: summary.RuntimeConnectionID,
		Period: metricsPeriod{From: summary.From, To: summary.To}, Available: summary.Available,
		Configured: summary.Configured, Status: status, Source: summary.Source,
		LastObservedAt: summary.LastObservedAt,
		Usage: metricsUsage{
			Requests: summary.RequestCount, InputTokens: summary.InputTokens,
			OutputTokens: summary.OutputTokens, CachedInputTokens: summary.CachedInputTokens,
			CacheCreationTokens: summary.CacheCreationTokens, ReasoningTokens: summary.ReasoningTokens,
			TotalTokens: summary.TotalTokens, EstimatedCostUSD: summary.EstimatedCostUSD,
		},
		Performance: metricsPerformance{
			AverageDurationMS: summary.AverageDurationMS, P95DurationMS: summary.P95DurationMS,
			ErrorRate: summary.ErrorRate,
		},
		Context: metricsContext{Utilization: summary.ContextUtilization, Quality: quality},
		Models:  summary.ModelBreakdown, TimeSeries: summary.TimeSeries,
	}
}

func telemetryHealthResponseFromDomain(run domain.TelemetryIngestionRun) map[string]any {
	response := map[string]any{
		"configured": true, "status": run.Status, "source": run.Source,
		"schema_version": run.SchemaVersion, "started_at": run.StartedAt,
		"finished_at": run.FinishedAt, "last_successful_at": run.LastSuccessfulAt,
		"records_accepted": run.Accepted, "records_rejected": run.Rejected,
		"records_deduplicated": run.Deduplicated,
	}
	if run.LastError != "" {
		detail := publicError(http.StatusServiceUnavailable, run.LastError).Error
		response["error_code"] = detail.Code
		response["message"] = detail.Message
		response["retryable"] = detail.Retryable
	}
	return response
}

// OTLP JSON support intentionally allowlists only usage and correlation
// attributes. Content-bearing attributes never enter UsageObservation.
func handleOTLPTraces(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Telemetry == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "telemetry_not_configured"})
			return
		}
		contentType := strings.ToLower(r.Header.Get("Content-Type"))
		isJSON := strings.Contains(contentType, "json")
		isProtobuf := strings.Contains(contentType, "protobuf") || strings.Contains(contentType, "x-protobuf")
		if !isJSON && !isProtobuf {
			writeJSON(w, http.StatusUnsupportedMediaType, errorResponse{Error: "content type must be OTLP JSON or protobuf"})
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxOTLPRequestBytes))
		if err != nil {
			writeJSON(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "otlp_request_too_large"})
			return
		}
		var observations []domain.UsageObservation
		var rejected int64
		if isProtobuf {
			observations, rejected, err = normalizeOTLPProtobuf(body)
		} else {
			observations, rejected, err = normalizeOTLPJSON(body)
		}
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		accepted, validationRejected, deduplicated, ingestErr := cfg.Telemetry.Ingest(r.Context(), observations)
		rejected += validationRejected
		if ingestErr != nil {
			isRejectedObservation := errors.Is(ingestErr, services.ErrInvalidUsageObservation) ||
				errors.Is(ingestErr, domain.ErrUnknownRuntimeConnection)
			if !isRejectedObservation || accepted == 0 {
				status := http.StatusInternalServerError
				if isRejectedObservation {
					status = http.StatusBadRequest
				}
				writeAPIError(w, status, ingestErr)
				return
			}
		}
		if isProtobuf {
			response := &collecttracev1.ExportTraceServiceResponse{}
			if rejected > 0 {
				response.PartialSuccess = &collecttracev1.ExportTracePartialSuccess{
					RejectedSpans: rejected,
					ErrorMessage:  "spans missing required Capcom correlation or valid usage",
				}
			}
			encoded, marshalErr := proto.Marshal(response)
			if marshalErr != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "encode_otlp_response"})
				return
			}
			w.Header().Set("Content-Type", "application/x-protobuf")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(encoded)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"accepted": accepted, "rejected": rejected, "deduplicated": deduplicated})
	}
}

type otlpRequest struct {
	ResourceSpans []struct {
		Resource struct {
			Attributes []otlpAttribute `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Spans []otlpSpan `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"resourceSpans"`
}

type otlpSpan struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	ParentSpanID      string          `json:"parentSpanId"`
	StartTimeUnixNano json.Number     `json:"startTimeUnixNano"`
	EndTimeUnixNano   json.Number     `json:"endTimeUnixNano"`
	Attributes        []otlpAttribute `json:"attributes"`
	Status            struct {
		Code any `json:"code"`
	} `json:"status"`
}

type otlpAttribute struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string      `json:"stringValue"`
		IntValue    json.Number `json:"intValue"`
		DoubleValue float64     `json:"doubleValue"`
		BoolValue   bool        `json:"boolValue"`
	} `json:"value"`
}

func normalizeOTLPJSON(body []byte) ([]domain.UsageObservation, int64, error) {
	var request otlpRequest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&request); err != nil {
		return nil, 0, fmt.Errorf("decode OTLP JSON: %w", err)
	}
	observations := make([]domain.UsageObservation, 0)
	var rejected int64
	for _, resourceSpans := range request.ResourceSpans {
		resourceAttributes := otlpAttributes(resourceSpans.Resource.Attributes)
		for _, scope := range resourceSpans.ScopeSpans {
			for _, span := range scope.Spans {
				attributes := make(map[string]any, len(resourceAttributes)+len(span.Attributes))
				for key, value := range resourceAttributes {
					attributes[key] = value
				}
				for key, value := range otlpAttributes(span.Attributes) {
					attributes[key] = value
				}
				runtimeID := stringAttribute(attributes, "capcom.runtime_connection.id")
				if !validCorrelationValue(runtimeID) || !validCorrelationValue(span.SpanID) {
					rejected++
					continue
				}
				input := intAttribute(attributes, "gen_ai.usage.input_tokens")
				output := intAttribute(attributes, "gen_ai.usage.output_tokens")
				total := intAttribute(attributes, "gen_ai.usage.total_tokens")
				if total == 0 {
					total = input + output
				}
				startedAt := unixNanoTime(span.StartTimeUnixNano)
				endedAt := unixNanoTime(span.EndTimeUnixNano)
				var duration *int64
				if startedAt != nil && endedAt != nil {
					value := endedAt.Sub(*startedAt).Milliseconds()
					duration = &value
				}
				observedAt := time.Now().UTC()
				if endedAt != nil {
					observedAt = *endedAt
				}
				errorCount := int64(0)
				if fmt.Sprint(span.Status.Code) == "2" || strings.EqualFold(fmt.Sprint(span.Status.Code), "STATUS_CODE_ERROR") {
					errorCount = 1
				}
				observations = append(observations, domain.UsageObservation{
					Source: domain.UsageSourceOTEL, SourceObservationID: span.SpanID,
					RuntimeConnectionID: runtimeID,
					RuntimeAgentID:      stringAttribute(attributes, "capcom.runtime_agent.id"),
					RuntimeExecutionID:  stringAttribute(attributes, "capcom.runtime_execution.id"),
					ParentExecutionID:   span.ParentSpanID,
					ThreadID:            stringAttribute(attributes, "gen_ai.conversation.id"),
					Model:               firstStringAttribute(attributes, "gen_ai.response.model", "gen_ai.request.model"),
					Provider:            stringAttribute(attributes, "gen_ai.system"),
					RequestCount:        1, InputTokens: input, OutputTokens: output, TotalTokens: total,
					CachedInputTokens: intAttribute(attributes, "gen_ai.usage.details.cached_input_tokens"),
					ReasoningTokens:   intAttribute(attributes, "gen_ai.usage.details.reasoning_tokens"),
					DurationMS:        duration, ErrorCount: errorCount, StartedAt: startedAt,
					EndedAt: endedAt, ObservedAt: observedAt,
					Attributes: allowlistedOTLPAttributes(attributes, span.TraceID),
				})
			}
		}
	}
	return observations, rejected, nil
}

func normalizeOTLPProtobuf(body []byte) ([]domain.UsageObservation, int64, error) {
	var request collecttracev1.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &request); err != nil {
		return nil, 0, fmt.Errorf("decode OTLP protobuf: %w", err)
	}
	observations := make([]domain.UsageObservation, 0)
	var rejected int64
	for _, resourceSpans := range request.ResourceSpans {
		resourceAttributes := map[string]any{}
		if resourceSpans.Resource != nil {
			resourceAttributes = protobufAttributes(resourceSpans.Resource.Attributes)
		}
		for _, scope := range resourceSpans.ScopeSpans {
			for _, span := range scope.Spans {
				attributes := make(map[string]any, len(resourceAttributes)+len(span.Attributes))
				for key, value := range resourceAttributes {
					attributes[key] = value
				}
				for key, value := range protobufAttributes(span.Attributes) {
					attributes[key] = value
				}
				runtimeID := stringAttribute(attributes, "capcom.runtime_connection.id")
				spanID := hex.EncodeToString(span.SpanId)
				if !validCorrelationValue(runtimeID) || !validCorrelationValue(spanID) {
					rejected++
					continue
				}
				input := intAttribute(attributes, "gen_ai.usage.input_tokens")
				output := intAttribute(attributes, "gen_ai.usage.output_tokens")
				total := intAttribute(attributes, "gen_ai.usage.total_tokens")
				if total == 0 {
					total = input + output
				}
				startedAt := uint64UnixNanoTime(span.StartTimeUnixNano)
				endedAt := uint64UnixNanoTime(span.EndTimeUnixNano)
				var duration *int64
				if startedAt != nil && endedAt != nil {
					value := endedAt.Sub(*startedAt).Milliseconds()
					duration = &value
				}
				observedAt := time.Now().UTC()
				if endedAt != nil {
					observedAt = *endedAt
				}
				errorCount := int64(0)
				if span.Status != nil && span.Status.Code == tracev1.Status_STATUS_CODE_ERROR {
					errorCount = 1
				}
				observations = append(observations, domain.UsageObservation{
					Source: domain.UsageSourceOTEL, SourceObservationID: spanID,
					RuntimeConnectionID: runtimeID,
					RuntimeAgentID:      stringAttribute(attributes, "capcom.runtime_agent.id"),
					RuntimeExecutionID:  stringAttribute(attributes, "capcom.runtime_execution.id"),
					ParentExecutionID:   hex.EncodeToString(span.ParentSpanId),
					ThreadID:            stringAttribute(attributes, "gen_ai.conversation.id"),
					Model:               firstStringAttribute(attributes, "gen_ai.response.model", "gen_ai.request.model"),
					Provider:            stringAttribute(attributes, "gen_ai.system"),
					RequestCount:        1, InputTokens: input, OutputTokens: output, TotalTokens: total,
					CachedInputTokens: intAttribute(attributes, "gen_ai.usage.details.cached_input_tokens"),
					ReasoningTokens:   intAttribute(attributes, "gen_ai.usage.details.reasoning_tokens"),
					DurationMS:        duration, ErrorCount: errorCount, StartedAt: startedAt,
					EndedAt: endedAt, ObservedAt: observedAt,
					Attributes: allowlistedOTLPAttributes(attributes, hex.EncodeToString(span.TraceId)),
				})
			}
		}
	}
	return observations, rejected, nil
}

func protobufAttributes(attributes []*commonv1.KeyValue) map[string]any {
	result := make(map[string]any, len(attributes))
	for _, attribute := range attributes {
		if attribute == nil || attribute.Value == nil {
			continue
		}
		switch value := attribute.Value.Value.(type) {
		case *commonv1.AnyValue_StringValue:
			result[attribute.Key] = value.StringValue
		case *commonv1.AnyValue_IntValue:
			result[attribute.Key] = value.IntValue
		case *commonv1.AnyValue_DoubleValue:
			result[attribute.Key] = value.DoubleValue
		case *commonv1.AnyValue_BoolValue:
			result[attribute.Key] = value.BoolValue
		}
	}
	return result
}

func otlpAttributes(attributes []otlpAttribute) map[string]any {
	result := make(map[string]any, len(attributes))
	for _, attribute := range attributes {
		switch {
		case attribute.Value.StringValue != "":
			result[attribute.Key] = attribute.Value.StringValue
		case attribute.Value.IntValue != "":
			value, _ := strconv.ParseInt(string(attribute.Value.IntValue), 10, 64)
			result[attribute.Key] = value
		case attribute.Value.DoubleValue != 0:
			result[attribute.Key] = attribute.Value.DoubleValue
		default:
			result[attribute.Key] = attribute.Value.BoolValue
		}
	}
	return result
}

func stringAttribute(attributes map[string]any, key string) string {
	value, _ := attributes[key].(string)
	return strings.TrimSpace(value)
}

func firstStringAttribute(attributes map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringAttribute(attributes, key); value != "" {
			return value
		}
	}
	return ""
}

func intAttribute(attributes map[string]any, key string) int64 {
	switch value := attributes[key].(type) {
	case int64:
		return value
	case float64:
		return int64(value)
	case string:
		parsed, _ := strconv.ParseInt(value, 10, 64)
		return parsed
	default:
		return 0
	}
}

func unixNanoTime(value json.Number) *time.Time {
	if value == "" {
		return nil
	}
	nanos, err := strconv.ParseInt(string(value), 10, 64)
	if err != nil {
		return nil
	}
	parsed := time.Unix(0, nanos).UTC()
	return &parsed
}

func uint64UnixNanoTime(value uint64) *time.Time {
	if value == 0 || value > uint64(^uint64(0)>>1) {
		return nil
	}
	parsed := time.Unix(0, int64(value)).UTC()
	return &parsed
}

func validCorrelationValue(value string) bool {
	length := len(strings.TrimSpace(value))
	return length > 0 && length <= 512
}

func allowlistedOTLPAttributes(attributes map[string]any, traceID string) map[string]any {
	result := map[string]any{"trace_id": traceID, "normalization_schema_version": "otel-genai-v1"}
	for _, key := range []string{"server.address", "service.name", "gen_ai.operation.name"} {
		if value, ok := attributes[key]; ok {
			if text, isText := value.(string); !isText || len(text) <= 1024 {
				result[key] = value
			}
		}
	}
	return result
}

package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"capcom/internal/domain"

	collecttracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type telemetryServiceStub struct{}

func (telemetryServiceStub) Summary(_ context.Context, query domain.UsageQuery) (domain.UsageSummary, error) {
	return domain.UsageSummary{
		From: query.From, To: query.To, AgentID: query.AgentID,
		Available: true, Configured: true, Source: domain.UsageSourceOTEL,
		RequestCount: 2, TotalTokens: 42,
	}, nil
}
func (telemetryServiceStub) Health(context.Context, string) (domain.TelemetryIngestionRun, error) {
	return domain.TelemetryIngestionRun{}, nil
}
func (telemetryServiceStub) Ingest(context.Context, []domain.UsageObservation) (int64, int64, int64, error) {
	return 0, 0, 0, nil
}

func TestAgentMetricsEndpoint(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version: "test", AdminToken: "test-admin-token", Telemetry: telemetryServiceStub{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(
		http.MethodGet,
		"/v1/agents/agent-1/metrics?from=2026-07-28T00:00:00Z&to=2026-07-29T00:00:00Z&interval=1h",
		nil,
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestMetricsEndpointRejectsInvalidSource(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version: "test", AdminToken: "test-admin-token", Telemetry: telemetryServiceStub{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/v1/metrics/summary?source=unknown", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestTelemetryHealthSanitizesCollectorError(t *testing.T) {
	response := telemetryHealthResponseFromDomain(domain.TelemetryIngestionRun{
		Status:    domain.TelemetryRunFailed,
		LastError: `gantry GET http://internal:8787/v1/usage returned 403: API key is missing required scope usage:read`,
	})
	if response["error_code"] != "TELEMETRY_PERMISSION_REQUIRED" {
		t.Fatalf("error_code = %v", response["error_code"])
	}
	if response["message"] != "The runtime credential needs the usage:read permission." {
		t.Fatalf("message = %v", response["message"])
	}
	if _, exposed := response["last_error"]; exposed {
		t.Fatal("technical collector error was exposed")
	}
}

func TestNormalizeOTLPJSON(t *testing.T) {
	body := []byte(`{
	  "resourceSpans": [{
	    "resource": {"attributes": [
	      {"key":"service.name","value":{"stringValue":"agent-service"}},
	      {"key":"capcom.runtime_connection.id","value":{"stringValue":"runtime-1"}}
	    ]},
	    "scopeSpans": [{"spans": [{
	      "traceId":"trace-1","spanId":"span-1",
	      "startTimeUnixNano":"1000000000","endTimeUnixNano":"3000000000",
	      "attributes":[
	        {"key":"capcom.runtime_agent.id","value":{"stringValue":"agent-1"}},
	        {"key":"gen_ai.request.model","value":{"stringValue":"gpt-test"}},
	        {"key":"gen_ai.usage.input_tokens","value":{"intValue":"20"}},
	        {"key":"gen_ai.usage.output_tokens","value":{"intValue":"5"}},
	        {"key":"gen_ai.prompt","value":{"stringValue":"must not persist"}}
	      ]
	    }]}]
	  }]
	}`)
	observations, rejected, err := normalizeOTLPJSON(body)
	if err != nil || rejected != 0 || len(observations) != 1 {
		t.Fatalf("normalizeOTLPJSON() = %d observations, %d rejected, %v", len(observations), rejected, err)
	}
	got := observations[0]
	if got.TotalTokens != 25 || got.RuntimeConnectionID != "runtime-1" || got.DurationMS == nil || *got.DurationMS != 2000 {
		t.Fatalf("unexpected observation: %#v", got)
	}
	if _, ok := got.Attributes["gen_ai.prompt"]; ok {
		t.Fatal("prompt attribute was persisted")
	}
}

func TestNormalizeOTLPJSONRequiresCorrelation(t *testing.T) {
	body := []byte(`{"resourceSpans":[{"scopeSpans":[{"spans":[{"spanId":"span-1"}]}]}]}`)
	observations, rejected, err := normalizeOTLPJSON(body)
	if err != nil || len(observations) != 0 || rejected != 1 {
		t.Fatalf("normalizeOTLPJSON() = %d observations, %d rejected, %v", len(observations), rejected, err)
	}
}

func TestNormalizeOTLPProtobuf(t *testing.T) {
	request := &collecttracev1.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{
				Key: "capcom.runtime_connection.id",
				Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{
					StringValue: "runtime-1",
				}},
			}}},
			ScopeSpans: []*tracev1.ScopeSpans{{Spans: []*tracev1.Span{{
				TraceId: []byte{1, 2, 3}, SpanId: []byte{4, 5, 6},
				StartTimeUnixNano: 1_000_000_000, EndTimeUnixNano: 2_500_000_000,
				Attributes: []*commonv1.KeyValue{
					protobufStringAttribute("gen_ai.request.model", "model-1"),
					protobufIntAttribute("gen_ai.usage.input_tokens", 12),
					protobufIntAttribute("gen_ai.usage.output_tokens", 3),
				},
			}}}},
		}},
	}
	body, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	observations, rejected, err := normalizeOTLPProtobuf(body)
	if err != nil || rejected != 0 || len(observations) != 1 {
		t.Fatalf("normalizeOTLPProtobuf() = %d observations, %d rejected, %v", len(observations), rejected, err)
	}
	if observations[0].TotalTokens != 15 || observations[0].DurationMS == nil || *observations[0].DurationMS != 1500 {
		t.Fatalf("unexpected observation: %#v", observations[0])
	}
}

func protobufStringAttribute(key, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key: key, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}},
	}
}

func protobufIntAttribute(key string, value int64) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key: key, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: value}},
	}
}

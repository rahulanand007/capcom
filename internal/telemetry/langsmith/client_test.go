package langsmith

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"capcom/internal/domain"
)

type credentials map[string]string

func (c credentials) Resolve(_ context.Context, ref string) (string, error) { return c[ref], nil }

func TestQueryUsageNormalizesLangSmithRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runs/query" || r.Header.Get("x-api-key") != "token" {
			t.Fatalf("unexpected request")
		}
		_, _ = w.Write([]byte(`{"runs":[{
		  "id":"run-uuid","run_type":"llm","trace_id":"trace-1",
		  "start_time":"2026-07-29T10:00:00Z","end_time":"2026-07-29T10:00:02Z",
		  "usage_metadata":{"input_tokens":20,"output_tokens":5,"total_tokens":25,"model":"model-1","total_cost":0.0125,"input_token_details":{"cached":4},"output_token_details":{"reasoning":2}},
		  "extra":{"metadata":{"langgraph_assistant_id":"assistant-1","langgraph_thread_id":"thread-1","langgraph_run_id":"server-run-1"}}
		}]}`))
	}))
	defer server.Close()
	conn := domain.RuntimeConnection{
		ID: "runtime-1",
		Metadata: map[string]any{
			"langsmith_project": "project", "langsmith_auth_ref": "smith-key",
			"langsmith_api_url": server.URL,
		},
	}
	got, err := NewClient(server.Client(), credentials{"smith-key": "token"}).QueryUsage(
		context.Background(), conn, domain.UsageQuery{From: time.Now().Add(-time.Hour), To: time.Now()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RuntimeAgentID != "assistant-1" || got[0].TotalTokens != 25 ||
		got[0].CachedInputTokens != 4 || got[0].ReasoningTokens != 2 ||
		got[0].TotalCostUSD == nil || string(*got[0].TotalCostUSD) != "0.0125" {
		t.Fatalf("QueryUsage() = %#v", got)
	}
}

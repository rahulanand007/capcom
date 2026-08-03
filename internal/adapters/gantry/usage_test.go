package gantry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"capcom/internal/domain"
)

func TestQueryUsageNormalizesNativeUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/usage" || r.URL.Query().Get("agent") != "agent:main" {
			t.Fatalf("unexpected request URL %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":[{
		  "id":"usage-1","agentId":"agent:main","runId":"run-1","model":"model-1",
		  "requestCount":2,"inputTokens":100,"outputTokens":20,
		  "timestamp":"2026-07-29T10:00:00Z"
		}]}`))
	}))
	defer server.Close()

	client := NewClient(server.Client(), staticCredentialResolver{"key": "token"})
	got, err := client.QueryUsage(context.Background(), domain.RuntimeConnection{
		ID: "runtime-1", BaseURL: server.URL, AuthRef: "key",
	}, domain.UsageQuery{
		From:           time.Date(2026, 7, 29, 9, 55, 0, 0, time.UTC),
		To:             time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		RuntimeAgentID: "agent:main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TotalTokens != 120 || got[0].Source != domain.UsageSourceGantryNative {
		t.Fatalf("QueryUsage() = %#v", got)
	}
}

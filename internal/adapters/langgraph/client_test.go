package langgraph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"capcom/internal/domain"
)

type staticCredentialResolver map[string]string

func (r staticCredentialResolver) Resolve(_ context.Context, ref string) (string, error) {
	return r[ref], nil
}

func TestCheckUsesAPIKeyAndReturnsCapabilities(t *testing.T) {
	server := newFixtureServer(t)
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"langgraph-key": "test-key"}).Check(
		context.Background(), domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "langgraph-key"},
	)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if got.Status != domain.RuntimeStatusActive || !got.Capabilities.ReadAgents || !got.Capabilities.ReadExecutions {
		t.Fatalf("unexpected check result: %#v", got)
	}
	if got.Capabilities.ReadAgentAccess || got.Capabilities.ReplaceAgentAccess {
		t.Fatalf("unsupported capabilities reported: %#v", got.Capabilities)
	}
	if got.Capabilities.DeleteAgent || got.Capabilities.CancelExecution {
		t.Fatalf("read-only connection reported controls: %#v", got.Capabilities)
	}
}

func TestCheckReportsControlCapabilitiesForControlEnabledConnection(t *testing.T) {
	server := newFixtureServer(t)
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"langgraph-key": "test-key"}).Check(
		context.Background(),
		domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "langgraph-key", Mode: domain.RuntimeModeControlEnabled},
	)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !got.Capabilities.DeleteAgent || !got.Capabilities.CancelExecution {
		t.Fatalf("missing control capabilities: %#v", got.Capabilities)
	}
}

func TestCollectSnapshotNormalizesAssistantsThreadsAndRuns(t *testing.T) {
	server := newFixtureServer(t)
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"langgraph-key": "test-key"}).CollectSnapshot(
		context.Background(), domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "langgraph-key"},
	)
	if err != nil {
		t.Fatalf("CollectSnapshot returned error: %v", err)
	}
	if len(got.Agents) != 2 || got.Agents[0].Agent.Name != "Support Agent" || got.Agents[1].Agent.Name != "triage" {
		t.Fatalf("unexpected agents: %#v", got.Agents)
	}
	if got.Agents[0].Access.Source != "" {
		t.Fatalf("unsupported access should not be persisted: %#v", got.Agents[0].Access)
	}
	if len(got.Executions) != 2 {
		t.Fatalf("executions = %d, want 2", len(got.Executions))
	}
	if got.Executions[0].Kind != "thread" || got.Executions[1].Kind != "run" || got.Executions[1].RuntimeAgentID == "" {
		t.Fatalf("unexpected executions: %#v", got.Executions)
	}
	if got.Executions[1].EndedAt == nil {
		t.Fatal("completed run has no ended_at")
	}
}

func TestReplaceAgentAccessIsUnsupported(t *testing.T) {
	_, err := NewClient(nil, staticCredentialResolver{}).ReplaceAgentAccess(
		context.Background(), domain.RuntimeConnection{}, "agent", domain.AccessDocument{},
	)
	if err == nil || !strings.Contains(err.Error(), "does not expose access replacement") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteAgentCallsAssistantEndpoint(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	err := NewClient(server.Client(), staticCredentialResolver{"key": "test-key"}).DeleteAgent(
		context.Background(),
		domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "key"},
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
	)
	if err != nil {
		t.Fatalf("DeleteAgent returned error: %v", err)
	}
	if method != http.MethodDelete || path != "/assistants/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("request = %s %s", method, path)
	}
}

func TestCancelExecutionInterruptsRunAndWaits(t *testing.T) {
	var method, path, query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, query = r.Method, r.URL.Path, r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := NewClient(server.Client(), staticCredentialResolver{"key": "test-key"}).CancelExecution(
		context.Background(),
		domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "key"},
		domain.RuntimeExecutionSnapshot{
			RuntimeExecutionID:       "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			ParentRuntimeExecutionID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Kind:                     "run",
		},
	)
	if err != nil {
		t.Fatalf("CancelExecution returned error: %v", err)
	}
	if method != http.MethodPost ||
		path != "/threads/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/runs/bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb/cancel" ||
		query != "action=interrupt&wait=true" {
		t.Fatalf("request = %s %s?%s", method, path, query)
	}
}

func TestCheckReportsUnauthorizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"detail":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	_, err := NewClient(server.Client(), staticCredentialResolver{"key": "bad"}).Check(
		context.Background(), domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "key"},
	)
	if err == nil || !strings.Contains(err.Error(), "returned 401") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			http.Error(w, "missing api key", http.StatusUnauthorized)
			return
		}
		var file string
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/ok":
			file = "ok.json"
		case r.Method == http.MethodGet && r.URL.Path == "/info":
			file = "info.json"
		case r.Method == http.MethodPost && r.URL.Path == "/assistants/search":
			file = "assistants.json"
		case r.Method == http.MethodPost && r.URL.Path == "/threads/search":
			file = "threads.json"
		case r.Method == http.MethodGet && r.URL.Path == "/threads/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/runs":
			file = "runs.json"
		default:
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request: %v", err)
			}
			if body["limit"] != float64(pageSize) || body["offset"] != float64(0) {
				t.Errorf("unexpected pagination: %#v", body)
			}
		}
		data, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Errorf("read fixture: %v", err)
			http.Error(w, "fixture error", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
}

package gantry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
)

func TestClientImplementsRuntimeAdapter(t *testing.T) {
	var _ runtimeadapter.Adapter = Client{}
}

func TestCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			writeTestJSON(t, w, map[string]any{"status": "ok"})
		case "/v1/doctor":
			writeTestJSON(t, w, map[string]any{"status": "ok", "checks": []map[string]any{{"id": "storage", "status": "ok", "message": "ready"}}})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).Check(context.Background(), domain.RuntimeConnection{
		BaseURL: server.URL,
		Mode:    domain.RuntimeModeReadOnly,
		AuthRef: "gantry-key",
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if got.Status != domain.RuntimeStatusActive {
		t.Fatalf("status = %q, want active", got.Status)
	}
	if !got.Capabilities.ReadAgents || !got.Capabilities.ReadAgentAccess {
		t.Fatalf("read capabilities were not set: %#v", got.Capabilities)
	}
	if !got.Capabilities.ReadAgentDelegates {
		t.Fatal("Gantry should advertise durable delegate relationships")
	}
	if got.Capabilities.ReplaceAgentAccess {
		t.Fatal("read-only connection should not support replace access")
	}
	if !got.Capabilities.ReadDiagnostics || !got.Capabilities.ReadInventory || !got.Capabilities.ReadCapabilityCatalog {
		t.Fatalf("catalog capabilities were not set: %#v", got.Capabilities)
	}
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].CheckID != "storage" {
		t.Fatalf("diagnostics = %#v", got.Diagnostics)
	}
}

func TestListAgents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents" {
			t.Fatalf("path = %q, want /v1/agents", r.URL.Path)
		}
		writeTestJSON(t, w, []map[string]any{
			{"id": "agent:main", "name": "main", "status": "active"},
		})
	}))
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).ListAgents(context.Background(), domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "gantry-key"})
	if err != nil {
		t.Fatalf("ListAgents returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("agent count = %d, want 1", len(got))
	}
	if got[0].RuntimeAgentID != "agent:main" {
		t.Fatalf("agent id = %q", got[0].RuntimeAgentID)
	}
	if got[0].Status != domain.AgentStatusEnabled {
		t.Fatalf("agent status = %q", got[0].Status)
	}
	if got[0].Kind != domain.AgentKindRegistered {
		t.Fatalf("agent kind = %q, want registered", got[0].Kind)
	}
}

func TestListAgentsAcceptsEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTestJSON(t, w, map[string]any{
			"agents": []map[string]any{{"id": "agent:main", "name": "main", "status": "active"}},
		})
	}))
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).ListAgents(
		context.Background(), domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "gantry-key"},
	)
	if err != nil {
		t.Fatalf("ListAgents returned error: %v", err)
	}
	if len(got) != 1 || got[0].RuntimeAgentID != "agent:main" {
		t.Fatalf("agents = %#v", got)
	}
}

func TestListAgentDelegates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents/agent:main/delegates" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		writeTestJSON(t, w, map[string]any{
			"agentId": "agent:main", "revision": 12,
			"delegates": []string{"reviewer", "missing"},
			"resolved": []map[string]any{
				{"ref": "reviewer", "agentId": "agent:reviewer", "toolName": "delegate_reviewer", "displayName": "Reviewer", "persona": "developer"},
				{"ref": "agent:incident", "agentId": "agent:incident", "toolName": "delegate_incident", "displayName": "Incident", "persona": "operations"},
			},
		})
	}))
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).ListAgentDelegates(
		context.Background(), domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "gantry-key"}, "agent:main",
	)
	if err != nil {
		t.Fatalf("ListAgentDelegates returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("delegations = %#v", got)
	}
	if !got[0].Configured || !got[0].Resolved || got[0].DelegateRuntimeAgentID != "agent:reviewer" {
		t.Fatalf("configured delegation = %#v", got[0])
	}
	if got[1].Configured || !got[1].Resolved {
		t.Fatalf("conversation-bound delegation = %#v", got[1])
	}
	if !got[2].Configured || got[2].Resolved || got[2].DelegateRef != "missing" || got[2].DelegateRuntimeAgentID != "agent:missing" {
		t.Fatalf("unresolved delegation = %#v", got[2])
	}
}

func TestListAgentSkills(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/agents/agent:main_agent/skills":
			writeTestJSON(t, w, map[string]any{"bindings": []map[string]any{{
				"id": "binding-1", "agentId": "agent:main_agent", "skillId": "skill:research",
				"status": "active", "configVersionId": "config-2",
			}}})
		case "/v1/skills":
			writeTestJSON(t, w, map[string]any{"skills": []map[string]any{{
				"id": "skill:research", "name": "Research", "description": "Search trusted sources",
				"source": "bundled", "status": "installed", "toolIds": []string{"tool:web"},
				"workflowRefs": []string{"workflow:report"},
			}}})
		default:
			t.Fatalf("path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).ListAgentSkills(
		context.Background(), domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "gantry-key"}, "agent:main_agent",
	)
	if err != nil {
		t.Fatalf("ListAgentSkills returned error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Research" || got[0].Description != "Search trusted sources" || len(got[0].ToolIDs) != 1 {
		t.Fatalf("skills = %#v", got)
	}
}

func TestGetAgentAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents/agent:main/access" {
			t.Fatalf("path = %q, want /v1/agents/agent:main/access", r.URL.Path)
		}
		writeTestJSON(t, w, map[string]any{
			"agentId": "agent:main",
			"selections": []map[string]any{
				{"id": "browser.use", "version": "builtin"},
			},
		})
	}))
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).GetAgentAccess(context.Background(), domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "gantry-key"}, "agent:main")
	if err != nil {
		t.Fatalf("GetAgentAccess returned error: %v", err)
	}
	if got.AgentID != "agent:main" {
		t.Fatalf("agent id = %q", got.AgentID)
	}
	if len(got.Selections) != 1 || got.Selections[0].ID != "browser.use" {
		t.Fatalf("selections = %#v", got.Selections)
	}
	if got.Raw["agentId"] != "agent:main" {
		t.Fatalf("raw payload not preserved: %#v", got.Raw)
	}
}

func TestReplaceAgentAccessPreservesSources(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents/agent:main/access" {
			t.Fatalf("path = %q, want /v1/agents/agent:main/access", r.URL.Path)
		}
		methods = append(methods, r.Method)
		switch r.Method {
		case http.MethodGet:
			writeTestJSON(t, w, map[string]any{
				"agentId": "agent:main",
				"sources": map[string]any{
					"skills":     []map[string]any{{"id": "skill:research"}},
					"mcpServers": []map[string]any{},
					"tools":      []map[string]any{{"id": "tool:browser"}},
				},
				"selections": []map[string]any{{"id": "browser.use", "version": "builtin"}},
			})
		case http.MethodPut:
			var body struct {
				Sources struct {
					Skills     []map[string]any `json:"skills"`
					MCPServers []map[string]any `json:"mcpServers"`
					Tools      []map[string]any `json:"tools"`
				} `json:"sources"`
				Selections []gantrySelection `json:"selections"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if len(body.Sources.Skills) != 1 || body.Sources.Skills[0]["id"] != "skill:research" {
				t.Fatalf("skill sources = %#v", body.Sources.Skills)
			}
			if len(body.Sources.Tools) != 1 || body.Sources.Tools[0]["id"] != "tool:browser" {
				t.Fatalf("tool sources = %#v", body.Sources.Tools)
			}
			if len(body.Selections) != 1 || body.Selections[0].ID != "filesystem.read" || body.Selections[0].Version != "catalog" {
				t.Fatalf("selections = %#v", body.Selections)
			}
			writeTestJSON(t, w, map[string]any{
				"agentId":    "agent:main",
				"sources":    body.Sources,
				"selections": body.Selections,
			})
		default:
			t.Fatalf("method = %q", r.Method)
		}
	}))
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).ReplaceAgentAccess(
		context.Background(),
		domain.RuntimeConnection{
			BaseURL: server.URL,
			AuthRef: "gantry-key",
			Mode:    domain.RuntimeModeControlEnabled,
		},
		"agent:main",
		domain.AccessDocument{Selections: []domain.AccessSelection{{
			ID:         "filesystem.read",
			Attributes: map[string]any{"version": "catalog"},
		}}},
	)
	if err != nil {
		t.Fatalf("ReplaceAgentAccess returned error: %v", err)
	}
	if len(methods) != 2 || methods[0] != http.MethodGet || methods[1] != http.MethodPut {
		t.Fatalf("methods = %#v, want GET then PUT", methods)
	}
	if len(got.Selections) != 1 || got.Selections[0].ID != "filesystem.read" {
		t.Fatalf("selections = %#v", got.Selections)
	}
}

func TestReplaceAgentAccessRejectsMissingSources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{
			"agentId":    "agent:main",
			"selections": []map[string]any{},
		})
	}))
	defer server.Close()

	_, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).ReplaceAgentAccess(
		context.Background(),
		domain.RuntimeConnection{
			BaseURL: server.URL,
			AuthRef: "gantry-key",
			Mode:    domain.RuntimeModeControlEnabled,
		},
		"agent:main",
		domain.AccessDocument{},
	)
	if err == nil || !strings.Contains(err.Error(), "does not contain sources") {
		t.Fatalf("error = %v, want missing sources error", err)
	}
}

func TestCollectSnapshotReturnsCompleteNormalizedState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			writeTestJSON(t, w, map[string]any{"status": "ok"})
		case "/v1/doctor":
			writeTestJSON(t, w, map[string]any{"status": "ok", "checks": []map[string]any{{"id": "storage", "status": "ok"}}})
		case "/v1/inventory":
			writeTestJSON(t, w, map[string]any{"inventory": map[string]any{
				"tools":      []map[string]any{{"id": "tool:browser", "displayName": "Browser", "status": "active"}},
				"skills":     []map[string]any{{"id": "skill:research", "name": "Research Brief", "status": "installed"}},
				"mcpServers": []map[string]any{{"id": "mcp:github", "displayName": "GitHub", "status": "active"}},
			}})
		case "/v1/capabilities":
			writeTestJSON(t, w, map[string]any{"capabilities": []map[string]any{{
				"id": "browser.use", "version": "builtin", "displayName": "Browser", "category": "Browser", "risk": "write",
			}}})
		case "/v1/agents":
			writeTestJSON(t, w, []map[string]any{{"id": "agent:main_agent", "name": "gantry", "status": "active"}})
		case "/v1/agents/agent:main_agent/skills":
			writeTestJSON(t, w, map[string]any{"bindings": []map[string]any{{"skillId": "skill:research", "status": "active"}}})
		case "/v1/skills":
			writeTestJSON(t, w, map[string]any{"skills": []map[string]any{{"id": "skill:research", "name": "Research Brief"}}})
		case "/v1/agents/agent:main_agent/access":
			writeTestJSON(t, w, map[string]any{"agentId": "agent:main_agent", "selections": []map[string]any{{"id": "browser.use"}}})
		case "/v1/agents/agent:main_agent/delegates":
			writeTestJSON(t, w, map[string]any{"agentId": "agent:main_agent", "revision": 3, "delegates": []string{"reviewer"}, "resolved": []map[string]any{{"ref": "reviewer", "agentId": "agent:reviewer", "toolName": "delegate_reviewer", "displayName": "Reviewer", "persona": "developer"}}})
		case "/v1/runs":
			writeTestJSON(t, w, map[string]any{"runs": []any{}})
		case "/v1/jobs":
			writeTestJSON(t, w, map[string]any{"jobs": []any{}})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	got, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).CollectSnapshot(
		context.Background(), domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "gantry-key"},
	)
	if err != nil {
		t.Fatalf("CollectSnapshot returned error: %v", err)
	}
	if len(got.Agents) != 1 || got.Agents[0].Agent.Kind != domain.AgentKindMain {
		t.Fatalf("agents = %#v", got.Agents)
	}
	if len(got.Agents[0].Skills) != 1 || got.Agents[0].Skills[0].Name != "Research Brief" {
		t.Fatalf("skills = %#v", got.Agents[0].Skills)
	}
	if len(got.Agents[0].Access.Selections) != 1 {
		t.Fatalf("access = %#v", got.Agents[0].Access)
	}
	if len(got.Diagnostics) != 1 || len(got.Inventory) != 3 || len(got.CapabilityCatalog) != 1 {
		t.Fatalf("runtime catalog snapshot = diagnostics:%d inventory:%d capabilities:%d", len(got.Diagnostics), len(got.Inventory), len(got.CapabilityCatalog))
	}
	if len(got.AgentDelegations) != 1 || got.AgentDelegations[0].DelegateRuntimeAgentID != "agent:reviewer" {
		t.Fatalf("delegations = %#v", got.AgentDelegations)
	}
}

func TestSetAgentStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/agents/agent:main_agent" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["status"] != "disabled" {
			t.Fatalf("body = %#v", body)
		}
		writeTestJSON(t, w, map[string]any{"id": "agent:main_agent", "name": "Main", "status": "disabled"})
	}))
	defer server.Close()
	got, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).SetAgentStatus(
		context.Background(), domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "gantry-key", Mode: domain.RuntimeModeControlEnabled},
		"agent:main_agent", domain.AgentStatusDisabled,
	)
	if err != nil {
		t.Fatalf("SetAgentStatus returned error: %v", err)
	}
	if got.Status != domain.AgentStatusDisabled {
		t.Fatalf("status = %q", got.Status)
	}
}

func TestNormalizeDelegatedExecutions(t *testing.T) {
	now := time.Now().UTC()
	events := []gantryRunEvent{
		{ID: "event-1", CreatedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{"taskId": "task-1", "taskKind": "delegated_agent", "subagentType": "researcher", "description": "Investigate"}},
		{ID: "event-2", CreatedAt: now.Add(time.Second).Format(time.RFC3339Nano), Payload: map[string]any{"taskId": "task-1", "status": "completed", "summary": "Done"}},
		{ID: "event-3", CreatedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{"taskId": "command-1", "taskKind": "async_command"}},
	}
	events[0].Metadata.RuntimeEventType = "task.started"
	events[1].Metadata.RuntimeEventType = "task.notification"
	got := normalizeDelegatedExecutions(gantryRun{RunID: "run-1", JobID: "job-1"}, "agent:main_agent", events, now)
	if len(got) != 1 || got[0].RuntimeExecutionID != "task-1" || got[0].RuntimeAgentID != "agent:main_agent" || got[0].Status != "completed" {
		t.Fatalf("executions = %#v", got)
	}
}

func TestClientSendsBearerCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		if r.URL.Path == "/v1/doctor" {
			writeTestJSON(t, w, map[string]any{"status": "ok", "checks": []any{}})
			return
		}
		writeTestJSON(t, w, map[string]any{"status": "ok"})
	}))
	defer server.Close()

	_, err := NewClient(server.Client(), staticCredentialResolver{"gantry-key": "test-token"}).Check(
		context.Background(),
		domain.RuntimeConnection{BaseURL: server.URL, AuthRef: "gantry-key", Mode: domain.RuntimeModeReadOnly},
	)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
}

type staticCredentialResolver map[string]string

func (r staticCredentialResolver) Resolve(_ context.Context, ref string) (string, error) {
	return r[ref], nil
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

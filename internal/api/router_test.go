package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
	"capcom/internal/services"
)

func TestHealthz(t *testing.T) {
	router := NewRouter(RouterConfig{Version: "test"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if contentType := rec.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", contentType)
	}

	var got healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	want := healthResponse{
		Status:  "ok",
		Service: "capcom",
		Version: "test",
	}
	if got != want {
		t.Fatalf("response = %#v, want %#v", got, want)
	}
}

func TestConsoleIsServedWithoutAdminToken(t *testing.T) {
	router := NewRouter(RouterConfig{Version: "test", AdminToken: "test-admin-token"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("Capcom Console")) {
		t.Fatalf("response did not contain console document")
	}
}

func TestNotFound(t *testing.T) {
	router := NewRouter(RouterConfig{Version: "test", AdminToken: "test-admin-token"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCORSPreflightBypassesAuth(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version:        "test",
		AdminToken:     "test-admin-token",
		AllowedOrigins: []string{"http://localhost:3000"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodOptions, "/v1/runtime-instances", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "http://localhost:3000" {
		t.Fatalf("allow-origin = %q, want http://localhost:3000", origin)
	}
	if methods := rec.Header().Get("Access-Control-Allow-Methods"); methods == "" {
		t.Fatalf("allow-methods header missing")
	}
}

func TestCORSAllowOriginOnDisallowedOrigin(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version:        "test",
		AdminToken:     "test-admin-token",
		AllowedOrigins: []string{"http://localhost:3000"},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if origin := rec.Header().Get("Access-Control-Allow-Origin"); origin != "" {
		t.Fatalf("allow-origin = %q, want empty for disallowed origin", origin)
	}
}

func TestCreateRuntimeConnection(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version:            "test",
		AdminToken:         "test-admin-token",
		RuntimeConnections: fakeRuntimeConnectionService{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := bytes.NewBufferString(`{
		"name":"local-gantry",
		"display_name":"Gantry Development",
		"environment":"development",
		"labels":{"team":"platform"},
		"runtime_type":"gantry",
		"mode":"read_only",
		"endpoint":"http://127.0.0.1:3000",
		"auth_ref":"gantry-key",
		"actor":"test",
		"reason":"setup"
	}`)
	req := authenticatedRequest(http.MethodPost, "/v1/runtime-connections", body)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got runtimeConnectionResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != "runtime-1" {
		t.Fatalf("id = %q, want runtime-1", got.ID)
	}
	if got.DisplayName != "Gantry Development" || got.Environment != "development" {
		t.Fatalf("instance identity = %#v", got)
	}
}

func TestUpdateRuntimeInstanceSettings(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version:            "test",
		AdminToken:         "test-admin-token",
		RuntimeConnections: fakeRuntimeConnectionService{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	body := bytes.NewBufferString(`{
		"display_name":"Gantry Staging",
		"environment":"staging",
		"labels":{"team":"platform"},
		"mode":"control_enabled",
		"endpoint":"http://127.0.0.1:8788",
		"auth_ref":"gantry-key",
		"description":"Staging control runtime",
		"sync_enabled":true,
		"sync_interval_seconds":90,
		"actor":"test",
		"reason":"update adapter settings"
	}`)
	req := authenticatedRequest(http.MethodPatch, "/v1/runtime-instances/runtime-1", body)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got runtimeConnectionResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Name != "local-gantry" || got.DisplayName != "Gantry Staging" || got.Environment != "staging" {
		t.Fatalf("instance settings = %#v", got)
	}
	if got.Mode != "control_enabled" || got.Endpoint != "http://127.0.0.1:8788" || got.SyncIntervalSeconds != 90 {
		t.Fatalf("instance settings = %#v", got)
	}
}

func TestRuntimeInstanceLiveAgentsAlias(t *testing.T) {
	router := NewRouter(RouterConfig{Version: "test", AdminToken: "test-admin-token", RuntimeConnections: fakeRuntimeConnectionService{}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/v1/runtime-instances/runtime-1/live/agents", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRuntimeInstancePersistedAgentsAreScoped(t *testing.T) {
	syncService := &fakeRuntimeSyncService{}
	router := NewRouter(RouterConfig{Version: "test", AdminToken: "test-admin-token", RuntimeSync: syncService}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/v1/runtime-instances/runtime-staging/agents", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if syncService.runtimeID != "runtime-staging" {
		t.Fatalf("runtime id = %q", syncService.runtimeID)
	}
}

func TestAgentDelegationsAreAgentScoped(t *testing.T) {
	syncService := &fakeRuntimeSyncService{}
	router := NewRouter(RouterConfig{Version: "test", AdminToken: "test-admin-token", RuntimeSync: syncService}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/v1/agents/agent-1/delegations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if syncService.agentID != "agent-1" {
		t.Fatalf("agent id = %q", syncService.agentID)
	}
	var got []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["delegate_runtime_agent_id"] != "agent:reviewer" {
		t.Fatalf("response = %#v", got)
	}
}

func TestRuntimeCatalogEndpointsRequireSyncService(t *testing.T) {
	router := NewRouter(RouterConfig{Version: "test", AdminToken: "test-admin-token"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	paths := []string{
		"/v1/runtime-instances/runtime-1/diagnostics",
		"/v1/runtime-instances/runtime-1/inventory",
		"/v1/runtime-instances/runtime-1/capabilities",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := authenticatedRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
			}
		})
	}
}

func TestCreateSecretDoesNotReturnValue(t *testing.T) {
	router := NewRouter(RouterConfig{Version: "test", AdminToken: "test-admin-token", Secrets: fakeSecretService{}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodPost, "/v1/secrets", bytes.NewBufferString(`{
		"name":"gantry-key","value":"top-secret","actor":"tester","reason":"setup"
	}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("top-secret")) || bytes.Contains(rec.Body.Bytes(), []byte("value")) {
		t.Fatalf("response exposed secret material: %s", rec.Body.String())
	}
}

func TestRuntimeConnectionEndpointsRequireDatabase(t *testing.T) {
	router := NewRouter(RouterConfig{Version: "test", AdminToken: "test-admin-token"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/v1/runtime-connections", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestListRuntimeConnections(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version:            "test",
		AdminToken:         "test-admin-token",
		RuntimeConnections: fakeRuntimeConnectionService{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/v1/runtime-connections", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []runtimeConnectionResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("response length = %d, want 1", len(got))
	}
}

func TestGetRuntimeConnection(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version:            "test",
		AdminToken:         "test-admin-token",
		RuntimeConnections: fakeRuntimeConnectionService{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/v1/runtime-connections/runtime-1", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestTestRuntimeConnection(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version:            "test",
		AdminToken:         "test-admin-token",
		RuntimeConnections: fakeRuntimeConnectionService{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodPost, "/v1/runtime-connections/runtime-1/test", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got runtimeConnectionTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Status != string(domain.RuntimeStatusActive) {
		t.Fatalf("status = %q, want active", got.Status)
	}
}

func TestListRuntimeAgents(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version: "test", AdminToken: "test-admin-token",
		RuntimeConnections: fakeRuntimeConnectionService{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/v1/runtime-connections/runtime-1/agents", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []runtimeAgentResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].RuntimeAgentID != "agent-1" {
		t.Fatalf("response = %#v", got)
	}
}

func TestGetRuntimeAgentAccess(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version: "test", AdminToken: "test-admin-token",
		RuntimeConnections: fakeRuntimeConnectionService{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/v1/runtime-connections/runtime-1/agents/agent-1/access", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got runtimeAgentAccessResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AgentID != "agent-1" || len(got.Selections) != 1 {
		t.Fatalf("response = %#v", got)
	}
}

func TestListRuntimeAgentSkills(t *testing.T) {
	router := NewRouter(RouterConfig{
		Version: "test", AdminToken: "test-admin-token",
		RuntimeConnections: fakeRuntimeConnectionService{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := authenticatedRequest(http.MethodGet, "/v1/runtime-connections/runtime-1/agents/agent-1/skills", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []runtimeAgentSkillResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].RuntimeSkillID != "skill-1" {
		t.Fatalf("response = %#v", got)
	}
}

func TestProtectedEndpointRequiresAdminToken(t *testing.T) {
	router := NewRouter(RouterConfig{Version: "test", AdminToken: "test-admin-token"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/v1/runtime-connections", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func authenticatedRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Authorization", "Bearer test-admin-token")
	return req
}

type fakeRuntimeConnectionService struct{}

type fakeRuntimeSyncService struct {
	runtimeID string
	agentID   string
}

func (*fakeRuntimeSyncService) Sync(context.Context, services.SyncRuntimeInput) (domain.RuntimeSyncRun, error) {
	return domain.RuntimeSyncRun{}, nil
}
func (*fakeRuntimeSyncService) ListRuns(context.Context, string, int) ([]domain.RuntimeSyncRun, error) {
	return nil, nil
}
func (*fakeRuntimeSyncService) GetRun(context.Context, string, string) (domain.RuntimeSyncRun, error) {
	return domain.RuntimeSyncRun{}, nil
}
func (s *fakeRuntimeSyncService) ListAgents(_ context.Context, runtimeID string) ([]domain.PersistedAgent, error) {
	s.runtimeID = runtimeID
	return []domain.PersistedAgent{{RuntimeConnectionID: runtimeID, RuntimeAgentID: "agent:main_agent", Agent: domain.Agent{Name: "Main"}}}, nil
}
func (*fakeRuntimeSyncService) GetAgent(context.Context, string) (domain.PersistedAgentDetail, error) {
	return domain.PersistedAgentDetail{}, nil
}
func (*fakeRuntimeSyncService) ListSubagentExecutions(context.Context, string, string) ([]domain.PersistedSubagentExecution, error) {
	return nil, nil
}

func (*fakeRuntimeSyncService) ListRuntimeExecutions(context.Context, string, string, string, int) ([]domain.PersistedRuntimeExecution, error) {
	return nil, nil
}
func (*fakeRuntimeSyncService) ListRuntimeDiagnostics(context.Context, string) ([]domain.PersistedRuntimeDiagnostic, error) {
	return nil, nil
}
func (*fakeRuntimeSyncService) ListRuntimeInventory(context.Context, string, string) ([]domain.PersistedRuntimeInventory, error) {
	return nil, nil
}
func (*fakeRuntimeSyncService) ListRuntimeCapabilities(context.Context, string) ([]domain.PersistedRuntimeCapability, error) {
	return nil, nil
}
func (s *fakeRuntimeSyncService) ListAgentDelegations(_ context.Context, runtimeID, agentID string) ([]domain.PersistedAgentDelegation, error) {
	s.runtimeID = runtimeID
	s.agentID = agentID
	return []domain.PersistedAgentDelegation{{RuntimeConnectionID: "runtime-1", AgentDelegationSnapshot: domain.AgentDelegationSnapshot{
		OrchestratorRuntimeAgentID: "agent:main", DelegateRuntimeAgentID: "agent:reviewer",
		DelegateRef: "reviewer", DisplayName: "Reviewer", Configured: true, Resolved: true,
	}}}, nil
}

func (fakeRuntimeConnectionService) Create(_ context.Context, input services.CreateRuntimeConnectionInput) (domain.RuntimeConnection, error) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	return domain.RuntimeConnection{
		ID:          "runtime-1",
		Name:        input.Name,
		DisplayName: input.DisplayName,
		Environment: input.Environment,
		Labels:      input.Labels,
		Kind:        input.Kind,
		Mode:        input.Mode,
		Status:      domain.RuntimeStatusPending,
		BaseURL:     input.Endpoint,
		AuthRef:     input.AuthRef,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (fakeRuntimeConnectionService) UpdateSettings(_ context.Context, input services.UpdateRuntimeInstanceSettingsInput) (domain.RuntimeConnection, error) {
	conn, err := fakeRuntimeConnectionService{}.Get(context.Background(), input.ID)
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	conn.DisplayName = input.DisplayName
	conn.Environment = input.Environment
	conn.Labels = input.Labels
	conn.Mode = input.Mode
	conn.BaseURL = input.Endpoint
	conn.AuthRef = input.AuthRef
	conn.SyncEnabled = input.SyncEnabled
	conn.SyncIntervalSeconds = input.SyncIntervalSeconds
	conn.Metadata = map[string]any{"description": input.Description}
	return conn, nil
}

func (fakeRuntimeConnectionService) Remove(context.Context, services.RemoveRuntimeInstanceInput) error {
	return nil
}

func (fakeRuntimeConnectionService) ConsolidateEndpoint(_ context.Context, input services.ConsolidateRuntimeEndpointInput) (domain.RuntimeConnection, error) {
	conn, err := fakeRuntimeConnectionService{}.Get(context.Background(), input.CanonicalID)
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	conn.Endpoints = []domain.RuntimeEndpoint{{
		ID: "endpoint-1", RuntimeConnectionID: conn.ID, URL: conn.BaseURL,
		Kind: domain.RuntimeEndpointCanonical, CreatedAt: conn.CreatedAt, UpdatedAt: conn.UpdatedAt,
	}}
	return conn, nil
}

type fakeSecretService struct{}

func (fakeSecretService) Create(_ context.Context, input services.StoreSecretInput) (domain.Secret, error) {
	return testSecret(input.Name), nil
}

func (fakeSecretService) Rotate(_ context.Context, input services.StoreSecretInput) (domain.Secret, error) {
	return testSecret(input.Name), nil
}

func testSecret(name string) domain.Secret {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	return domain.Secret{ID: "secret-1", Name: name, CreatedAt: now, UpdatedAt: now}
}

func (fakeRuntimeConnectionService) Get(context.Context, string) (domain.RuntimeConnection, error) {
	now := time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC)
	return domain.RuntimeConnection{
		ID:          "runtime-1",
		Name:        "local-gantry",
		DisplayName: "Gantry Development",
		Environment: "development",
		Labels:      map[string]string{"team": "platform"},
		Kind:        domain.RuntimeKindGantry,
		Mode:        domain.RuntimeModeReadOnly,
		Status:      domain.RuntimeStatusPending,
		BaseURL:     "http://127.0.0.1:3000",
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (fakeRuntimeConnectionService) List(context.Context) ([]domain.RuntimeConnection, error) {
	conn, err := fakeRuntimeConnectionService{}.Get(context.Background(), "runtime-1")
	if err != nil {
		return nil, err
	}
	return []domain.RuntimeConnection{conn}, nil
}

func (fakeRuntimeConnectionService) Test(context.Context, string) (*runtimeadapter.CheckResult, error) {
	return &runtimeadapter.CheckResult{
		Status:  domain.RuntimeStatusActive,
		Message: "ok",
		Capabilities: runtimeadapter.Capabilities{
			ReadAgents:      true,
			ReadAgentAccess: true,
		},
	}, nil
}

func (fakeRuntimeConnectionService) ListAgents(context.Context, string) ([]domain.AgentSnapshot, error) {
	return []domain.AgentSnapshot{{
		RuntimeAgentID: "agent-1", Kind: domain.AgentKindRegistered, Name: "Research agent", Status: domain.AgentStatusEnabled,
		ObservedAt: time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC),
	}}, nil
}

func (fakeRuntimeConnectionService) ListAgentSkills(context.Context, string, string) ([]domain.AgentSkillSnapshot, error) {
	return []domain.AgentSkillSnapshot{{
		RuntimeSkillID: "skill-1", Name: "Research", Status: "active",
		ObservedAt: time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC),
	}}, nil
}

func (fakeRuntimeConnectionService) GetAgentAccess(context.Context, string, string) (*domain.AccessDocument, error) {
	return &domain.AccessDocument{
		AgentID: "agent-1", Source: "gantry",
		ObservedAt: time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC),
		Selections: []domain.AccessSelection{{Kind: "capability", ID: "web", Name: "web", Allowed: true}},
	}, nil
}

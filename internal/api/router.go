package api

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
	"capcom/internal/services"
)

type RouterConfig struct {
	Version            string
	AdminToken         string
	AllowedOrigins     []string
	RuntimeConnections RuntimeConnectionService
	Secrets            SecretService
	RuntimeSync        RuntimeSyncService
	ControlActions     ControlActionService
}

type ControlActionService interface {
	ReconcileAccess(ctx context.Context, input services.ReconcileAccessInput) (domain.ControlAction, error)
	SetAgentStatus(ctx context.Context, input services.SetAgentStatusInput) (domain.ControlAction, error)
	DeleteAgent(ctx context.Context, input services.DeleteAgentInput) (domain.ControlAction, error)
	CancelExecution(ctx context.Context, input services.CancelExecutionInput) (domain.ControlAction, error)
}

type RuntimeSyncService interface {
	Sync(ctx context.Context, input services.SyncRuntimeInput) (domain.RuntimeSyncRun, error)
	ListRuns(ctx context.Context, runtimeID string, limit int) ([]domain.RuntimeSyncRun, error)
	GetRun(ctx context.Context, runtimeID, runID string) (domain.RuntimeSyncRun, error)
	ListAgents(ctx context.Context, runtimeID string) ([]domain.PersistedAgent, error)
	GetAgent(ctx context.Context, agentID string) (domain.PersistedAgentDetail, error)
	ListSubagentExecutions(ctx context.Context, runtimeID, agentID string) ([]domain.PersistedSubagentExecution, error)
	ListRuntimeExecutions(ctx context.Context, runtimeID, agentID, kind string, limit int) ([]domain.PersistedRuntimeExecution, error)
	ListRuntimeDiagnostics(ctx context.Context, runtimeID string) ([]domain.PersistedRuntimeDiagnostic, error)
	ListRuntimeInventory(ctx context.Context, runtimeID, kind string) ([]domain.PersistedRuntimeInventory, error)
	ListRuntimeCapabilities(ctx context.Context, runtimeID string) ([]domain.PersistedRuntimeCapability, error)
	ListAgentDelegations(ctx context.Context, runtimeID, agentID string) ([]domain.PersistedAgentDelegation, error)
}

type RuntimeConnectionService interface {
	Create(ctx context.Context, input services.CreateRuntimeConnectionInput) (domain.RuntimeConnection, error)
	UpdateIdentity(ctx context.Context, input services.UpdateRuntimeInstanceIdentityInput) (domain.RuntimeConnection, error)
	Get(ctx context.Context, id string) (domain.RuntimeConnection, error)
	List(ctx context.Context) ([]domain.RuntimeConnection, error)
	Test(ctx context.Context, id string) (*runtimeadapter.CheckResult, error)
	ListAgents(ctx context.Context, id string) ([]domain.AgentSnapshot, error)
	ListAgentSkills(ctx context.Context, id string, runtimeAgentID string) ([]domain.AgentSkillSnapshot, error)
	GetAgentAccess(ctx context.Context, id string, runtimeAgentID string) (*domain.AccessDocument, error)
}

//go:embed ui/*
var uiFiles embed.FS

type SecretService interface {
	Create(ctx context.Context, input services.StoreSecretInput) (domain.Secret, error)
	Rotate(ctx context.Context, input services.StoreSecretInput) (domain.Secret, error)
}

func NewRouter(cfg RouterConfig, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	ui, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		panic("load embedded console: " + err.Error())
	}
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(ui))))
	mux.HandleFunc("GET /{$}", serveConsole(ui))
	mux.HandleFunc("GET /healthz", handleHealth(cfg))
	mux.HandleFunc("POST /v1/secrets", handleCreateSecret(cfg))
	mux.HandleFunc("PUT /v1/secrets/{name}", handleRotateSecret(cfg))
	mux.HandleFunc("POST /v1/runtime-connections", handleCreateRuntimeConnection(cfg))
	mux.HandleFunc("GET /v1/runtime-connections", handleListRuntimeConnections(cfg))
	mux.HandleFunc("GET /v1/runtime-connections/{id}", handleGetRuntimeConnection(cfg))
	mux.HandleFunc("PATCH /v1/runtime-connections/{id}", handleUpdateRuntimeInstanceIdentity(cfg))
	mux.HandleFunc("POST /v1/runtime-connections/{id}/test", handleTestRuntimeConnection(cfg))
	mux.HandleFunc("GET /v1/runtime-connections/{id}/agents", handleListRuntimeAgents(cfg))
	mux.HandleFunc("GET /v1/runtime-connections/{id}/agents/{agentID}/skills", handleListRuntimeAgentSkills(cfg))
	mux.HandleFunc("GET /v1/runtime-connections/{id}/agents/{agentID}/access", handleGetRuntimeAgentAccess(cfg))
	mux.HandleFunc("POST /v1/runtime-connections/{id}/sync", handleSyncRuntime(cfg))
	mux.HandleFunc("GET /v1/runtime-connections/{id}/sync-runs", handleListSyncRuns(cfg))
	mux.HandleFunc("GET /v1/runtime-connections/{id}/sync-runs/{runID}", handleGetSyncRun(cfg))
	mux.HandleFunc("GET /v1/agents", handleListPersistedAgents(cfg))
	mux.HandleFunc("GET /v1/agents/{id}", handleGetPersistedAgent(cfg))
	mux.HandleFunc("GET /v1/agents/{id}/skills", handleGetPersistedAgentSkills(cfg))
	mux.HandleFunc("GET /v1/agents/{id}/access", handleGetPersistedAgentAccess(cfg))
	mux.HandleFunc("GET /v1/agents/{id}/delegations", handleListAgentDelegations(cfg))
	mux.HandleFunc("GET /v1/subagent-executions", handleListSubagentExecutions(cfg))
	mux.HandleFunc("GET /v1/runtime-executions", handleListRuntimeExecutions(cfg))
	// Runtime instances are the user-facing connection boundary. The older
	// runtime-connections routes remain available for compatibility.
	mux.HandleFunc("POST /v1/runtime-instances", handleCreateRuntimeConnection(cfg))
	mux.HandleFunc("GET /v1/runtime-instances", handleListRuntimeConnections(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}", handleGetRuntimeConnection(cfg))
	mux.HandleFunc("PATCH /v1/runtime-instances/{id}", handleUpdateRuntimeInstanceIdentity(cfg))
	mux.HandleFunc("POST /v1/runtime-instances/{id}/test", handleTestRuntimeConnection(cfg))
	mux.HandleFunc("POST /v1/runtime-instances/{id}/sync", handleSyncRuntime(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/sync-runs", handleListSyncRuns(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/agents", handleListInstanceAgents(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/subagent-executions", handleListInstanceSubagentExecutions(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/executions", handleListInstanceRuntimeExecutions(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/diagnostics", handleListRuntimeDiagnostics(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/inventory", handleListRuntimeInventory(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/capabilities", handleListRuntimeCapabilities(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/agent-delegations", handleListInstanceAgentDelegations(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/live/agents", handleListRuntimeAgents(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/live/agents/{agentID}/skills", handleListRuntimeAgentSkills(cfg))
	mux.HandleFunc("GET /v1/runtime-instances/{id}/live/agents/{agentID}/access", handleGetRuntimeAgentAccess(cfg))
	mux.HandleFunc("POST /v1/agents/{id}/actions/reconcile-access", handleReconcileAgentAccess(cfg))
	mux.HandleFunc("POST /v1/agents/{id}/actions/set-status", handleSetAgentStatus(cfg))
	mux.HandleFunc("POST /v1/agents/{id}/actions/delete", handleDeleteAgent(cfg))
	mux.HandleFunc("POST /v1/runtime-executions/{id}/actions/cancel", handleCancelExecution(cfg))
	mux.HandleFunc("/", handleNotFound)

	return requestLogger(corsMiddleware(adminAuth(mux, cfg.AdminToken), cfg.AllowedOrigins), logger)
}

func serveConsole(ui fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		content, err := fs.ReadFile(ui, "index.html")
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "console_unavailable"})
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(content)
	}
}

func handleCreateSecret(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Secrets == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "secret_storage_not_configured"})
			return
		}
		var req storeSecretRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_json"})
			return
		}
		secret, err := cfg.Secrets.Create(r.Context(), services.StoreSecretInput{
			Name: req.Name, Value: req.Value, Actor: req.Actor, Reason: req.Reason,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, secretResponseFromDomain(secret))
	}
}

func handleRotateSecret(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Secrets == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "secret_storage_not_configured"})
			return
		}
		var req storeSecretRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_json"})
			return
		}
		secret, err := cfg.Secrets.Rotate(r.Context(), services.StoreSecretInput{
			Name: r.PathValue("name"), Value: req.Value, Actor: req.Actor, Reason: req.Reason,
		})
		if err != nil {
			if errors.Is(err, services.ErrSecretNotFound) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "not_found"})
				return
			}
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, secretResponseFromDomain(secret))
	}
}

func handleHealth(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{
			Status:  "ok",
			Service: "capcom",
			Version: cfg.Version,
		})
	}
}

func handleNotFound(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusNotFound, errorResponse{
		Error: "not_found",
	})
}

func handleCreateRuntimeConnection(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeConnections == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "database_not_configured"})
			return
		}

		var req createRuntimeConnectionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_json"})
			return
		}

		conn, err := cfg.RuntimeConnections.Create(r.Context(), services.CreateRuntimeConnectionInput{
			Name:        req.Name,
			DisplayName: req.DisplayName,
			Environment: req.Environment,
			Labels:      req.Labels,
			Kind:        domain.RuntimeKind(req.RuntimeType),
			Mode:        domain.RuntimeMode(req.Mode),
			Endpoint:    req.Endpoint,
			Actor:       req.Actor,
			Reason:      req.Reason,
			Description: req.Description,
			AuthRef:     req.AuthRef,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, runtimeConnectionResponseFromDomain(conn))
	}
}

func handleListRuntimeConnections(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeConnections == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "database_not_configured"})
			return
		}

		conns, err := cfg.RuntimeConnections.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "list_runtime_connections_failed"})
			return
		}

		response := make([]runtimeConnectionResponse, 0, len(conns))
		for _, conn := range conns {
			response = append(response, runtimeConnectionResponseFromDomain(conn))
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func handleGetRuntimeConnection(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeConnections == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "database_not_configured"})
			return
		}

		conn, err := cfg.RuntimeConnections.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "not_found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "get_runtime_connection_failed"})
			return
		}

		writeJSON(w, http.StatusOK, runtimeConnectionResponseFromDomain(conn))
	}
}

func handleUpdateRuntimeInstanceIdentity(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeConnections == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "database_not_configured"})
			return
		}
		var req updateRuntimeInstanceIdentityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_json"})
			return
		}
		conn, err := cfg.RuntimeConnections.UpdateIdentity(r.Context(), services.UpdateRuntimeInstanceIdentityInput{ID: r.PathValue("id"), DisplayName: req.DisplayName, Environment: req.Environment, Labels: req.Labels, Actor: req.Actor, Reason: req.Reason})
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, sql.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, runtimeConnectionResponseFromDomain(conn))
	}
}

func handleTestRuntimeConnection(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeConnections == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "database_not_configured"})
			return
		}

		result, err := cfg.RuntimeConnections.Test(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "not_found"})
				return
			}
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, runtimeConnectionTestResponse{
			Status:       string(result.Status),
			Message:      result.Message,
			Capabilities: result.Capabilities,
			Metadata:     result.Metadata,
		})
	}
}

func handleListRuntimeAgents(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeConnections == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "database_not_configured"})
			return
		}
		agents, err := cfg.RuntimeConnections.ListAgents(r.Context(), r.PathValue("id"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "not_found"})
				return
			}
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
			return
		}
		response := make([]runtimeAgentResponse, 0, len(agents))
		for _, agent := range agents {
			response = append(response, runtimeAgentResponseFromDomain(agent))
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func handleGetRuntimeAgentAccess(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeConnections == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "database_not_configured"})
			return
		}
		access, err := cfg.RuntimeConnections.GetAgentAccess(r.Context(), r.PathValue("id"), r.PathValue("agentID"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "not_found"})
				return
			}
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, runtimeAgentAccessResponseFromDomain(*access))
	}
}

func handleListRuntimeAgentSkills(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeConnections == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "database_not_configured"})
			return
		}
		skills, err := cfg.RuntimeConnections.ListAgentSkills(r.Context(), r.PathValue("id"), r.PathValue("agentID"))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "not_found"})
				return
			}
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
			return
		}
		response := make([]runtimeAgentSkillResponse, 0, len(skills))
		for _, skill := range skills {
			response = append(response, runtimeAgentSkillResponseFromDomain(skill))
		}
		writeJSON(w, http.StatusOK, response)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Default().Error("write json response", "error", err)
	}
}

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type createRuntimeConnectionRequest struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Environment string            `json:"environment"`
	Labels      map[string]string `json:"labels"`
	RuntimeType string            `json:"runtime_type"`
	Mode        string            `json:"mode"`
	Endpoint    string            `json:"endpoint"`
	Actor       string            `json:"actor"`
	Reason      string            `json:"reason"`
	Description string            `json:"description"`
	AuthRef     string            `json:"auth_ref"`
}

type updateRuntimeInstanceIdentityRequest struct {
	DisplayName string            `json:"display_name"`
	Environment string            `json:"environment"`
	Labels      map[string]string `json:"labels"`
	Actor       string            `json:"actor"`
	Reason      string            `json:"reason"`
}

type storeSecretRequest struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

type secretResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type runtimeConnectionResponse struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	DisplayName         string            `json:"display_name"`
	Environment         string            `json:"environment"`
	Labels              map[string]string `json:"labels"`
	Description         string            `json:"description,omitempty"`
	RuntimeType         string            `json:"runtime_type"`
	Mode                string            `json:"mode"`
	Status              string            `json:"status"`
	Endpoint            string            `json:"endpoint"`
	AuthRef             string            `json:"auth_ref"`
	LastSyncedAt        *string           `json:"last_synced_at,omitempty"`
	CreatedAt           string            `json:"created_at"`
	UpdatedAt           string            `json:"updated_at"`
	SyncEnabled         bool              `json:"sync_enabled"`
	SyncIntervalSeconds int               `json:"sync_interval_seconds"`
	LastSyncStatus      string            `json:"last_sync_status,omitempty"`
	LastSyncStartedAt   *string           `json:"last_sync_started_at,omitempty"`
	LastSyncFinishedAt  *string           `json:"last_sync_finished_at,omitempty"`
	LastSyncDurationMS  int64             `json:"last_sync_duration_ms,omitempty"`
	LastError           string            `json:"last_error,omitempty"`
}

type runtimeConnectionTestResponse struct {
	Status       string                      `json:"status"`
	Message      string                      `json:"message"`
	Capabilities runtimeadapter.Capabilities `json:"capabilities"`
	Metadata     map[string]any              `json:"metadata,omitempty"`
}

type runtimeAgentResponse struct {
	RuntimeAgentID       string         `json:"runtime_agent_id"`
	ParentRuntimeAgentID string         `json:"parent_runtime_agent_id,omitempty"`
	Kind                 string         `json:"kind"`
	Name                 string         `json:"name"`
	Status               string         `json:"status"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	ObservedAt           string         `json:"observed_at"`
}

type runtimeAgentSkillResponse struct {
	RuntimeSkillID string         `json:"runtime_skill_id"`
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	Source         string         `json:"source,omitempty"`
	Status         string         `json:"status"`
	Version        string         `json:"version,omitempty"`
	ToolIDs        []string       `json:"tool_ids"`
	WorkflowRefs   []string       `json:"workflow_refs"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	ObservedAt     string         `json:"observed_at"`
}

type runtimeAgentAccessResponse struct {
	AgentID    string                   `json:"agent_id"`
	Selections []runtimeAccessSelection `json:"selections"`
	ObservedAt string                   `json:"observed_at"`
	Source     string                   `json:"source"`
}

type runtimeAccessSelection struct {
	Kind       string         `json:"kind"`
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Allowed    bool           `json:"allowed"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

func runtimeConnectionResponseFromDomain(conn domain.RuntimeConnection) runtimeConnectionResponse {
	var lastSyncedAt *string
	if conn.LastSyncedAt != nil {
		formatted := conn.LastSyncedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
		lastSyncedAt = &formatted
	}
	var lastSyncStartedAt, lastSyncFinishedAt *string
	if conn.LastSyncStartedAt != nil {
		value := conn.LastSyncStartedAt.UTC().Format(time.RFC3339)
		lastSyncStartedAt = &value
	}
	if conn.LastSyncFinishedAt != nil {
		value := conn.LastSyncFinishedAt.UTC().Format(time.RFC3339)
		lastSyncFinishedAt = &value
	}

	return runtimeConnectionResponse{
		ID:                  conn.ID,
		Name:                conn.Name,
		DisplayName:         conn.DisplayName,
		Environment:         conn.Environment,
		Labels:              conn.Labels,
		Description:         mapValue(conn.Metadata, "description"),
		RuntimeType:         string(conn.Kind),
		Mode:                string(conn.Mode),
		Status:              string(conn.Status),
		Endpoint:            conn.BaseURL,
		AuthRef:             conn.AuthRef,
		LastSyncedAt:        lastSyncedAt,
		CreatedAt:           conn.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:           conn.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		SyncEnabled:         conn.SyncEnabled,
		SyncIntervalSeconds: conn.SyncIntervalSeconds,
		LastSyncStatus:      string(conn.LastSyncStatus),
		LastSyncStartedAt:   lastSyncStartedAt,
		LastSyncFinishedAt:  lastSyncFinishedAt,
		LastSyncDurationMS:  conn.LastSyncDurationMS,
		LastError:           conn.LastError,
	}
}

func mapValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}

func secretResponseFromDomain(secret domain.Secret) secretResponse {
	return secretResponse{
		ID:        secret.ID,
		Name:      secret.Name,
		CreatedAt: secret.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: secret.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

func runtimeAgentResponseFromDomain(agent domain.AgentSnapshot) runtimeAgentResponse {
	return runtimeAgentResponse{
		RuntimeAgentID: agent.RuntimeAgentID, ParentRuntimeAgentID: agent.ParentRuntimeAgentID,
		Kind: string(agent.Kind), Name: agent.Name, Status: string(agent.Status),
		Metadata: agent.Metadata, ObservedAt: agent.ObservedAt.UTC().Format(time.RFC3339),
	}
}

func runtimeAgentSkillResponseFromDomain(skill domain.AgentSkillSnapshot) runtimeAgentSkillResponse {
	return runtimeAgentSkillResponse{
		RuntimeSkillID: skill.RuntimeSkillID, Name: skill.Name, Status: skill.Status,
		Description: skill.Description, Source: skill.Source,
		Version: skill.Version, ToolIDs: skill.ToolIDs, WorkflowRefs: skill.WorkflowRefs, Metadata: skill.Metadata,
		ObservedAt: skill.ObservedAt.UTC().Format(time.RFC3339),
	}
}

func runtimeAgentAccessResponseFromDomain(access domain.AccessDocument) runtimeAgentAccessResponse {
	selections := make([]runtimeAccessSelection, 0, len(access.Selections))
	for _, selection := range access.Selections {
		selections = append(selections, runtimeAccessSelection{
			Kind: selection.Kind, ID: selection.ID, Name: selection.Name,
			Allowed: selection.Allowed, Attributes: selection.Attributes,
		})
	}
	return runtimeAgentAccessResponse{
		AgentID: access.AgentID, Selections: selections,
		ObservedAt: access.ObservedAt.UTC().Format(time.RFC3339), Source: access.Source,
	}
}

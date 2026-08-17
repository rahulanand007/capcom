package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"capcom/internal/domain"
	"capcom/internal/services"
)

type syncRuntimeRequest struct {
	Actor  string `json:"actor"`
	Reason string `json:"reason"`
}

func handleSyncRuntime(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		var req syncRuntimeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_json"})
			return
		}
		run, err := cfg.RuntimeSync.Sync(r.Context(), services.SyncRuntimeInput{RuntimeConnectionID: r.PathValue("id"), Trigger: domain.SyncTriggerManual, Actor: requestActor(r, req.Actor), Reason: req.Reason})
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, services.ErrSyncConflict) {
				status = http.StatusConflict
			}
			if errors.Is(err, sql.ErrNoRows) {
				status = http.StatusNotFound
			}
			if run.ID == "" && status == http.StatusBadGateway {
				status = http.StatusBadRequest
			}
			writeAPIErrorWith(w, status, err, map[string]any{"sync_run": syncRunResponse(run)})
			return
		}
		writeJSON(w, http.StatusOK, syncRunResponse(run))
	}
}

func handleListSyncRuns(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		runs, err := cfg.RuntimeSync.ListRuns(r.Context(), r.PathValue("id"), limit)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		out := make([]map[string]any, 0, len(runs))
		for _, run := range runs {
			out = append(out, syncRunResponse(run))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleGetSyncRun(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		run, err := cfg.RuntimeSync.GetRun(r.Context(), r.PathValue("id"), r.PathValue("runID"))
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, sql.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeAPIError(w, status, err)
			return
		}
		writeJSON(w, http.StatusOK, syncRunResponse(run))
	}
}

func handleListPersistedAgents(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		agents, err := cfg.RuntimeSync.ListAgents(r.Context(), r.URL.Query().Get("runtime_connection_id"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		out := make([]map[string]any, 0, len(agents))
		for _, agent := range agents {
			out = append(out, persistedAgentResponse(agent))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleListInstanceAgents(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		agents, err := cfg.RuntimeSync.ListAgents(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		out := make([]map[string]any, 0, len(agents))
		for _, agent := range agents {
			out = append(out, persistedAgentResponse(agent))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleGetPersistedAgent(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail, ok := loadPersistedAgent(w, r, cfg)
		if ok {
			writeJSON(w, http.StatusOK, persistedAgentResponse(detail.Agent))
		}
	}
}

func handleGetPersistedAgentSkills(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail, ok := loadPersistedAgent(w, r, cfg)
		if ok {
			out := make([]runtimeAgentSkillResponse, 0, len(detail.Skills))
			for _, skill := range detail.Skills {
				out = append(out, runtimeAgentSkillResponseFromDomain(skill))
			}
			writeJSON(w, http.StatusOK, out)
		}
	}
}

func handleGetPersistedAgentAccess(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail, ok := loadPersistedAgent(w, r, cfg)
		if ok {
			writeJSON(w, http.StatusOK, runtimeAgentAccessResponseFromDomain(detail.Access))
		}
	}
}

func handleListSubagentExecutions(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		writeSubagentExecutions(w, r, cfg, r.URL.Query().Get("runtime_connection_id"), r.URL.Query().Get("agent_id"))
	}
}

func writeSubagentExecutions(w http.ResponseWriter, r *http.Request, cfg RouterConfig, runtimeID, agentID string) {
	items, err := cfg.RuntimeSync.ListSubagentExecutions(r.Context(), runtimeID, agentID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{"id": item.ID, "runtime_connection_id": item.RuntimeConnectionID,
			"runtime_execution_id": item.RuntimeExecutionID, "parent_run_id": item.ParentRunID,
			"runtime_agent_id": item.RuntimeAgentID, "subagent_type": item.SubagentType,
			"status": item.Status, "description": item.Description, "summary": item.Summary,
			"started_at": item.StartedAt, "ended_at": item.EndedAt, "observed_at": item.ObservedAt,
			"metadata": item.Metadata})
	}
	writeJSON(w, http.StatusOK, out)
}

func handleListInstanceSubagentExecutions(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		writeSubagentExecutions(w, r, cfg, r.PathValue("id"), r.URL.Query().Get("agent_id"))
	}
}

func handleListRuntimeExecutions(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		writeRuntimeExecutions(w, r, cfg, r.URL.Query().Get("runtime_connection_id"))
	}
}

func handleListInstanceRuntimeExecutions(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.RuntimeSync == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
			return
		}
		writeRuntimeExecutions(w, r, cfg, r.PathValue("id"))
	}
}

func writeRuntimeExecutions(w http.ResponseWriter, r *http.Request, cfg RouterConfig, runtimeID string) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, err := cfg.RuntimeSync.ListRuntimeExecutions(r.Context(), runtimeID,
		r.URL.Query().Get("runtime_agent_id"), r.URL.Query().Get("kind"), limit)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"id": item.ID, "runtime_connection_id": item.RuntimeConnectionID,
			"runtime_execution_id":        item.RuntimeExecutionID,
			"parent_runtime_execution_id": item.ParentRuntimeExecutionID,
			"runtime_agent_id":            item.RuntimeAgentID, "kind": item.Kind, "status": item.Status,
			"started_at": item.StartedAt, "ended_at": item.EndedAt, "observed_at": item.ObservedAt,
			"metadata": item.Metadata,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func loadPersistedAgent(w http.ResponseWriter, r *http.Request, cfg RouterConfig) (domain.PersistedAgentDetail, bool) {
	if cfg.RuntimeSync == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
		return domain.PersistedAgentDetail{}, false
	}
	detail, err := cfg.RuntimeSync.GetAgent(r.Context(), r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, sql.ErrNoRows) {
			status = http.StatusNotFound
		}
		writeAPIError(w, status, err)
		return domain.PersistedAgentDetail{}, false
	}
	return detail, true
}

func syncRunResponse(run domain.RuntimeSyncRun) map[string]any {
	errorMessage := ""
	if run.ErrorMessage != "" {
		errorMessage = publicError(http.StatusBadGateway, run.ErrorMessage).Error.Message
	}
	return map[string]any{"id": run.ID, "runtime_connection_id": run.RuntimeConnectionID, "trigger": run.Trigger, "status": run.Status,
		"started_at": run.StartedAt, "finished_at": run.FinishedAt, "duration_ms": run.DurationMS, "agents_seen": run.AgentsSeen,
		"skills_seen": run.SkillsSeen, "bindings_seen": run.BindingsSeen, "access_documents_seen": run.AccessDocumentsSeen,
		"executions_seen":  run.ExecutionsSeen,
		"diagnostics_seen": run.DiagnosticsSeen, "inventory_seen": run.InventorySeen, "capabilities_seen": run.CapabilitiesSeen,
		"delegations_seen": run.DelegationsSeen,
		"error_code":       run.ErrorCode, "error_message": errorMessage}
}

func persistedAgentResponse(agent domain.PersistedAgent) map[string]any {
	return map[string]any{"id": agent.ID, "name": agent.Name, "status": agent.Status, "kind": agent.Kind, "metadata": agent.Metadata,
		"runtime_connection_id": agent.RuntimeConnectionID, "runtime_agent_id": agent.RuntimeAgentID,
		"parent_runtime_agent_id": agent.ParentRuntimeAgentID, "freshness": agent.Freshness, "observed_at": agent.ObservedAt,
		"last_successful_sync_at": agent.LastSuccessfulSyncAt, "runtime_status": agent.RuntimeStatus}
}

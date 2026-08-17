package api

import (
	"net/http"

	"capcom/internal/domain"
)

func handleListAgentDelegations(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeAgentDelegations(w, r, cfg, "", r.PathValue("id"))
	}
}

func handleListInstanceAgentDelegations(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeAgentDelegations(w, r, cfg, r.PathValue("id"), "")
	}
}

func writeAgentDelegations(w http.ResponseWriter, r *http.Request, cfg RouterConfig, runtimeID, agentID string) {
	if cfg.RuntimeSync == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "sync_not_configured"})
		return
	}
	items, err := cfg.RuntimeSync.ListAgentDelegations(r.Context(), runtimeID, agentID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, agentDelegationResponse(item))
	}
	writeJSON(w, http.StatusOK, out)
}

func agentDelegationResponse(item domain.PersistedAgentDelegation) map[string]any {
	return map[string]any{
		"id": item.ID, "runtime_connection_id": item.RuntimeConnectionID,
		"orchestrator_runtime_agent_id": item.OrchestratorRuntimeAgentID,
		"delegate_runtime_agent_id":     item.DelegateRuntimeAgentID, "delegate_ref": item.DelegateRef,
		"tool_name": item.ToolName, "display_name": item.DisplayName, "persona": item.Persona,
		"configured": item.Configured, "resolved": item.Resolved, "revision": item.Revision,
		"status": item.Status, "observed_at": item.ObservedAt, "metadata": item.Metadata,
	}
}

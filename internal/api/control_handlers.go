package api

import (
	"encoding/json"
	"net/http"

	"capcom/internal/domain"
	"capcom/internal/services"
)

type reconcileAccessRequest struct {
	Selections     []runtimeAccessSelection `json:"selections"`
	Actor          string                   `json:"actor"`
	Reason         string                   `json:"reason"`
	IdempotencyKey string                   `json:"idempotency_key"`
	DryRun         bool                     `json:"dry_run"`
}

type setAgentStatusRequest struct {
	Status         domain.AgentStatus `json:"status"`
	Actor          string             `json:"actor"`
	Reason         string             `json:"reason"`
	IdempotencyKey string             `json:"idempotency_key"`
	DryRun         bool               `json:"dry_run"`
}

type deleteAgentRequest struct {
	Confirmation   string `json:"confirmation"`
	Actor          string `json:"actor"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
	DryRun         bool   `json:"dry_run"`
}

type cancelExecutionRequest struct {
	Actor          string `json:"actor"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
	DryRun         bool   `json:"dry_run"`
}

func handleSetAgentStatus(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.ControlActions == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "control_actions_not_configured"})
			return
		}
		var req setAgentStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_json"})
			return
		}
		action, err := cfg.ControlActions.SetAgentStatus(r.Context(), services.SetAgentStatusInput{
			AgentID: r.PathValue("id"), Status: req.Status, Actor: req.Actor, Reason: req.Reason,
			IdempotencyKey: req.IdempotencyKey, DryRun: req.DryRun,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "action": controlActionResponse(action)})
			return
		}
		writeJSON(w, http.StatusOK, controlActionResponse(action))
	}
}

func handleReconcileAgentAccess(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.ControlActions == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "control_actions_not_configured"})
			return
		}
		var req reconcileAccessRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_json"})
			return
		}
		selections := make([]domain.AccessSelection, 0, len(req.Selections))
		for _, item := range req.Selections {
			selections = append(selections, domain.AccessSelection{Kind: item.Kind, ID: item.ID, Name: item.Name, Allowed: item.Allowed, Attributes: item.Attributes})
		}
		action, err := cfg.ControlActions.ReconcileAccess(r.Context(), services.ReconcileAccessInput{
			AgentID: r.PathValue("id"), Access: domain.AccessDocument{Selections: selections, Source: "capcom"},
			Actor: req.Actor, Reason: req.Reason, IdempotencyKey: req.IdempotencyKey, DryRun: req.DryRun,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "action": controlActionResponse(action)})
			return
		}
		writeJSON(w, http.StatusOK, controlActionResponse(action))
	}
}

func handleDeleteAgent(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.ControlActions == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "control_actions_not_configured"})
			return
		}
		var req deleteAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_json"})
			return
		}
		action, err := cfg.ControlActions.DeleteAgent(r.Context(), services.DeleteAgentInput{
			AgentID: r.PathValue("id"), Confirmation: req.Confirmation, Actor: req.Actor, Reason: req.Reason,
			IdempotencyKey: req.IdempotencyKey, DryRun: req.DryRun,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "action": controlActionResponse(action)})
			return
		}
		writeJSON(w, http.StatusOK, controlActionResponse(action))
	}
}

func handleCancelExecution(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.ControlActions == nil {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "control_actions_not_configured"})
			return
		}
		var req cancelExecutionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid_json"})
			return
		}
		action, err := cfg.ControlActions.CancelExecution(r.Context(), services.CancelExecutionInput{
			ExecutionID: r.PathValue("id"), Actor: req.Actor, Reason: req.Reason,
			IdempotencyKey: req.IdempotencyKey, DryRun: req.DryRun,
		})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "action": controlActionResponse(action)})
			return
		}
		writeJSON(w, http.StatusOK, controlActionResponse(action))
	}
}

func controlActionResponse(action domain.ControlAction) map[string]any {
	return map[string]any{"id": action.ID, "runtime_connection_id": action.RuntimeConnectionID, "agent_id": action.AgentID,
		"action_type": action.Type, "status": action.Status, "actor": action.Actor, "reason": action.Reason,
		"idempotency_key": action.IdempotencyKey, "result": action.Result, "created_at": action.CreatedAt, "updated_at": action.UpdatedAt}
}

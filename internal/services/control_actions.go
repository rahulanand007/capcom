package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
)

type ControlActionRepository interface {
	Create(ctx context.Context, action domain.ControlAction) (domain.ControlAction, error)
	FindByIdempotencyKey(ctx context.Context, key string) (domain.ControlAction, error)
	Update(ctx context.Context, action domain.ControlAction, runtimeRequest, runtimeResponse map[string]any, errorText string) (domain.ControlAction, error)
}

type PostActionSyncer interface {
	Sync(ctx context.Context, input SyncRuntimeInput) (domain.RuntimeSyncRun, error)
}

type ControlActionService struct {
	runtimes RuntimeConnectionRepository
	agents   RuntimeSyncRepository
	actions  ControlActionRepository
	audit    AuditRepository
	syncer   PostActionSyncer
	adapters map[domain.RuntimeKind]runtimeadapter.Adapter
}

type ReconcileAccessInput struct {
	AgentID        string
	Access         domain.AccessDocument
	Actor          string
	Reason         string
	IdempotencyKey string
	DryRun         bool
}

type SetAgentStatusInput struct {
	AgentID        string
	Status         domain.AgentStatus
	Actor          string
	Reason         string
	IdempotencyKey string
	DryRun         bool
}

type DeleteAgentInput struct {
	AgentID        string
	Confirmation   string
	Actor          string
	Reason         string
	IdempotencyKey string
	DryRun         bool
}

type CancelExecutionInput struct {
	ExecutionID    string
	Actor          string
	Reason         string
	IdempotencyKey string
	DryRun         bool
}

func NewControlActionService(runtimes RuntimeConnectionRepository, agents RuntimeSyncRepository, actions ControlActionRepository, audit AuditRepository, syncer PostActionSyncer) ControlActionService {
	return ControlActionService{runtimes: runtimes, agents: agents, actions: actions, audit: audit, syncer: syncer, adapters: map[domain.RuntimeKind]runtimeadapter.Adapter{}}
}
func (s ControlActionService) WithAdapter(adapter runtimeadapter.Adapter) ControlActionService {
	if adapter != nil {
		s.adapters[adapter.Kind()] = adapter
	}
	return s
}

func (s ControlActionService) ReconcileAccess(ctx context.Context, input ReconcileAccessInput) (domain.ControlAction, error) {
	if strings.TrimSpace(input.AgentID) == "" || strings.TrimSpace(input.Actor) == "" || strings.TrimSpace(input.Reason) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return domain.ControlAction{}, fmt.Errorf("agent id, actor, reason, and idempotency_key are required")
	}
	for _, selection := range input.Access.Selections {
		if strings.TrimSpace(selection.ID) == "" {
			return domain.ControlAction{}, fmt.Errorf("every access selection requires an id")
		}
	}
	if existing, err := s.actions.FindByIdempotencyKey(ctx, input.IdempotencyKey); err == nil {
		return existing, nil
	} else if err != sql.ErrNoRows {
		return domain.ControlAction{}, err
	}
	detail, err := s.agents.GetPersistedAgent(ctx, input.AgentID)
	if err != nil {
		return domain.ControlAction{}, err
	}
	conn, err := s.runtimes.Get(ctx, detail.Agent.RuntimeConnectionID)
	if err != nil {
		return domain.ControlAction{}, err
	}
	before := accessMap(detail.Access)
	requested := accessMap(input.Access)
	action := domain.ControlAction{RuntimeConnectionID: conn.ID, AgentID: input.AgentID, Type: "replace_agent_access", Status: domain.ControlActionQueued,
		Actor: strings.TrimSpace(input.Actor), Reason: strings.TrimSpace(input.Reason), IdempotencyKey: strings.TrimSpace(input.IdempotencyKey), Before: before, After: requested}
	if conn.Mode != domain.RuntimeModeControlEnabled {
		action.Status = domain.ControlActionRejected
		action, err = s.actions.Create(ctx, action)
		if err != nil {
			return action, err
		}
		action, _ = s.actions.Update(ctx, action, requested, nil, "runtime connection is read-only")
		s.auditAction(ctx, action, "control_action.rejected", "rejected", before, requested, map[string]any{"reason": "read_only_runtime"})
		return action, fmt.Errorf("runtime connection is read-only")
	}
	adapter, ok := s.adapters[conn.Kind]
	if !ok {
		action.Status = domain.ControlActionRejected
		action, err = s.actions.Create(ctx, action)
		if err != nil {
			return action, err
		}
		action, _ = s.actions.Update(ctx, action, requested, nil, "runtime adapter is not registered")
		s.auditAction(ctx, action, "control_action.rejected", "rejected", before, requested, map[string]any{"reason": "adapter_unavailable"})
		return action, fmt.Errorf("runtime adapter %q is not registered", conn.Kind)
	}
	action, err = s.actions.Create(ctx, action)
	if err != nil {
		return action, err
	}
	s.auditAction(ctx, action, "control_action.requested", "succeeded", before, requested, nil)
	if input.DryRun {
		action.Status = domain.ControlActionSucceeded
		result := map[string]any{"dry_run": true, "validated": true}
		action, err = s.actions.Update(ctx, action, requested, result, "")
		s.auditAction(ctx, action, "control_action.dry_run_succeeded", "succeeded", before, requested, result)
		return action, err
	}
	action.Status = domain.ControlActionRunning
	action, err = s.actions.Update(ctx, action, requested, nil, "")
	if err != nil {
		return action, err
	}
	observed, callErr := adapter.ReplaceAgentAccess(ctx, conn, detail.Agent.RuntimeAgentID, input.Access)
	if callErr != nil {
		action.Status = domain.ControlActionFailed
		action, _ = s.actions.Update(ctx, action, requested, nil, sanitizeRuntimeError(callErr))
		s.auditAction(ctx, action, "control_action.failed", "failed", before, requested, map[string]any{"error": sanitizeRuntimeError(callErr)})
		return action, fmt.Errorf("replace runtime access: %w", callErr)
	}
	result := accessMap(*observed)
	action.Status = domain.ControlActionSucceeded
	action, err = s.actions.Update(ctx, action, requested, result, "")
	if err != nil {
		return action, err
	}
	if s.syncer != nil {
		_, syncErr := s.syncer.Sync(ctx, SyncRuntimeInput{RuntimeConnectionID: conn.ID, Trigger: domain.SyncTriggerPostAction, Actor: input.Actor, Reason: "verify access reconciliation: " + input.Reason})
		if syncErr != nil {
			result["verification_sync_error"] = sanitizeRuntimeError(syncErr)
		}
	}
	s.auditAction(ctx, action, "control_action.succeeded", "succeeded", before, requested, result)
	return action, nil
}

func (s ControlActionService) SetAgentStatus(ctx context.Context, input SetAgentStatusInput) (domain.ControlAction, error) {
	if strings.TrimSpace(input.AgentID) == "" || strings.TrimSpace(input.Actor) == "" || strings.TrimSpace(input.Reason) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return domain.ControlAction{}, fmt.Errorf("agent id, actor, reason, and idempotency_key are required")
	}
	if input.Status != domain.AgentStatusEnabled && input.Status != domain.AgentStatusDisabled {
		return domain.ControlAction{}, fmt.Errorf("status must be %q or %q", domain.AgentStatusEnabled, domain.AgentStatusDisabled)
	}
	if existing, err := s.actions.FindByIdempotencyKey(ctx, input.IdempotencyKey); err == nil {
		return existing, nil
	} else if err != sql.ErrNoRows {
		return domain.ControlAction{}, err
	}
	detail, err := s.agents.GetPersistedAgent(ctx, input.AgentID)
	if err != nil {
		return domain.ControlAction{}, err
	}
	conn, err := s.runtimes.Get(ctx, detail.Agent.RuntimeConnectionID)
	if err != nil {
		return domain.ControlAction{}, err
	}
	actionType := "enable_agent"
	if input.Status == domain.AgentStatusDisabled {
		actionType = "disable_agent"
	}
	before := map[string]any{"status": detail.Agent.Status}
	after := map[string]any{"status": input.Status}
	action := domain.ControlAction{RuntimeConnectionID: conn.ID, AgentID: input.AgentID, Type: actionType,
		Status: domain.ControlActionQueued, Actor: strings.TrimSpace(input.Actor), Reason: strings.TrimSpace(input.Reason),
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey), Before: before, After: after}
	if conn.Mode != domain.RuntimeModeControlEnabled {
		return s.rejectStatusAction(ctx, action, before, after, "runtime connection is read-only", "read_only_runtime")
	}
	adapter, ok := s.adapters[conn.Kind]
	if !ok {
		return s.rejectStatusAction(ctx, action, before, after, "runtime adapter is not registered", "adapter_unavailable")
	}
	check, checkErr := adapter.Check(ctx, conn)
	if checkErr != nil {
		return s.rejectStatusAction(ctx, action, before, after, sanitizeRuntimeError(checkErr), "adapter_check_failed")
	}
	if !check.Capabilities.SetAgentStatus {
		return s.rejectStatusAction(ctx, action, before, after, "runtime adapter does not support agent status control", "unsupported_action")
	}
	action, err = s.actions.Create(ctx, action)
	if err != nil {
		return action, err
	}
	s.auditStatusAction(ctx, action, "control_action.requested", "succeeded", before, after, nil)
	if input.DryRun {
		action.Status = domain.ControlActionSucceeded
		result := map[string]any{"dry_run": true, "validated": true, "status": input.Status}
		action, err = s.actions.Update(ctx, action, after, result, "")
		s.auditStatusAction(ctx, action, "control_action.dry_run_succeeded", "succeeded", before, after, result)
		return action, err
	}
	action.Status = domain.ControlActionRunning
	action, err = s.actions.Update(ctx, action, after, nil, "")
	if err != nil {
		return action, err
	}
	observed, callErr := adapter.SetAgentStatus(ctx, conn, detail.Agent.RuntimeAgentID, input.Status)
	if callErr != nil {
		action.Status = domain.ControlActionFailed
		action, _ = s.actions.Update(ctx, action, after, nil, sanitizeRuntimeError(callErr))
		s.auditStatusAction(ctx, action, "control_action.failed", "failed", before, after, map[string]any{"error": sanitizeRuntimeError(callErr)})
		return action, fmt.Errorf("set runtime agent status: %w", callErr)
	}
	result := map[string]any{"status": observed.Status, "runtime_agent_id": observed.RuntimeAgentID}
	action.Status = domain.ControlActionSucceeded
	action, err = s.actions.Update(ctx, action, after, result, "")
	if err != nil {
		return action, err
	}
	if s.syncer != nil {
		_, syncErr := s.syncer.Sync(ctx, SyncRuntimeInput{RuntimeConnectionID: conn.ID, Trigger: domain.SyncTriggerPostAction,
			Actor: input.Actor, Reason: "verify agent status change: " + input.Reason})
		if syncErr != nil {
			result["verification_sync_error"] = sanitizeRuntimeError(syncErr)
		}
	}
	s.auditStatusAction(ctx, action, "control_action.succeeded", "succeeded", before, after, result)
	return action, nil
}

func (s ControlActionService) DeleteAgent(ctx context.Context, input DeleteAgentInput) (domain.ControlAction, error) {
	if strings.TrimSpace(input.AgentID) == "" || strings.TrimSpace(input.Actor) == "" || strings.TrimSpace(input.Reason) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return domain.ControlAction{}, fmt.Errorf("agent id, actor, reason, and idempotency_key are required")
	}
	if existing, err := s.actions.FindByIdempotencyKey(ctx, input.IdempotencyKey); err == nil {
		return existing, nil
	} else if err != sql.ErrNoRows {
		return domain.ControlAction{}, err
	}
	detail, err := s.agents.GetPersistedAgent(ctx, input.AgentID)
	if err != nil {
		return domain.ControlAction{}, err
	}
	conn, err := s.runtimes.Get(ctx, detail.Agent.RuntimeConnectionID)
	if err != nil {
		return domain.ControlAction{}, err
	}
	before := map[string]any{
		"runtime_agent_id": detail.Agent.RuntimeAgentID,
		"name":             detail.Agent.Name,
		"status":           detail.Agent.Status,
	}
	after := map[string]any{"deleted": true}
	action := domain.ControlAction{
		RuntimeConnectionID: conn.ID,
		AgentID:             input.AgentID,
		Type:                "delete_agent",
		Status:              domain.ControlActionQueued,
		Actor:               strings.TrimSpace(input.Actor),
		Reason:              strings.TrimSpace(input.Reason),
		IdempotencyKey:      strings.TrimSpace(input.IdempotencyKey),
		Before:              before,
		After:               after,
	}
	if strings.TrimSpace(input.Confirmation) != detail.Agent.RuntimeAgentID {
		message := fmt.Sprintf("confirmation must exactly match runtime agent id %q", detail.Agent.RuntimeAgentID)
		rejectErr := s.rejectControlAction(ctx, action, "agent", input.AgentID, before, after, message, "confirmation_mismatch")
		return actionFromError(rejectErr, action), rejectErr
	}
	if conn.Kind == domain.RuntimeKindLangGraph && isLangGraphSystemManaged(detail.Agent.Metadata) {
		message := "LangGraph system-managed assistants are recreated from graph configuration and cannot be deleted through Capcom"
		rejectErr := s.rejectControlAction(ctx, action, "agent", input.AgentID, before, after, message, "system_managed_agent")
		return actionFromError(rejectErr, action), rejectErr
	}
	adapter, err := s.validateCapability(ctx, conn, action, before, after, "agent", input.AgentID, func(capabilities runtimeadapter.Capabilities) bool {
		return capabilities.DeleteAgent
	}, "runtime adapter does not support agent deletion")
	if err != nil {
		return actionFromError(err, action), err
	}
	action, err = s.actions.Create(ctx, action)
	if err != nil {
		return action, err
	}
	s.auditControlAction(ctx, action, "control_action.requested", "succeeded", "agent", input.AgentID, before, after, nil)
	if input.DryRun {
		action.Status = domain.ControlActionSucceeded
		result := map[string]any{"dry_run": true, "validated": true, "runtime_agent_id": detail.Agent.RuntimeAgentID}
		action, err = s.actions.Update(ctx, action, after, result, "")
		s.auditControlAction(ctx, action, "control_action.dry_run_succeeded", "succeeded", "agent", input.AgentID, before, after, result)
		return action, err
	}
	action.Status = domain.ControlActionRunning
	action, err = s.actions.Update(ctx, action, after, nil, "")
	if err != nil {
		return action, err
	}
	if callErr := adapter.DeleteAgent(ctx, conn, detail.Agent.RuntimeAgentID); callErr != nil {
		return s.failControlAction(ctx, action, "agent", input.AgentID, before, after, "delete runtime agent", callErr)
	}
	result := map[string]any{"deleted": true, "runtime_agent_id": detail.Agent.RuntimeAgentID}
	if err := s.agents.MarkAgentDeleted(ctx, input.AgentID); err != nil {
		result["local_tombstone_error"] = sanitizeRuntimeError(err)
	}
	action.Status = domain.ControlActionSucceeded
	action, err = s.actions.Update(ctx, action, after, result, "")
	if err != nil {
		return action, err
	}
	s.runVerificationSync(ctx, conn.ID, input.Actor, "verify agent deletion: "+input.Reason, result)
	s.auditControlAction(ctx, action, "control_action.succeeded", "succeeded", "agent", input.AgentID, before, after, result)
	return action, nil
}

func isLangGraphSystemManaged(metadata map[string]any) bool {
	assistantMetadata, ok := metadata["assistant_metadata"].(map[string]any)
	if !ok {
		return false
	}
	createdBy, _ := assistantMetadata["created_by"].(string)
	return strings.EqualFold(strings.TrimSpace(createdBy), "system")
}

func (s ControlActionService) CancelExecution(ctx context.Context, input CancelExecutionInput) (domain.ControlAction, error) {
	if strings.TrimSpace(input.ExecutionID) == "" || strings.TrimSpace(input.Actor) == "" || strings.TrimSpace(input.Reason) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		return domain.ControlAction{}, fmt.Errorf("execution id, actor, reason, and idempotency_key are required")
	}
	if existing, err := s.actions.FindByIdempotencyKey(ctx, input.IdempotencyKey); err == nil {
		return existing, nil
	} else if err != sql.ErrNoRows {
		return domain.ControlAction{}, err
	}
	execution, err := s.agents.GetRuntimeExecution(ctx, input.ExecutionID)
	if err != nil {
		return domain.ControlAction{}, err
	}
	conn, err := s.runtimes.Get(ctx, execution.RuntimeConnectionID)
	if err != nil {
		return domain.ControlAction{}, err
	}
	agentID := s.findExecutionAgentID(ctx, execution)
	before := map[string]any{
		"runtime_execution_id": execution.RuntimeExecutionID,
		"thread_id":            execution.ParentRuntimeExecutionID,
		"status":               execution.Status,
	}
	after := map[string]any{"status": "interrupted"}
	action := domain.ControlAction{
		RuntimeConnectionID: conn.ID,
		AgentID:             agentID,
		Type:                "cancel_execution",
		Status:              domain.ControlActionQueued,
		Actor:               strings.TrimSpace(input.Actor),
		Reason:              strings.TrimSpace(input.Reason),
		IdempotencyKey:      strings.TrimSpace(input.IdempotencyKey),
		Before:              before,
		After:               after,
	}
	if execution.Kind != "run" {
		rejectErr := s.rejectControlAction(ctx, action, "runtime_execution", input.ExecutionID, before, after, "only run executions can be cancelled", "invalid_execution_kind")
		return actionFromError(rejectErr, action), rejectErr
	}
	if execution.Status != "pending" && execution.Status != "running" {
		message := fmt.Sprintf("execution status %q cannot be cancelled", execution.Status)
		rejectErr := s.rejectControlAction(ctx, action, "runtime_execution", input.ExecutionID, before, after, message, "invalid_execution_status")
		return actionFromError(rejectErr, action), rejectErr
	}
	adapter, err := s.validateCapability(ctx, conn, action, before, after, "runtime_execution", input.ExecutionID, func(capabilities runtimeadapter.Capabilities) bool {
		return capabilities.CancelExecution
	}, "runtime adapter does not support execution cancellation")
	if err != nil {
		return actionFromError(err, action), err
	}
	action, err = s.actions.Create(ctx, action)
	if err != nil {
		return action, err
	}
	s.auditControlAction(ctx, action, "control_action.requested", "succeeded", "runtime_execution", input.ExecutionID, before, after, nil)
	if input.DryRun {
		action.Status = domain.ControlActionSucceeded
		result := map[string]any{"dry_run": true, "validated": true, "runtime_execution_id": execution.RuntimeExecutionID}
		action, err = s.actions.Update(ctx, action, after, result, "")
		s.auditControlAction(ctx, action, "control_action.dry_run_succeeded", "succeeded", "runtime_execution", input.ExecutionID, before, after, result)
		return action, err
	}
	action.Status = domain.ControlActionRunning
	action, err = s.actions.Update(ctx, action, after, nil, "")
	if err != nil {
		return action, err
	}
	if callErr := adapter.CancelExecution(ctx, conn, execution.RuntimeExecutionSnapshot); callErr != nil {
		return s.failControlAction(ctx, action, "runtime_execution", input.ExecutionID, before, after, "cancel runtime execution", callErr)
	}
	result := map[string]any{"status": "interrupted", "runtime_execution_id": execution.RuntimeExecutionID}
	action.Status = domain.ControlActionSucceeded
	action, err = s.actions.Update(ctx, action, after, result, "")
	if err != nil {
		return action, err
	}
	s.runVerificationSync(ctx, conn.ID, input.Actor, "verify execution cancellation: "+input.Reason, result)
	s.auditControlAction(ctx, action, "control_action.succeeded", "succeeded", "runtime_execution", input.ExecutionID, before, after, result)
	return action, nil
}

type capabilityPredicate func(runtimeadapter.Capabilities) bool

type rejectedActionError struct {
	action domain.ControlAction
	err    error
}

func (e rejectedActionError) Error() string { return e.err.Error() }

func actionFromError(err error, fallback domain.ControlAction) domain.ControlAction {
	var rejected rejectedActionError
	if errors.As(err, &rejected) {
		return rejected.action
	}
	return fallback
}

func (s ControlActionService) validateCapability(
	ctx context.Context,
	conn domain.RuntimeConnection,
	action domain.ControlAction,
	before, after map[string]any,
	targetType, targetID string,
	supported capabilityPredicate,
	unsupportedMessage string,
) (runtimeadapter.Adapter, error) {
	if conn.Mode != domain.RuntimeModeControlEnabled {
		return nil, s.rejectControlAction(ctx, action, targetType, targetID, before, after, "runtime connection is read-only", "read_only_runtime")
	}
	adapter, ok := s.adapters[conn.Kind]
	if !ok {
		return nil, s.rejectControlAction(ctx, action, targetType, targetID, before, after, "runtime adapter is not registered", "adapter_unavailable")
	}
	check, err := adapter.Check(ctx, conn)
	if err != nil {
		return nil, s.rejectControlAction(ctx, action, targetType, targetID, before, after, sanitizeRuntimeError(err), "adapter_check_failed")
	}
	if !supported(check.Capabilities) {
		return nil, s.rejectControlAction(ctx, action, targetType, targetID, before, after, unsupportedMessage, "unsupported_action")
	}
	return adapter, nil
}

func (s ControlActionService) rejectControlAction(
	ctx context.Context,
	action domain.ControlAction,
	targetType, targetID string,
	before, after map[string]any,
	message, reason string,
) error {
	action.Status = domain.ControlActionRejected
	created, err := s.actions.Create(ctx, action)
	if err != nil {
		return err
	}
	created, _ = s.actions.Update(ctx, created, after, nil, message)
	s.auditControlAction(ctx, created, "control_action.rejected", "rejected", targetType, targetID, before, after, map[string]any{"reason": reason})
	return rejectedActionError{action: created, err: fmt.Errorf("%s", message)}
}

func (s ControlActionService) failControlAction(
	ctx context.Context,
	action domain.ControlAction,
	targetType, targetID string,
	before, after map[string]any,
	prefix string,
	callErr error,
) (domain.ControlAction, error) {
	action.Status = domain.ControlActionFailed
	message := sanitizeRuntimeError(callErr)
	action, _ = s.actions.Update(ctx, action, after, nil, message)
	s.auditControlAction(ctx, action, "control_action.failed", "failed", targetType, targetID, before, after, map[string]any{"error": message})
	return action, fmt.Errorf("%s: %w", prefix, callErr)
}

func (s ControlActionService) runVerificationSync(ctx context.Context, runtimeID, actor, reason string, result map[string]any) {
	if s.syncer == nil {
		return
	}
	if _, err := s.syncer.Sync(ctx, SyncRuntimeInput{
		RuntimeConnectionID: runtimeID,
		Trigger:             domain.SyncTriggerPostAction,
		Actor:               actor,
		Reason:              reason,
	}); err != nil {
		result["verification_sync_error"] = sanitizeRuntimeError(err)
	}
}

func (s ControlActionService) findExecutionAgentID(ctx context.Context, execution domain.PersistedRuntimeExecution) string {
	agents, err := s.agents.ListPersistedAgents(ctx, execution.RuntimeConnectionID)
	if err != nil {
		return ""
	}
	for _, agent := range agents {
		if agent.RuntimeAgentID == execution.RuntimeAgentID {
			return agent.ID
		}
	}
	return ""
}

func (s ControlActionService) auditControlAction(
	ctx context.Context,
	action domain.ControlAction,
	eventType, result, targetType, targetID string,
	before, after, metadata map[string]any,
) {
	if s.audit == nil {
		return
	}
	_, _ = s.audit.Create(ctx, domain.AuditEvent{
		RuntimeConnectionID: action.RuntimeConnectionID,
		AgentID:             action.AgentID,
		ControlActionID:     action.ID,
		Actor:               action.Actor,
		EventType:           eventType,
		TargetType:          targetType,
		TargetID:            targetID,
		Reason:              action.Reason,
		Before:              before,
		After:               after,
		Result:              result,
		Metadata:            metadata,
	})
}

func (s ControlActionService) rejectStatusAction(ctx context.Context, action domain.ControlAction, before, after map[string]any, message, reason string) (domain.ControlAction, error) {
	action.Status = domain.ControlActionRejected
	created, err := s.actions.Create(ctx, action)
	if err != nil {
		return action, err
	}
	created, _ = s.actions.Update(ctx, created, after, nil, message)
	s.auditStatusAction(ctx, created, "control_action.rejected", "rejected", before, after, map[string]any{"reason": reason})
	return created, fmt.Errorf("%s", message)
}

func (s ControlActionService) auditStatusAction(ctx context.Context, action domain.ControlAction, eventType, result string, before, after, metadata map[string]any) {
	if s.audit == nil {
		return
	}
	_, _ = s.audit.Create(ctx, domain.AuditEvent{RuntimeConnectionID: action.RuntimeConnectionID, AgentID: action.AgentID,
		ControlActionID: action.ID, Actor: action.Actor, EventType: eventType, TargetType: "agent_status",
		TargetID: action.AgentID, Reason: action.Reason, Before: before, After: after, Result: result, Metadata: metadata})
}

func accessMap(access domain.AccessDocument) map[string]any {
	data, _ := json.Marshal(access)
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}
func (s ControlActionService) auditAction(ctx context.Context, action domain.ControlAction, eventType, result string, before, after, metadata map[string]any) {
	if s.audit == nil {
		return
	}
	_, _ = s.audit.Create(ctx, domain.AuditEvent{RuntimeConnectionID: action.RuntimeConnectionID, AgentID: action.AgentID, ControlActionID: action.ID, Actor: action.Actor, EventType: eventType, TargetType: "agent_access", TargetID: action.AgentID, Reason: action.Reason, Before: before, After: after, Result: result, Metadata: metadata})
}

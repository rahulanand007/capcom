package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
)

var ErrSyncConflict = errors.New("runtime sync already in progress")

type RuntimeSyncRepository interface {
	TryLock(ctx context.Context, runtimeID string) (release func(), acquired bool, err error)
	CreateRun(ctx context.Context, run domain.RuntimeSyncRun) (domain.RuntimeSyncRun, error)
	FailRun(ctx context.Context, run domain.RuntimeSyncRun, code, message string) (domain.RuntimeSyncRun, error)
	PersistSnapshot(ctx context.Context, run domain.RuntimeSyncRun, snapshot domain.RuntimeSnapshot, missingThreshold int) (domain.RuntimeSyncRun, error)
	ListRuns(ctx context.Context, runtimeID string, limit int) ([]domain.RuntimeSyncRun, error)
	GetRun(ctx context.Context, runtimeID, runID string) (domain.RuntimeSyncRun, error)
	ListPersistedAgents(ctx context.Context, runtimeID string) ([]domain.PersistedAgent, error)
	GetPersistedAgent(ctx context.Context, agentID string) (domain.PersistedAgentDetail, error)
	MarkAgentDeleted(ctx context.Context, agentID string) error
	ListSubagentExecutions(ctx context.Context, runtimeID, agentID string) ([]domain.PersistedSubagentExecution, error)
	ListRuntimeExecutions(ctx context.Context, runtimeID, agentID, kind string, limit int) ([]domain.PersistedRuntimeExecution, error)
	GetRuntimeExecution(ctx context.Context, executionID string) (domain.PersistedRuntimeExecution, error)
	ListRuntimeDiagnostics(ctx context.Context, runtimeID string) ([]domain.PersistedRuntimeDiagnostic, error)
	ListRuntimeInventory(ctx context.Context, runtimeID, kind string) ([]domain.PersistedRuntimeInventory, error)
	ListRuntimeCapabilities(ctx context.Context, runtimeID string) ([]domain.PersistedRuntimeCapability, error)
	ListAgentDelegations(ctx context.Context, runtimeID, runtimeAgentID string) ([]domain.PersistedAgentDelegation, error)
}

type RuntimeSyncService struct {
	runtimes         RuntimeConnectionRepository
	store            RuntimeSyncRepository
	audit            AuditRepository
	adapters         map[domain.RuntimeKind]runtimeadapter.Adapter
	missingThreshold int
}

type SyncRuntimeInput struct {
	RuntimeConnectionID string
	Trigger             domain.SyncTrigger
	Actor               string
	Reason              string
}

func NewRuntimeSyncService(runtimes RuntimeConnectionRepository, store RuntimeSyncRepository, audit AuditRepository, missingThreshold int) RuntimeSyncService {
	if missingThreshold <= 0 {
		missingThreshold = 3
	}
	return RuntimeSyncService{runtimes: runtimes, store: store, audit: audit, adapters: map[domain.RuntimeKind]runtimeadapter.Adapter{}, missingThreshold: missingThreshold}
}

func (s RuntimeSyncService) WithAdapter(adapter runtimeadapter.Adapter) RuntimeSyncService {
	if adapter != nil {
		s.adapters[adapter.Kind()] = adapter
	}
	return s
}

func (s RuntimeSyncService) Sync(ctx context.Context, input SyncRuntimeInput) (domain.RuntimeSyncRun, error) {
	if strings.TrimSpace(input.RuntimeConnectionID) == "" {
		return domain.RuntimeSyncRun{}, fmt.Errorf("runtime connection id is required")
	}
	if input.Trigger == "" {
		input.Trigger = domain.SyncTriggerManual
	}
	if input.Trigger == domain.SyncTriggerManual && (strings.TrimSpace(input.Actor) == "" || strings.TrimSpace(input.Reason) == "") {
		return domain.RuntimeSyncRun{}, fmt.Errorf("actor and reason are required")
	}
	conn, err := s.runtimes.Get(ctx, input.RuntimeConnectionID)
	if err != nil {
		return domain.RuntimeSyncRun{}, err
	}
	adapter, ok := s.adapters[conn.Kind]
	if !ok {
		return domain.RuntimeSyncRun{}, fmt.Errorf("runtime adapter %q is not registered", conn.Kind)
	}
	release, acquired, err := s.store.TryLock(ctx, conn.ID)
	if err != nil {
		return domain.RuntimeSyncRun{}, err
	}
	if !acquired {
		return domain.RuntimeSyncRun{}, ErrSyncConflict
	}
	defer release()

	run, err := s.store.CreateRun(ctx, domain.RuntimeSyncRun{RuntimeConnectionID: conn.ID, Trigger: input.Trigger})
	if err != nil {
		return domain.RuntimeSyncRun{}, err
	}
	snapshot, collectErr := adapter.CollectSnapshot(ctx, conn)
	if collectErr != nil {
		failed, failErr := s.store.FailRun(ctx, run, "adapter_error", sanitizeRuntimeError(collectErr))
		s.auditSync(ctx, failed, input, "runtime.sync_failed", "failed")
		if failErr != nil {
			return failed, errors.Join(collectErr, failErr)
		}
		return failed, fmt.Errorf("collect runtime snapshot: %w", collectErr)
	}
	if err := validateRuntimeSnapshot(*snapshot); err != nil {
		failed, failErr := s.store.FailRun(ctx, run, "invalid_snapshot", err.Error())
		s.auditSync(ctx, failed, input, "runtime.sync_failed", "failed")
		if failErr != nil {
			return failed, errors.Join(err, failErr)
		}
		return failed, err
	}
	completed, err := s.store.PersistSnapshot(ctx, run, *snapshot, s.missingThreshold)
	if err != nil {
		failed, failErr := s.store.FailRun(ctx, run, "persistence_error", sanitizeRuntimeError(err))
		s.auditSync(ctx, failed, input, "runtime.sync_failed", "failed")
		if failErr != nil {
			return failed, errors.Join(err, failErr)
		}
		return failed, err
	}
	s.auditSync(ctx, completed, input, "runtime.sync_succeeded", "succeeded")
	return completed, nil
}

func (s RuntimeSyncService) ListRuns(ctx context.Context, runtimeID string, limit int) ([]domain.RuntimeSyncRun, error) {
	return s.store.ListRuns(ctx, runtimeID, limit)
}

func (s RuntimeSyncService) GetRun(ctx context.Context, runtimeID, runID string) (domain.RuntimeSyncRun, error) {
	return s.store.GetRun(ctx, runtimeID, runID)
}

func (s RuntimeSyncService) ListAgents(ctx context.Context, runtimeID string) ([]domain.PersistedAgent, error) {
	return s.store.ListPersistedAgents(ctx, runtimeID)
}

func (s RuntimeSyncService) GetAgent(ctx context.Context, agentID string) (domain.PersistedAgentDetail, error) {
	if strings.TrimSpace(agentID) == "" {
		return domain.PersistedAgentDetail{}, fmt.Errorf("agent id is required")
	}
	return s.store.GetPersistedAgent(ctx, agentID)
}

func (s RuntimeSyncService) ListSubagentExecutions(ctx context.Context, runtimeID, agentID string) ([]domain.PersistedSubagentExecution, error) {
	return s.store.ListSubagentExecutions(ctx, runtimeID, agentID)
}

func (s RuntimeSyncService) ListRuntimeExecutions(ctx context.Context, runtimeID, agentID, kind string, limit int) ([]domain.PersistedRuntimeExecution, error) {
	return s.store.ListRuntimeExecutions(ctx, runtimeID, agentID, kind, limit)
}

func (s RuntimeSyncService) ListRuntimeDiagnostics(ctx context.Context, runtimeID string) ([]domain.PersistedRuntimeDiagnostic, error) {
	return s.store.ListRuntimeDiagnostics(ctx, runtimeID)
}

func (s RuntimeSyncService) ListRuntimeInventory(ctx context.Context, runtimeID, kind string) ([]domain.PersistedRuntimeInventory, error) {
	return s.store.ListRuntimeInventory(ctx, runtimeID, kind)
}

func (s RuntimeSyncService) ListRuntimeCapabilities(ctx context.Context, runtimeID string) ([]domain.PersistedRuntimeCapability, error) {
	return s.store.ListRuntimeCapabilities(ctx, runtimeID)
}

func (s RuntimeSyncService) ListAgentDelegations(ctx context.Context, runtimeID, agentID string) ([]domain.PersistedAgentDelegation, error) {
	runtimeAgentID := ""
	if strings.TrimSpace(agentID) != "" {
		detail, err := s.GetAgent(ctx, agentID)
		if err != nil {
			return nil, err
		}
		if runtimeID != "" && detail.Agent.RuntimeConnectionID != runtimeID {
			return nil, fmt.Errorf("agent does not belong to runtime instance")
		}
		runtimeID = detail.Agent.RuntimeConnectionID
		runtimeAgentID = detail.Agent.RuntimeAgentID
	}
	return s.store.ListAgentDelegations(ctx, runtimeID, runtimeAgentID)
}

func validateRuntimeSnapshot(snapshot domain.RuntimeSnapshot) error {
	diagnostics := map[string]struct{}{}
	for _, item := range snapshot.Diagnostics {
		id := strings.TrimSpace(item.CheckID)
		if id == "" {
			return fmt.Errorf("snapshot contains a diagnostic without check id")
		}
		if _, ok := diagnostics[id]; ok {
			return fmt.Errorf("snapshot contains duplicate diagnostic %q", id)
		}
		diagnostics[id] = struct{}{}
	}
	inventory := map[string]struct{}{}
	for _, item := range snapshot.Inventory {
		if strings.TrimSpace(item.Kind) == "" || strings.TrimSpace(item.RuntimeItemID) == "" {
			return fmt.Errorf("snapshot contains inventory without kind or runtime id")
		}
		key := item.Kind + ":" + item.RuntimeItemID
		if _, ok := inventory[key]; ok {
			return fmt.Errorf("snapshot contains duplicate inventory item %q", key)
		}
		inventory[key] = struct{}{}
	}
	capabilities := map[string]struct{}{}
	for _, item := range snapshot.CapabilityCatalog {
		if strings.TrimSpace(item.RuntimeCapabilityID) == "" || strings.TrimSpace(item.Version) == "" {
			return fmt.Errorf("snapshot contains capability without runtime id or version")
		}
		key := item.RuntimeCapabilityID + ":" + item.Version
		if _, ok := capabilities[key]; ok {
			return fmt.Errorf("snapshot contains duplicate capability %q", key)
		}
		capabilities[key] = struct{}{}
	}
	delegations := map[string]struct{}{}
	for _, item := range snapshot.AgentDelegations {
		orchestrator := strings.TrimSpace(item.OrchestratorRuntimeAgentID)
		delegate := strings.TrimSpace(item.DelegateRuntimeAgentID)
		ref := strings.TrimSpace(item.DelegateRef)
		if orchestrator == "" || (delegate == "" && ref == "") {
			return fmt.Errorf("snapshot contains delegation without orchestrator or delegate identity")
		}
		key := orchestrator + ":ref:" + ref
		if ref == "" {
			key = orchestrator + ":agent:" + delegate
		}
		if _, ok := delegations[key]; ok {
			return fmt.Errorf("snapshot contains duplicate agent delegation %q", key)
		}
		delegations[key] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, item := range snapshot.Agents {
		id := strings.TrimSpace(item.Agent.RuntimeAgentID)
		if id == "" {
			return fmt.Errorf("snapshot contains an agent without runtime id")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("snapshot contains duplicate runtime agent id %q", id)
		}
		seen[id] = struct{}{}
		if item.Access.AgentID != "" && item.Access.AgentID != id {
			return fmt.Errorf("access document agent id %q does not match %q", item.Access.AgentID, id)
		}
		skills := map[string]struct{}{}
		for _, skill := range item.Skills {
			if strings.TrimSpace(skill.RuntimeSkillID) == "" {
				return fmt.Errorf("agent %q has skill without runtime id", id)
			}
			if _, ok := skills[skill.RuntimeSkillID]; ok {
				return fmt.Errorf("agent %q has duplicate skill %q", id, skill.RuntimeSkillID)
			}
			skills[skill.RuntimeSkillID] = struct{}{}
		}
	}
	executions := map[string]struct{}{}
	for _, execution := range snapshot.Executions {
		id := strings.TrimSpace(execution.RuntimeExecutionID)
		if id == "" || strings.TrimSpace(execution.Kind) == "" {
			return fmt.Errorf("snapshot contains a runtime execution without kind or runtime id")
		}
		key := execution.Kind + ":" + id
		if _, ok := executions[key]; ok {
			return fmt.Errorf("snapshot contains duplicate runtime execution %q", key)
		}
		executions[key] = struct{}{}
	}
	executions = map[string]struct{}{}
	for _, execution := range snapshot.SubagentExecutions {
		id := strings.TrimSpace(execution.RuntimeExecutionID)
		if id == "" {
			return fmt.Errorf("snapshot contains a subagent execution without runtime id")
		}
		if _, ok := executions[id]; ok {
			return fmt.Errorf("snapshot contains duplicate subagent execution id %q", id)
		}
		executions[id] = struct{}{}
	}
	return nil
}

func (s RuntimeSyncService) auditSync(ctx context.Context, run domain.RuntimeSyncRun, input SyncRuntimeInput, eventType, result string) {
	if s.audit == nil {
		return
	}
	actor := strings.TrimSpace(input.Actor)
	if actor == "" {
		actor = "capcom-sync-worker"
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		reason = "scheduled runtime synchronization"
	}
	_, _ = s.audit.Create(ctx, domain.AuditEvent{RuntimeConnectionID: run.RuntimeConnectionID, Actor: actor,
		EventType: eventType, TargetType: "runtime_sync_run", TargetID: run.ID, Reason: reason, Result: result,
		Metadata: map[string]any{"trigger": run.Trigger, "status": run.Status, "agents_seen": run.AgentsSeen,
			"skills_seen": run.SkillsSeen, "delegations_seen": run.DelegationsSeen,
			"duration_ms": run.DurationMS, "error_code": run.ErrorCode}})
}

func sanitizeRuntimeError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	return message
}

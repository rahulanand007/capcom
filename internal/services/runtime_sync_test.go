package services

import (
	"context"
	"errors"
	"testing"
	"time"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
)

func TestRuntimeSyncServicePersistsCompleteSnapshot(t *testing.T) {
	store := &fakeSyncStore{lock: true}
	service := NewRuntimeSyncService(fakeRuntimeRepo{}, store, fakeAuditRepo{}, 3).WithAdapter(snapshotAdapter{})
	run, err := service.Sync(context.Background(), SyncRuntimeInput{RuntimeConnectionID: "runtime-1", Trigger: domain.SyncTriggerManual, Actor: "tester", Reason: "verify sync"})
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}
	if run.Status != domain.SyncStatusSucceeded || store.persisted == nil {
		t.Fatalf("run = %#v", run)
	}
	if len(store.persisted.Agents) != 1 || store.persisted.Agents[0].Skills[0].RuntimeSkillID != "skill-1" {
		t.Fatalf("snapshot = %#v", store.persisted)
	}
}

func TestRuntimeSyncServicePreservesStateWhenAdapterFails(t *testing.T) {
	store := &fakeSyncStore{lock: true}
	service := NewRuntimeSyncService(fakeRuntimeRepo{}, store, fakeAuditRepo{}, 3).WithAdapter(snapshotAdapter{err: errors.New("runtime unavailable")})
	run, err := service.Sync(context.Background(), SyncRuntimeInput{RuntimeConnectionID: "runtime-1", Trigger: domain.SyncTriggerManual, Actor: "tester", Reason: "verify failure"})
	if err == nil {
		t.Fatal("Sync returned nil error")
	}
	if run.Status != domain.SyncStatusFailed || store.persisted != nil || !store.failed {
		t.Fatalf("run=%#v persisted=%#v", run, store.persisted)
	}
}

func TestRuntimeSyncServiceRejectsOverlap(t *testing.T) {
	service := NewRuntimeSyncService(fakeRuntimeRepo{}, &fakeSyncStore{}, nil, 3).WithAdapter(snapshotAdapter{})
	_, err := service.Sync(context.Background(), SyncRuntimeInput{RuntimeConnectionID: "runtime-1", Trigger: domain.SyncTriggerManual, Actor: "tester", Reason: "overlap"})
	if !errors.Is(err, ErrSyncConflict) {
		t.Fatalf("error = %v", err)
	}
}

type snapshotAdapter struct{ err error }

func (snapshotAdapter) Kind() domain.RuntimeKind { return domain.RuntimeKindGantry }
func (snapshotAdapter) Check(context.Context, domain.RuntimeConnection) (*runtimeadapter.CheckResult, error) {
	return &runtimeadapter.CheckResult{}, nil
}
func (snapshotAdapter) ListAgents(context.Context, domain.RuntimeConnection) ([]domain.AgentSnapshot, error) {
	return nil, nil
}
func (snapshotAdapter) ListAgentSkills(context.Context, domain.RuntimeConnection, string) ([]domain.AgentSkillSnapshot, error) {
	return nil, nil
}
func (snapshotAdapter) GetAgentAccess(context.Context, domain.RuntimeConnection, string) (*domain.AccessDocument, error) {
	return nil, nil
}
func (snapshotAdapter) ReplaceAgentAccess(context.Context, domain.RuntimeConnection, string, domain.AccessDocument) (*domain.AccessDocument, error) {
	return nil, nil
}
func (snapshotAdapter) SetAgentStatus(context.Context, domain.RuntimeConnection, string, domain.AgentStatus) (*domain.AgentSnapshot, error) {
	return &domain.AgentSnapshot{}, nil
}
func (snapshotAdapter) DeleteAgent(context.Context, domain.RuntimeConnection, string) error {
	return nil
}
func (snapshotAdapter) CancelExecution(context.Context, domain.RuntimeConnection, domain.RuntimeExecutionSnapshot) error {
	return nil
}
func (a snapshotAdapter) CollectSnapshot(context.Context, domain.RuntimeConnection) (*domain.RuntimeSnapshot, error) {
	if a.err != nil {
		return nil, a.err
	}
	now := time.Now().UTC()
	return &domain.RuntimeSnapshot{ObservedAt: now, Agents: []domain.SnapshotAgent{{Agent: domain.AgentSnapshot{RuntimeAgentID: "agent-1", Name: "Agent", Status: domain.AgentStatusEnabled, ObservedAt: now}, Skills: []domain.AgentSkillSnapshot{{RuntimeSkillID: "skill-1", Name: "Skill", ObservedAt: now}}, Access: domain.AccessDocument{AgentID: "agent-1", ObservedAt: now}}}}, nil
}

type fakeSyncStore struct {
	lock      bool
	failed    bool
	persisted *domain.RuntimeSnapshot
}

func (s *fakeSyncStore) TryLock(context.Context, string) (func(), bool, error) {
	return func() {}, s.lock, nil
}
func (s *fakeSyncStore) CreateRun(_ context.Context, run domain.RuntimeSyncRun) (domain.RuntimeSyncRun, error) {
	run.ID = "run-1"
	run.Status = domain.SyncStatusRunning
	run.StartedAt = time.Now().UTC()
	return run, nil
}
func (s *fakeSyncStore) FailRun(_ context.Context, run domain.RuntimeSyncRun, code, message string) (domain.RuntimeSyncRun, error) {
	s.failed = true
	run.Status = domain.SyncStatusFailed
	run.ErrorCode = code
	run.ErrorMessage = message
	return run, nil
}
func (s *fakeSyncStore) PersistSnapshot(_ context.Context, run domain.RuntimeSyncRun, snapshot domain.RuntimeSnapshot, _ int) (domain.RuntimeSyncRun, error) {
	s.persisted = &snapshot
	run.Status = domain.SyncStatusSucceeded
	return run, nil
}
func (*fakeSyncStore) ListRuns(context.Context, string, int) ([]domain.RuntimeSyncRun, error) {
	return nil, nil
}
func (*fakeSyncStore) GetRun(context.Context, string, string) (domain.RuntimeSyncRun, error) {
	return domain.RuntimeSyncRun{}, nil
}
func (*fakeSyncStore) ListPersistedAgents(context.Context, string) ([]domain.PersistedAgent, error) {
	return nil, nil
}
func (*fakeSyncStore) GetPersistedAgent(context.Context, string) (domain.PersistedAgentDetail, error) {
	return domain.PersistedAgentDetail{}, nil
}
func (*fakeSyncStore) MarkAgentDeleted(context.Context, string) error { return nil }
func (*fakeSyncStore) ListSubagentExecutions(context.Context, string, string) ([]domain.PersistedSubagentExecution, error) {
	return nil, nil
}

func (*fakeSyncStore) ListRuntimeExecutions(context.Context, string, string, string, int) ([]domain.PersistedRuntimeExecution, error) {
	return nil, nil
}
func (*fakeSyncStore) GetRuntimeExecution(context.Context, string) (domain.PersistedRuntimeExecution, error) {
	return domain.PersistedRuntimeExecution{}, nil
}
func (*fakeSyncStore) ListRuntimeDiagnostics(context.Context, string) ([]domain.PersistedRuntimeDiagnostic, error) {
	return nil, nil
}
func (*fakeSyncStore) ListRuntimeInventory(context.Context, string, string) ([]domain.PersistedRuntimeInventory, error) {
	return nil, nil
}
func (*fakeSyncStore) ListRuntimeCapabilities(context.Context, string) ([]domain.PersistedRuntimeCapability, error) {
	return nil, nil
}
func (*fakeSyncStore) ListAgentDelegations(context.Context, string, string) ([]domain.PersistedAgentDelegation, error) {
	return nil, nil
}

func TestValidateRuntimeSnapshotRejectsDuplicateDelegationRefs(t *testing.T) {
	snapshot := domain.RuntimeSnapshot{AgentDelegations: []domain.AgentDelegationSnapshot{
		{OrchestratorRuntimeAgentID: "agent:main", DelegateRuntimeAgentID: "agent:first", DelegateRef: "reviewer"},
		{OrchestratorRuntimeAgentID: "agent:main", DelegateRuntimeAgentID: "agent:second", DelegateRef: "reviewer"},
	}}
	if err := validateRuntimeSnapshot(snapshot); err == nil {
		t.Fatal("expected duplicate canonical delegate reference to be rejected")
	}
}

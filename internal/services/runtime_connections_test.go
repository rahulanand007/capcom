package services

import (
	"context"
	"fmt"
	"testing"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
)

func TestRuntimeConnectionServiceCreateValidatesInput(t *testing.T) {
	service := NewRuntimeConnectionService(fakeRuntimeRepo{}, nil).WithCredentialResolver(fakeCredentialResolver{})

	_, err := service.Create(context.Background(), CreateRuntimeConnectionInput{
		Name:     "",
		Kind:     domain.RuntimeKindGantry,
		Mode:     domain.RuntimeModeReadOnly,
		Endpoint: "http://127.0.0.1:3000",
		AuthRef:  "gantry-key",
	})
	if err == nil {
		t.Fatal("Create returned nil error")
	}
}

func TestRuntimeConnectionServiceCreate(t *testing.T) {
	service := NewRuntimeConnectionService(fakeRuntimeRepo{}, fakeAuditRepo{}).WithCredentialResolver(fakeCredentialResolver{})

	conn, err := service.Create(context.Background(), CreateRuntimeConnectionInput{
		Name:        "local-gantry",
		DisplayName: "Gantry Development",
		Environment: "development",
		Labels:      map[string]string{"Team": "Platform"},
		Kind:        domain.RuntimeKindGantry,
		Mode:        domain.RuntimeModeReadOnly,
		Endpoint:    "http://127.0.0.1:3000",
		AuthRef:     "gantry-key",
		Actor:       "test",
		Reason:      "integration setup",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if conn.Status != domain.RuntimeStatusPending {
		t.Fatalf("status = %q, want %q", conn.Status, domain.RuntimeStatusPending)
	}
	if conn.DisplayName != "Gantry Development" || conn.Environment != "development" || conn.Labels["team"] != "Platform" {
		t.Fatalf("instance identity = %#v", conn)
	}
}

func TestRuntimeConnectionServiceCreateAllowsLangGraphControlMode(t *testing.T) {
	service := NewRuntimeConnectionService(fakeRuntimeRepo{}, nil).WithCredentialResolver(fakeCredentialResolver{})

	conn, err := service.Create(context.Background(), CreateRuntimeConnectionInput{
		Name: "local-langgraph", Kind: domain.RuntimeKindLangGraph,
		Mode: domain.RuntimeModeControlEnabled, Endpoint: "http://127.0.0.1:2024",
		AuthRef: "langgraph-key", Actor: "test", Reason: "enable controls",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if conn.Mode != domain.RuntimeModeControlEnabled {
		t.Fatalf("mode = %q, want %q", conn.Mode, domain.RuntimeModeControlEnabled)
	}
}

func TestRuntimeConnectionServiceUpdateSettings(t *testing.T) {
	service := NewRuntimeConnectionService(fakeRuntimeRepo{}, fakeAuditRepo{}).WithCredentialResolver(fakeCredentialResolver{})

	conn, err := service.UpdateSettings(context.Background(), UpdateRuntimeInstanceSettingsInput{
		ID: "runtime-1", DisplayName: "Gantry Production", Environment: "production",
		Labels: map[string]string{"Team": "Platform"}, Mode: domain.RuntimeModeControlEnabled,
		Endpoint: "HTTP://127.0.0.1:8787/", AuthRef: "gantry-key",
		Description: "Primary control runtime", SyncEnabled: true, SyncIntervalSeconds: 90,
		Actor: "test", Reason: "promote runtime",
	})
	if err != nil {
		t.Fatalf("UpdateSettings returned error: %v", err)
	}
	if conn.DisplayName != "Gantry Production" || conn.Environment != "production" {
		t.Fatalf("identity = %#v", conn)
	}
	if conn.Mode != domain.RuntimeModeControlEnabled || conn.BaseURL != "http://127.0.0.1:8787" {
		t.Fatalf("connection settings = %#v", conn)
	}
	if !conn.SyncEnabled || conn.SyncIntervalSeconds != 90 || conn.Metadata["description"] != "Primary control runtime" {
		t.Fatalf("sync settings = %#v", conn)
	}
}

func TestRuntimeConnectionServiceUpdateSettingsValidatesSyncInterval(t *testing.T) {
	service := NewRuntimeConnectionService(fakeRuntimeRepo{}, nil).WithCredentialResolver(fakeCredentialResolver{})
	_, err := service.UpdateSettings(context.Background(), UpdateRuntimeInstanceSettingsInput{
		ID: "runtime-1", DisplayName: "Gantry", Environment: "development",
		Mode: domain.RuntimeModeReadOnly, Endpoint: "http://127.0.0.1:8787",
		AuthRef: "gantry-key", SyncIntervalSeconds: 5, Actor: "test", Reason: "test",
	})
	if err == nil {
		t.Fatal("UpdateSettings returned nil error")
	}
}

func TestRuntimeConnectionServiceRemoveArchivesWithConfirmation(t *testing.T) {
	repository := &archivingRuntimeRepo{}
	service := NewRuntimeConnectionService(repository, fakeAuditRepo{})

	err := service.Remove(context.Background(), RemoveRuntimeInstanceInput{
		ID: "runtime-1", Confirmation: "runtime", Actor: "test",
		Reason: "duplicate connection",
	})
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if repository.archivedID != "runtime-1" {
		t.Fatalf("archived id = %q, want runtime-1", repository.archivedID)
	}
}

func TestRuntimeConnectionServiceRemoveRejectsWrongConfirmation(t *testing.T) {
	repository := &archivingRuntimeRepo{}
	service := NewRuntimeConnectionService(repository, nil)

	err := service.Remove(context.Background(), RemoveRuntimeInstanceInput{
		ID: "runtime-1", Confirmation: "wrong", Actor: "test",
		Reason: "duplicate connection",
	})
	if err == nil {
		t.Fatal("Remove returned nil error")
	}
	if repository.archivedID != "" {
		t.Fatalf("archived id = %q, want empty", repository.archivedID)
	}
}

func TestRuntimeConnectionServiceConsolidatesDuplicateAsEndpoint(t *testing.T) {
	repository := &consolidatingRuntimeRepo{}
	service := NewRuntimeConnectionService(repository, fakeAuditRepo{})

	conn, err := service.ConsolidateEndpoint(context.Background(), ConsolidateRuntimeEndpointInput{
		CanonicalID: "runtime-1", DuplicateID: "runtime-2",
		Kind: domain.RuntimeEndpointRelay, Confirmation: "runtime",
		Actor: "test", Reason: "same runtime through local relay",
	})
	if err != nil {
		t.Fatalf("ConsolidateEndpoint returned error: %v", err)
	}
	if conn.ID != "runtime-1" || repository.canonicalID != "runtime-1" ||
		repository.duplicateID != "runtime-2" || repository.kind != domain.RuntimeEndpointRelay {
		t.Fatalf("consolidation = %#v, repository = %#v", conn, repository)
	}
}

func TestRuntimeConnectionServiceRoutesSameAgentIDToSelectedInstance(t *testing.T) {
	adapter := &recordingAdapter{}
	service := NewRuntimeConnectionService(multiRuntimeRepo{}, nil).WithAdapter(adapter)

	if _, err := service.ListAgents(context.Background(), "runtime-staging"); err != nil {
		t.Fatalf("ListAgents returned error: %v", err)
	}
	if adapter.connection.ID != "runtime-staging" || adapter.connection.BaseURL != "http://127.0.0.1:8788" || adapter.connection.AuthRef != "gantry-staging-key" {
		t.Fatalf("adapter received wrong instance: %#v", adapter.connection)
	}
}

func TestRuntimeConnectionServiceTest(t *testing.T) {
	service := NewRuntimeConnectionService(fakeRuntimeRepo{}, nil).WithCredentialResolver(fakeCredentialResolver{}).WithAdapter(fakeAdapter{})

	got, err := service.Test(context.Background(), "runtime-1")
	if err != nil {
		t.Fatalf("Test returned error: %v", err)
	}
	if got.Status != domain.RuntimeStatusActive {
		t.Fatalf("status = %q, want active", got.Status)
	}
}

func TestRuntimeConnectionServiceReadsAgentsThroughAdapter(t *testing.T) {
	service := NewRuntimeConnectionService(fakeRuntimeRepo{}, nil).WithAdapter(fakeAdapter{})

	agents, err := service.ListAgents(context.Background(), "runtime-1")
	if err != nil {
		t.Fatalf("ListAgents returned error: %v", err)
	}
	if len(agents) != 1 || agents[0].RuntimeAgentID != "agent-1" {
		t.Fatalf("agents = %#v", agents)
	}

	access, err := service.GetAgentAccess(context.Background(), "runtime-1", "agent-1")
	if err != nil {
		t.Fatalf("GetAgentAccess returned error: %v", err)
	}
	if access.AgentID != "agent-1" {
		t.Fatalf("agent id = %q", access.AgentID)
	}

	skills, err := service.ListAgentSkills(context.Background(), "runtime-1", "agent-1")
	if err != nil {
		t.Fatalf("ListAgentSkills returned error: %v", err)
	}
	if len(skills) != 1 || skills[0].RuntimeSkillID != "skill-1" {
		t.Fatalf("skills = %#v", skills)
	}
}

type fakeRuntimeRepo struct{}

func (fakeRuntimeRepo) Create(_ context.Context, conn domain.RuntimeConnection) (domain.RuntimeConnection, error) {
	conn.ID = "runtime-1"
	return conn, nil
}

func (fakeRuntimeRepo) UpdateSettings(_ context.Context, conn domain.RuntimeConnection) (domain.RuntimeConnection, error) {
	return conn, nil
}

func (fakeRuntimeRepo) Get(_ context.Context, id string) (domain.RuntimeConnection, error) {
	return domain.RuntimeConnection{ID: id, Name: "runtime", Kind: domain.RuntimeKindGantry, AuthRef: "gantry-key", Metadata: map[string]any{}}, nil
}

func (fakeRuntimeRepo) List(context.Context) ([]domain.RuntimeConnection, error) {
	return []domain.RuntimeConnection{{ID: "runtime-1", Name: "runtime"}}, nil
}

type archivingRuntimeRepo struct {
	fakeRuntimeRepo
	archivedID string
}

type consolidatingRuntimeRepo struct {
	fakeRuntimeRepo
	canonicalID string
	duplicateID string
	kind        domain.RuntimeEndpointKind
}

func (r *consolidatingRuntimeRepo) ConsolidateEndpoint(
	_ context.Context,
	canonicalID, duplicateID string,
	kind domain.RuntimeEndpointKind,
	_, _ string,
) error {
	r.canonicalID = canonicalID
	r.duplicateID = duplicateID
	r.kind = kind
	return nil
}

func (r *archivingRuntimeRepo) Archive(_ context.Context, id, _, _ string) error {
	r.archivedID = id
	return nil
}

type multiRuntimeRepo struct{}

func (multiRuntimeRepo) Create(context.Context, domain.RuntimeConnection) (domain.RuntimeConnection, error) {
	return domain.RuntimeConnection{}, fmt.Errorf("not implemented")
}
func (multiRuntimeRepo) UpdateSettings(_ context.Context, conn domain.RuntimeConnection) (domain.RuntimeConnection, error) {
	return conn, nil
}
func (multiRuntimeRepo) Get(_ context.Context, id string) (domain.RuntimeConnection, error) {
	ports := map[string]string{"runtime-dev": "8787", "runtime-staging": "8788"}
	port, ok := ports[id]
	if !ok {
		return domain.RuntimeConnection{}, fmt.Errorf("unknown runtime %q", id)
	}
	return domain.RuntimeConnection{ID: id, Kind: domain.RuntimeKindGantry, BaseURL: "http://127.0.0.1:" + port, AuthRef: "gantry-" + id[len("runtime-"):] + "-key"}, nil
}
func (multiRuntimeRepo) List(context.Context) ([]domain.RuntimeConnection, error) { return nil, nil }

type fakeAuditRepo struct{}

func (fakeAuditRepo) Create(_ context.Context, event domain.AuditEvent) (domain.AuditEvent, error) {
	event.ID = "audit-1"
	return event, nil
}

type fakeAdapter struct{}

type recordingAdapter struct{ connection domain.RuntimeConnection }

func (a *recordingAdapter) Kind() domain.RuntimeKind { return domain.RuntimeKindGantry }
func (a *recordingAdapter) Check(_ context.Context, conn domain.RuntimeConnection) (*runtimeadapter.CheckResult, error) {
	a.connection = conn
	return &runtimeadapter.CheckResult{}, nil
}
func (a *recordingAdapter) ListAgents(_ context.Context, conn domain.RuntimeConnection) ([]domain.AgentSnapshot, error) {
	a.connection = conn
	return []domain.AgentSnapshot{{RuntimeAgentID: "agent:main_agent"}}, nil
}
func (a *recordingAdapter) ListAgentSkills(_ context.Context, conn domain.RuntimeConnection, _ string) ([]domain.AgentSkillSnapshot, error) {
	a.connection = conn
	return nil, nil
}
func (a *recordingAdapter) GetAgentAccess(_ context.Context, conn domain.RuntimeConnection, _ string) (*domain.AccessDocument, error) {
	a.connection = conn
	return &domain.AccessDocument{}, nil
}
func (a *recordingAdapter) ReplaceAgentAccess(_ context.Context, conn domain.RuntimeConnection, _ string, _ domain.AccessDocument) (*domain.AccessDocument, error) {
	a.connection = conn
	return &domain.AccessDocument{}, nil
}
func (a *recordingAdapter) SetAgentStatus(_ context.Context, conn domain.RuntimeConnection, _ string, _ domain.AgentStatus) (*domain.AgentSnapshot, error) {
	a.connection = conn
	return &domain.AgentSnapshot{}, nil
}
func (a *recordingAdapter) DeleteAgent(_ context.Context, conn domain.RuntimeConnection, _ string) error {
	a.connection = conn
	return nil
}
func (a *recordingAdapter) CancelExecution(_ context.Context, conn domain.RuntimeConnection, _ domain.RuntimeExecutionSnapshot) error {
	a.connection = conn
	return nil
}
func (a *recordingAdapter) CollectSnapshot(_ context.Context, conn domain.RuntimeConnection) (*domain.RuntimeSnapshot, error) {
	a.connection = conn
	return &domain.RuntimeSnapshot{}, nil
}

type fakeCredentialResolver struct{}

func (fakeCredentialResolver) Resolve(context.Context, string) (string, error) {
	return "token", nil
}

func (fakeAdapter) Kind() domain.RuntimeKind {
	return domain.RuntimeKindGantry
}

func (fakeAdapter) Check(context.Context, domain.RuntimeConnection) (*runtimeadapter.CheckResult, error) {
	return &runtimeadapter.CheckResult{Status: domain.RuntimeStatusActive}, nil
}

func (fakeAdapter) ListAgents(context.Context, domain.RuntimeConnection) ([]domain.AgentSnapshot, error) {
	return []domain.AgentSnapshot{{RuntimeAgentID: "agent-1"}}, nil
}

func (fakeAdapter) ListAgentSkills(context.Context, domain.RuntimeConnection, string) ([]domain.AgentSkillSnapshot, error) {
	return []domain.AgentSkillSnapshot{{RuntimeSkillID: "skill-1"}}, nil
}

func (fakeAdapter) GetAgentAccess(context.Context, domain.RuntimeConnection, string) (*domain.AccessDocument, error) {
	return &domain.AccessDocument{AgentID: "agent-1"}, nil
}

func (fakeAdapter) ReplaceAgentAccess(context.Context, domain.RuntimeConnection, string, domain.AccessDocument) (*domain.AccessDocument, error) {
	return nil, nil
}

func (fakeAdapter) SetAgentStatus(context.Context, domain.RuntimeConnection, string, domain.AgentStatus) (*domain.AgentSnapshot, error) {
	return &domain.AgentSnapshot{}, nil
}
func (fakeAdapter) DeleteAgent(context.Context, domain.RuntimeConnection, string) error { return nil }
func (fakeAdapter) CancelExecution(context.Context, domain.RuntimeConnection, domain.RuntimeExecutionSnapshot) error {
	return nil
}

func (fakeAdapter) CollectSnapshot(context.Context, domain.RuntimeConnection) (*domain.RuntimeSnapshot, error) {
	return &domain.RuntimeSnapshot{}, nil
}

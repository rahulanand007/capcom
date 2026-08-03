package runtime

import (
	"context"

	"capcom/internal/domain"
)

type CredentialResolver interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

type Adapter interface {
	Kind() domain.RuntimeKind
	Check(ctx context.Context, conn domain.RuntimeConnection) (*CheckResult, error)
	ListAgents(ctx context.Context, conn domain.RuntimeConnection) ([]domain.AgentSnapshot, error)
	ListAgentSkills(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string) ([]domain.AgentSkillSnapshot, error)
	GetAgentAccess(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string) (*domain.AccessDocument, error)
	ReplaceAgentAccess(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string, access domain.AccessDocument) (*domain.AccessDocument, error)
	SetAgentStatus(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string, status domain.AgentStatus) (*domain.AgentSnapshot, error)
	DeleteAgent(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string) error
	CancelExecution(ctx context.Context, conn domain.RuntimeConnection, execution domain.RuntimeExecutionSnapshot) error
	CollectSnapshot(ctx context.Context, conn domain.RuntimeConnection) (*domain.RuntimeSnapshot, error)
}

// UsageReader is an optional telemetry capability implemented beside Adapter.
// It does not participate in inventory, access, or control synchronization.
type UsageReader interface {
	QueryUsage(ctx context.Context, conn domain.RuntimeConnection, query domain.UsageQuery) ([]domain.UsageObservation, error)
}

// UsageReaderConfigurator lets optional connectors distinguish unavailable
// telemetry from a measured zero.
type UsageReaderConfigurator interface {
	TelemetryConfigured(conn domain.RuntimeConnection) bool
}

type CheckResult struct {
	Status       domain.RuntimeStatus
	Message      string
	Capabilities Capabilities
	Metadata     map[string]any
	Diagnostics  []domain.RuntimeDiagnosticSnapshot
}

type Capabilities struct {
	ReadAgents             bool `json:"read_agents"`
	ReadAgentHierarchy     bool `json:"read_agent_hierarchy"`
	ReadAgentDelegates     bool `json:"read_agent_delegates"`
	ReadAgentSkills        bool `json:"read_agent_skills"`
	ReadAgentAccess        bool `json:"read_agent_access"`
	ReplaceAgentAccess     bool `json:"replace_agent_access"`
	ReadSubagentExecutions bool `json:"read_subagent_executions"`
	ReadExecutions         bool `json:"read_executions"`
	ReadDiagnostics        bool `json:"read_diagnostics"`
	ReadInventory          bool `json:"read_inventory"`
	ReadCapabilityCatalog  bool `json:"read_capability_catalog"`
	SetAgentStatus         bool `json:"set_agent_status"`
	DeleteAgent            bool `json:"delete_agent"`
	CancelExecution        bool `json:"cancel_execution"`
}

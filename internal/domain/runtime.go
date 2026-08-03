package domain

import "time"

type RuntimeKind string

const (
	RuntimeKindGantry    RuntimeKind = "gantry"
	RuntimeKindLangGraph RuntimeKind = "langgraph"
)

type RuntimeMode string

const (
	RuntimeModeReadOnly       RuntimeMode = "read_only"
	RuntimeModeControlEnabled RuntimeMode = "control_enabled"
)

type RuntimeStatus string

const (
	RuntimeStatusPending  RuntimeStatus = "pending"
	RuntimeStatusActive   RuntimeStatus = "active"
	RuntimeStatusDegraded RuntimeStatus = "degraded"
	RuntimeStatusDisabled RuntimeStatus = "disabled"
	RuntimeStatusFailed   RuntimeStatus = "failed"
)

type RuntimeEndpointKind string

const (
	RuntimeEndpointCanonical RuntimeEndpointKind = "canonical"
	RuntimeEndpointAlias     RuntimeEndpointKind = "alias"
	RuntimeEndpointRelay     RuntimeEndpointKind = "relay"
)

type RuntimeEndpoint struct {
	ID                        string
	RuntimeConnectionID       string
	URL                       string
	Kind                      RuntimeEndpointKind
	AuthRef                   string
	SourceRuntimeConnectionID string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type RuntimeConnection struct {
	ID                  string
	Name                string
	DisplayName         string
	Environment         string
	Kind                RuntimeKind
	Mode                RuntimeMode
	Status              RuntimeStatus
	BaseURL             string
	AuthRef             string
	Metadata            map[string]any
	Labels              map[string]string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	LastSyncedAt        *time.Time
	SyncEnabled         bool
	SyncIntervalSeconds int
	LastSyncStatus      SyncStatus
	LastSyncStartedAt   *time.Time
	LastSyncFinishedAt  *time.Time
	LastSyncDurationMS  int64
	LastError           string
	Endpoints           []RuntimeEndpoint
}

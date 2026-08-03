package services

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
)

type RuntimeConnectionRepository interface {
	Create(ctx context.Context, conn domain.RuntimeConnection) (domain.RuntimeConnection, error)
	UpdateSettings(ctx context.Context, conn domain.RuntimeConnection) (domain.RuntimeConnection, error)
	Get(ctx context.Context, id string) (domain.RuntimeConnection, error)
	List(ctx context.Context) ([]domain.RuntimeConnection, error)
}

type AuditRepository interface {
	Create(ctx context.Context, event domain.AuditEvent) (domain.AuditEvent, error)
}

type RuntimeConnectionService struct {
	runtimes RuntimeConnectionRepository
	audit    AuditRepository
	secrets  runtimeadapter.CredentialResolver
	adapters map[domain.RuntimeKind]runtimeadapter.Adapter
}

type CreateRuntimeConnectionInput struct {
	Name        string
	DisplayName string
	Environment string
	Labels      map[string]string
	Kind        domain.RuntimeKind
	Mode        domain.RuntimeMode
	Endpoint    string
	Actor       string
	Reason      string
	Description string
	AuthRef     string
}

type UpdateRuntimeInstanceSettingsInput struct {
	ID                  string
	DisplayName         string
	Environment         string
	Labels              map[string]string
	Mode                domain.RuntimeMode
	Endpoint            string
	AuthRef             string
	Description         string
	SyncEnabled         bool
	SyncIntervalSeconds int
	Actor               string
	Reason              string
}

type RemoveRuntimeInstanceInput struct {
	ID           string
	Confirmation string
	Actor        string
	Reason       string
}

type ConsolidateRuntimeEndpointInput struct {
	CanonicalID  string
	DuplicateID  string
	Kind         domain.RuntimeEndpointKind
	Confirmation string
	Actor        string
	Reason       string
}

type runtimeConnectionArchiver interface {
	Archive(ctx context.Context, id, actor, reason string) error
}

type runtimeEndpointConsolidator interface {
	ConsolidateEndpoint(ctx context.Context, canonicalID, duplicateID string, kind domain.RuntimeEndpointKind, actor, reason string) error
}

func (s RuntimeConnectionService) WithCredentialResolver(resolver runtimeadapter.CredentialResolver) RuntimeConnectionService {
	s.secrets = resolver
	return s
}

func NewRuntimeConnectionService(runtimes RuntimeConnectionRepository, audit AuditRepository) RuntimeConnectionService {
	return RuntimeConnectionService{
		runtimes: runtimes,
		audit:    audit,
		adapters: map[domain.RuntimeKind]runtimeadapter.Adapter{},
	}
}

func (s RuntimeConnectionService) WithAdapter(adapter runtimeadapter.Adapter) RuntimeConnectionService {
	if adapter == nil {
		return s
	}
	if s.adapters == nil {
		s.adapters = map[domain.RuntimeKind]runtimeadapter.Adapter{}
	}
	s.adapters[adapter.Kind()] = adapter
	return s
}

func (s RuntimeConnectionService) Create(ctx context.Context, input CreateRuntimeConnectionInput) (domain.RuntimeConnection, error) {
	if s.runtimes == nil {
		return domain.RuntimeConnection{}, fmt.Errorf("runtime repository is required")
	}
	if err := validateCreateRuntimeConnection(input); err != nil {
		return domain.RuntimeConnection{}, err
	}
	if s.secrets == nil {
		return domain.RuntimeConnection{}, fmt.Errorf("credential resolver is required")
	}
	if _, err := s.secrets.Resolve(ctx, input.AuthRef); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("resolve auth_ref: %w", err)
	}

	endpoint, err := normalizeRuntimeEndpoint(input.Endpoint)
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(input.Name)
	}
	environment := strings.ToLower(strings.TrimSpace(input.Environment))
	if environment == "" {
		environment = "unspecified"
	}
	conn, err := s.runtimes.Create(ctx, domain.RuntimeConnection{
		Name:        strings.TrimSpace(input.Name),
		DisplayName: displayName,
		Environment: environment,
		Kind:        input.Kind,
		Mode:        input.Mode,
		Status:      domain.RuntimeStatusPending,
		BaseURL:     endpoint,
		AuthRef:     strings.TrimSpace(input.AuthRef),
		Labels:      normalizeLabels(input.Labels),
		Metadata: map[string]any{
			"description": strings.TrimSpace(input.Description),
		},
	})
	if err != nil {
		return domain.RuntimeConnection{}, err
	}

	if s.audit != nil && strings.TrimSpace(input.Actor) != "" && strings.TrimSpace(input.Reason) != "" {
		if _, err := s.audit.Create(ctx, domain.AuditEvent{
			RuntimeConnectionID: conn.ID,
			Actor:               strings.TrimSpace(input.Actor),
			EventType:           "runtime_connection.created",
			TargetType:          "runtime_connection",
			TargetID:            conn.ID,
			Reason:              strings.TrimSpace(input.Reason),
			Result:              "succeeded",
			After: map[string]any{
				"id":           conn.ID,
				"name":         conn.Name,
				"display_name": conn.DisplayName,
				"environment":  conn.Environment,
				"runtime_type": string(conn.Kind),
				"mode":         string(conn.Mode),
				"status":       string(conn.Status),
				"endpoint":     conn.BaseURL,
				"auth_ref":     conn.AuthRef,
			},
		}); err != nil {
			return domain.RuntimeConnection{}, fmt.Errorf("audit runtime connection creation: %w", err)
		}
	}

	return conn, nil
}

func (s RuntimeConnectionService) Get(ctx context.Context, id string) (domain.RuntimeConnection, error) {
	if strings.TrimSpace(id) == "" {
		return domain.RuntimeConnection{}, fmt.Errorf("id is required")
	}
	return s.runtimes.Get(ctx, strings.TrimSpace(id))
}

func (s RuntimeConnectionService) List(ctx context.Context) ([]domain.RuntimeConnection, error) {
	return s.runtimes.List(ctx)
}

func (s RuntimeConnectionService) UpdateSettings(ctx context.Context, input UpdateRuntimeInstanceSettingsInput) (domain.RuntimeConnection, error) {
	if strings.TrimSpace(input.ID) == "" || strings.TrimSpace(input.DisplayName) == "" {
		return domain.RuntimeConnection{}, fmt.Errorf("id and display_name are required")
	}
	environment := strings.ToLower(strings.TrimSpace(input.Environment))
	if !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`).MatchString(environment) {
		return domain.RuntimeConnection{}, fmt.Errorf("environment must start with a letter and contain only lowercase letters, digits, underscores, or hyphens")
	}
	if strings.TrimSpace(input.Actor) == "" || strings.TrimSpace(input.Reason) == "" {
		return domain.RuntimeConnection{}, fmt.Errorf("actor and reason are required")
	}
	switch input.Mode {
	case domain.RuntimeModeReadOnly, domain.RuntimeModeControlEnabled:
	default:
		return domain.RuntimeConnection{}, fmt.Errorf("mode must be one of %q, %q", domain.RuntimeModeReadOnly, domain.RuntimeModeControlEnabled)
	}
	endpoint, err := normalizeRuntimeEndpoint(input.Endpoint)
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	authRef := strings.TrimSpace(input.AuthRef)
	if authRef == "" {
		return domain.RuntimeConnection{}, fmt.Errorf("auth_ref is required")
	}
	if input.SyncIntervalSeconds < 15 || input.SyncIntervalSeconds > 86400 {
		return domain.RuntimeConnection{}, fmt.Errorf("sync_interval_seconds must be between 15 and 86400")
	}
	if s.secrets == nil {
		return domain.RuntimeConnection{}, fmt.Errorf("credential resolver is required")
	}
	if _, err := s.secrets.Resolve(ctx, authRef); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("resolve auth_ref: %w", err)
	}
	before, err := s.runtimes.Get(ctx, strings.TrimSpace(input.ID))
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	updated := before
	updated.DisplayName = strings.TrimSpace(input.DisplayName)
	updated.Environment = environment
	updated.Labels = normalizeLabels(input.Labels)
	updated.Mode = input.Mode
	updated.BaseURL = endpoint
	updated.AuthRef = authRef
	updated.SyncEnabled = input.SyncEnabled
	updated.SyncIntervalSeconds = input.SyncIntervalSeconds
	if updated.Metadata == nil {
		updated.Metadata = map[string]any{}
	}
	updated.Metadata["description"] = strings.TrimSpace(input.Description)
	updated, err = s.runtimes.UpdateSettings(ctx, updated)
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	if s.audit != nil {
		_, err = s.audit.Create(ctx, domain.AuditEvent{RuntimeConnectionID: updated.ID, Actor: strings.TrimSpace(input.Actor), EventType: "runtime_instance.settings_updated", TargetType: "runtime_instance", TargetID: updated.ID, Reason: strings.TrimSpace(input.Reason), Result: "succeeded", Before: instanceSettingsAudit(before), After: instanceSettingsAudit(updated)})
		if err != nil {
			return domain.RuntimeConnection{}, fmt.Errorf("audit runtime instance settings update: %w", err)
		}
	}
	return updated, nil
}

func (s RuntimeConnectionService) Remove(ctx context.Context, input RemoveRuntimeInstanceInput) error {
	id := strings.TrimSpace(input.ID)
	actor := strings.TrimSpace(input.Actor)
	reason := strings.TrimSpace(input.Reason)
	if id == "" || actor == "" || reason == "" {
		return fmt.Errorf("id, actor, and reason are required")
	}
	before, err := s.runtimes.Get(ctx, id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.Confirmation) != before.Name {
		return fmt.Errorf("confirmation must exactly match the stable key %q", before.Name)
	}
	archiver, ok := s.runtimes.(runtimeConnectionArchiver)
	if !ok {
		return fmt.Errorf("runtime repository does not support removal")
	}
	if err := archiver.Archive(ctx, id, actor, reason); err != nil {
		return err
	}
	if s.audit != nil {
		if _, err := s.audit.Create(ctx, domain.AuditEvent{
			RuntimeConnectionID: before.ID,
			Actor:               actor,
			EventType:           "runtime_instance.removed",
			TargetType:          "runtime_instance",
			TargetID:            before.ID,
			Reason:              reason,
			Result:              "succeeded",
			Before:              instanceSettingsAudit(before),
			After: map[string]any{
				"archived":     true,
				"sync_enabled": false,
				"status":       domain.RuntimeStatusDisabled,
			},
		}); err != nil {
			return fmt.Errorf("audit runtime instance removal: %w", err)
		}
	}
	return nil
}

func (s RuntimeConnectionService) ConsolidateEndpoint(
	ctx context.Context,
	input ConsolidateRuntimeEndpointInput,
) (domain.RuntimeConnection, error) {
	canonicalID := strings.TrimSpace(input.CanonicalID)
	duplicateID := strings.TrimSpace(input.DuplicateID)
	actor := strings.TrimSpace(input.Actor)
	reason := strings.TrimSpace(input.Reason)
	if canonicalID == "" || duplicateID == "" || actor == "" || reason == "" {
		return domain.RuntimeConnection{}, fmt.Errorf("canonical_id, duplicate_id, actor, and reason are required")
	}
	if canonicalID == duplicateID {
		return domain.RuntimeConnection{}, fmt.Errorf("canonical and duplicate runtime instances must be different")
	}
	switch input.Kind {
	case domain.RuntimeEndpointAlias, domain.RuntimeEndpointRelay:
	default:
		return domain.RuntimeConnection{}, fmt.Errorf("kind must be one of %q or %q", domain.RuntimeEndpointAlias, domain.RuntimeEndpointRelay)
	}
	canonical, err := s.runtimes.Get(ctx, canonicalID)
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	duplicate, err := s.runtimes.Get(ctx, duplicateID)
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	if canonical.Kind != duplicate.Kind {
		return domain.RuntimeConnection{}, fmt.Errorf("runtime types must match")
	}
	if strings.TrimSpace(input.Confirmation) != duplicate.Name {
		return domain.RuntimeConnection{}, fmt.Errorf("confirmation must exactly match the duplicate stable key %q", duplicate.Name)
	}
	consolidator, ok := s.runtimes.(runtimeEndpointConsolidator)
	if !ok {
		return domain.RuntimeConnection{}, fmt.Errorf("runtime repository does not support endpoint consolidation")
	}
	if err := consolidator.ConsolidateEndpoint(ctx, canonicalID, duplicateID, input.Kind, actor, reason); err != nil {
		return domain.RuntimeConnection{}, err
	}
	updated, err := s.runtimes.Get(ctx, canonicalID)
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	if s.audit != nil {
		if _, err := s.audit.Create(ctx, domain.AuditEvent{
			RuntimeConnectionID: canonical.ID,
			Actor:               actor,
			EventType:           "runtime_endpoint.consolidated",
			TargetType:          "runtime_endpoint",
			TargetID:            duplicate.ID,
			Reason:              reason,
			Result:              "succeeded",
			Before: map[string]any{
				"runtime_instance_id": duplicate.ID,
				"name":                duplicate.Name,
				"endpoint":            duplicate.BaseURL,
			},
			After: map[string]any{
				"canonical_runtime_instance_id": canonical.ID,
				"endpoint":                      duplicate.BaseURL,
				"kind":                          input.Kind,
			},
		}); err != nil {
			return domain.RuntimeConnection{}, fmt.Errorf("audit endpoint consolidation: %w", err)
		}
	}
	return updated, nil
}

func instanceSettingsAudit(conn domain.RuntimeConnection) map[string]any {
	return map[string]any{
		"name":                  conn.Name,
		"display_name":          conn.DisplayName,
		"environment":           conn.Environment,
		"labels":                conn.Labels,
		"description":           conn.Metadata["description"],
		"endpoint":              conn.BaseURL,
		"runtime_type":          conn.Kind,
		"mode":                  conn.Mode,
		"auth_ref":              conn.AuthRef,
		"sync_enabled":          conn.SyncEnabled,
		"sync_interval_seconds": conn.SyncIntervalSeconds,
	}
}

func (s RuntimeConnectionService) Test(ctx context.Context, id string) (*runtimeadapter.CheckResult, error) {
	conn, adapter, err := s.connectionAdapter(ctx, id)
	if err != nil {
		return nil, err
	}

	return adapter.Check(ctx, conn)
}

func (s RuntimeConnectionService) ListAgents(ctx context.Context, id string) ([]domain.AgentSnapshot, error) {
	conn, adapter, err := s.connectionAdapter(ctx, id)
	if err != nil {
		return nil, err
	}

	return adapter.ListAgents(ctx, conn)
}

func (s RuntimeConnectionService) GetAgentAccess(ctx context.Context, id string, runtimeAgentID string) (*domain.AccessDocument, error) {
	if strings.TrimSpace(runtimeAgentID) == "" {
		return nil, fmt.Errorf("runtime agent id is required")
	}
	conn, adapter, err := s.connectionAdapter(ctx, id)
	if err != nil {
		return nil, err
	}

	return adapter.GetAgentAccess(ctx, conn, strings.TrimSpace(runtimeAgentID))
}

func (s RuntimeConnectionService) ListAgentSkills(ctx context.Context, id string, runtimeAgentID string) ([]domain.AgentSkillSnapshot, error) {
	if strings.TrimSpace(runtimeAgentID) == "" {
		return nil, fmt.Errorf("runtime agent id is required")
	}
	conn, adapter, err := s.connectionAdapter(ctx, id)
	if err != nil {
		return nil, err
	}
	return adapter.ListAgentSkills(ctx, conn, strings.TrimSpace(runtimeAgentID))
}

func (s RuntimeConnectionService) connectionAdapter(ctx context.Context, id string) (domain.RuntimeConnection, runtimeadapter.Adapter, error) {
	conn, err := s.Get(ctx, id)
	if err != nil {
		return domain.RuntimeConnection{}, nil, err
	}
	adapter, ok := s.adapters[conn.Kind]
	if !ok {
		return domain.RuntimeConnection{}, nil, fmt.Errorf("runtime adapter %q is not registered", conn.Kind)
	}
	return conn, adapter, nil
}

func validateCreateRuntimeConnection(input CreateRuntimeConnectionInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("name is required")
	}
	switch input.Kind {
	case domain.RuntimeKindGantry, domain.RuntimeKindLangGraph:
	default:
		return fmt.Errorf("runtime_type must be one of %q, %q", domain.RuntimeKindGantry, domain.RuntimeKindLangGraph)
	}
	switch input.Mode {
	case domain.RuntimeModeReadOnly, domain.RuntimeModeControlEnabled:
	default:
		return fmt.Errorf("mode must be one of %q, %q", domain.RuntimeModeReadOnly, domain.RuntimeModeControlEnabled)
	}
	if strings.TrimSpace(input.Endpoint) == "" {
		return fmt.Errorf("endpoint is required")
	}
	environment := strings.ToLower(strings.TrimSpace(input.Environment))
	if environment != "" && !regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`).MatchString(environment) {
		return fmt.Errorf("environment must start with a letter and contain only lowercase letters, digits, underscores, or hyphens")
	}
	if strings.TrimSpace(input.AuthRef) == "" {
		return fmt.Errorf("auth_ref is required")
	}
	if strings.TrimSpace(input.Actor) == "" {
		return fmt.Errorf("actor is required")
	}
	if strings.TrimSpace(input.Reason) == "" {
		return fmt.Errorf("reason is required")
	}
	return nil
}

func normalizeRuntimeEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("endpoint must be an absolute http or https URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeLabels(labels map[string]string) map[string]string {
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

package gantry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
)

const defaultTimeout = 10 * time.Second

type Client struct {
	httpClient  *http.Client
	timeout     time.Duration
	credentials runtimeadapter.CredentialResolver
}

func NewClient(httpClient *http.Client, credentials runtimeadapter.CredentialResolver) Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return Client{
		httpClient:  httpClient,
		timeout:     defaultTimeout,
		credentials: credentials,
	}
}

func (c Client) Kind() domain.RuntimeKind {
	return domain.RuntimeKindGantry
}

func (c Client) Check(ctx context.Context, conn domain.RuntimeConnection) (*runtimeadapter.CheckResult, error) {
	var health map[string]any
	if err := c.doJSON(ctx, conn, http.MethodGet, "/v1/health", nil, &health); err != nil {
		return nil, err
	}
	var doctor gantryDoctor
	if err := c.doJSON(ctx, conn, http.MethodGet, "/v1/doctor", nil, &doctor); err != nil {
		return nil, fmt.Errorf("gantry doctor failed: %w", err)
	}
	diagnostics, status := normalizeDoctor(doctor, time.Now().UTC())
	message := "gantry health and doctor checks succeeded"
	if status == domain.RuntimeStatusDegraded {
		message = "gantry doctor reported warnings"
	}
	if status == domain.RuntimeStatusFailed {
		message = "gantry doctor reported failing checks"
	}

	return &runtimeadapter.CheckResult{
		Status:  status,
		Message: message,
		Capabilities: runtimeadapter.Capabilities{
			ReadAgents:             true,
			ReadAgentHierarchy:     false,
			ReadAgentDelegates:     true,
			ReadAgentSkills:        true,
			ReadAgentAccess:        true,
			ReplaceAgentAccess:     conn.Mode == domain.RuntimeModeControlEnabled,
			ReadSubagentExecutions: true,
			ReadDiagnostics:        true,
			ReadInventory:          true,
			ReadCapabilityCatalog:  true,
			SetAgentStatus:         conn.Mode == domain.RuntimeModeControlEnabled,
		},
		Metadata:    map[string]any{"health": health, "doctor_status": doctor.Status},
		Diagnostics: diagnostics,
	}, nil
}

func (c Client) ListAgents(ctx context.Context, conn domain.RuntimeConnection) ([]domain.AgentSnapshot, error) {
	var payload json.RawMessage
	if err := c.doJSON(ctx, conn, http.MethodGet, "/v1/agents", nil, &payload); err != nil {
		return nil, err
	}
	agents, err := decodeAgentList(payload)
	if err != nil {
		return nil, fmt.Errorf("decode gantry agent list: %w", err)
	}

	snapshots := make([]domain.AgentSnapshot, 0, len(agents))
	observedAt := time.Now().UTC()
	for _, agent := range agents {
		kind := gantryAgentKind(agent.ID)
		if agent.ParentID != "" {
			kind = domain.AgentKindSubagent
		}
		snapshots = append(snapshots, domain.AgentSnapshot{
			RuntimeAgentID:       agent.ID,
			ParentRuntimeAgentID: agent.ParentID,
			Kind:                 kind,
			Name:                 agent.DisplayName(),
			Status:               agent.StatusDomain(),
			Metadata: map[string]any{
				"description":               agent.Description,
				"app_id":                    agent.AppID,
				"agent_harness":             agent.Harness,
				"current_config_version_id": agent.ConfigID,
				"runtime_created_at":        agent.CreatedAt,
				"runtime_updated_at":        agent.UpdatedAt,
			},
			ObservedAt: observedAt,
		})
	}
	return snapshots, nil
}

func (c Client) ListAgentDelegates(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string) ([]domain.AgentDelegationSnapshot, error) {
	var payload gantryAgentDelegates
	path := fmt.Sprintf("/v1/agents/%s/delegates", url.PathEscape(runtimeAgentID))
	if err := c.doJSON(ctx, conn, http.MethodGet, path, nil, &payload); err != nil {
		return nil, err
	}

	observedAt := time.Now().UTC()
	configured := make(map[string]struct{}, len(payload.Delegates))
	for _, ref := range payload.Delegates {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			configured[ref] = struct{}{}
		}
	}

	items := make([]domain.AgentDelegationSnapshot, 0, len(payload.Resolved)+len(configured))
	seenRefs := make(map[string]struct{}, len(payload.Resolved))
	for _, delegate := range payload.Resolved {
		ref := strings.TrimSpace(delegate.Ref)
		delegateID := strings.TrimSpace(delegate.AgentID)
		if ref == "" {
			ref = delegateID
		}
		if ref == "" || delegateID == "" {
			continue
		}
		_, isConfigured := configured[ref]
		seenRefs[ref] = struct{}{}
		items = append(items, domain.AgentDelegationSnapshot{
			OrchestratorRuntimeAgentID: runtimeAgentID,
			DelegateRuntimeAgentID:     delegateID,
			DelegateRef:                ref,
			ToolName:                   delegate.ToolName,
			DisplayName:                delegate.DisplayName,
			Persona:                    delegate.Persona,
			Configured:                 isConfigured,
			Resolved:                   true,
			Revision:                   payload.Revision,
			Status:                     "active",
			ObservedAt:                 observedAt,
			Metadata:                   map[string]any{"source": delegationSource(isConfigured)},
			Raw:                        delegate.Raw,
		})
	}
	for _, ref := range payload.Delegates {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seenRefs[ref]; ok {
			continue
		}
		items = append(items, domain.AgentDelegationSnapshot{
			OrchestratorRuntimeAgentID: runtimeAgentID,
			DelegateRuntimeAgentID:     gantryAgentIDForDelegateRef(ref),
			DelegateRef:                ref,
			Configured:                 true,
			Resolved:                   false,
			Revision:                   payload.Revision,
			Status:                     "active",
			ObservedAt:                 observedAt,
			Metadata: map[string]any{
				"source":                  "configured",
				"runtime_agent_id_source": "gantry_settings_folder",
			},
			Raw: map[string]any{"ref": ref},
		})
	}
	return items, nil
}

func gantryAgentIDForDelegateRef(ref string) string {
	if strings.HasPrefix(ref, "agent:") {
		return ref
	}
	return "agent:" + ref
}

func delegationSource(configured bool) string {
	if configured {
		return "configured"
	}
	return "conversation_bound"
}

func (c Client) CollectSnapshot(ctx context.Context, conn domain.RuntimeConnection) (*domain.RuntimeSnapshot, error) {
	check, err := c.Check(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("check gantry before snapshot: %w", err)
	}
	agents, err := c.ListAgents(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("list agents for snapshot: %w", err)
	}
	inventory, err := c.ListInventory(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("list inventory for snapshot: %w", err)
	}
	capabilities, err := c.ListCapabilities(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("list capabilities for snapshot: %w", err)
	}

	observedAt := time.Now().UTC()
	snapshot := &domain.RuntimeSnapshot{
		ObservedAt: observedAt,
		Metadata:   check.Metadata,
		Capabilities: map[string]bool{
			"read_agents":              check.Capabilities.ReadAgents,
			"read_agent_hierarchy":     check.Capabilities.ReadAgentHierarchy,
			"read_agent_delegates":     check.Capabilities.ReadAgentDelegates,
			"read_agent_skills":        check.Capabilities.ReadAgentSkills,
			"read_agent_access":        check.Capabilities.ReadAgentAccess,
			"replace_agent_access":     check.Capabilities.ReplaceAgentAccess,
			"read_subagent_executions": check.Capabilities.ReadSubagentExecutions,
			"read_diagnostics":         check.Capabilities.ReadDiagnostics,
			"read_inventory":           check.Capabilities.ReadInventory,
			"read_capability_catalog":  check.Capabilities.ReadCapabilityCatalog,
			"set_agent_status":         check.Capabilities.SetAgentStatus,
		},
		Agents:            make([]domain.SnapshotAgent, 0, len(agents)),
		Diagnostics:       check.Diagnostics,
		Inventory:         inventory,
		CapabilityCatalog: capabilities,
	}
	items := make([]domain.SnapshotAgent, len(agents))
	delegationItems := make([][]domain.AgentDelegationSnapshot, len(agents))
	jobs := make(chan int)
	workerCount := len(agents)
	if workerCount > 4 {
		workerCount = 4
	}
	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				agent := agents[index]
				skills, err := c.ListAgentSkills(ctx, conn, agent.RuntimeAgentID)
				if err != nil {
					errOnce.Do(func() { firstErr = fmt.Errorf("list skills for agent %q: %w", agent.RuntimeAgentID, err) })
					continue
				}
				access, err := c.GetAgentAccess(ctx, conn, agent.RuntimeAgentID)
				if err != nil {
					errOnce.Do(func() { firstErr = fmt.Errorf("get access for agent %q: %w", agent.RuntimeAgentID, err) })
					continue
				}
				delegations, err := c.ListAgentDelegates(ctx, conn, agent.RuntimeAgentID)
				if err != nil {
					errOnce.Do(func() { firstErr = fmt.Errorf("list delegates for agent %q: %w", agent.RuntimeAgentID, err) })
					continue
				}
				agent.ObservedAt = observedAt
				access.ObservedAt = observedAt
				for i := range skills {
					skills[i].ObservedAt = observedAt
				}
				items[index] = domain.SnapshotAgent{Agent: agent, Skills: skills, Access: *access}
				delegationItems[index] = delegations
			}
		}()
	}
	for index := range agents {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	snapshot.Agents = items
	for _, delegations := range delegationItems {
		snapshot.AgentDelegations = append(snapshot.AgentDelegations, delegations...)
	}
	executions, err := c.listSubagentExecutions(ctx, conn, observedAt)
	if err != nil {
		return nil, fmt.Errorf("list subagent executions for snapshot: %w", err)
	}
	snapshot.SubagentExecutions = executions
	return snapshot, nil
}

func (c Client) ListInventory(ctx context.Context, conn domain.RuntimeConnection) ([]domain.RuntimeInventorySnapshot, error) {
	var payload gantryInventory
	if err := c.doJSON(ctx, conn, http.MethodGet, "/v1/inventory", nil, &payload); err != nil {
		return nil, err
	}
	observedAt := time.Now().UTC()
	items := make([]domain.RuntimeInventorySnapshot, 0, len(payload.Inventory.Tools)+len(payload.Inventory.Skills)+len(payload.Inventory.MCPServers))
	for _, group := range []struct {
		kind  string
		items []map[string]any
	}{{"tool", payload.Inventory.Tools}, {"skill", payload.Inventory.Skills}, {"mcp_server", payload.Inventory.MCPServers}} {
		for _, raw := range group.items {
			id := mapString(raw, "id")
			if id == "" {
				continue
			}
			name := mapString(raw, "displayName")
			if name == "" {
				name = mapString(raw, "name")
			}
			if name == "" {
				name = id
			}
			provider := mapString(raw, "provider")
			if provider == "" {
				provider = mapString(raw, "createdSource")
			}
			items = append(items, domain.RuntimeInventorySnapshot{
				RuntimeItemID: id, Kind: group.kind, Name: name, Status: mapString(raw, "status"),
				Provider: provider, Source: mapString(raw, "source"), ObservedAt: observedAt,
				Metadata: inventoryMetadata(raw), Raw: raw,
			})
		}
	}
	return items, nil
}

func (c Client) ListCapabilities(ctx context.Context, conn domain.RuntimeConnection) ([]domain.RuntimeCapabilitySnapshot, error) {
	var payload gantryCapabilityList
	if err := c.doJSON(ctx, conn, http.MethodGet, "/v1/capabilities", nil, &payload); err != nil {
		return nil, err
	}
	observedAt := time.Now().UTC()
	items := make([]domain.RuntimeCapabilitySnapshot, 0, len(payload.Capabilities))
	for _, capability := range payload.Capabilities {
		if strings.TrimSpace(capability.ID) == "" {
			continue
		}
		version := fmt.Sprint(capability.Version)
		if capability.Version == nil || version == "<nil>" {
			version = "unknown"
		}
		items = append(items, domain.RuntimeCapabilitySnapshot{
			RuntimeCapabilityID: capability.ID, Version: version, Name: capability.DisplayName,
			Category: capability.Category, Risk: capability.Risk, Can: capability.Can, Cannot: capability.Cannot,
			Source: capability.Source, ObservedAt: observedAt, Metadata: capabilityMetadata(capability.Raw), Raw: capability.Raw,
		})
	}
	return items, nil
}

func (c Client) listSubagentExecutions(ctx context.Context, conn domain.RuntimeConnection, observedAt time.Time) ([]domain.SubagentExecutionSnapshot, error) {
	var runsPayload struct {
		Runs []gantryRun `json:"runs"`
	}
	if err := c.doJSON(ctx, conn, http.MethodGet, "/v1/runs", nil, &runsPayload); err != nil {
		return nil, err
	}
	var jobsPayload struct {
		Jobs []gantryJob `json:"jobs"`
	}
	if err := c.doJSON(ctx, conn, http.MethodGet, "/v1/jobs?limit=100", nil, &jobsPayload); err != nil {
		return nil, err
	}
	owners := make(map[string]string, len(jobsPayload.Jobs))
	for _, job := range jobsPayload.Jobs {
		if job.Target != nil {
			owners[job.JobID] = job.Target.AgentID
		}
	}
	var result []domain.SubagentExecutionSnapshot
	for _, run := range runsPayload.Runs {
		var eventsPayload struct {
			Events []gantryRunEvent `json:"events"`
		}
		path := fmt.Sprintf("/v1/runs/%s/events", url.PathEscape(run.RunID))
		if err := c.doJSON(ctx, conn, http.MethodGet, path, nil, &eventsPayload); err != nil {
			return nil, err
		}
		result = append(result, normalizeDelegatedExecutions(run, owners[run.JobID], eventsPayload.Events, observedAt)...)
	}
	return result, nil
}

func normalizeDelegatedExecutions(run gantryRun, runtimeAgentID string, events []gantryRunEvent, observedAt time.Time) []domain.SubagentExecutionSnapshot {
	byTask := map[string]*domain.SubagentExecutionSnapshot{}
	order := []string{}
	for _, event := range events {
		taskID := mapString(event.Payload, "taskId")
		if taskID == "" {
			continue
		}
		item := byTask[taskID]
		kind := mapString(event.Payload, "taskKind")
		if item == nil {
			if kind != "delegated_agent" {
				continue
			}
			item = &domain.SubagentExecutionSnapshot{RuntimeExecutionID: taskID, ParentRunID: run.RunID, RuntimeAgentID: runtimeAgentID, Status: "running", ObservedAt: observedAt, Metadata: map[string]any{"job_id": run.JobID}, Raw: map[string]any{}}
			byTask[taskID] = item
			order = append(order, taskID)
		}
		if value := mapString(event.Payload, "subagentType"); value != "" {
			item.SubagentType = value
		}
		if value := mapString(event.Payload, "description"); value != "" {
			item.Description = value
		}
		if value := mapString(event.Payload, "summary"); value != "" {
			item.Summary = value
		}
		status := mapString(event.Payload, "status")
		if patch, ok := event.Payload["patch"].(map[string]any); ok {
			if value := mapString(patch, "status"); value != "" {
				status = value
			}
		}
		if status != "" {
			item.Status = status
		}
		if event.Metadata.RuntimeEventType == "task.started" && item.StartedAt == nil {
			item.StartedAt = parseGantryTime(event.CreatedAt)
		}
		if status == "completed" || status == "failed" || status == "cancelled" || status == "timed_out" {
			item.EndedAt = parseGantryTime(event.CreatedAt)
		}
		item.Raw[event.ID] = event.Payload
	}
	result := make([]domain.SubagentExecutionSnapshot, 0, len(order))
	for _, id := range order {
		result = append(result, *byTask[id])
	}
	return result
}

func mapString(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func parseGantryTime(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func (c Client) ListAgentSkills(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string) ([]domain.AgentSkillSnapshot, error) {
	path := fmt.Sprintf("/v1/agents/%s/skills", url.PathEscape(runtimeAgentID))
	var payload struct {
		Bindings []gantrySkillBinding `json:"bindings"`
	}
	if err := c.doJSON(ctx, conn, http.MethodGet, path, nil, &payload); err != nil {
		return nil, err
	}
	var catalogPayload struct {
		Skills []gantrySkill `json:"skills"`
	}
	if err := c.doJSON(ctx, conn, http.MethodGet, "/v1/skills", nil, &catalogPayload); err != nil {
		return nil, err
	}
	catalog := make(map[string]gantrySkill, len(catalogPayload.Skills))
	for _, skill := range catalogPayload.Skills {
		catalog[skill.ID] = skill
	}

	observedAt := time.Now().UTC()
	skills := make([]domain.AgentSkillSnapshot, 0, len(payload.Bindings))
	for _, binding := range payload.Bindings {
		detail := catalog[binding.SkillID]
		name := detail.Name
		if name == "" {
			name = binding.SkillID
		}
		skills = append(skills, domain.AgentSkillSnapshot{
			RuntimeSkillID: binding.SkillID,
			Name:           name,
			Description:    detail.Description,
			Source:         detail.Source,
			Status:         binding.Status,
			Version:        binding.ConfigVersionID,
			ToolIDs:        detail.ToolIDs,
			WorkflowRefs:   detail.WorkflowRefs,
			Metadata: map[string]any{
				"binding_id":         binding.ID,
				"catalog_status":     detail.Status,
				"prompt_refs":        detail.PromptRefs,
				"required_env_vars":  detail.RequiredEnvVars,
				"action_permissions": detail.ActionPermissions,
			},
			ObservedAt: observedAt,
		})
	}
	return skills, nil
}

func gantryAgentKind(runtimeAgentID string) domain.AgentKind {
	if runtimeAgentID == "agent:main_agent" {
		return domain.AgentKindMain
	}
	return domain.AgentKindRegistered
}

func decodeAgentList(payload json.RawMessage) ([]gantryAgent, error) {
	var envelope struct {
		Agents []gantryAgent `json:"agents"`
	}
	if err := json.Unmarshal(payload, &envelope); err == nil && envelope.Agents != nil {
		return envelope.Agents, nil
	}

	var agents []gantryAgent
	if err := json.Unmarshal(payload, &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

func (c Client) GetAgentAccess(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string) (*domain.AccessDocument, error) {
	path := fmt.Sprintf("/v1/agents/%s/access", url.PathEscape(runtimeAgentID))
	var access gantryAccess
	if err := c.doJSON(ctx, conn, http.MethodGet, path, nil, &access); err != nil {
		return nil, err
	}

	raw := access.Raw
	if raw == nil {
		raw = map[string]any{}
	}

	return &domain.AccessDocument{
		AgentID:    access.AgentID,
		Selections: normalizeSelections(access.Selections),
		Raw:        raw,
		ObservedAt: time.Now().UTC(),
		Source:     "gantry",
	}, nil
}

func (c Client) ReplaceAgentAccess(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string, access domain.AccessDocument) (*domain.AccessDocument, error) {
	if conn.Mode != domain.RuntimeModeControlEnabled {
		return nil, fmt.Errorf("runtime connection is read-only")
	}

	current, err := c.GetAgentAccess(ctx, conn, runtimeAgentID)
	if err != nil {
		return nil, fmt.Errorf("read current gantry access before replacement: %w", err)
	}
	sources, ok := current.Raw["sources"]
	if !ok || sources == nil {
		return nil, fmt.Errorf("current gantry access document does not contain sources")
	}

	path := fmt.Sprintf("/v1/agents/%s/access", url.PathEscape(runtimeAgentID))
	body := map[string]any{
		"sources":    sources,
		"selections": gantrySelections(access.Selections),
	}

	var response gantryAccess
	if err := c.doJSON(ctx, conn, http.MethodPut, path, body, &response); err != nil {
		return nil, err
	}

	return &domain.AccessDocument{
		AgentID:    response.AgentID,
		Selections: normalizeSelections(response.Selections),
		Raw:        response.Raw,
		ObservedAt: time.Now().UTC(),
		Source:     "gantry",
	}, nil
}

func gantrySelections(selections []domain.AccessSelection) []gantrySelection {
	result := make([]gantrySelection, 0, len(selections))
	for _, selection := range selections {
		version, _ := selection.Attributes["version"].(string)
		result = append(result, gantrySelection{
			ID:      strings.TrimSpace(selection.ID),
			Version: strings.TrimSpace(version),
		})
	}
	return result
}

func (c Client) SetAgentStatus(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string, status domain.AgentStatus) (*domain.AgentSnapshot, error) {
	if conn.Mode != domain.RuntimeModeControlEnabled {
		return nil, fmt.Errorf("runtime connection is read-only")
	}
	var runtimeStatus string
	switch status {
	case domain.AgentStatusEnabled:
		runtimeStatus = "active"
	case domain.AgentStatusDisabled:
		runtimeStatus = "disabled"
	default:
		return nil, fmt.Errorf("agent status must be %q or %q", domain.AgentStatusEnabled, domain.AgentStatusDisabled)
	}
	path := fmt.Sprintf("/v1/agents/%s", url.PathEscape(runtimeAgentID))
	var response gantryAgent
	if err := c.doJSON(ctx, conn, http.MethodPatch, path, map[string]any{"status": runtimeStatus}, &response); err != nil {
		return nil, err
	}
	return &domain.AgentSnapshot{
		RuntimeAgentID: response.ID, Kind: gantryAgentKind(response.ID), Name: response.DisplayName(),
		Status: response.StatusDomain(), ObservedAt: time.Now().UTC(), Metadata: response.Raw,
	}, nil
}

func normalizeDoctor(doctor gantryDoctor, observedAt time.Time) ([]domain.RuntimeDiagnosticSnapshot, domain.RuntimeStatus) {
	status := domain.RuntimeStatusActive
	diagnostics := make([]domain.RuntimeDiagnosticSnapshot, 0, len(doctor.Checks))
	for _, check := range doctor.Checks {
		normalized := strings.ToLower(strings.TrimSpace(check.Status))
		switch normalized {
		case "warn", "warning", "degraded":
			if status == domain.RuntimeStatusActive {
				status = domain.RuntimeStatusDegraded
			}
		case "fail", "failed", "error", "critical":
			status = domain.RuntimeStatusFailed
		}
		diagnostics = append(diagnostics, domain.RuntimeDiagnosticSnapshot{
			CheckID: check.ID, Status: normalized, Message: check.Message, ObservedAt: observedAt,
			Metadata: map[string]any{}, Raw: check.Raw,
		})
	}
	switch strings.ToLower(strings.TrimSpace(doctor.Status)) {
	case "warn", "warning", "degraded":
		if status == domain.RuntimeStatusActive {
			status = domain.RuntimeStatusDegraded
		}
	case "fail", "failed", "error", "critical":
		status = domain.RuntimeStatusFailed
	}
	return diagnostics, status
}

func inventoryMetadata(raw map[string]any) map[string]any {
	return map[string]any{
		"description": mapString(raw, "description"), "category": mapString(raw, "category"),
		"risk": mapString(raw, "risk"), "app_id": mapString(raw, "appId"),
	}
}

func capabilityMetadata(raw map[string]any) map[string]any {
	metadata := make(map[string]any, len(raw))
	for key, value := range raw {
		switch key {
		case "id", "version", "displayName", "category", "risk", "can", "cannot", "source":
			continue
		default:
			metadata[key] = value
		}
	}
	return metadata
}

func (c Client) doJSON(ctx context.Context, conn domain.RuntimeConnection, method string, path string, body any, out any) error {
	base, err := normalizeBaseURL(conn.BaseURL)
	if err != nil {
		return err
	}

	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, method, base+path, reader)
	if err != nil {
		return fmt.Errorf("build gantry request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.credentials == nil {
		return fmt.Errorf("gantry credential resolver is required")
	}
	token, err := c.credentials.Resolve(requestCtx, conn.AuthRef)
	if err != nil {
		return fmt.Errorf("resolve gantry credential %q: %w", conn.AuthRef, err)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("gantry credential %q is empty", conn.AuthRef)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call gantry %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("gantry %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(limited)))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode gantry response: %w", err)
	}
	return nil
}

func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return "", fmt.Errorf("gantry endpoint is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse gantry endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("gantry endpoint must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("gantry endpoint host is required")
	}
	return trimmed, nil
}

func normalizeSelections(selections []gantrySelection) []domain.AccessSelection {
	normalized := make([]domain.AccessSelection, 0, len(selections))
	for _, selection := range selections {
		normalized = append(normalized, domain.AccessSelection{
			Kind:    "capability",
			ID:      selection.ID,
			Name:    selection.ID,
			Allowed: true,
			Attributes: map[string]any{
				"version": selection.Version,
			},
		})
	}
	return normalized
}

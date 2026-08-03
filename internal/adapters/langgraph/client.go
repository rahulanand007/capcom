package langgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
)

const (
	defaultTimeout    = 15 * time.Second
	pageSize          = 100
	maxAssistantPages = 100
	maxThreadPages    = 2
	maxRunPages       = 10
)

type Client struct {
	httpClient  *http.Client
	timeout     time.Duration
	credentials runtimeadapter.CredentialResolver
}

func NewClient(httpClient *http.Client, credentials runtimeadapter.CredentialResolver) Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return Client{httpClient: httpClient, timeout: defaultTimeout, credentials: credentials}
}

func (c Client) Kind() domain.RuntimeKind { return domain.RuntimeKindLangGraph }

func (c Client) Check(ctx context.Context, conn domain.RuntimeConnection) (*runtimeadapter.CheckResult, error) {
	var health map[string]any
	if err := c.doJSON(ctx, conn, http.MethodGet, "/ok", nil, &health); err != nil {
		return nil, err
	}
	var info serverInfo
	if err := c.doJSON(ctx, conn, http.MethodGet, "/info", nil, &info); err != nil {
		var responseErr *httpResponseError
		if !errors.As(err, &responseErr) || responseErr.StatusCode != http.StatusNotFound {
			return nil, fmt.Errorf("read langgraph server info: %w", err)
		}
	}
	return &runtimeadapter.CheckResult{
		Status: domain.RuntimeStatusActive, Message: "langgraph agent server health check succeeded",
		Capabilities: runtimeadapter.Capabilities{
			ReadAgents:      true,
			ReadExecutions:  true,
			DeleteAgent:     conn.Mode == domain.RuntimeModeControlEnabled,
			CancelExecution: conn.Mode == domain.RuntimeModeControlEnabled,
		},
		Metadata: map[string]any{"health": health, "version": info.Version,
			"langgraph_py_version": info.LangGraphPyVersion, "flags": info.Flags, "server_metadata": info.Metadata},
	}, nil
}

func (c Client) ListAgents(ctx context.Context, conn domain.RuntimeConnection) ([]domain.AgentSnapshot, error) {
	items, err := c.listAssistants(ctx, conn)
	if err != nil {
		return nil, err
	}
	observedAt := time.Now().UTC()
	result := make([]domain.AgentSnapshot, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = item.GraphID
		}
		if name == "" {
			name = item.AssistantID
		}
		result = append(result, domain.AgentSnapshot{
			RuntimeAgentID: item.AssistantID, Kind: domain.AgentKindRegistered, Name: name,
			Status: domain.AgentStatusEnabled, ObservedAt: observedAt,
			Metadata: map[string]any{"description": item.Description, "graph_id": item.GraphID,
				"version": item.Version, "config": item.Config, "context": item.Context,
				"assistant_metadata": item.Metadata, "runtime_created_at": item.CreatedAt,
				"runtime_updated_at": item.UpdatedAt},
		})
	}
	return result, nil
}

func (c Client) ListAgentSkills(context.Context, domain.RuntimeConnection, string) ([]domain.AgentSkillSnapshot, error) {
	return []domain.AgentSkillSnapshot{}, nil
}

func (c Client) GetAgentAccess(_ context.Context, _ domain.RuntimeConnection, runtimeAgentID string) (*domain.AccessDocument, error) {
	return &domain.AccessDocument{AgentID: runtimeAgentID, Selections: []domain.AccessSelection{}, Raw: map[string]any{"supported": false}}, nil
}

func (c Client) ReplaceAgentAccess(context.Context, domain.RuntimeConnection, string, domain.AccessDocument) (*domain.AccessDocument, error) {
	return nil, fmt.Errorf("langgraph agent server does not expose access replacement")
}

func (c Client) SetAgentStatus(context.Context, domain.RuntimeConnection, string, domain.AgentStatus) (*domain.AgentSnapshot, error) {
	return nil, fmt.Errorf("langgraph agent server status control is not supported")
}

func (c Client) DeleteAgent(ctx context.Context, conn domain.RuntimeConnection, runtimeAgentID string) error {
	path := fmt.Sprintf("/assistants/%s", url.PathEscape(runtimeAgentID))
	return c.doJSON(ctx, conn, http.MethodDelete, path, nil, nil)
}

func (c Client) CancelExecution(ctx context.Context, conn domain.RuntimeConnection, execution domain.RuntimeExecutionSnapshot) error {
	if execution.Kind != "run" {
		return fmt.Errorf("langgraph can only cancel run executions")
	}
	if strings.TrimSpace(execution.ParentRuntimeExecutionID) == "" {
		return fmt.Errorf("langgraph run is missing its parent thread id")
	}
	path := fmt.Sprintf(
		"/threads/%s/runs/%s/cancel?action=interrupt&wait=true",
		url.PathEscape(execution.ParentRuntimeExecutionID),
		url.PathEscape(execution.RuntimeExecutionID),
	)
	return c.doJSON(ctx, conn, http.MethodPost, path, nil, nil)
}

func (c Client) CollectSnapshot(ctx context.Context, conn domain.RuntimeConnection) (*domain.RuntimeSnapshot, error) {
	check, err := c.Check(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("check langgraph before snapshot: %w", err)
	}
	agents, err := c.ListAgents(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("list langgraph assistants: %w", err)
	}
	threads, err := c.listThreads(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("list langgraph threads: %w", err)
	}
	observedAt := time.Now().UTC()
	snapshot := &domain.RuntimeSnapshot{
		ObservedAt: observedAt, Metadata: check.Metadata,
		Capabilities: map[string]bool{"read_agents": true, "read_agent_hierarchy": false,
			"read_agent_skills": false, "read_agent_access": false, "replace_agent_access": false,
			"read_subagent_executions": false, "read_executions": true,
			"delete_agent":     conn.Mode == domain.RuntimeModeControlEnabled,
			"cancel_execution": conn.Mode == domain.RuntimeModeControlEnabled},
		Agents: make([]domain.SnapshotAgent, 0, len(agents)),
	}
	for _, agent := range agents {
		agent.ObservedAt = observedAt
		snapshot.Agents = append(snapshot.Agents, domain.SnapshotAgent{Agent: agent, Skills: []domain.AgentSkillSnapshot{}})
	}
	for _, item := range threads {
		snapshot.Executions = append(snapshot.Executions, normalizeThread(item, observedAt))
		runs, err := c.listRuns(ctx, conn, item.ThreadID)
		if err != nil {
			return nil, fmt.Errorf("list runs for thread %q: %w", item.ThreadID, err)
		}
		for _, itemRun := range runs {
			snapshot.Executions = append(snapshot.Executions, normalizeRun(itemRun, observedAt))
		}
	}
	return snapshot, nil
}

func (c Client) listAssistants(ctx context.Context, conn domain.RuntimeConnection) ([]assistant, error) {
	var result []assistant
	for page := 0; page < maxAssistantPages; page++ {
		var items []assistant
		body := map[string]any{"limit": pageSize, "offset": page * pageSize, "sort_by": "assistant_id", "sort_order": "asc"}
		if err := c.doJSON(ctx, conn, http.MethodPost, "/assistants/search", body, &items); err != nil {
			return nil, err
		}
		result = append(result, items...)
		if len(items) < pageSize {
			return result, nil
		}
	}
	return nil, fmt.Errorf("langgraph assistant pagination exceeded %d pages", maxAssistantPages)
}

func (c Client) listThreads(ctx context.Context, conn domain.RuntimeConnection) ([]thread, error) {
	var result []thread
	for page := 0; page < maxThreadPages; page++ {
		var items []thread
		body := map[string]any{"limit": pageSize, "offset": page * pageSize, "sort_by": "updated_at", "sort_order": "desc"}
		if err := c.doJSON(ctx, conn, http.MethodPost, "/threads/search", body, &items); err != nil {
			return nil, err
		}
		result = append(result, items...)
		if len(items) < pageSize {
			return result, nil
		}
	}
	return result, nil
}

func (c Client) listRuns(ctx context.Context, conn domain.RuntimeConnection, threadID string) ([]run, error) {
	var result []run
	for page := 0; page < maxRunPages; page++ {
		var items []run
		path := fmt.Sprintf("/threads/%s/runs?limit=%d&offset=%d", url.PathEscape(threadID), pageSize, page*pageSize)
		if err := c.doJSON(ctx, conn, http.MethodGet, path, nil, &items); err != nil {
			return nil, err
		}
		result = append(result, items...)
		if len(items) < pageSize {
			return result, nil
		}
	}
	return result, nil
}

func normalizeThread(item thread, fallback time.Time) domain.RuntimeExecutionSnapshot {
	return domain.RuntimeExecutionSnapshot{RuntimeExecutionID: item.ThreadID, Kind: "thread", Status: item.Status,
		StartedAt: parseTime(item.CreatedAt), ObservedAt: parseTimeValue(item.UpdatedAt, fallback),
		Metadata: map[string]any{"state_updated_at": item.StateUpdated, "config": item.Config,
			"thread_metadata": item.Metadata}, Raw: item.Raw}
}

func normalizeRun(item run, fallback time.Time) domain.RuntimeExecutionSnapshot {
	var ended *time.Time
	if item.Status == "success" || item.Status == "error" || item.Status == "timeout" || item.Status == "interrupted" {
		ended = parseTime(item.UpdatedAt)
	}
	return domain.RuntimeExecutionSnapshot{RuntimeExecutionID: item.RunID,
		ParentRuntimeExecutionID: item.ThreadID, RuntimeAgentID: item.AssistantID, Kind: "run", Status: item.Status,
		StartedAt: parseTime(item.CreatedAt), EndedAt: ended, ObservedAt: parseTimeValue(item.UpdatedAt, fallback),
		Metadata: map[string]any{"multitask_strategy": item.MultitaskStrategy,
			"langsmith_session_name": item.LangSmithSessionName, "run_metadata": item.Metadata}, Raw: item.Raw}
}

func parseTime(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseTimeValue(value string, fallback time.Time) time.Time {
	if parsed := parseTime(value); parsed != nil {
		return *parsed
	}
	return fallback
}

func (c Client) doJSON(ctx context.Context, conn domain.RuntimeConnection, method, path string, body, out any) error {
	base, err := normalizeBaseURL(conn.BaseURL)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal langgraph request: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, base+path, reader)
	if err != nil {
		return fmt.Errorf("build langgraph request: %w", err)
	}
	if c.credentials == nil {
		return fmt.Errorf("langgraph credential resolver is required")
	}
	apiKey, err := c.credentials.Resolve(requestCtx, conn.AuthRef)
	if err != nil {
		return fmt.Errorf("resolve langgraph credential %q: %w", conn.AuthRef, err)
	}
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("langgraph credential %q is empty", conn.AuthRef)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Api-Key", apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call langgraph %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &httpResponseError{Method: method, Path: path, StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(limited))}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode langgraph response: %w", err)
	}
	return nil
}

type httpResponseError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *httpResponseError) Error() string {
	return fmt.Sprintf("langgraph %s %s returned %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

func normalizeBaseURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" {
		return "", fmt.Errorf("langgraph endpoint is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("parse langgraph endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("langgraph endpoint must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("langgraph endpoint host is required")
	}
	return value, nil
}

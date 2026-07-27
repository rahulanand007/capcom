# 03 - Gantry Adapter Contract

## Snapshot

This contract targets Gantry `main` at commit `42c065a0`, package `@gantry/runtime@1.2.52`.

## Transport

Gantry can be reached by:

- loopback HTTP base URL, for example `http://127.0.0.1:8787`
- Unix/socket transport where configured
- hosted/remote base URL in future deployments

Capcom V1 should implement base URL first. Socket transport can be added behind the same adapter interface.

## Auth

Use Gantry Control API keys. V1 should support two connection modes:

| Mode | Required Scopes |
|---|---|
| read_only | `sessions:read`, `agents:admin`, `jobs:read`, `skills:read`, `mcp:read` |
| control_enabled | read-only scopes plus `jobs:write`, `skills:admin`, `mcp:admin` |

Gantry currently uses `agents:admin` for agent inventory and access reads, so read-only mode is "no mutation scopes" rather than purely read-named scopes.

## Adapter Interface

```go
type RuntimeAdapter interface {
    Health(ctx context.Context) (RuntimeHealth, error)
    Doctor(ctx context.Context) (RuntimeDoctor, error)
    ListAgents(ctx context.Context) ([]RuntimeAgent, error)
    ListAgentSkills(ctx context.Context, externalAgentID string) ([]RuntimeAgentSkill, error)
    GetAgentAdmin(ctx context.Context, externalAgentID string) (RuntimeAgentAdmin, error)
    GetAgentAccess(ctx context.Context, externalAgentID string) (RuntimeAgentAccess, error)
    ReplaceAgentAccess(ctx context.Context, externalAgentID string, access RuntimeAgentAccessDesired) (RuntimeAgentAccess, error)
    PatchAgentStatus(ctx context.Context, externalAgentID string, status string) (RuntimeAgent, error)
    ListInventory(ctx context.Context) (RuntimeInventory, error)
    ListCapabilities(ctx context.Context) ([]RuntimeCapability, error)
    ListEvents(ctx context.Context, cursor RuntimeCursor) ([]RuntimeEvent, RuntimeCursor, error)
    ExecuteControlAction(ctx context.Context, action RuntimeControlAction) (RuntimeControlResult, error)
}
```

## Endpoint Mapping

| Adapter Method | Gantry Endpoint |
|---|---|
| Health | `GET /v1/health` |
| Doctor | `GET /v1/doctor` |
| ListAgents | `GET /v1/agents` |
| ListAgentSkills | `GET /v1/agents/{agentId}/skills` |
| GetAgentAdmin | `GET /v1/agents/{agentId}/admin` |
| GetAgentAccess | `GET /v1/agents/{agentId}/access` |
| ReplaceAgentAccess | `PUT /v1/agents/{agentId}/access` |
| PatchAgentStatus | `PATCH /v1/agents/{agentId}` |
| ListAgentDelegates | `GET /v1/agents/{agentId}/delegates` |
| ListInventory | `GET /v1/inventory` |
| ListCapabilities | `GET /v1/capabilities` |
| ListAgentEvents | `GET /v1/sessions/{sessionId}/events` or run/job event routes |
| ListRuns | `GET /v1/runs` |
| ListRunEvents | `GET /v1/runs/{runId}/events` |
| ResolveRunOwner | `GET /v1/jobs?limit=100` (`target.agentId`) |

Doctor, inventory, and capability normalization are implemented. Doctor checks
are stored by `(runtime_connection_id, check_id)`. Global inventory is stored as
runtime-neutral `tool`, `skill`, and `mcp_server` items. Immutable capability
manifests retain typed identity, version, category, risk, `can`, `cannot`, and
source fields while preserving remaining Gantry metadata in JSONB.

The instance-scoped Capcom read APIs are:

- `GET /v1/runtime-instances/{id}/diagnostics`
- `GET /v1/runtime-instances/{id}/inventory?kind=tool|skill|mcp_server`
- `GET /v1/runtime-instances/{id}/capabilities`
- `GET /v1/runtime-instances/{id}/agent-delegations`

Gantry doctor warnings normalize the runtime to `degraded`; failed/error checks
normalize it to `failed`. A failed collection preserves the last known catalog.

Current Gantry returns agent inventory as `{ "agents": [...] }`. The adapter
also accepts the earlier bare-array response so recorded fixtures and compatible
older runtimes continue to work.

Gantry's durable inventory does not expose parent-child identity for native
execution-time subagents. Capcom classifies `agent:main_agent` as `main` and
other returned durable agents as `registered`; it must not infer that every
secondary registered agent is a subagent. Hierarchy capability therefore stays
false.

Gantry now exposes configured and resolved callable-agent relationships through
`GET /v1/agents/{agentId}/delegates`. Capcom stores these as directed,
many-to-many delegation edges rather than forcing them into the parent field.
Each edge retains the desired-state revision, configured versus
conversation-bound provenance, resolution status, delegate reference, runtime
agent identity, generated tool name, display name, and persona. Missing edges
become stale only after repeated complete successful snapshots. Agent-scoped
reads use `GET /v1/agents/{id}/delegations`.

Gantry canonicalizes configured delegate references to settings folder names.
Capcom treats that reference as the stable edge identity and derives the
deterministic `agent:<folder>` runtime ID even when Gantry's `resolved` roster
is empty. The edge remains `resolved=false` until Gantry reports it as callable.

Ephemeral delegated/subagent runs remain execution history and do not become
durable delegation edges or durable agents.

Capcom recognizes a Gantry subagent execution only after a run event reports
`taskKind: delegated_agent`. It correlates `task.started`, `task.progress`,
`task.updated`, and `task.notification` by `taskId`; ordinary async command
tasks are ignored. The owning durable agent is resolved from the run's `job_id`
to the job target. Gantry currently exposes job runs only through `/v1/runs`,
so no executions are reported when the runtime has no job runs.

`ListAgentSkills` joins the agent binding response with `GET /v1/skills` so
normalized skill snapshots include the catalog name, description, source,
tools, workflows, and action-permission metadata. Bindings determine which
skills the agent can access; catalog membership alone does not grant access.

## Access Document Mapping

Gantry `/v1/agents/{agentId}/access` returns the key actual-state payload for V1:

```json
{
  "agentId": "agent:abc",
  "sources": {
    "skills": [],
    "mcpServers": [],
    "tools": []
  },
  "selections": [
    { "id": "browser.use", "version": "builtin" }
  ],
  "toolAccess": {
    "configuredTools": [],
    "defaultTools": [],
    "availableButGatedTools": [],
    "requestableAdminTools": [],
    "source": "..."
  },
  "summary": {
    "connected": [],
    "allowed": [],
    "needsAttention": [],
    "suggestedCleanup": []
  },
  "updatedAt": "..."
}
```

Capcom should normalize:

- `sources.skills[]`
- `sources.mcpServers[]`
- `sources.tools[]`
- `selections[]`
- `toolAccess`
- `summary`

Do not collapse this to a flat list of tools.

## Mutations

### Replace Agent Access

Use:

```text
PUT /v1/agents/{agentId}/access
```

Body:

```json
{
  "sources": {
    "skills": [],
    "mcpServers": [],
    "tools": []
  },
  "selections": [
    { "id": "browser.use", "version": "builtin" }
  ]
}
```

Before replacing access, the adapter must read the current Gantry access
document and preserve its complete `sources` object. Capcom changes only the
normalized capability `selections`; omitting `sources` would make Gantry reject
the full-document replacement and could otherwise discard existing bindings.

### Disable/Enable Agent

Use:

```text
PATCH /v1/agents/{agentId}
```

Body:

```json
{ "status": "disabled" }
```

or:

```json
{ "status": "active" }
```

Capcom exposes this as
`POST /v1/agents/{id}/actions/set-status` with normalized `enabled` or
`disabled` status. It requires a `control_enabled` connection, actor, reason,
idempotency key, adapter capability validation, audit events, and a post-action
sync. Dry-run performs every validation without calling Gantry.

### Skills

Use specific skill routes when source binding is the action:

- `GET /v1/skills`
- `GET /v1/agents/{agentId}/skills`
- `PUT /v1/agents/{agentId}/skills/{skillId}`
- `DELETE /v1/agents/{agentId}/skills/{skillId}`

### MCP Servers

Use specific MCP routes when source binding is the action:

- `GET /v1/mcp-servers`
- `POST /v1/mcp-servers`
- `POST /v1/mcp-servers/{serverId}/test`
- `POST /v1/mcp-servers/{serverId}/disable`
- `GET /v1/agents/{agentId}/mcp-servers`
- `PUT/PATCH/DELETE /v1/agents/{agentId}/mcp-servers/{serverId}`

## Failure Behavior

| Gantry Failure | Capcom Behavior |
|---|---|
| health unauthorized | reject or degrade runtime connection |
| doctor warning | allow connection only if check is non-blocking |
| doctor failure | reject activation |
| agent not found | mark imported agent missing after repeated syncs; do not delete immediately |
| mutation 4xx | mark control action failed and audit |
| mutation timeout | mark unknown/failed, sync actual state before retry |
| runtime unavailable | mark runtime degraded, preserve last known state |

## Adapter Tests

V1 should include:

- unit tests for request construction
- unit tests for response normalization
- contract tests using recorded Gantry JSON fixtures
- integration test behind env flag when Gantry is running

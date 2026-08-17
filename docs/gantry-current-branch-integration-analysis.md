# Gantry Current Branch Integration Analysis

## Snapshot

Reviewed Gantry source checkout:

- Branch: `main`
- New head: `42c065a0`
- Package: `@gantry/runtime@1.2.52`
- Node engine: `>=24 <26`
- Existing untracked files left untouched:
  - `apps/core/src/shared/control-socket-path.ts`
  - `apps/core/test/unit/shared/control-socket-path.test.ts`

The pull fast-forwarded from `d0815175` to `42c065a0` and introduced a large company-brain/memory surface, new routes, migrations, and tests.

## What Changed That Matters To Capcom

The current branch still exposes the control-plane shape Capcom needs, but the integration should be more precise than the earlier draft docs:

- Agent inventory is available through `/v1/agents`.
- Agent detail and bound conversations are available through `/v1/agents/{agentId}/admin`.
- Actual agent access is explicitly modeled through `/v1/agents/{agentId}/access`.
- Capability catalog is split across `/v1/inventory`, `/v1/capabilities`, and `/v1/capabilities/{capabilityId}`.
- Skills are first-class objects with install, file-read, bind, and unbind routes.
- MCP servers are first-class objects with connect, list, test, disable, bind, update, and unbind routes.
- Jobs and runs are still a strong event/control surface.
- New brain endpoints expose app-scoped company knowledge import/status and should be treated as memory/governance inventory, not a P0 Capcom control surface.

## Current Gantry API Surfaces For Capcom

| Capcom Need | Gantry Surface | Scope |
|---|---|---|
| Runtime health | `GET /healthz`, `GET /readyz`, `GET /v1/health`, `GET /v1/doctor` | unauthenticated for `/healthz` and `/readyz`; `sessions:read` for v1 health/doctor |
| Runtime status/read model | `GET /v1/status` | `agents:admin` |
| Agent list | `GET /v1/agents` | `agents:admin` |
| Agent create | `POST /v1/agents` | `agents:admin` |
| Agent read | `GET /v1/agents/{agentId}` | `agents:admin` |
| Agent update/disable | `PATCH /v1/agents/{agentId}` | `agents:admin` |
| Agent admin detail | `GET /v1/agents/{agentId}/admin` | `agents:admin` |
| Agent access read | `GET /v1/agents/{agentId}/access` | `agents:admin` |
| Agent access replace | `PUT /v1/agents/{agentId}/access` | `agents:admin` |
| Inventory | `GET /v1/inventory` | `agents:admin` |
| Capability catalog | `GET /v1/capabilities`, `GET /v1/capabilities/{capabilityId}` | `agents:admin` |
| Skill list/install | `GET /v1/skills`, `POST /v1/skills/install` | `skills:read`, `skills:admin` |
| Agent skill bindings | `GET /v1/agents/{agentId}/skills`, `PUT/DELETE /v1/agents/{agentId}/skills/{skillId}` | `skills:read`, `skills:admin` |
| MCP server inventory | `GET /v1/mcp-servers`, `GET /v1/mcp-servers/{serverId}` | `mcp:read` |
| MCP connect/test/disable | `POST /v1/mcp-servers`, `POST /v1/mcp-servers/{serverId}/test`, `POST /v1/mcp-servers/{serverId}/disable` | `mcp:admin` |
| Agent MCP bindings | `GET /v1/agents/{agentId}/mcp-servers`, `PUT/PATCH/DELETE /v1/agents/{agentId}/mcp-servers/{serverId}` | `mcp:read`, `mcp:admin`, `agents:admin` |
| Sessions/events | `POST /v1/sessions/ensure`, `GET /v1/sessions/{sessionId}/events`, `GET /v1/sessions/{sessionId}/wait` | `sessions:read/write` |
| Jobs/runs/events | `GET/POST/PATCH/DELETE /v1/jobs`, job pause/resume/trigger, `GET /v1/runs`, `GET /v1/runs/{runId}/events` | `jobs:read/write` |
| Webhooks | `GET/POST/PATCH/DELETE /v1/webhooks`, test, replay, purge | `webhooks:read/write` |
| Company brain | `GET /v1/brain/status`, `POST /v1/brain/import` | `memory:read/admin` |

## Recommended Capcom Adapter Contract For Gantry

Capcom should not wrap every Gantry endpoint directly. The adapter should expose a small stable interface:

```text
Health()
Doctor()
ListAgents()
GetAgentAdmin(agentId)
GetAgentAccess(agentId)
ReplaceAgentAccess(agentId, accessDocument)
PatchAgentStatus(agentId, status)
ListInventory()
ListCapabilities()
ListAgentEvents(agentId, cursor)
ListRuns(cursor)
ListRunEvents(runId, cursor)
ExecuteControlAction(action)
```

Keep skills and MCP as normalized access sources:

```text
ActualAccess.sources.skills[]
ActualAccess.sources.mcpServers[]
ActualAccess.sources.tools[]
ActualAccess.selections[]
ActualAccess.toolAccess
ActualAccess.summary
```

Then map Capcom actions to the most specific Gantry route:

| Capcom Action | Preferred Gantry Mutation |
|---|---|
| Disable agent | `PATCH /v1/agents/{agentId}` with `status: disabled` |
| Enable agent | `PATCH /v1/agents/{agentId}` with `status: active` |
| Replace full access document | `PUT /v1/agents/{agentId}/access` |
| Bind skill | `PUT /v1/agents/{agentId}/skills/{skillId}` |
| Unbind skill | `DELETE /v1/agents/{agentId}/skills/{skillId}` |
| Connect MCP server | `POST /v1/mcp-servers` |
| Disable MCP server | `POST /v1/mcp-servers/{serverId}/disable` |
| Bind MCP server to agent | `PUT /v1/agents/{agentId}/mcp-servers/{serverId}` |
| Update MCP binding policy | `PATCH /v1/agents/{agentId}/mcp-servers/{serverId}` |
| Unbind MCP server | `DELETE /v1/agents/{agentId}/mcp-servers/{serverId}` |
| Pause job | `POST /v1/jobs/{jobId}/pause` |
| Resume job | `POST /v1/jobs/{jobId}/resume` |
| Trigger job | `POST /v1/jobs/{jobId}/trigger` |

## Important Product Interpretation

Gantry already has a strong internal access model:

- Sources are inventory: skills, MCP servers, tools.
- Durable authority is represented as reviewed capability selections.
- Some visible tools can still be blocked if not backed by selected semantic capabilities.
- MCP server test can report diagnostics showing visible tools, approved tools, and blocked-by-review tools.

Capcom should therefore avoid a naive "tool list equals authority" model.

Capcom should store:

- connected sources
- selected capabilities
- runtime tool access view
- access summary
- diagnostics
- drift records between desired and actual selected authority

## P0 Integration Plan

1. Update Gantry contract docs to use `/v1/agents/{agentId}/access` as the canonical full access read/replace endpoint.
2. Generate a local OpenAPI snapshot from `/openapi.json` after Gantry starts.
3. Build a Gantry adapter around health, doctor, agents, admin detail, inventory, capabilities, access, sessions, runs, jobs.
4. Store raw Gantry responses alongside normalized Capcom actual state for debugging.
5. Implement drift only on agent status and access selections for the first demo.
6. Add skills/MCP source drift in the next slice.
7. Keep company brain as read-only inventory/post-MVP unless memory governance becomes the demo wedge.

## Risks

| Risk | Mitigation |
|---|---|
| Gantry auth scopes are broad for P0 | Support read-only mode and separate control-enabled credentials |
| Access semantics are richer than simple tools | Normalize sources, selections, tool access, and summary separately |
| Skills/MCP mutations sync settings as side effect | Treat Gantry as source of truth for actual state after every mutation |
| Webhooks add delivery complexity | Use polling/streaming first; keep webhooks Phase 2 |
| Company brain distracts from governance wedge | Track as memory/source inventory, not MVP control action |

# Capcom - Gantry Integration Contract

## Source Snapshot

Latest Gantry source checked from the upstream repository:

- Branch: `main`
- Pulled to: `origin/main` fast-forward at commit `42c065a0`
- Package: `@gantry/runtime@1.2.52`
- Node requirement: `>=24 <26`

Local status after pull:

- Dependencies installed with `npm.cmd ci`.
- Build passed with `npm.cmd run build`.
- Docker/Postgres setup is blocked because Docker Desktop/Linux engine is not running or not reachable.
- Two pre-existing untracked files remain untouched:
  - `apps/core/src/shared/control-socket-path.ts`
  - `apps/core/test/unit/shared/control-socket-path.test.ts`

Latest inspection notes are in `gantry-current-branch-integration-analysis.md`.

## Runtime Transport

Gantry Control API supports:

- Unix socket transport for local runtime.
- Loopback TCP when `GANTRY_CONTROL_HOST` and `GANTRY_CONTROL_PORT` are set.
- Remote/fleet `baseUrl` for hosted control roles.

SDK client shape:

```ts
createClient({
  apiKey,
  socketPath,
  baseUrl,
  timeoutMs,
});
```

Capcom MVP should use HTTP loopback or socket transport first. Webhook delivery is not required for local MVP.

## Required Gantry Runtime Environment

Gantry reads runtime secrets from `<GANTRY_HOME>/.env`.

Important values:

- `GANTRY_DATABASE_URL`
- `SECRET_ENCRYPTION_KEY`
- `GANTRY_IPC_AUTH_SECRET`
- `GANTRY_CONTROL_API_KEY`
- `GANTRY_CONTROL_API_KEYS_JSON`
- `GANTRY_CONTROL_HOST`
- `GANTRY_CONTROL_PORT`
- `GANTRY_CONTROL_SOCKET_PATH`
- `GANTRY_CONTROL_BASE_URL`

Gantry also requires `<GANTRY_HOME>/settings.yaml`.

## Local Setup Status

Current attempted setup:

```text
npm.cmd ci              passed
npm.cmd run build       passed
docker compose up -d postgres failed
gantry doctor           blocked by missing settings.yaml when using a new GANTRY_HOME
```

Docker failure:

```text
dockerDesktopLinuxEngine pipe not found
```

Interpretation:

Docker Desktop is not running, not installed, or not exposing the Linux engine. Gantry can continue only after a Postgres database is reachable and runtime settings are initialized.

## MVP API Surface To Use

Health/readiness:

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /v1/health`
- `GET /v1/doctor`

Agent and access inventory:

- `GET /v1/agents`
- `POST /v1/agents`
- `GET /v1/agents/:agentId`
- `PATCH /v1/agents/:agentId`
- `GET /v1/agents/:agentId/admin`
- `GET /v1/inventory`
- `GET /v1/capabilities`
- `GET /v1/capabilities/:capabilityId`
- `GET /v1/agents/:agentId/access`
- `PUT /v1/agents/:agentId/access`
- `GET /v1/skills`
- `POST /v1/skills/install`
- `GET /v1/agents/:agentId/skills`
- `PUT /v1/agents/:agentId/skills/:skillId`
- `DELETE /v1/agents/:agentId/skills/:skillId`
- `GET /v1/mcp-servers`
- `POST /v1/mcp-servers`
- `GET /v1/mcp-servers/:serverId`
- `POST /v1/mcp-servers/:serverId/test`
- `POST /v1/mcp-servers/:serverId/disable`
- `GET /v1/agents/:agentId/mcp-servers`
- `PUT /v1/agents/:agentId/mcp-servers/:serverId`
- `PATCH /v1/agents/:agentId/mcp-servers/:serverId`
- `DELETE /v1/agents/:agentId/mcp-servers/:serverId`

Provider and conversation discovery:

- `GET /v1/providers`
- `GET /v1/provider-accounts`
- `POST /v1/provider-accounts`
- `GET /v1/provider-accounts/:providerAccountId`
- `PATCH /v1/provider-accounts/:providerAccountId`
- `POST /v1/provider-accounts/:providerAccountId/discover-conversations`
- `GET /v1/conversations`
- `GET /v1/conversations/:conversationId`
- `GET /v1/conversations/:conversationId/approvers`
- `PUT /v1/conversations/:conversationId/approvers`
- `GET /v1/agents/:agentId/conversation-installs`
- `PUT /v1/agents/:agentId/conversation-installs/:conversationId`
- `PATCH /v1/agents/:agentId/conversation-installs/:conversationId`
- `DELETE /v1/agents/:agentId/conversation-installs/:conversationId`

Sessions/events:

- `POST /v1/sessions/ensure`
- `GET /v1/sessions/:sessionId`
- `GET /v1/sessions/:sessionId/messages`
- `POST /v1/sessions/:sessionId/messages`
- `GET /v1/sessions/:sessionId/events`
- `GET /v1/sessions/:sessionId/wait`
- SDK SSE: `client.sessions.stream(sessionId)`

Jobs/runs:

- `GET /v1/jobs`
- `GET /v1/jobs/:jobId`
- `PATCH /v1/jobs/:jobId`
- `DELETE /v1/jobs/:jobId`
- `POST /v1/jobs/:jobId/pause`
- `POST /v1/jobs/:jobId/resume`
- `POST /v1/jobs/:jobId/trigger`
- `GET /v1/runs`
- `GET /v1/runs/:runId`
- `GET /v1/runs/:runId/events`

Company brain/memory:

- `GET /v1/brain/status`
- `POST /v1/brain/import`

Phase 2 webhook surface:

- `POST /v1/webhooks`
- `GET /v1/webhooks`
- webhook update/delete/test/replay/dead-letter operations

## Capcom Normalization

Capcom should map Gantry state into these internal concepts:

- RuntimeConnection
- Agent
- AgentDesiredState
- AgentActualState
- AgentEvent
- DriftRecord
- ControlAction
- AuditLog

For MVP drift, compare:

- desired selected capabilities
- actual `GET /v1/agents/:agentId/access`
- actual visible sources from Gantry
- actual capabilities from Gantry inventory
- skill and MCP server bindings as source/access drift after the first P0 demo slice

## MVP Control Rules

- Read-only mode must omit mutation scopes.
- Control mode requires scoped keys and a human reason.
- Capcom should prefer `PUT /v1/agents/:agentId/access` for deterministic replacement of access state.
- Capcom should not edit Gantry `settings.yaml`, generated provider config, provider folders, or Gantry DB tables directly.
- Capcom should not rely on webhooks for local MVP.

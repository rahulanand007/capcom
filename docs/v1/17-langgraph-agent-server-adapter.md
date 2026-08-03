# 17 - LangGraph Agent Server Adapter

## Status

The LangGraph Agent Server adapter was implemented and live-tested on
2026-07-21 against `langgraph-api` 0.11.1 and `langgraph` 1.2.9. Audited
assistant deletion and run cancellation were added on 2026-07-29.

LangGraph is Capcom's second runtime adapter. Assistants are durable agents;
threads and runs are runtime-neutral executions. They are not skills or
subagents.

## Supported Surface

| Capcom behavior | Agent Server endpoint | Support |
|---|---|---|
| Health | `GET /ok` | Implemented |
| Server metadata | `GET /info` | Implemented; optional when hardened deployments disable metadata routes |
| Assistant inventory | `POST /assistants/search` | Implemented with pagination |
| Recent thread inventory | `POST /threads/search` | Implemented; bounded to the 200 most recently updated threads per sync |
| Runs for each imported thread | `GET /threads/{threadId}/runs` | Implemented with bounded pagination |
| Skills | No stable equivalent | Explicitly unsupported |
| Effective access | No stable equivalent | Explicitly unsupported |
| Agent hierarchy | No stable equivalent | Explicitly unsupported |
| Access replacement | No stable equivalent | Explicitly rejected |
| Delete assistant | `DELETE /assistants/{assistantId}` | Implemented for `control_enabled` connections |
| Cancel run | `POST /threads/{threadId}/runs/{runId}/cancel?action=interrupt&wait=true` | Implemented for active runs on `control_enabled` connections |
| Invocation | Run creation APIs | Deferred |

## Authentication

Hosted and managed deployments use the `X-Api-Key` header with a LangSmith API
key. Capcom stores the key through its encrypted secret service and resolves it
immediately before each adapter request.

`langgraph dev` uses no-op authentication. Capcom still requires an `auth_ref`
so the connection contract stays consistent; use any non-empty local-only
placeholder. Never use that placeholder for a hosted deployment.

The adapter supports `read_only` and `control_enabled` connections. Control
capability flags are false in `read_only` mode and every mutation is rejected
before the runtime call.

## Normalization

### Assistant To Agent

| LangGraph field | Capcom field |
|---|---|
| `assistant_id` | `runtime_agent_id` |
| `name`, then `graph_id`, then `assistant_id` | agent name fallback order |
| assistant exists in search response | `enabled` status |
| `graph_id`, `version`, `description` | typed metadata entries |
| `config`, `context`, `metadata` | runtime metadata JSON |

Assistants are classified as `registered`. Capcom does not infer a main agent
or subagent relationship from graph configuration.

### Thread And Run To Execution

Threads are stored as `kind=thread`. Runs are stored as `kind=run`, with:

- `run_id` as `runtime_execution_id`
- `thread_id` as `parent_runtime_execution_id`
- `assistant_id` as `runtime_agent_id`
- terminal run timestamps represented by `ended_at`
- vendor metadata and the raw response retained in JSONB

The table uniqueness boundary is
`(runtime_connection_id, kind, runtime_execution_id)`.

## Capabilities

The adapter reports:

```json
{
  "read_agents": true,
  "read_agent_hierarchy": false,
  "read_agent_skills": false,
  "read_agent_access": false,
  "replace_agent_access": false,
  "read_subagent_executions": false,
  "read_executions": true,
  "delete_agent": true,
  "cancel_execution": true
}
```

The two control flags above are true only for a `control_enabled` connection.
Unsupported access documents are not persisted as authoritative access state.

## Control Semantics

Assistant deletion requires actor, reason, idempotency key, and an exact
confirmation matching the runtime assistant ID. LangGraph deletes all versions
of the assistant. Capcom retains the imported agent as a disabled tombstone
with `metadata.runtime_deleted=true` so actions and audit events keep a stable
target.

Run cancellation is permitted only for persisted `kind=run` executions whose
last observed status is `pending` or `running`. Capcom uses LangGraph
`interrupt`, not `rollback`, and waits for cancellation to settle. This keeps
the run record and checkpoints available for audit and debugging. Both actions
support dry-run validation and trigger a post-action sync after a real
mutation.

## Local Agent Server

The deterministic fixture is in `examples/langgraph-agent-server`. It does not
call an LLM and disables LangSmith tracing.

From the repository root on Windows:

```powershell
python -m venv .venv-langgraph
.\.venv-langgraph\Scripts\python.exe -m pip install "langgraph-cli[inmem]" "langgraph>=0.6,<2"
Copy-Item examples\langgraph-agent-server\.env.example examples\langgraph-agent-server\.env
Set-Location examples\langgraph-agent-server
..\..\.venv-langgraph\Scripts\langgraph.exe dev --no-browser --no-reload --host 0.0.0.0 --port 2024
```

Agent Server endpoints:

- health: `http://127.0.0.1:2024/ok`
- API docs: `http://127.0.0.1:2024/docs`

When Capcom runs in Docker, register
`http://langgraph.internal:2024`, not loopback. The Compose API service maps
`langgraph.internal` to the host gateway.

## Live Verification

After storing a secret and creating a `langgraph` runtime instance:

1. `POST /v1/runtime-instances/{id}/test` must return `active`,
   `read_agents=true`, and `read_executions=true`.
2. Create a thread and run through Agent Server.
3. `POST /v1/runtime-instances/{id}/sync` must succeed.
4. `GET /v1/runtime-instances/{id}/agents` must include the default
   `capcom_demo` assistant.
5. `GET /v1/runtime-instances/{id}/executions` must include one `thread` and
   its child `run`.
6. On a `control_enabled` instance, a pending/running run can be interrupted
   through `POST /v1/runtime-executions/{id}/actions/cancel`.
7. A non-default test assistant can be deleted through
   `POST /v1/agents/{id}/actions/delete` after exact-ID confirmation.

The default assistant generated for each configured graph reports
`metadata.created_by=system`. LangGraph recreates it after deletion, so Capcom
must label it as system-managed and reject delete requests before calling the
adapter. Delete control applies only to user-created assistants.

The 2026-07-21 live test imported one assistant and two execution records from
a deterministic successful run.

## Test Coverage

- HMAC or bearer tokens are not used; `X-Api-Key` request construction is tested.
- Health, metadata, assistant, thread, and run fixtures are recorded under the
  adapter's `testdata` directory.
- Normalization checks agent identity, graph fallback names, parent execution
  identity, terminal timestamps, and unsupported access behavior.
- Unauthorized responses preserve HTTP status context.
- Delete and cancel request paths, methods, and interrupt/wait query semantics
  are covered by adapter tests.
- The complete Go suite and Next.js production build pass.

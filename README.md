# Capcom

Capcom is a runtime-agnostic AgentOps control plane. Gantry is the first runtime adapter, but the core architecture is designed so other agent runtimes can be added behind the same adapter boundary.

## Current Status

Implementation has started with the first backend slices:

- Go module at the repository root.
- `capcom-server` binary with graceful shutdown.
- `capcom` CLI placeholder.
- Environment-based config loading.
- Structured JSON logging.
- `GET /healthz` endpoint.
- Runtime-neutral domain shell.
- Runtime adapter interface for Gantry and future adapters.
- LangGraph Agent Server read/control adapter with `X-Api-Key` authentication.
- `.env` loading for local development.
- OpenAPI contract at `api/openapi.yaml`.
- Postgres configuration.
- `database/sql` Postgres connection setup through pgx.
- File-based migration runner.
- Initial V1 database schema.
- `capcom migrate up` CLI command.
- `capcom-server` can connect to Postgres through `CAPCOM_DATABASE_URL`.
- Runtime connection REST APIs.
- Multi-instance identity with stable keys, display names, environments, labels, and isolated credentials.
- Gantry runtime adapter read-path health check.
- Gantry doctor diagnostics plus normalized tool, skill, MCP server, and capability catalogs.
- Runtime connection test endpoint.
- AES-256-GCM encrypted runtime secret storage.
- Audited secret creation and rotation APIs.
- Runtime connections persist secret references rather than credentials.
- Gantry adapter Bearer authentication resolved at request time.
- Live runtime-neutral agent and access inspection through the selected adapter.
- Main/registered/subagent classification and live current-skill inspection.
- Durable Gantry agent-to-agent delegation edges with configured, resolved, and conversation-bound provenance.
- Gantry delegated-task ingestion as separate ephemeral subagent executions.
- LangGraph assistant inventory plus runtime-neutral thread/run execution ingestion.
- Expandable LangGraph thread/run activity in each runtime instance panel.
- Selected-agent details with enriched skill descriptions, tools, workflows, and effective access.
- Transactional manual and periodic runtime synchronization.
- Persisted agents, hierarchy, assigned skills, and effective access.
- Agent detail views show callable delegates, reverse "delegated by" relationships, harness, persona, and callable tool identity.
- Live, cached, and stale freshness semantics with last-known-state retention.
- Sync history and database-backed overlap protection.
- Audited, idempotent access reconciliation with read-only rejection and dry-run validation.
- Audited Gantry agent enable/disable actions with dry-run validation and post-action verification sync.
- Audited LangGraph assistant deletion with exact-ID confirmation and retained Capcom tombstones.
- Audited LangGraph run cancellation using interrupt semantics with post-action verification sync.
- Next.js + shadcn/ui operator console in `web/` (dark-first with a light theme), a
  separate frontend that calls the Go API.
- Email/password signup and login with Argon2id credentials, revocable
  server-side sessions, CSRF protection, and automatic organization ownership.
- Server-side API proxy forwards secure browser session cookies and never injects
  the local automation token.
- In-console add-instance flow: adapter picker plus a credential form that stores the
  runtime secret and creates the runtime instance.
- In-console adapter settings for per-instance identity, endpoint, control mode,
  credential reference, labels, description, and automatic sync schedule.
- Configurable CORS via `CAPCOM_CORS_ALLOWED_ORIGINS`, with preflight `OPTIONS` bypassing admin auth.
- Docker Compose stack (Postgres + migrations + API + console).
- Unit tests for config, API health, and CORS behavior.
- Post-V1 signed Gantry webhook receiver documented; the LangGraph Agent Server read/control adapter is implemented and live-tested.
- Generic persisted runtime executions with instance, agent, kind, and parent filtering.

The next implementation slice is desired-state manifest apply followed by drift
detection against the durable runtime snapshots.

## Repository Layout

```text
cmd/
  capcom-server/        # API server entrypoint
  capcom/               # CLI entrypoint
internal/
  adapters/runtime/     # Runtime-neutral adapter interface
  adapters/langgraph/   # LangGraph Agent Server adapter and contract fixtures
  api/                  # HTTP router, handlers, auth, and error boundary
  config/               # Environment config
  domain/               # Runtime-neutral Capcom domain types
  store/                # Postgres connection, migrations, repositories
web/                    # Next.js + shadcn operator console (primary UI)
migrations/             # SQL migrations
examples/
  langgraph-agent-server/ # Deterministic local Agent Server fixture
api/
  openapi.yaml          # Current REST API contract
Dockerfile              # Go API + capcom CLI image
docker-compose.yml      # Postgres + migrations + API + console
docs/
  v1/                   # V1 architecture and implementation docs
  console-redesign/     # Console rebuild build spec + design handoff
  Architecture/         # Architecture diagram assets
```

> The console UI lives in `web/` as a separate Next.js app. The Go API does not
> publish browser assets or accept browser-managed administrator tokens.

## Run Locally

### With Docker (recommended)

Brings up the whole stack: Postgres, migrations, the Go API, and the Next.js console:

```bash
cp .env.example .env
# Replace CAPCOM_POSTGRES_PASSWORD and CAPCOM_SECRET_KEY.
docker compose up --build
```

Generate a database password with a password generator and a 32-byte encryption
key with `openssl rand -base64 32`. Keep the Compose database password URL-safe.
For host-run Go commands, also put the same password (percent-encoded when needed)
in `CAPCOM_DATABASE_URL`. Compose fails closed when its required values
are missing; it never supplies a default password, encryption key, or platform
administrator token.

Then open `http://localhost:3000/login` and create the first local account. It
claims ownership of existing local runtime records; later signups receive isolated
personal organizations. Add a runtime with **+ Add instance**, pick an adapter,
then paste the runtime token. Stop with
`docker compose down` (add `-v` to also drop the Postgres volume).

### Backend only (Go)

For backend development, copy the example environment file once:

```bash
cp .env.example .env
```

Then run the server:

```bash
make run
```

Default server address:

```text
:8080
```

### Frontend console (web/)

The primary console is a separate Next.js + shadcn app in `web/`. To run it against a
backend started with `make run`:

```bash
cd web
npm install        # first time only
npm run dev        # http://localhost:3000
```

Point it at the API with the server-only `CAPCOM_API_URL` setting (default
`http://127.0.0.1:8081`). Create an email/password account at `/login`; the Go API
issues an opaque, revocable session and the Next.js proxy forwards its cookies
without injecting an administrator credential. `CAPCOM_ADMIN_TOKEN` remains an
optional local automation/CLI compatibility path. See [web/README.md](web/README.md).

The Agents view separates durable Gantry agents from ephemeral subagent
executions. Gantry must have emitted a `delegated_agent` task lifecycle event
inside a job run before the lower table contains rows; registered agents are
never relabeled as subagents.

Multiple Gantry installations use the same adapter implementation but separate
runtime instances. Give each installation a unique endpoint, secret reference,
stable key, display name, and environment. The console selector shows all three
identity signals and keeps agents and delegated executions scoped to the
selected instance.

The Docker development API resolves host-side Gantry instances through the
`gantry.internal` host-gateway alias declared in `docker-compose.yml`. Use
endpoints such as `http://gantry.internal:8787`, `:8788`, and `:8789` when
Capcom runs in Docker; loopback endpoints only work when the API runs on the
host.

### Connect a LangGraph Agent Server

1. From the LangGraph application directory, install the development CLI and
   bind Agent Server beyond loopback so the Dockerized Capcom API can reach it:

   ```powershell
   python -m pip install -U "langgraph-cli[inmem]"
   langgraph dev --host 0.0.0.0 --port 2024 --no-browser
   ```

2. Verify `http://127.0.0.1:2024/ok` on the host. If Capcom runs in Docker, use
   `http://langgraph.internal:2024` in Capcom; if Capcom runs directly on the
   host, use `http://127.0.0.1:2024`.
3. Sign in to Capcom, choose **Connect an adapter**, select **LangGraph**, and
   enter a stable instance name, the endpoint above, and `read_only` mode.
4. Local `langgraph dev` does not validate authentication, but Capcom's adapter
   contract requires a secret reference. Create a local-only placeholder secret
   such as `langgraph-local-key` with a non-empty placeholder value. For a
   LangSmith-hosted deployment, store a real LangSmith API key instead; Capcom
   sends it as `X-Api-Key`.
5. Save, run **Test connection**, then **Re-import**. Assistants appear as agents;
   recent threads and runs appear as runtime executions. Switch to
   `control_enabled` only when assistant deletion and run cancellation are
   intentionally required.

Agent Server exposes its runtime API documentation at `/docs`. Capcom currently
reads `/ok`, `/info`, assistant search, thread search, and thread runs. See the
[LangGraph adapter contract](docs/v1/17-langgraph-agent-server-adapter.md) for
the exact endpoint and capability matrix. The upstream behavior is documented in
LangChain's [CLI reference](https://docs.langchain.com/langsmith/cli) and
[Agent Server API reference](https://docs.langchain.com/langsmith/server-api-ref).

Health check:

```bash
curl http://127.0.0.1:8080/healthz
```

Expected response:

```json
{
  "status": "ok",
  "service": "capcom",
  "version": "dev"
}
```

Run with the local development database:

```bash
make migrate-up
make run
```

## Configuration

| Variable | Default | Description |
|---|---:|---|
| `CAPCOM_HTTP_ADDR` | `:8080` | HTTP listen address |
| `CAPCOM_HTTP_READ_HEADER_TIMEOUT` | `5s` | HTTP read-header timeout |
| `CAPCOM_HTTP_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown timeout |
| `CAPCOM_SERVICE_VERSION` | `dev` | Version reported by health responses |
| `CAPCOM_LOG_LEVEL` | `info` | One of `debug`, `info`, `warn`, `error` |
| `CAPCOM_DEPLOYMENT_MODE` | `local` | Set `hosted` outside local development; startup then rejects an admin token or insecure cookies |
| `CAPCOM_ADMIN_TOKEN` | empty | Optional local automation/CLI bearer token; do not expose it to browsers |
| `CAPCOM_SECURE_COOKIES` | `true` | Keep enabled behind HTTPS; set `false` only for local HTTP development |
| `CAPCOM_CORS_ALLOWED_ORIGINS` | `http://localhost:3000,http://127.0.0.1:3000` | Comma-separated browser origins allowed to call the API (the Next.js console). Preflight `OPTIONS` bypasses admin auth |
| `CAPCOM_SECRET_KEY` | empty | Base64-encoded 32-byte AES key; required when Postgres is configured |
| `CAPCOM_DATABASE_URL` | empty | Postgres connection string, required for migrations |
| `CAPCOM_DATABASE_MAX_OPEN_CONNS` | `10` | Maximum open Postgres connections |
| `CAPCOM_DATABASE_MAX_IDLE_CONNS` | `5` | Maximum idle Postgres connections |
| `CAPCOM_DATABASE_CONN_MAX_LIFETIME` | `30m` | Maximum Postgres connection lifetime |
| `CAPCOM_SYNC_WORKER_ENABLED` | `true` | Enables periodic runtime synchronization |
| `CAPCOM_SYNC_WORKER_TICK` | `5s` | Scheduler scan interval |
| `CAPCOM_SYNC_MAX_CONCURRENCY` | `4` | Maximum concurrent runtime syncs |
| `CAPCOM_SYNC_REQUEST_TIMEOUT` | `30s` | Timeout for one scheduled sync |
| `CAPCOM_SYNC_MISSING_THRESHOLD` | `3` | Successful absences before stale marking |

## Development Commands

```bash
make test
make vet
make tidy
make migrate-up
make run
```

Equivalent direct Go commands:

```bash
go test ./...
go vet ./...
go mod tidy
go run ./cmd/capcom migrate up
go run ./cmd/capcom-server
```

Repository integration tests run only when `CAPCOM_TEST_DATABASE_URL` points to
a database whose name contains `test`. They clean up records they create and
refuse the development `capcom` database to prevent fixture connections from
appearing in the console.

## Database

Capcom uses Postgres for V1 persistence. Local development values live in `.env`.

For a new checkout:

```bash
cp .env.example .env
make migrate-up
```

`.env.example` contains placeholders only. Never commit the populated `.env`.
For a host-run API, change the database hostname in `CAPCOM_DATABASE_URL` from
`postgres` to the published Postgres host and port.

The initial schema creates:

- `runtime_connections`
- `agents`
- `agent_runtime_bindings`
- `access_desired_state`
- `access_actual_state`
- `drift_findings`
- `control_actions`
- `audit_events`
- `secrets`
- `schema_migrations`

## Runtime Connection API

Generate a local Capcom encryption key once and add it to `.env` as
`CAPCOM_SECRET_KEY=<value>` (base64-encoded 32-byte key):

```bash
openssl rand -base64 32
```

Browser users create an account at `/login` and use the resulting server-side
session. For local CLI automation, optionally set a separate high-entropy
`CAPCOM_ADMIN_TOKEN` and send it as `Authorization: Bearer <admin-token>`. The
examples below use that compatibility path. Leave this variable unset in hosted
deployments; it bypasses tenant authorization and is not a production login mechanism.

Store the Gantry Control API token. The response contains metadata only:

```bash
curl -X POST http://127.0.0.1:8080/v1/secrets \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "gantry-control-api-key",
    "value": "<gantry-token>",
    "actor": "local-dev",
    "reason": "configure Gantry authentication"
  }'
```

Create a Gantry runtime connection:

```bash
curl -X POST http://127.0.0.1:8080/v1/runtime-connections \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "local-gantry",
    "runtime_type": "gantry",
    "mode": "read_only",
    "endpoint": "http://127.0.0.1:8787",
    "auth_ref": "gantry-control-api-key",
    "actor": "local-dev",
    "reason": "initial Gantry connection"
  }'
```

List runtime connections:

```bash
curl http://127.0.0.1:8080/v1/runtime-connections \
  -H "Authorization: Bearer <admin-token>"
```

Get one runtime connection:

```bash
curl http://127.0.0.1:8080/v1/runtime-connections/<runtime-id> \
  -H "Authorization: Bearer <admin-token>"
```

Test a runtime connection through its adapter:

```bash
curl -X POST http://127.0.0.1:8080/v1/runtime-connections/<runtime-id>/test \
  -H "Authorization: Bearer <admin-token>"
```

Read live agents through the configured adapter:

```bash
curl http://127.0.0.1:8080/v1/runtime-connections/<runtime-id>/agents \
  -H "Authorization: Bearer <admin-token>"
```

Read one live agent's canonical access document:

```bash
curl http://127.0.0.1:8080/v1/runtime-connections/<runtime-id>/agents/<runtime-agent-id>/access \
  -H "Authorization: Bearer <admin-token>"
```

Read one live agent's current skill bindings:

```bash
curl http://127.0.0.1:8080/v1/runtime-connections/<runtime-id>/agents/<runtime-agent-id>/skills \
  -H "Authorization: Bearer <admin-token>"
```

These nested agent endpoints are inspection reads. The upcoming sync loop will
persist normalized agents and access state for drift detection.

Runtime connection APIs return `503 database_not_configured` when the server is started without `CAPCOM_DATABASE_URL`.

Rotate a stored credential without changing runtime connections:

```bash
curl -X PUT http://127.0.0.1:8080/v1/secrets/gantry-control-api-key \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "value": "<new-gantry-token>",
    "actor": "local-dev",
    "reason": "scheduled credential rotation"
  }'
```

Capcom never returns secret values. Keep `CAPCOM_SECRET_KEY` stable: changing it
without re-encrypting stored secrets makes existing references undecryptable.

For Gantry runtime connections, `/test` calls Gantry `GET /v1/health` and returns adapter capabilities:

```json
{
  "status": "active",
  "message": "gantry health check succeeded",
  "capabilities": {
    "read_agents": true,
    "read_agent_access": true,
    "replace_agent_access": false
  }
}
```

## API Contract

The current REST contract is maintained in [api/openapi.yaml](api/openapi.yaml).

Use this file as the source of truth for Postman imports, generated clients, and future server-side validation. Do not hand-maintain a separate Postman collection as the primary contract.

## Documentation

The V1 source of truth is [docs/v1/README.md](docs/v1/README.md).

Important docs:

- [Execution implementation plan](docs/v1/13-execution-implementation-plan.md)
- [Development rules](docs/v1/11-development-rules.md)
- [Go coding rulebook](docs/v1/12-go-coding-rulebook.md)
- [Architecture overview](docs/v1/01-architecture-overview.md)
- [Gantry adapter contract](docs/v1/03-gantry-adapter-contract.md)
- [Adapter roadmap and webhook plan](docs/v1/16-adapter-roadmap-and-webhook-plan.md)
- [LangGraph Agent Server adapter](docs/v1/17-langgraph-agent-server-adapter.md)
- [Public release and ownership transfer checklist](docs/public-release-and-transfer-checklist.md)
- [Hosted product foundation plan](docs/post-v1/01-hosted-product-foundation.md)

Documentation and examples must remain portable. Use repository-relative links and
generic placeholders such as `<path-to-capcom>` or `<path-to-gantry>` instead of
developer-specific workstation paths.

## Implementation Rule

Update this README whenever the project gains a new runnable command, package, endpoint, configuration variable, setup requirement, or major implementation milestone.

# Capcom

Capcom is an open-source, runtime-agnostic control plane for operating AI agents. It connects to agent runtimes through adapters and gives operators one place to inspect runtime health, agent topology, capabilities, executions, and usage metrics.

## 1. What is Capcom?

Capcom sits beside your agent runtimes; it does not replace them. Each adapter normalizes a runtime's native API into a common model so Gantry, LangGraph, and future runtimes can be viewed through the same console.

The current product includes:

- a Go API, sync workers, encrypted runtime credentials, and Postgres storage;
- a Next.js operator console with organization-scoped email/password accounts;
- runtime and synchronization health shown as separate signals;
- agent hierarchy, skills, capabilities, executions, and live usage metrics;
- read-only connections by default, with explicit control-mode elevation;
- audited runtime actions and last-known-state retention during outages.

Capcom is under active development. The implementation contract and architecture decisions live in [`docs/v1`](docs/v1/README.md).

## 2. What problem does it solve?

Agent fleets quickly become fragmented across frameworks, processes, and environments. Every runtime exposes different health checks, identity models, permissions, execution data, and telemetry. Operators otherwise have to switch between tools and raw APIs to answer basic questions:

- Which runtimes and agents are online?
- Which agent delegated work to another agent?
- What can each agent access?
- Is the runtime unhealthy, or did synchronization simply fail?
- Where are token usage, latency, errors, and cost changing?
- Can a risky control action be attributed and audited?

Capcom provides a common operational layer while keeping runtime-specific logic inside adapters.

## 3. Clone and start the project

### Prerequisites

- Git
- Docker Desktop with Docker Compose
- OpenSSL, or another secure random-value generator

### Start the complete stack

```bash
git clone https://github.com/rahulanand007/capcom.git
cd capcom
cp .env.example .env
```

On Windows PowerShell, use `Copy-Item .env.example .env` instead of `cp`.

Edit `.env` and replace at least these two values:

```dotenv
CAPCOM_POSTGRES_PASSWORD=<a-unique-url-safe-password>
CAPCOM_SECRET_KEY=<a-base64-encoded-32-byte-key>
```

Generate the encryption key with:

```bash
openssl rand -base64 32
```

Then start Postgres, database migrations, the Go API, and the web console:

```bash
docker compose up --build
```

Open [http://localhost:3000/login](http://localhost:3000/login), create a local account, and sign in. The API is available at [http://localhost:8081/healthz](http://localhost:8081/healthz).

Useful commands:

```bash
docker compose ps
docker compose logs -f api web
docker compose down
```

Do not add `-v` to `docker compose down` unless you intentionally want to delete the local Postgres volume.

## 4. Connect a LangChain/LangGraph runtime

Capcom connects to a LangChain agent through its LangGraph Agent Server API. For a local LangGraph project, install the development CLI and expose the server to Docker:

```bash
python -m pip install -U "langgraph-cli[inmem]"
langgraph dev --host 0.0.0.0 --port 2024 --no-browser
```

Verify [http://127.0.0.1:2024/ok](http://127.0.0.1:2024/ok), then in Capcom:

1. Select **Connect an adapter** and choose **LangGraph**.
2. Enter a stable instance name and start in **read only** mode.
3. When Capcom runs with this repository's Docker Compose stack, use `http://langgraph.internal:2024`. When both processes run directly on the host, use `http://127.0.0.1:2024`.
4. For local `langgraph dev`, provide a non-empty disposable placeholder secret; its no-op authentication does not validate the value. For a hosted LangSmith deployment, store its real API key. Capcom sends it as `X-Api-Key`.
5. Save the connection, test it, and select **Re-import**.

Assistants are imported as agents, while recent threads and runs appear as executions. Enable control mode only when assistant deletion or run cancellation is intentionally required. See the complete [LangGraph adapter contract](docs/v1/17-langgraph-agent-server-adapter.md).

## 5. Connect Gantry

Install and start [Gantry](https://github.com/knacklabs/gantry) first, including its Postgres database and Control API. Configure a Gantry Control API key in the Gantry runtime environment. A read-only Capcom connection needs these scopes:

```text
sessions:read
agents:admin
jobs:read
skills:read
mcp:read
usage:read
```

`usage:read` is required for the Metrics page. Follow Gantry's repository instructions to generate the token and restart Gantry after changing its environment.

Verify the local Control API before connecting it:

```bash
curl http://127.0.0.1:8787/v1/health
```

Then in Capcom:

1. Select **Connect an adapter** and choose **Gantry**.
2. Enter a stable instance name, environment, and **read only** mode.
3. Use `http://gantry.internal:8787` when Capcom runs in Docker, or `http://127.0.0.1:8787` when Capcom runs directly on the host.
4. Paste the Gantry Control API token into the credential field. Capcom encrypts it before storage; do not put it in the endpoint URL.
5. Save the connection, test it, and select **Re-import**.

Capcom will import the Gantry installation as one runtime instance and classify its registered and delegated agents beneath it. A hosted Capcom deployment cannot directly reach a Gantry endpoint bound to localhost; expose Gantry through an authenticated HTTPS endpoint or a secure network connector first.

# Capcom Gantry Local Runbook

## Goal

Run a local Gantry runtime and connect Capcom to it through the Gantry Control API. The MVP should use socket or loopback HTTP transport and should not require signed webhooks.

## Prerequisites

- Windows host with PowerShell.
- Docker Desktop running with the Linux engine available.
- Node.js compatible with Gantry package requirements: `>=24 <26`.
- Local Gantry source checkout available.
- A writable `GANTRY_HOME` directory with `.env` and `settings.yaml`.
- Postgres reachable by Gantry.

## Current Known State

From the latest local integration snapshot:

- Gantry package: `@gantry/runtime@1.2.52`.
- Gantry dependencies install successfully with `npm.cmd ci`.
- Gantry build passes with `npm.cmd run build`.
- Docker/Postgres was blocked because the Docker Desktop Linux engine was not reachable.
- `gantry doctor` was blocked when a fresh `GANTRY_HOME` did not contain `settings.yaml`.

## Environment Variables

Gantry reads runtime secrets from `<GANTRY_HOME>/.env`.

Required or commonly used values:

```text
GANTRY_DATABASE_URL=
SECRET_ENCRYPTION_KEY=
GANTRY_IPC_AUTH_SECRET=
GANTRY_CONTROL_API_KEY=
GANTRY_CONTROL_API_KEYS_JSON=
GANTRY_CONTROL_HOST=127.0.0.1
GANTRY_CONTROL_PORT=8787
GANTRY_CONTROL_SOCKET_PATH=
GANTRY_CONTROL_BASE_URL=http://127.0.0.1:8787
```

For MVP control mode, the Gantry API key should include scopes for:

```text
agents:admin
sessions:read
jobs:read
jobs:write
providers:read
conversations:read
```

Webhook scopes can be omitted for the local MVP unless webhook mode is explicitly being tested.

## Startup Procedure

1. Start Docker Desktop.
2. Confirm Docker is reachable:

```powershell
docker ps
```

3. Start Gantry Postgres from the Gantry repo:

```powershell
cd <path-to-gantry>
docker compose up -d postgres
```

4. Confirm `GANTRY_HOME` has both `.env` and `settings.yaml`.
5. Install and build Gantry if needed:

```powershell
npm.cmd ci
npm.cmd run build
```

6. Start the Gantry runtime/control server using the Gantry project command for local development.
7. Verify health:

```powershell
Invoke-RestMethod http://127.0.0.1:8787/healthz
Invoke-RestMethod http://127.0.0.1:8787/v1/health
Invoke-RestMethod http://127.0.0.1:8787/v1/doctor
```

8. Capture the OpenAPI snapshot once the runtime is live:

```powershell
Invoke-RestMethod http://127.0.0.1:8787/openapi.json | ConvertTo-Json -Depth 100 > docs\gantry-openapi.snapshot.json
```

## Capcom Connection Procedure

Create a Capcom runtime connection using:

- type: `gantry`
- mode: `control_enabled` or `read_only`
- base URL: `http://127.0.0.1:8787`
- API key: Gantry Control API key stored as a secret reference

Expected connection behavior:

- `GET /v1/health` must pass before activation.
- `GET /v1/doctor` must pass or return only non-blocking warnings.
- Invalid credentials must not create an active runtime connection.
- Runtime outages must mark the connection degraded and preserve last known imported state.

## Verification Commands

Health and runtime readiness:

```powershell
Invoke-RestMethod http://127.0.0.1:8787/v1/health
Invoke-RestMethod http://127.0.0.1:8787/v1/doctor
```

Inventory:

```powershell
Invoke-RestMethod http://127.0.0.1:8787/v1/agents
Invoke-RestMethod http://127.0.0.1:8787/v1/inventory
Invoke-RestMethod http://127.0.0.1:8787/v1/capabilities
```

Agent access:

```powershell
Invoke-RestMethod http://127.0.0.1:8787/v1/agents/<agentId>/access
```

Events:

```powershell
Invoke-RestMethod http://127.0.0.1:8787/v1/runs
Invoke-RestMethod http://127.0.0.1:8787/v1/jobs
```

## Troubleshooting

| Symptom | Likely Cause | Action |
|---|---|---|
| Docker pipe not found | Docker Desktop is stopped or Linux engine unavailable | Start Docker Desktop and confirm `docker ps` works |
| `gantry doctor` fails for missing settings | `GANTRY_HOME` points to a fresh directory | Generate or copy a valid `settings.yaml` |
| Health passes but doctor fails | Runtime config or database issue | Treat connection as failed unless the failure is documented as non-blocking |
| Agents import but events do not | Event stream/list endpoint unavailable or no sessions/runs exist | Use polling fallback and preserve cursor state |
| Control action fails | API key missing write/admin scope | Mark action failed, write audit entry, and do not retry indefinitely |

## Do Not Do

- Do not read Gantry database tables directly.
- Do not edit Gantry `settings.yaml` from Capcom.
- Do not require public webhook callbacks for the local MVP.
- Do not delete imported Capcom agents when Gantry is temporarily unavailable.


# 10 - V1 Build Plan

## Build Principle

Build one excellent end-to-end control loop before adding more integrations.

## Milestone 0 - Repository Scaffold

Deliver:

- Go module
- server binary
- CLI binary
- config loading
- structured logging
- Postgres connection
- migrations runner
- `GET /healthz`

Acceptance:

- server starts locally
- migrations apply
- health endpoint returns ok

## Milestone 1 - Runtime Connections

Deliver:

- runtime connection table
- secret storage
- create/list/get runtime APIs
- Gantry health/doctor adapter
- connection test endpoint

Acceptance:

- valid Gantry credentials activate
- invalid credentials fail
- read-only/control mode stored

## Milestone 2 - Gantry Import

Deliver:

- Gantry `ListAgents`
- `GetAgentAdmin`
- `GetAgentAccess`
- `ListInventory`
- agent and actual-state persistence
- manual sync endpoint

Acceptance:

- Gantry agents appear in Capcom
- actual access document is stored
- runtime outage preserves last known state

## Milestone 3 - Desired State And Manifests

Deliver:

- YAML parser/validator
- `RuntimeConnection` manifest
- `Agent` manifest
- apply endpoint
- CLI `capcom apply -f`

Acceptance:

- applying manifest stores desired state
- invalid schema returns useful error
- audit row is written

## Milestone 4 - Drift Detection

Deliver:

- compare desired status vs actual status
- compare desired selections vs actual selections
- drift table
- drift list APIs
- CLI `capcom diff agent`

Acceptance:

- extra capability creates drift
- missing capability creates drift
- resolved drift is marked resolved after sync/apply

## Milestone 5 - Control Actions

Deliver:

- control action table
- disable/enable agent
- restrict capability
- replace access
- pre/post audit entries

Acceptance:

- read-only runtime rejects mutation
- control-enabled runtime executes mutation
- audit shows actor, reason, before, after, result

## Milestone 6 - Minimal Dashboard

Deliver screens:

- Runtime Connections
- Fleet View
- Agent Detail
- Drift
- Audit Log

Acceptance:

- demo can be completed from dashboard
- dashboard never hides runtime degraded state

## Milestone 7 - Demo Hardening

Deliver:

- seed/demo manifests
- smoke test script
- recorded demo checklist
- docs update from implementation findings

Acceptance:

- full demo completes twice from clean local setup

## Test Strategy

Unit tests:

- manifest validation
- drift comparison
- control action validation
- Gantry response normalization

Integration tests:

- Postgres repositories
- migration apply
- API endpoint happy paths

Contract tests:

- recorded Gantry JSON fixtures
- adapter request/response mappings

Manual tests:

- local Gantry runtime
- invalid credentials
- runtime outage
- capability drift and restriction

## V1 Demo Script

1. Start Gantry.
2. Start Capcom.
3. Add Gantry runtime connection.
4. Sync agents.
5. Open Fleet View.
6. Open Agent Detail.
7. Apply desired Agent manifest.
8. Introduce extra capability in Gantry.
9. Sync Capcom.
10. Show drift.
11. Restrict capability from Capcom.
12. Sync again.
13. Show drift resolved.
14. Show audit log.

## Explicit Post-V1 Backlog

- full RBAC/SSO
- signed Gantry webhook inbox, deduplication, and targeted-sync triggers
- enforce mode
- Kubernetes operator
- LangGraph Agent Server invocation and broader lifecycle actions (assistant
  deletion and run cancellation implemented 2026-07-29)
- Amazon Bedrock AgentCore runtime adapter
- Microsoft Foundry Agent Service runtime adapter
- Vertex AI Agent Engine runtime adapter
- OpenTelemetry/OpenInference ingestion for embedded agents
- LangSmith observability, Langfuse, and Phoenix telemetry connectors
- Microsoft/identity registry integrations
- business value and topology metadata
- multi-runtime adapter marketplace

See `16-adapter-roadmap-and-webhook-plan.md` for the Gantry completion matrix,
webhook design, adapter ranking, and enterprise deployment research.

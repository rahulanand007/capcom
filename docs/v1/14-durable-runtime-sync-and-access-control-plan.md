# 14 - Durable Runtime Sync And Access Control Plan

Status: implemented on 2026-07-14. Desired-state apply and drift detection remain
the next V1 slices.

## Purpose

This plan takes Capcom from live Gantry inspection to a durable, runtime-neutral
control-plane loop. It covers:

- manual and periodic runtime synchronization
- persisted agents, hierarchy, skills, skill bindings, and effective access
- last-known-state behavior when a runtime is unavailable
- fresh, cached, and stale data semantics
- manual sync controls and sync visibility in the console
- adapter contract fixtures and integration coverage
- audited access reconciliation after the read path is dependable

The implementation must preserve the product boundary: Gantry supplies runtime
state through an adapter, while Capcom services and storage operate only on
normalized domain types.

## Current Baseline

Already implemented:

- encrypted runtime credentials referenced by `auth_ref`
- persisted runtime connections and append-only audit events
- Gantry health, agent, hierarchy, access, and current-skill reads
- runtime-neutral adapter snapshots
- live diagnostic endpoints under `/v1/runtime-connections/{id}/agents`
- an embedded console that can inspect live agent details and skills

Missing from the durable loop:

- sync-run persistence and orchestration
- repositories for imported agents and actual state
- persisted skill catalog and agent-skill bindings
- missing-agent tracking without deletion
- scheduled polling and overlap protection
- persisted fleet APIs and freshness metadata
- audited access mutation and post-action verification

## Scope And Non-Goals

### In Scope

- One-process worker scheduling with database-backed overlap protection.
- Manual sync and configurable periodic sync.
- Full-snapshot synchronization for the Gantry adapter.
- Last known state retained across adapter failures.
- Three consecutive successful absences before an agent is marked stale.
- Read APIs backed by Postgres for fleet and agent views.
- A single safe V1 control action: replace an agent's effective access document.

### Out Of Scope

- Distributed worker coordination beyond a Postgres lock.
- Webhooks, streaming ingestion, or runtime event replay.
- Capcom-managed Gantry skill installation.
- Automatic remediation without an explicit operator request.
- Generic workflow execution, job scheduling, or provider management.
- Hard deletion of imported agents, skills, or bindings.

## Architectural Decisions

### Live Versus Persisted Reads

Keep the existing nested runtime endpoints as explicit diagnostic reads:

```text
GET /v1/runtime-connections/{id}/agents
GET /v1/runtime-connections/{id}/agents/{runtimeAgentId}/access
GET /v1/runtime-connections/{id}/agents/{runtimeAgentId}/skills
```

Add persisted fleet endpoints for normal console operation:

```text
GET /v1/agents
GET /v1/agents/{id}
GET /v1/agents/{id}/access
GET /v1/agents/{id}/skills
```

Every persisted response includes observation and freshness metadata. A runtime
failure never causes the API to silently substitute an empty collection.

### Snapshot Boundary

The adapter returns one normalized `RuntimeSnapshot` containing:

- runtime health and capabilities
- all agents and hierarchy references
- skills assigned to each agent
- effective access for each agent
- observation timestamps and optional raw payloads

The sync service validates the complete snapshot before beginning the database
replacement transaction. Adapter DTOs do not cross into services or stores.

### Atomicity

Network calls occur outside database transactions. After a complete snapshot is
available, one transaction upserts agents, bindings, skills, skill assignments,
and access documents, then marks the run successful. Partial adapter reads do
not replace previously stored state.

### Concurrency

Use a Postgres advisory lock keyed by runtime connection ID. Manual and periodic
sync attempts share the same lock. If the lock is held, return a conflict result
instead of starting an overlapping sync.

### Freshness States

Persisted reads expose:

- `live`: observed by a successful sync inside the configured freshness window
- `cached`: last sync succeeded but the freshness window has elapsed
- `stale`: the last sync failed, the runtime is degraded, or the entity crossed
  its missing-sync threshold

The frontend derives no freshness state from browser time alone; the API returns
the state and timestamps.

## Data Model Changes

Add migration `003_runtime_sync.sql`.

### runtime_sync_runs

Typed fields:

- `id uuid primary key`
- `runtime_connection_id uuid not null`
- `trigger text`: `manual`, `scheduled`, or `post_action`
- `status text`: `running`, `succeeded`, `failed`, or `skipped`
- `started_at`, `finished_at`, `duration_ms`
- `agents_seen`, `skills_seen`, `bindings_seen`, `access_documents_seen`
- `error_code`, `error_message`
- `metadata_json jsonb`

Indexes:

- `(runtime_connection_id, started_at desc)`
- `(status, started_at desc)`

### runtime_connections additions

- `sync_enabled boolean not null default true`
- `sync_interval_seconds integer not null default 60`
- `last_sync_status text`
- `last_sync_started_at timestamptz`
- `last_sync_finished_at timestamptz`
- `last_sync_duration_ms bigint`
- retain `last_sync_at` as the last successful completion time
- retain `last_error` as the operator-facing latest failure

Validate interval bounds in the service, with a V1 range of 15 to 86,400
seconds.

### agent_runtime_bindings additions

- `kind text`: `main`, `registered`, or `subagent`
- `parent_runtime_agent_id text`
- `last_seen_at timestamptz`
- `last_seen_sync_run_id uuid`
- `missing_successful_syncs integer not null default 0`
- `raw_runtime_json jsonb`

The runtime ID remains the adapter identity. Parent relationships are stored as
runtime references so a parent can be resolved after all bindings are upserted.

### runtime_skills

- runtime-scoped normalized skill identity
- `runtime_connection_id`, `runtime_skill_id`, name, description, source, status,
  version
- `tool_ids_json`, `workflow_refs_json`, `metadata_json`, `raw_runtime_json`
- `observed_at`, `last_seen_sync_run_id`, `missing_successful_syncs`
- unique `(runtime_connection_id, runtime_skill_id)`

### agent_skill_bindings

- `agent_id`, `runtime_skill_id`
- `status`: `active` or `stale`
- `observed_at`, `last_seen_sync_run_id`, `missing_successful_syncs`
- unique `(agent_id, runtime_skill_id)`

### access_actual_state additions

- `last_seen_sync_run_id uuid`
- `freshness_status text`
- retain normalized `access_json` and debugging-only `raw_runtime_json`

No migration hard-deletes existing state. Backfills use conservative defaults
and preserve all existing rows.

## Domain And Adapter Contracts

### Domain Types

Add runtime-neutral types for:

- `RuntimeSnapshot`
- `RuntimeSyncRun`
- `SyncTrigger` and `SyncStatus`
- `SnapshotAgent` with skills and access
- `FreshnessStatus`
- `SyncSummary`

The snapshot should carry stable runtime IDs, not database IDs. Database IDs are
resolved by repositories during persistence.

### Adapter Interface

Add `CollectSnapshot(ctx, connection)` to the adapter contract. Gantry may build
it from its current endpoint methods, but the sync service receives one complete
normalized result.

Keep the existing fine-grained methods for diagnostics and control validation.
Future adapters implement the same snapshot contract without importing Gantry
types into core packages.

### Gantry Collection Order

1. Check health and declared capabilities.
2. List agents.
3. For each agent, fetch access and assigned skills with bounded concurrency.
4. Join skill bindings with the Gantry skill catalog.
5. Validate IDs, hierarchy references, and observation timestamps.
6. Return the complete normalized snapshot.

Use a small worker limit, default `4`, and cancel remaining requests when a
required collection step fails. Retry only transient reads with capped backoff.

## Sync Service

Add a dedicated `RuntimeSyncService`; do not expand HTTP handlers or the runtime
connection service into orchestration layers.

### Manual Or Scheduled Sync Flow

1. Validate the runtime connection and registered adapter.
2. Attempt to acquire the runtime sync lock.
3. Create a `running` sync-run record.
4. Collect and validate a complete adapter snapshot.
5. Persist the snapshot in one database transaction.
6. Increment missing counters only after a complete successful snapshot.
7. Mark entities stale when the counter reaches `3`.
8. Reset counters for observed entities and reactivate returning entities.
9. Mark the runtime active and complete the sync run.
10. Write a summarized `runtime.sync_succeeded` audit event.
11. Release the lock.

### Failure Flow

On health, authentication, timeout, normalization, or persistence failure:

- mark the sync run failed with a stable error code
- mark the runtime degraded and save a sanitized `last_error`
- do not increment missing counters
- do not overwrite imported agents, skills, bindings, or access
- do not resolve drift
- write `runtime.sync_failed` audit metadata without secrets or raw headers
- release the lock

### Idempotency

Upsert by runtime-scoped external identity. Repeating the same snapshot changes
only observation timestamps and sync-run references. It must not create duplicate
agents, skills, access records, or audit rows beyond the new sync attempt itself.

## Repository Work

Create focused repository interfaces and Postgres implementations for:

- sync runs
- imported agents and runtime bindings
- runtime skills and agent-skill bindings
- actual access state
- runtime sync status updates
- runtime sync locking

Expose a single transactional `PersistRuntimeSnapshot` store operation so the
service cannot accidentally commit a half-imported snapshot. Repositories contain
SQL and mapping logic only; missing thresholds and freshness decisions remain in
the service/domain layer.

## API Contract

### Manual Sync

```text
POST /v1/runtime-connections/{id}/sync
```

Request:

```json
{
  "actor": "operator@example.com",
  "reason": "refresh Gantry state before access review"
}
```

V1 waits for completion and returns `200` with the sync summary. Return `409` if
that runtime is already syncing, `404` for an unknown runtime, and `502` for a
completed adapter failure. The failed run remains queryable.

### Sync History

```text
GET /v1/runtime-connections/{id}/sync-runs?limit=20
GET /v1/runtime-connections/{id}/sync-runs/{runId}
```

### Persisted Fleet Reads

```text
GET /v1/agents?runtime_connection_id={id}&status={status}
GET /v1/agents/{id}
GET /v1/agents/{id}/access
GET /v1/agents/{id}/skills
```

Responses include `observed_at`, `freshness`, `last_successful_sync_at`, and
runtime connection status. Update `api/openapi.yaml` in the same implementation
slice as each endpoint.

## Periodic Worker

Add `internal/workers/runtime_sync.go` with `Run(ctx)` lifecycle semantics.

- Poll for due runtime connections at a short fixed tick.
- Schedule each enabled runtime according to its own interval.
- Reuse `RuntimeSyncService`; do not duplicate sync logic.
- Limit concurrent runtime syncs with a configurable semaphore.
- Respect context cancellation and server graceful shutdown.
- Skip disabled connections.
- Record lock conflicts as skipped runs only when operationally useful; avoid
  noisy audit events for expected overlap.

Configuration:

```text
CAPCOM_SYNC_WORKER_ENABLED=true
CAPCOM_SYNC_WORKER_TICK=5s
CAPCOM_SYNC_MAX_CONCURRENCY=4
CAPCOM_SYNC_REQUEST_TIMEOUT=30s
CAPCOM_SYNC_MISSING_THRESHOLD=3
```

Defaults belong in Go config, with validation and `.env.example` updates.

## Console Changes

### Runtime View

Add:

- icon-based `Sync now` action with tooltip
- current sync state and last result
- last successful sync time and duration
- next scheduled sync time
- imported counts and latest sanitized error

Disable the action while a sync is running. Refresh the selected runtime and
persisted fleet after completion.

### Agent View

Switch the normal table and detail drawer to persisted fleet endpoints. Show:

- fresh, cached, or stale state
- observation timestamp
- source runtime
- main, registered, or subagent relationship
- persisted skills and effective access

The fleet list is a topology projection over persisted agents and durable
`agent_delegations`; it does not rewrite many-to-many delegation edges into
`parent_runtime_agent_id`. Build and render the projection independently for
each runtime instance using this deterministic order:

1. main agents, ordered by name
2. agents reachable from those mains, breadth-first and ordered by name
3. standalone roots and their reachable delegates
4. rootless/cyclic components, ordered by name and explicitly marked as cycles

Render a durable agent once even when it has multiple orchestrators and show
all incoming relationships as `Delegated by` labels. Indent reachable delegates
and draw a connector from their displayed depth. Show unresolved delegate
references on the orchestrator rather than inventing an agent row. Main,
delegated, standalone, stale, and cycle state must remain visibly distinct.

Fleet search matches agent, runtime, role, freshness, orchestrator, and
unresolved-reference fields. When a delegate matches, retain every transitive
orchestrator as dimmed context so a filtered result never loses its delegation
provenance. Cycles must be guarded by visited-node tracking.

Keep a separate live inspection command for diagnostics rather than mixing live
and cached values in one response.

### Failure Experience

When Gantry is unavailable, retain rows and details, label them stale, and show
the last successful observation. Never replace a failed read with `0 agents` or
`0 skills`.

## Controlled Access Phase

Begin only after durable sync acceptance gates pass.

### API

```text
POST /v1/agents/{id}/actions/reconcile-access
```

Request requires:

- complete desired access document
- `actor`
- `reason`
- `idempotency_key`
- optional `dry_run`

### Validation

- Resolve the persisted agent and runtime binding.
- Reject `read_only` runtime connections.
- Confirm the adapter declares `ReplaceAgentAccess` support.
- Validate selections against the latest persisted inventory.
- Require fresh actual access before mutation; trigger or request a sync when
  state is stale.
- Reject unchanged requests unless explicitly submitted as a dry run.

### Execution

1. Store a queued control action and immutable pre-action audit record.
2. Move the action to running.
3. Call the runtime adapter without holding a database transaction open.
4. Store the sanitized runtime result and terminal action state.
5. Run a `post_action` sync.
6. Mark the action verified only when the observed access matches the request.
7. Write the post-action audit record with before, requested, observed, and
   result data.

Unknown timeout outcomes are not automatically retried. A fresh sync determines
actual state before an operator can submit another action.

### Console

Add an access editor only for `control_enabled` runtimes. Include a dry-run
preview, actor, reason, confirmation, action result, and link to audit history.
The UI must clearly separate viewing assigned skills from changing effective
access; Capcom does not install skills in Gantry.

## Test Strategy

### Unit Tests

- snapshot validation and normalization
- freshness calculation
- missing-counter threshold and returning-agent recovery
- idempotent sync decisions
- retry classification and sanitized errors
- read-only and unsupported-action rejection
- access reconciliation validation and idempotency

### Adapter Contract Tests

Add recorded or handcrafted Gantry fixtures for:

- main agent with two skills
- registered agent and parented subagent
- full access document
- empty skill bindings
- missing parent reference
- unauthorized, timeout, malformed payload, and unavailable runtime responses

Normal tests must not require a live Gantry server.

### Repository Integration Tests

- first import and repeated import
- hierarchy persistence
- skill catalog and binding upserts
- transactional rollback on an invalid snapshot
- three successful absences mark stale
- failed sync preserves rows and counters
- returning agent becomes active
- advisory lock blocks overlap
- sync history and runtime status updates

### API Tests

- manual sync success, failure, conflict, auth, and validation
- persisted fleet filtering and freshness fields
- fleet topology ordering for multiple mains, cycles, unresolved delegates,
  multiple orchestrators, and ancestor-preserving search
- stale data remains readable after adapter failure
- control request actor/reason/idempotency requirements
- secrets and authorization headers never appear in responses

### End-To-End Verification

1. Start Gantry and Capcom.
2. Sync and verify the main agent and its two Gantry-installed skills.
3. Stop Gantry and run sync; confirm degraded runtime and retained stale data.
4. Restart Gantry and sync; confirm recovery without duplicate records.
5. Run three successful snapshots with an agent absent; confirm stale marking.
6. On a control-enabled test connection, dry-run and reconcile access.
7. Confirm post-action sync and audit history.

## Implementation Sequence

### Slice 1 - Schema And Domain Foundation

- Add migration and domain types.
- Add config defaults and validation.
- Update database and architecture docs if implementation changes the model.

Gate: migrations run forward on a populated local database and repository tests
can insert all new entities.

### Slice 2 - Snapshot Adapter Contract

- Add the runtime-neutral snapshot contract.
- Implement Gantry collection with bounded concurrency.
- Add fixture contract tests.

Gate: one adapter call returns a validated normalized snapshot containing the
main agent, hierarchy, skills, and access.

### Slice 3 - Transactional Persistence

- Implement repositories, sync lock, and `PersistRuntimeSnapshot`.
- Cover idempotency, missing counters, and rollback behavior.

Gate: repeated snapshots are duplicate-free and partial failure changes no
last-known runtime state.

### Slice 4 - Manual Sync API

- Implement `RuntimeSyncService`.
- Add sync and sync-history endpoints.
- Update OpenAPI and API tests.

Gate: an operator can run and inspect a complete audited sync through HTTP.

### Slice 5 - Persisted Fleet APIs And Console

- Add persisted agent, skill, and access endpoints.
- Switch the primary console views to persisted reads.
- Add Sync now and freshness/error presentation.

Gate: the console remains useful and truthful while Gantry is stopped.

### Slice 6 - Periodic Worker

- Add scheduling, graceful shutdown, concurrency limits, and due-runtime queries.
- Add deterministic worker tests with a controllable clock.

Gate: periodic sync runs without overlap and stops cleanly with the server.

### Slice 7 - Controlled Access Backend

- Add control-action validation, lifecycle, adapter mutation, and post-action sync.
- Add complete audit and failure-path tests.

Gate: read-only connections reject changes and a control-enabled connection can
be safely reconciled and verified.

### Slice 8 - Access Console And UAT

- Add dry-run preview and confirmation UI.
- Execute the end-to-end verification checklist.
- Update README run and test instructions.

Gate: the V1 demonstration works without database inspection or manual API
payload construction.

## Completion Criteria

- Manual and periodic sync import normalized agents, hierarchy, skills, bindings,
  and access.
- Repeated syncs are idempotent and non-overlapping.
- Runtime failures preserve last known state and expose stale/degraded status.
- Missing entities are never removed after one snapshot.
- The console distinguishes live, cached, and stale data.
- Gantry fixtures cover the adapter contract without a live dependency.
- Access replacement requires a control-enabled connection, actor, reason, and
  idempotency key.
- Every control action has before, request, observed result, and audit history.
- Capcom never installs Gantry skills and never reads Gantry database tables.

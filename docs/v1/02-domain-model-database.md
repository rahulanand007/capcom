# 02 - Domain Model And Database Design

## Entity Overview

| Entity | Purpose |
|---|---|
| RuntimeConnection | Distinguishable runtime instance and credential reference |
| RuntimeSyncRun | One runtime sync attempt and result |
| Agent | Normalized agent registry record |
| AgentDesiredState | Approved Capcom state for an agent |
| AgentActualState | Latest observed runtime state |
| AgentEvent | Normalized runtime event timeline |
| DriftRecord | Desired-vs-actual mismatch |
| ControlAction | Operator-requested runtime mutation |
| AuditLog | Immutable record of sync and mutation outcomes |
| SecretRef | Local encrypted or external secret reference |
| User | Email/password identity |
| Organization | Tenant and ownership boundary |
| Membership | User role in an organization |
| Session | Revocable hashed browser session |

## ID Strategy

- Use Capcom UUIDs for internal primary keys.
- Preserve runtime IDs as `external_id`.
- Runtime identity is always `(runtime_connection_id, external_id)`.
- Agent names are display identifiers, not stable keys.

## Tables

### Identity and tenant ownership

Migrations `013_hosted_identity.sql` and `014_identity_lifecycle_fields.sql` add
`users`, `password_credentials`, `organizations`, `organization_memberships`,
and `user_sessions`. Password rows
store Argon2id encoded hashes. Session rows store only SHA-256 hashes of opaque
session and CSRF tokens with idle and absolute expiration timestamps.

`runtime_connections`, `secrets`, and `audit_events` carry a non-null
`organization_id`. Child runtime records inherit their tenant through the runtime
foreign key and repository reads join that ownership boundary. Existing local
records are assigned to the deterministic `capcom-local` bootstrap organization;
the first signup claims its owner membership.

Migration `015_tenant_resource_ownership.sql` also materializes non-null tenant
ownership on agents, control actions, sync runs, normalized usage observations,
and telemetry ingestion runs. Their write paths derive the organization from the
referenced runtime connection; they never accept it from an API request.

### Runtime diagnostics and catalog

Migration `007_runtime_inventory_and_diagnostics.sql` adds three
instance-scoped last-known-state tables:

| Table | Identity | Purpose |
|---|---|---|
| `runtime_diagnostics` | runtime connection + check id | Latest normalized doctor result and message |
| `runtime_inventory_items` | runtime connection + kind + runtime item id | Onboarded tools, skills, and MCP servers |
| `runtime_capabilities` | runtime connection + capability id + version | Approved immutable capability manifests |

Frequently filtered identity, status, kind, category, risk, source, and
observation fields use typed columns. Adapter-specific policy and raw payloads
remain JSONB. A failed sync does not delete the last successful catalog.

### runtime_connections

| Column | Type | Notes |
|---|---|---|
| id | uuid pk | Internal id |
| organization_id | uuid fk | Owning tenant |
| name | text | Stable tenant-scoped instance key, for example `gantry-development` |
| display_name | text | Mutable operator-facing name |
| environment | text | Environment slug such as `development` or `production` |
| labels_json | jsonb | Team, region, owner, and other display/filter labels |
| runtime_type | text | `gantry` in V1 |
| mode | text | `read_only`, `control_enabled` |
| endpoint_kind | text | `base_url`, `socket` |
| endpoint | text | URL or socket path |
| auth_ref | text | Secret ref |
| status | text | `pending`, `active`, `degraded`, `disabled`, `failed` |
| last_sync_at | timestamptz null | Last successful sync |
| last_error | text null | Latest failure |
| sync_enabled | boolean | Enables periodic polling |
| sync_interval_seconds | integer | Per-runtime polling interval |
| last_sync_status | text null | Latest run state |
| last_sync_started_at | timestamptz null | Latest attempt start |
| last_sync_finished_at | timestamptz null | Latest attempt finish |
| last_sync_duration_ms | bigint null | Latest attempt duration |
| created_at | timestamptz |  |
| updated_at | timestamptz |  |

Indexes:

- unique `(organization_id, name)` for active instances
- unique normalized `(organization_id, runtime_type, endpoint)` for active instances
- index `(runtime_type, status)`
- index `(environment, status)`

### secrets

| Column | Type | Notes |
|---|---|---|
| id | uuid pk | Generated identifier |
| organization_id | uuid fk | Owning tenant |
| name | text | Tenant-scoped stable reference used by `runtime_connections.auth_ref` |
| ciphertext | bytea | Versioned AES-256-GCM payload; never returned by APIs |
| created_at | timestamptz | Creation time |
| updated_at | timestamptz | Last rotation time |

The AES key comes from `CAPCOM_SECRET_KEY` and is never stored in Postgres. The
secret name is authenticated as AES-GCM associated data so ciphertext cannot be
silently reassigned to a different reference.

### runtime_sync_runs

| Column | Type | Notes |
|---|---|---|
| id | uuid pk |  |
| runtime_connection_id | uuid fk |  |
| trigger | text | `manual`, `scheduled`, `post_action` |
| status | text | `running`, `succeeded`, `failed`, `skipped` |
| started_at | timestamptz |  |
| finished_at | timestamptz null |  |
| agents_seen | integer |  |
| skills_seen | integer | Assigned skills observed |
| bindings_seen | integer | Agent-skill bindings observed |
| access_documents_seen | integer | Access documents observed |
| duration_ms | bigint null |  |
| error_code | text null | Stable failure category |
| error_message | text null | Sanitized failure detail |

### agents and agent_runtime_bindings

| Column | Type | Notes |
|---|---|---|
| id | uuid pk | Capcom agent id |
| name | text | Runtime display name |
| status | text | `unknown`, `enabled`, `disabled`, `stale` |
| owner_business | text null | Desired/metadata |
| owner_technical | text null | Desired/metadata |
| escalation_contact | text null | Desired/metadata |
| purpose | text null | Desired/metadata |
| environment | text null | Desired/metadata |
| risk_level | text | `low`, `medium`, `high`, `critical` |
| created_at | timestamptz |  |
| updated_at | timestamptz |  |

Indexes:

- index `(status)`
- index `(risk_level)`

Runtime identity and observation state live in `agent_runtime_bindings`:

| Column | Type | Notes |
|---|---|---|
| agent_id | uuid fk | Capcom agent id |
| runtime_connection_id | uuid fk | Source runtime |
| runtime_agent_id | text | Stable runtime identity |
| kind | text | `main`, `registered`, `subagent` |
| parent_runtime_agent_id | text null | Runtime parent reference |
| last_seen_at | timestamptz null | Last successful observation |
| last_seen_sync_run_id | uuid fk null | Sync provenance |
| missing_successful_syncs | integer | Consecutive successful absences |
| raw_runtime_json | jsonb | Debug/audit payload |

Unique key: `(runtime_connection_id, runtime_agent_id)`.

`registered` means a durable runtime agent without a parent relationship. Capcom
must not infer `subagent` merely because an agent is not the main agent. When an
adapter cannot read hierarchy, `parent_external_id` remains null and the runtime
capability reports hierarchy as unsupported.

### runtime_skills and agent_skill_bindings

| Column | Type | Notes |
|---|---|---|
| id | uuid pk |  |
| runtime_connection_id | uuid fk | Source runtime |
| runtime_skill_id | text | Stable runtime skill identifier |
| name | text | Runtime display name or identifier fallback |
| status | text | Current binding status |
| version | text null | Runtime config/version reference |
| metadata_json | jsonb | Adapter-normalized metadata |
| tool_ids_json | jsonb | Normalized tool references |
| workflow_refs_json | jsonb | Normalized workflow references |
| observed_at | timestamptz | Last successful observation |
| last_seen_sync_run_id | uuid fk null | Sync provenance |
| missing_successful_syncs | integer | Consecutive successful absences |

Unique key: `(runtime_connection_id, runtime_skill_id)`.

`agent_skill_bindings` links agents to `runtime_skills` and stores binding
status, observation time, sync provenance, and missing counters. Missing skills
or bindings are marked stale after repeated complete successful snapshots; they
are not hard-deleted.

### agent_delegations

| Column | Type | Notes |
|---|---|---|
| runtime_connection_id | uuid fk | Source runtime instance |
| orchestrator_runtime_agent_id | text | Agent that can call the delegate |
| delegate_key | text | Stable resolved runtime id or unresolved reference key |
| delegate_runtime_agent_id | text | Resolved runtime agent id, when available |
| delegate_ref | text | Runtime desired-state reference |
| tool_name, display_name, persona | text | Normalized callable-agent presentation |
| configured, resolved | boolean | Desired-state and runtime resolution signals |
| revision | integer | Runtime desired-state revision |
| status | text | `active` or `stale` |
| observed_at | timestamptz | Last successful observation |
| last_seen_sync_run_id | uuid fk null | Sync provenance |
| missing_successful_syncs | integer | Consecutive successful absences |
| metadata_json, raw_runtime_json | jsonb | Normalized metadata and source payload |

Unique key: `(runtime_connection_id, orchestrator_runtime_agent_id,
delegate_key)`. Delegations are directed many-to-many edges and must not be
stored in `parent_runtime_agent_id`. `delegate_key` is derived from the
adapter's canonical `delegate_ref`, which remains stable when a later snapshot
enriches the edge with `delegate_runtime_agent_id`.

Ephemeral subagent/delegation executions are not stored in this table or as
durable agents. They are stored separately in `subagent_executions` when an
adapter exposes delegated task lifecycle events.

### subagent_executions

| Column | Type | Notes |
|---|---|---|
| runtime_connection_id | uuid fk | Source runtime |
| runtime_execution_id | text | Stable delegated task id |
| parent_run_id | text | Runtime run that created the task |
| runtime_agent_id | text | Owning durable runtime agent, when resolvable |
| subagent_type | text | Runtime-reported execution role/type |
| status | text | Latest lifecycle status |
| description, summary | text | Bounded runtime task context |
| started_at, ended_at | timestamptz null | Lifecycle timestamps |
| observed_at | timestamptz | Last successful observation |
| metadata_json, raw_runtime_json | jsonb | Normalized metadata and audit/debug payload |

Unique key: `(runtime_connection_id, runtime_execution_id)`. These observations
never create rows in `agents` or `agent_runtime_bindings`.

### agent_desired_states

| Column | Type | Notes |
|---|---|---|
| id | uuid pk |  |
| agent_id | uuid fk |  |
| manifest_version | text | `capcom.ai/v1alpha1` |
| desired_status | text | `active`, `disabled` |
| desired_access_json | jsonb | Sources and selected capabilities |
| approvals_json | jsonb | Approval requirements |
| policies_json | jsonb | Drift mode, limits |
| manifest_json | jsonb | Full normalized manifest |
| source | text | `api`, `cli`, `dashboard`, `yaml` |
| applied_by | text | Actor |
| applied_at | timestamptz |  |

Indexes:

- unique `(agent_id)`
- gin `(desired_access_json)`

### access_actual_state

| Column | Type | Notes |
|---|---|---|
| id | uuid pk |  |
| agent_id | uuid fk |  |
| runtime_status | text |  |
| access_json | jsonb | Normalized Gantry `/access` response |
| admin_json | jsonb | Normalized admin detail |
| inventory_refs_json | jsonb | Referenced capabilities/sources |
| raw_runtime_json | jsonb | Raw payload for debugging |
| observed_at | timestamptz |  |
| last_seen_sync_run_id | uuid fk null | Sync provenance |
| freshness_status | text | `live`, `cached`, `stale` |

Indexes:

- unique `(agent_id)`
- gin `(access_json)`

### agent_events

| Column | Type | Notes |
|---|---|---|
| id | uuid pk |  |
| runtime_connection_id | uuid fk |  |
| agent_id | uuid fk null | May be unknown |
| external_event_id | text | Runtime event id |
| event_type | text | Normalized type |
| severity | text | `info`, `warning`, `critical` |
| payload_json | jsonb | Raw/normalized event |
| occurred_at | timestamptz |  |
| ingested_at | timestamptz |  |

Indexes:

- unique `(runtime_connection_id, external_event_id)`
- index `(agent_id, occurred_at desc)`

### drift_records

| Column | Type | Notes |
|---|---|---|
| id | uuid pk |  |
| agent_id | uuid fk |  |
| drift_type | text | `status`, `capability`, `source`, `approval` |
| field_path | text | Human/debug path |
| desired_json | jsonb | Desired value |
| actual_json | jsonb | Actual value |
| severity | text | `info`, `warning`, `critical` |
| mode | text | `observe`, `approval`, `enforce` |
| status | text | `open`, `acknowledged`, `resolved`, `ignored` |
| first_seen_at | timestamptz |  |
| last_seen_at | timestamptz |  |
| resolved_at | timestamptz null |  |

Indexes:

- index `(agent_id, status)`
- unique open drift key: `(agent_id, drift_type, field_path)` where status in open/acknowledged

### control_actions

| Column | Type | Notes |
|---|---|---|
| id | uuid pk |  |
| runtime_connection_id | uuid fk |  |
| agent_id | uuid fk null |  |
| action_type | text | `disable_agent`, `enable_agent`, `replace_access`, `restrict_capability` |
| requested_by | text | Actor |
| reason | text | Required |
| parameters_json | jsonb | Request |
| status | text | `pending`, `running`, `succeeded`, `failed`, `cancelled` |
| runtime_request_json | jsonb null | Sent request |
| runtime_response_json | jsonb null | Runtime result |
| error | text null |  |
| created_at | timestamptz |  |
| finished_at | timestamptz null |  |

### audit_logs

| Column | Type | Notes |
|---|---|---|
| id | uuid pk |  |
| actor | text |  |
| action | text |  |
| target_type | text |  |
| target_id | text |  |
| reason | text null |  |
| before_json | jsonb null |  |
| after_json | jsonb null |  |
| result | text | `succeeded`, `failed` |
| metadata_json | jsonb | Runtime ids, request ids |
| created_at | timestamptz |  |

Audit logs are append-only in application code.

## JSON Shape Guidance

Use JSONB for runtime-specific or frequently changing payloads:

- `agent_actual_states.access_json`
- `agent_actual_states.admin_json`
- `agent_desired_states.desired_access_json`
- `drift_records.desired_json`
- `drift_records.actual_json`

Do not put core query fields only in JSON. Runtime status, risk, mode, and timestamps should be typed columns.

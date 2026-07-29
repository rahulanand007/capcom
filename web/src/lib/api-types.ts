export type JsonObject = Record<string, unknown>

export type HealthResponse = {
  status: string
  service: string
  version: string
}

export type RuntimeMode = "read_only" | "control_enabled"
export type RuntimeType =
  | "gantry"
  | "langgraph"
  | "temporal"
  | "letta"
  | "crewai"
export type RuntimeStatus =
  | "pending"
  | "active"
  | "degraded"
  | "disabled"
  | "failed"

export type RuntimeCapabilities = {
  read_agents: boolean
  read_agent_hierarchy: boolean
  read_agent_delegates?: boolean
  read_agent_skills: boolean
  read_agent_access: boolean
  replace_agent_access: boolean
  read_subagent_executions?: boolean
  read_executions?: boolean
  read_diagnostics?: boolean
  read_inventory?: boolean
  read_capability_catalog?: boolean
  set_agent_status?: boolean
  delete_agent?: boolean
  cancel_execution?: boolean
}

export type RuntimeInstance = {
  id: string
  name: string
  display_name: string
  environment: string
  labels: Record<string, string>
  description?: string
  runtime_type: string
  mode: RuntimeMode
  status: RuntimeStatus
  endpoint: string
  auth_ref: string
  last_synced_at?: string | null
  created_at: string
  updated_at: string
  sync_enabled: boolean
  sync_interval_seconds: number
  last_sync_status?: string
  last_sync_started_at?: string | null
  last_sync_finished_at?: string | null
  last_sync_duration_ms?: number
  last_error?: string
}

export type CreateSecretRequest = {
  name: string
  value: string
  actor: string
  reason: string
}

export type CreateRuntimeInstanceRequest = {
  name: string
  display_name: string
  environment: string
  labels?: Record<string, string>
  runtime_type: RuntimeType
  mode: RuntimeMode
  endpoint: string
  auth_ref: string
  actor: string
  reason: string
  description?: string
}

export type RuntimeConnectionTestResult = {
  status: "active" | "degraded" | "failed"
  message: string
  capabilities: RuntimeCapabilities
  metadata?: JsonObject
}

export type AgentKind = "main" | "registered" | "subagent"
export type AgentFreshness = "live" | "cached" | "stale"

export type PersistedAgent = {
  id: string
  name: string
  status: string
  kind: AgentKind
  metadata?: JsonObject
  runtime_connection_id: string
  runtime_agent_id: string
  parent_runtime_agent_id?: string
  freshness: AgentFreshness
  observed_at: string
  last_successful_sync_at?: string | null
  runtime_status?: string
}

export type RuntimeAgentSkill = {
  runtime_skill_id: string
  name: string
  description?: string
  source?: string
  status: string
  version?: string
  tool_ids: string[]
  workflow_refs: string[]
  metadata?: JsonObject
  observed_at: string
}

export type RuntimeAccessSelection = {
  kind: string
  id: string
  name: string
  allowed: boolean
  attributes?: JsonObject
}

export type RuntimeAgentAccess = {
  agent_id: string
  selections: RuntimeAccessSelection[]
  observed_at: string
  source: string
}

export type AgentDelegation = {
  id: string
  runtime_connection_id: string
  orchestrator_runtime_agent_id: string
  delegate_runtime_agent_id?: string
  delegate_ref: string
  tool_name?: string
  display_name?: string
  persona?: string
  configured: boolean
  resolved: boolean
  revision: number
  status: "active" | "stale" | string
  observed_at: string
  metadata?: JsonObject
}

export type SubagentExecution = {
  id: string
  runtime_connection_id: string
  runtime_execution_id: string
  parent_run_id: string
  runtime_agent_id?: string
  subagent_type?: string
  status: string
  description?: string
  summary?: string
  started_at?: string | null
  ended_at?: string | null
  observed_at: string
  metadata?: JsonObject
}

export type RuntimeExecution = {
  id: string
  runtime_connection_id: string
  runtime_execution_id: string
  parent_runtime_execution_id?: string
  runtime_agent_id?: string
  kind: string
  status: string
  started_at?: string | null
  ended_at?: string | null
  observed_at: string
  metadata?: JsonObject
}

export type RuntimeSyncRun = {
  id: string
  runtime_connection_id: string
  trigger: "manual" | "scheduled" | "post_action"
  status: "running" | "succeeded" | "failed" | "skipped"
  started_at: string
  finished_at?: string | null
  duration_ms?: number
  agents_seen?: number
  skills_seen?: number
  bindings_seen?: number
  access_documents_seen?: number
  executions_seen?: number
  diagnostics_seen?: number
  inventory_seen?: number
  capabilities_seen?: number
  delegations_seen?: number
  error_code?: string
  error_message?: string
}

export type RuntimeDiagnostic = {
  id: string
  runtime_connection_id: string
  check_id: string
  status: string
  message: string
  observed_at: string
  metadata?: JsonObject
}

export type RuntimeInventoryItem = {
  id: string
  runtime_connection_id: string
  runtime_item_id: string
  kind: "tool" | "skill" | "mcp_server" | string
  name: string
  status: string
  provider?: string
  source?: string
  observed_at: string
  metadata?: JsonObject
}

export type RuntimeCapability = {
  id: string
  runtime_connection_id: string
  runtime_capability_id: string
  version: string
  name: string
  category: string
  risk: string
  can?: string
  cannot?: string
  source?: string
  observed_at: string
  metadata?: JsonObject
}

export type SetAgentStatusRequest = {
  status: "enabled" | "disabled"
  actor: string
  reason: string
  idempotency_key: string
  dry_run?: boolean
}

export type DeleteAgentRequest = {
  confirmation: string
  actor: string
  reason: string
  idempotency_key: string
  dry_run?: boolean
}

export type CancelExecutionRequest = {
  actor: string
  reason: string
  idempotency_key: string
  dry_run?: boolean
}

export type SyncRuntimeRequest = {
  actor: string
  reason: string
}

export type ReconcileAccessRequest = {
  selections: RuntimeAccessSelection[]
  actor: string
  reason: string
  idempotency_key: string
  dry_run?: boolean
}

export type ControlAction = {
  id: string
  runtime_connection_id: string
  agent_id: string
  action_type: string
  status: "queued" | "running" | "succeeded" | "failed" | "rejected"
  actor: string
  reason: string
  idempotency_key: string
  result?: JsonObject
  created_at?: string
  updated_at?: string
}

export type ErrorResponse = {
  error: string
}

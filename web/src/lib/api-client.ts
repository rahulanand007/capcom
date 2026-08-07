import type {
  AgentDelegation,
  CreateRuntimeInstanceRequest,
  ConsolidateRuntimeEndpointRequest,
  CreateSecretRequest,
  CancelExecutionRequest,
  ControlAction,
  DeleteAgentRequest,
  HealthResponse,
  MetricSummary,
  TelemetryHealth,
  PersistedAgent,
  ReconcileAccessRequest,
  RemoveRuntimeInstanceRequest,
  RuntimeAgentAccess,
  RuntimeAgentSkill,
  RuntimeConnectionTestResult,
  RuntimeCapability,
  RuntimeDiagnostic,
  RuntimeExecution,
  RuntimeInventoryItem,
  RuntimeInstance,
  RuntimeSyncRun,
  SubagentExecution,
  SetAgentStatusRequest,
  SyncRuntimeRequest,
  UpdateRuntimeInstanceSettingsRequest,
} from "@/lib/api-types"

const API_BASE_URL = "/api/capcom"

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly data: unknown
  ) {
    super(message)
    this.name = "ApiError"
  }
}

export function getApiBaseUrl() {
  return API_BASE_URL
}

type RequestOptions = Omit<RequestInit, "body"> & {
  body?: unknown
}

async function request<T>(path: string, options: RequestOptions = {}) {
  const url = `${getApiBaseUrl()}${path}`
  const headers = new Headers(options.headers)

  if (options.body !== undefined) {
    headers.set("Content-Type", "application/json")
  }

  const response = await fetch(url, {
    ...options,
    headers,
    body:
      options.body === undefined ? undefined : JSON.stringify(options.body),
  })

  const contentType = response.headers.get("content-type")
  const data = contentType?.includes("application/json")
    ? await response.json()
    : await response.text()

  if (!response.ok) {
    throw new ApiError(errorMessage(data, response.statusText), response.status, data)
  }

  return data as T
}

function errorMessage(data: unknown, fallback: string) {
  if (
    data &&
    typeof data === "object" &&
    "error" in data &&
    typeof data.error === "string"
  ) {
    return data.error
  }
  if (
    data &&
    typeof data === "object" &&
    "error" in data &&
    data.error &&
    typeof data.error === "object" &&
    "message" in data.error &&
    typeof data.error.message === "string"
  ) {
    return data.error.message
  }
  return fallback || "Request failed"
}

function searchParams(params: Record<string, string | undefined>) {
  const out = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value) {
      out.set(key, value)
    }
  }
  const query = out.toString()
  return query ? `?${query}` : ""
}

const METRICS_RANGES = {
  "24h": { durationMs: 24 * 60 * 60 * 1000, interval: "1h" },
  "7d": { durationMs: 7 * 24 * 60 * 60 * 1000, interval: "6h" },
  "30d": { durationMs: 30 * 24 * 60 * 60 * 1000, interval: "1d" },
} as const

export const capcomApi = {
  health: () => request<HealthResponse>("/healthz"),
  createSecret: (body: CreateSecretRequest) =>
    request("/v1/secrets", {
      method: "POST",
      body,
    }),
  createRuntimeInstance: (body: CreateRuntimeInstanceRequest) =>
    request<RuntimeInstance>("/v1/runtime-instances", {
      method: "POST",
      body,
    }),
  listRuntimeInstances: () => request<RuntimeInstance[]>("/v1/runtime-instances"),
  getRuntimeInstance: (id: string) =>
    request<RuntimeInstance>(`/v1/runtime-instances/${id}`),
  updateRuntimeInstanceSettings: (
    id: string,
    body: UpdateRuntimeInstanceSettingsRequest
  ) =>
    request<RuntimeInstance>(`/v1/runtime-instances/${id}`, {
      method: "PATCH",
      body,
    }),
  removeRuntimeInstance: (id: string, body: RemoveRuntimeInstanceRequest) =>
    request<void>(`/v1/runtime-instances/${id}`, {
      method: "DELETE",
      body,
    }),
  consolidateRuntimeEndpoint: (
    canonicalId: string,
    body: ConsolidateRuntimeEndpointRequest
  ) =>
    request<RuntimeInstance>(
      `/v1/runtime-instances/${canonicalId}/endpoints/consolidate`,
      { method: "POST", body }
    ),
  testRuntimeInstance: (id: string) =>
    request<RuntimeConnectionTestResult>(`/v1/runtime-instances/${id}/test`, {
      method: "POST",
    }),
  syncRuntimeInstance: (id: string, body: SyncRuntimeRequest) =>
    request<RuntimeSyncRun>(`/v1/runtime-instances/${id}/sync`, {
      method: "POST",
      body,
    }),
  listRuntimeInstanceSyncRuns: (id: string) =>
    request<RuntimeSyncRun[]>(`/v1/runtime-instances/${id}/sync-runs`),
  listRuntimeInstanceAgents: (id: string) =>
    request<PersistedAgent[]>(`/v1/runtime-instances/${id}/agents`),
  listRuntimeInstanceExecutions: (id: string) =>
    request<RuntimeExecution[]>(`/v1/runtime-instances/${id}/executions`),
  listRuntimeInstanceDiagnostics: (id: string) =>
    request<RuntimeDiagnostic[]>(`/v1/runtime-instances/${id}/diagnostics`),
  listRuntimeInstanceInventory: (id: string) =>
    request<RuntimeInventoryItem[]>(`/v1/runtime-instances/${id}/inventory`),
  listRuntimeInstanceCapabilities: (id: string) =>
    request<RuntimeCapability[]>(`/v1/runtime-instances/${id}/capabilities`),
  listRuntimeInstanceAgentDelegations: (id: string) =>
    request<AgentDelegation[]>(`/v1/runtime-instances/${id}/agent-delegations`),
  listRuntimeInstanceSubagentExecutions: (id: string, agentId?: string) =>
    request<SubagentExecution[]>(
      `/v1/runtime-instances/${id}/subagent-executions${searchParams({
        agent_id: agentId,
      })}`
    ),
  listPersistedAgents: (runtimeConnectionId?: string) =>
    request<PersistedAgent[]>(
      `/v1/agents${searchParams({
        runtime_connection_id: runtimeConnectionId,
      })}`
    ),
  getPersistedAgent: (id: string) => request<PersistedAgent>(`/v1/agents/${id}`),
  getAgentMetrics: (id: string, range: keyof typeof METRICS_RANGES = "24h") =>
    request<MetricSummary>(metricsURL(`/v1/agents/${id}/metrics`, range)),
  getRuntimeMetrics: (id: string, range: keyof typeof METRICS_RANGES = "24h") =>
    request<MetricSummary>(metricsURL(`/v1/runtime-instances/${id}/metrics`, range)),
  getMetricsSummary: (range: keyof typeof METRICS_RANGES = "24h") =>
    request<MetricSummary>(metricsURL("/v1/metrics/summary", range)),
  getRuntimeTelemetryHealth: (id: string) =>
    request<TelemetryHealth>(`/v1/runtime-instances/${id}/telemetry-health`),
  getFleetTelemetryHealth: async () => {
    const instances = await request<RuntimeInstance[]>("/v1/runtime-instances")
    return Promise.all(
      instances.map(async (instance) => ({
        ...(await request<TelemetryHealth>(
          `/v1/runtime-instances/${instance.id}/telemetry-health`
        )),
        runtime_connection_id: instance.id,
        runtime_display_name: instance.display_name || instance.name,
      }))
    )
  },
  listAgentSkills: (id: string) =>
    request<RuntimeAgentSkill[]>(`/v1/agents/${id}/skills`),
  getAgentAccess: (id: string) =>
    request<RuntimeAgentAccess>(`/v1/agents/${id}/access`),
  listAgentDelegations: (id: string) =>
    request<AgentDelegation[]>(`/v1/agents/${id}/delegations`),
  listSubagentExecutions: (params: {
    runtimeConnectionId?: string
    agentId?: string
  }) =>
    request<SubagentExecution[]>(
      `/v1/subagent-executions${searchParams({
        runtime_connection_id: params.runtimeConnectionId,
        agent_id: params.agentId,
      })}`
    ),
  reconcileAgentAccess: (id: string, body: ReconcileAccessRequest) =>
    request<ControlAction>(`/v1/agents/${id}/actions/reconcile-access`, {
      method: "POST",
      body,
    }),
  setAgentStatus: (id: string, body: SetAgentStatusRequest) =>
    request<ControlAction>(`/v1/agents/${id}/actions/set-status`, {
      method: "POST",
      body,
    }),
  deleteAgent: (id: string, body: DeleteAgentRequest) =>
    request<ControlAction>(`/v1/agents/${id}/actions/delete`, {
      method: "POST",
      body,
    }),
  cancelExecution: (id: string, body: CancelExecutionRequest) =>
    request<ControlAction>(`/v1/runtime-executions/${id}/actions/cancel`, {
      method: "POST",
      body,
    }),
}

function metricsURL(path: string, range: keyof typeof METRICS_RANGES) {
  const to = new Date()
  const config = METRICS_RANGES[range]
  const from = new Date(to.getTime() - config.durationMs)
  return `${path}${searchParams({
    from: from.toISOString(),
    to: to.toISOString(),
    interval: config.interval,
  })}`
}

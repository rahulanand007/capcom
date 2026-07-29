import type {
  AgentDelegation,
  CreateRuntimeInstanceRequest,
  CreateSecretRequest,
  CancelExecutionRequest,
  ControlAction,
  DeleteAgentRequest,
  HealthResponse,
  PersistedAgent,
  ReconcileAccessRequest,
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

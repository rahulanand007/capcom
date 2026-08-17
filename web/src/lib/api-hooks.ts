"use client"

import {
  useMutation,
  useQuery,
  useQueryClient,
  useQueries,
  type UseQueryOptions,
} from "@tanstack/react-query"

import { capcomApi } from "@/lib/api-client"
import type {
  AgentDelegation,
  CancelExecutionRequest,
  CreateRuntimeInstanceRequest,
  ConsolidateRuntimeEndpointRequest,
  CreateSecretRequest,
  HealthResponse,
  MetricSummary,
  TelemetryHealth,
  PersistedAgent,
  RuntimeAgentAccess,
  RuntimeAgentSkill,
  RuntimeConnectionTestResult,
  RuntimeCapability,
  RuntimeDiagnostic,
  RuntimeExecution,
  RuntimeInventoryItem,
  RuntimeInstance,
  RemoveRuntimeInstanceRequest,
  RuntimeSyncRun,
  ReconcileAccessRequest,
  SetAgentStatusRequest,
  SubagentExecution,
  SyncRuntimeRequest,
  ControlAction,
  DeleteAgentRequest,
  UpdateRuntimeInstanceSettingsRequest,
} from "@/lib/api-types"

export const queryKeys = {
  health: ["health"] as const,
  runtimeInstances: ["runtime-instances"] as const,
  runtimeInstance: (id: string) => ["runtime-instances", id] as const,
  runtimeInstanceTest: (id: string) =>
    ["runtime-instances", id, "test"] as const,
  runtimeInstanceSyncRuns: (id: string) =>
    ["runtime-instances", id, "sync-runs"] as const,
  runtimeInstanceAgents: (id: string) =>
    ["runtime-instances", id, "agents"] as const,
  runtimeInstanceExecutions: (id: string) =>
    ["runtime-instances", id, "executions"] as const,
  runtimeInstanceDiagnostics: (id: string) =>
    ["runtime-instances", id, "diagnostics"] as const,
  runtimeInstanceInventory: (id: string) =>
    ["runtime-instances", id, "inventory"] as const,
  runtimeInstanceCapabilities: (id: string) =>
    ["runtime-instances", id, "capabilities"] as const,
  runtimeInstanceAgentDelegations: (id: string) =>
    ["runtime-instances", id, "agent-delegations"] as const,
  runtimeInstanceSubagentExecutions: (id: string, agentId?: string) =>
    ["runtime-instances", id, "subagent-executions", agentId ?? "all"] as const,
  persistedAgents: (runtimeConnectionId?: string) =>
    ["agents", runtimeConnectionId ?? "fleet"] as const,
  persistedAgent: (id: string) => ["agents", id] as const,
  agentSkills: (id: string) => ["agents", id, "skills"] as const,
  agentAccess: (id: string) => ["agents", id, "access"] as const,
  agentDelegations: (id: string) => ["agents", id, "delegations"] as const,
  agentMetrics: (id: string) => ["agents", id, "metrics"] as const,
  metricsSummary: (range: string) => ["metrics", "summary", range] as const,
  scopedMetrics: (range: string, runtimeID?: string, agentID?: string) =>
    ["metrics", "scope", range, runtimeID ?? "fleet", agentID ?? "all"] as const,
  telemetryHealth: ["metrics", "telemetry-health"] as const,
  subagentExecutions: (runtimeConnectionId?: string, agentId?: string) =>
    [
      "subagent-executions",
      runtimeConnectionId ?? "all",
      agentId ?? "all",
    ] as const,
}

export function useHealthQuery(
  options?: Omit<UseQueryOptions<HealthResponse>, "queryKey" | "queryFn">
) {
  return useQuery({
    queryKey: queryKeys.health,
    queryFn: capcomApi.health,
    refetchInterval: 30_000,
    ...options,
  })
}

export function useRuntimeInstancesQuery(enabled = true) {
  return useQuery<RuntimeInstance[]>({
    queryKey: queryKeys.runtimeInstances,
    queryFn: capcomApi.listRuntimeInstances,
    enabled,
  })
}

export function useRuntimeInstanceQuery(id: string | undefined) {
  return useQuery<RuntimeInstance>({
    queryKey: queryKeys.runtimeInstance(id ?? ""),
    queryFn: () => capcomApi.getRuntimeInstance(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useTestRuntimeInstanceMutation(id: string) {
  return useMutation<RuntimeConnectionTestResult>({
    mutationKey: queryKeys.runtimeInstanceTest(id),
    mutationFn: () => capcomApi.testRuntimeInstance(id),
  })
}

export function usePersistedAgentsQuery(runtimeConnectionId?: string) {
  return useQuery<PersistedAgent[]>({
    queryKey: queryKeys.persistedAgents(runtimeConnectionId),
    queryFn: () => capcomApi.listPersistedAgents(runtimeConnectionId),
  })
}

export function useRuntimeInstanceAgentsQuery(id: string | undefined) {
  return useQuery<PersistedAgent[]>({
    queryKey: queryKeys.runtimeInstanceAgents(id ?? ""),
    queryFn: () => capcomApi.listRuntimeInstanceAgents(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useRuntimeInstanceAgentDelegationsQuery(id: string | undefined) {
  return useQuery<AgentDelegation[]>({
    queryKey: queryKeys.runtimeInstanceAgentDelegations(id ?? ""),
    queryFn: () => capcomApi.listRuntimeInstanceAgentDelegations(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useRuntimeInstanceExecutionsQuery(id: string | undefined) {
  return useQuery<RuntimeExecution[]>({
    queryKey: queryKeys.runtimeInstanceExecutions(id ?? ""),
    queryFn: () => capcomApi.listRuntimeInstanceExecutions(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useRuntimeInstanceDiagnosticsQuery(id: string | undefined) {
  return useQuery<RuntimeDiagnostic[]>({
    queryKey: queryKeys.runtimeInstanceDiagnostics(id ?? ""),
    queryFn: () => capcomApi.listRuntimeInstanceDiagnostics(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useRuntimeInstanceInventoryQuery(id: string | undefined) {
  return useQuery<RuntimeInventoryItem[]>({
    queryKey: queryKeys.runtimeInstanceInventory(id ?? ""),
    queryFn: () => capcomApi.listRuntimeInstanceInventory(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useRuntimeInstanceCapabilitiesQuery(id: string | undefined) {
  return useQuery<RuntimeCapability[]>({
    queryKey: queryKeys.runtimeInstanceCapabilities(id ?? ""),
    queryFn: () => capcomApi.listRuntimeInstanceCapabilities(id ?? ""),
    enabled: Boolean(id),
  })
}

export function usePersistedAgentQuery(id: string | undefined) {
  return useQuery<PersistedAgent>({
    queryKey: queryKeys.persistedAgent(id ?? ""),
    queryFn: () => capcomApi.getPersistedAgent(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useAgentSkillsQuery(id: string | undefined) {
  return useQuery<RuntimeAgentSkill[]>({
    queryKey: queryKeys.agentSkills(id ?? ""),
    queryFn: () => capcomApi.listAgentSkills(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useAgentAccessQuery(id: string | undefined) {
  return useQuery<RuntimeAgentAccess>({
    queryKey: queryKeys.agentAccess(id ?? ""),
    queryFn: () => capcomApi.getAgentAccess(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useAgentDelegationsQuery(id: string | undefined) {
  return useQuery<AgentDelegation[]>({
    queryKey: queryKeys.agentDelegations(id ?? ""),
    queryFn: () => capcomApi.listAgentDelegations(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useReconcileAgentAccessMutation(id: string) {
  const queryClient = useQueryClient()
  return useMutation<ControlAction, Error, ReconcileAccessRequest>({
    mutationFn: (body) => capcomApi.reconcileAgentAccess(id, body),
    onSuccess: async (action) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.agentAccess(id) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgent(id) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgents() }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceAgents(
            action.runtime_connection_id
          ),
        }),
      ])
    },
  })
}

export function useSetAgentStatusMutation(id: string) {
  const queryClient = useQueryClient()
  return useMutation<ControlAction, Error, SetAgentStatusRequest>({
    mutationFn: (body) => capcomApi.setAgentStatus(id, body),
    onSuccess: async (action) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgent(id) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgents() }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceAgents(action.runtime_connection_id),
        }),
      ])
    },
  })
}

export function useFleetAgentDelegationsQuery(instanceIDs: string[]) {
  const queries = useQueries({
    queries: instanceIDs.map((id) => ({
      queryKey: queryKeys.runtimeInstanceAgentDelegations(id),
      queryFn: () => capcomApi.listRuntimeInstanceAgentDelegations(id),
    })),
  })

  return {
    data: queries.flatMap((query) => query.data ?? []),
    isLoading: queries.some((query) => query.isLoading),
    isError: queries.some((query) => query.isError),
  }
}

export function useAgentMetricsQuery(id: string | undefined) {
  return useQuery<MetricSummary>({
    queryKey: queryKeys.agentMetrics(id ?? ""),
    queryFn: () => capcomApi.getAgentMetrics(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useMetricsSummaryQuery(range: "24h" | "7d" | "30d") {
  return useQuery<MetricSummary>({
    queryKey: queryKeys.metricsSummary(range),
    queryFn: () => capcomApi.getMetricsSummary(range),
    refetchInterval: 30_000,
    refetchIntervalInBackground: true,
    staleTime: 15_000,
  })
}

export function useScopedMetricsQuery(
  range: "24h" | "7d" | "30d",
  runtimeID?: string,
  agentID?: string,
  live = true
) {
  return useQuery<MetricSummary>({
    queryKey: queryKeys.scopedMetrics(range, runtimeID, agentID),
    queryFn: () =>
      agentID
        ? capcomApi.getAgentMetrics(agentID, range)
        : runtimeID
          ? capcomApi.getRuntimeMetrics(runtimeID, range)
          : capcomApi.getMetricsSummary(range),
    refetchInterval: live ? 30_000 : false,
    refetchIntervalInBackground: live,
    staleTime: 15_000,
  })
}

export function useFleetTelemetryHealthQuery(live = true) {
  return useQuery<TelemetryHealth[]>({
    queryKey: queryKeys.telemetryHealth,
    queryFn: capcomApi.getFleetTelemetryHealth,
    refetchInterval: live ? 30_000 : false,
    refetchIntervalInBackground: live,
    staleTime: 15_000,
  })
}

export function useUpdateRuntimeInstanceSettingsMutation(id: string) {
  const queryClient = useQueryClient()
  return useMutation<
    RuntimeInstance,
    Error,
    UpdateRuntimeInstanceSettingsRequest
  >({
    mutationFn: (body) => capcomApi.updateRuntimeInstanceSettings(id, body),
    onSuccess: async (instance) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstances }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstance(instance.id),
        }),
      ])
    },
  })
}

export function useRemoveRuntimeInstanceMutation(id: string) {
  const queryClient = useQueryClient()
  return useMutation<void, Error, RemoveRuntimeInstanceRequest>({
    mutationFn: (body) => capcomApi.removeRuntimeInstance(id, body),
    onSuccess: async () => {
      queryClient.removeQueries({ queryKey: queryKeys.runtimeInstance(id) })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstances }),
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgents() }),
      ])
    },
  })
}

export function useConsolidateRuntimeEndpointMutation(canonicalId: string) {
  const queryClient = useQueryClient()
  return useMutation<
    RuntimeInstance,
    Error,
    ConsolidateRuntimeEndpointRequest
  >({
    mutationFn: (body) =>
      capcomApi.consolidateRuntimeEndpoint(canonicalId, body),
    onSuccess: async (instance) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstances }),
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgents() }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstance(instance.id),
        }),
      ])
    },
  })
}

export function useDeleteAgentMutation(id: string) {
  const queryClient = useQueryClient()
  return useMutation<ControlAction, Error, DeleteAgentRequest>({
    mutationFn: (body) => capcomApi.deleteAgent(id, body),
    onSuccess: async (action) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgent(id) }),
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgents() }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceAgents(action.runtime_connection_id),
        }),
      ])
    },
  })
}

export function useCancelExecutionMutation(
  executionId: string,
  runtimeConnectionId: string
) {
  const queryClient = useQueryClient()
  return useMutation<ControlAction, Error, CancelExecutionRequest>({
    mutationFn: (body) => capcomApi.cancelExecution(executionId, body),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.runtimeInstanceExecutions(runtimeConnectionId),
      })
    },
  })
}

export function useSubagentExecutionsQuery(params: {
  runtimeConnectionId?: string
  agentId?: string
}) {
  return useQuery<SubagentExecution[]>({
    queryKey: queryKeys.subagentExecutions(
      params.runtimeConnectionId,
      params.agentId
    ),
    queryFn: () => capcomApi.listSubagentExecutions(params),
  })
}

export function useRuntimeInstanceSubagentExecutionsQuery(
  id: string | undefined,
  agentId?: string
) {
  return useQuery<SubagentExecution[]>({
    queryKey: queryKeys.runtimeInstanceSubagentExecutions(id ?? "", agentId),
    queryFn: () => capcomApi.listRuntimeInstanceSubagentExecutions(id ?? "", agentId),
    enabled: Boolean(id),
  })
}

export function useRuntimeInstanceSyncRunsQuery(id: string | undefined) {
  return useQuery<RuntimeSyncRun[]>({
    queryKey: queryKeys.runtimeInstanceSyncRuns(id ?? ""),
    queryFn: () => capcomApi.listRuntimeInstanceSyncRuns(id ?? ""),
    enabled: Boolean(id),
  })
}

export function useSyncRuntimeInstanceMutation(id: string) {
  const queryClient = useQueryClient()
  return useMutation<RuntimeSyncRun, Error, SyncRuntimeRequest>({
    mutationFn: (body) => capcomApi.syncRuntimeInstance(id, body),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstances }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstance(id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceAgents(id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceExecutions(id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceDiagnostics(id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceInventory(id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceCapabilities(id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceAgentDelegations(id),
        }),
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgents() }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceSyncRuns(id),
        }),
      ])
    },
  })
}

export function useSyncRuntimeInstancesMutation(ids: string[]) {
  const queryClient = useQueryClient()

  return useMutation<RuntimeSyncRun[], Error, SyncRuntimeRequest>({
    mutationFn: (body) =>
      Promise.all(ids.map((id) => capcomApi.syncRuntimeInstance(id, body))),
    onSuccess: async (runs) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstances }),
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgents() }),
        queryClient.invalidateQueries({ queryKey: queryKeys.subagentExecutions() }),
        ...runs.flatMap((run) => {
          const id = run.runtime_connection_id
          return [
            queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstance(id) }),
            queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstanceAgents(id) }),
            queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstanceExecutions(id) }),
            queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstanceDiagnostics(id) }),
            queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstanceInventory(id) }),
            queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstanceCapabilities(id) }),
            queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstanceAgentDelegations(id) }),
            queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstanceSyncRuns(id) }),
          ]
        }),
      ])
    },
  })
}

export type AddRuntimeInstanceInput = {
  secret: CreateSecretRequest
  runtimeInstance: CreateRuntimeInstanceRequest
}

export function useAddRuntimeInstanceMutation() {
  const queryClient = useQueryClient()

  return useMutation<RuntimeInstance, Error, AddRuntimeInstanceInput>({
    mutationFn: async ({ secret, runtimeInstance }) => {
      await capcomApi.createSecret(secret)
      return capcomApi.createRuntimeInstance(runtimeInstance)
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstances }),
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgents() }),
      ])
      await Promise.all([
        queryClient.refetchQueries({ queryKey: queryKeys.runtimeInstances }),
        queryClient.refetchQueries({ queryKey: queryKeys.persistedAgents() }),
      ])
    },
  })
}

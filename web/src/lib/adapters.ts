import type { PersistedAgent, RuntimeInstance } from "@/lib/api-types"

export type DerivedStatus = "ok" | "stale" | "failed"

export type AdapterInstance = {
  instance: RuntimeInstance
  status: DerivedStatus
  badge: string
  message: string
  updated: string
  agentCount: number
  needsAttention: boolean
}

export type AdapterModel = {
  id: string
  name: string
  status: DerivedStatus
  badge: string
  instances: AdapterInstance[]
  instanceCount: number
  agentCount: number
  footer: string
}

export type AttentionItem = {
  adapterId: string
  adapterName: string
  instanceId: string
  instanceName: string
  instanceStableKey: string
  status: DerivedStatus
  badge: "stale" | "failed"
  message: string
  action: string
}

const FRESHNESS_BUDGET_MS = 5 * 60 * 1000

export function buildAdaptersModel(
  instances: RuntimeInstance[],
  agents: PersistedAgent[],
  now = new Date()
) {
  const agentsByRuntime = new Map<string, number>()
  for (const agent of agents) {
    if (agent.status === "disabled" || agent.metadata?.runtime_deleted) {
      continue
    }
    agentsByRuntime.set(
      agent.runtime_connection_id,
      (agentsByRuntime.get(agent.runtime_connection_id) ?? 0) + 1
    )
  }

  const groups = new Map<string, RuntimeInstance[]>()
  for (const instance of instances) {
    const bucket = groups.get(instance.runtime_type) ?? []
    bucket.push(instance)
    groups.set(instance.runtime_type, bucket)
  }

  const adapters = Array.from(groups.entries())
    .map(([runtimeType, runtimeInstances]) => {
      const derivedInstances = runtimeInstances
        .slice()
        .sort((a, b) => a.display_name.localeCompare(b.display_name))
        .map((instance) =>
          deriveInstance(instance, agentsByRuntime.get(instance.id) ?? 0, now)
        )

      const failed = derivedInstances.filter((item) => item.status === "failed")
      const stale = derivedInstances.filter((item) => item.status === "stale")
      const status: DerivedStatus = failed.length
        ? "failed"
        : stale.length
          ? "stale"
          : "ok"
      const firstIssue = failed[0] ?? stale[0]
      const agentCount = derivedInstances.reduce(
        (sum, item) => sum + item.agentCount,
        0
      )

      return {
        id: runtimeType,
        name: displayRuntimeType(runtimeType),
        status,
        badge:
          status === "failed"
            ? "import failed"
            : status === "stale"
              ? `${stale.length} stale`
              : "healthy",
        instances: derivedInstances,
        instanceCount: derivedInstances.length,
        agentCount,
        footer: firstIssue
          ? firstIssue.message
          : `All state fresh - updated ${derivedInstances[0]?.updated ?? "never"}`,
      } satisfies AdapterModel
    })
    .sort((a, b) => a.name.localeCompare(b.name))

  const attention = adapters.flatMap((adapter) =>
    adapter.instances
      .filter((item) => item.needsAttention)
      .map((item) => ({
        adapterId: adapter.id,
        adapterName: adapter.name,
        instanceId: item.instance.id,
        instanceName: item.instance.display_name || item.instance.name,
        instanceStableKey: item.instance.name,
        status: item.status,
        badge: item.status === "failed" ? ("failed" as const) : ("stale" as const),
        message: item.message,
        action: item.status === "failed" ? "Retry import" : "Re-import",
      }))
  )

  return { adapters, attention }
}

export function deriveWorstStatus(
  statuses: Array<DerivedStatus | "unknown">
): DerivedStatus | "unknown" {
  if (statuses.includes("failed")) {
    return "failed"
  }
  if (statuses.includes("stale")) {
    return "stale"
  }
  if (statuses.includes("ok")) {
    return "ok"
  }
  return "unknown"
}

export function displayRuntimeType(runtimeType: string) {
  const known: Record<string, string> = {
    gantry: "Gantry",
    langgraph: "LangGraph",
    temporal: "Temporal",
    letta: "Letta",
    crewai: "CrewAI",
  }
  return (
    known[runtimeType] ??
    runtimeType
      .split(/[-_]/)
      .filter(Boolean)
      .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
      .join(" ")
  )
}

export function statusLabel(status: DerivedStatus | "unknown") {
  if (status === "failed") {
    return "needs attention"
  }
  if (status === "stale") {
    return "stale imports"
  }
  if (status === "ok") {
    return "all systems normal"
  }
  return "checking API"
}

export function statusClass(status: DerivedStatus | "unknown") {
  if (status === "failed") {
    return {
      dot: "bg-[var(--dg)] shadow-[0_0_0_3px_var(--dgd)]",
      text: "text-[var(--dg)]",
      badge: "bg-[var(--dgd)] text-[var(--dg)]",
    }
  }
  if (status === "stale") {
    return {
      dot: "bg-[var(--wn)] shadow-[0_0_0_3px_var(--wnd)]",
      text: "text-[var(--wn)]",
      badge: "bg-[var(--wnd)] text-[var(--wn)]",
    }
  }
  if (status === "ok") {
    return {
      dot: "bg-[var(--ac)] shadow-[0_0_0_3px_var(--acd)]",
      text: "text-[var(--ac)]",
      badge: "bg-[var(--acd)] text-[var(--ac)]",
    }
  }
  return {
    dot: "bg-[var(--fa)] shadow-[0_0_0_3px_var(--sl)]",
    text: "text-[var(--fa)]",
    badge: "bg-[var(--sl)] text-[var(--fa)]",
  }
}

export function relativeTime(value?: string | null, now = new Date()) {
  if (!value) {
    return "never"
  }
  const date = new Date(value)
  const diffMs = Math.max(0, now.getTime() - date.getTime())
  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 5) {
    return "just now"
  }
  if (seconds < 60) {
    return `${seconds}s ago`
  }
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return `${minutes}m ago`
  }
  const hours = Math.floor(minutes / 60)
  if (hours < 24) {
    return `${hours}h ago`
  }
  return `${Math.floor(hours / 24)}d ago`
}

function deriveInstance(
  instance: RuntimeInstance,
  agentCount: number,
  now: Date
): AdapterInstance {
  const freshnessBudgetMs =
    instance.sync_interval_seconds > 0
      ? Math.max(FRESHNESS_BUDGET_MS, instance.sync_interval_seconds * 2 * 1000)
      : FRESHNESS_BUDGET_MS
  const lastSynced = instance.last_synced_at
    ? new Date(instance.last_synced_at)
    : null
  const ageMs = lastSynced ? now.getTime() - lastSynced.getTime() : Infinity
  const failed =
    instance.status === "failed" ||
    instance.last_sync_status === "failed" ||
    Boolean(instance.last_error)
  const stale =
    !failed &&
    (instance.status === "degraded" ||
      instance.status === "disabled" ||
      instance.status === "pending" ||
      ageMs > freshnessBudgetMs)
  const status: DerivedStatus = failed ? "failed" : stale ? "stale" : "ok"
  const updated = relativeTime(instance.last_synced_at, now)
  const name = instance.display_name || instance.name
  const message =
    status === "failed"
      ? `${name} import failed - ${instance.last_error || "check the endpoint."}`
      : status === "stale"
        ? `${name} state is ${updated.replace(" ago", "")} old - older than the 5m freshness budget.`
        : `${name} updated ${updated}`

  return {
    instance,
    status,
    badge: status === "failed" ? "failed" : status === "stale" ? "stale" : "ok",
    message,
    updated,
    agentCount,
    needsAttention: status !== "ok",
  }
}

"use client"

import * as React from "react"
import { ChevronDown, ChevronRight } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import {
  TableCell,
  TableRow,
} from "@/components/ui/table"
import type { PersistedAgent, RuntimeInstance } from "@/lib/api-types"
import type { AgentTopologyRow } from "@/lib/agent-topology"
import {
  useAgentAccessQuery,
  useAgentSkillsQuery,
} from "@/lib/api-hooks"
import { displayRuntimeType } from "@/lib/adapters"
import { cn } from "@/lib/utils"

export type AgentLocation = {
  adapterName: string
  instanceName: string
}

type AgentTableRowProps = {
  agent: PersistedAgent
  location?: AgentLocation
  topology?: AgentTopologyRow
  onAgentClick?: (agent: PersistedAgent) => void
  hasChildren?: boolean
  expanded?: boolean
  onToggle?: () => void
}

export function AgentTableRow({
  agent,
  location,
  topology,
  onAgentClick,
  hasChildren = false,
  expanded = true,
  onToggle,
}: AgentTableRowProps) {
  return (
    <TableRow
      data-agent-id={agent.id}
      className="cursor-pointer border-[var(--sl)] hover:bg-[var(--sl)]"
      onClick={() => onAgentClick?.(agent)}
    >
      <TableCell className={cn("min-w-[220px] px-[18px] whitespace-normal", topology?.depth ? "py-2.5" : "py-3")}>
        <AgentIdentity
          agent={agent}
          topology={topology}
          hasChildren={hasChildren}
          expanded={expanded}
          onToggle={onToggle}
        />
      </TableCell>
      {location ? (
        <TableCell className="min-w-[220px] px-[18px] py-3 whitespace-normal">
          <div className="font-hud text-[12px] text-[var(--mu)]">
            {location.adapterName} · {location.instanceName}
          </div>
        </TableCell>
      ) : null}
      <TableCell className="px-[18px] py-3">
        <AgentSkillCount agentId={agent.id} />
      </TableCell>
      <TableCell className="min-w-[240px] px-[18px] py-3 whitespace-normal">
        <AgentAccessChips agentId={agent.id} />
      </TableCell>
      <TableCell className="px-[18px] py-3">
        <AgentStatusPill agent={agent} />
      </TableCell>
    </TableRow>
  )
}

export function AgentIdentity({
  agent,
  topology,
  hasChildren = false,
  expanded = true,
  onToggle,
}: {
  agent: PersistedAgent
  topology?: AgentTopologyRow
  hasChildren?: boolean
  expanded?: boolean
  onToggle?: () => void
}) {
  const depth = Math.min(topology?.depth ?? 0, 4)
  const visibleOrchestrators = topology?.delegatedBy.slice(0, 2) ?? []

  return (
    <div
      className={cn(
        "flex min-w-0 items-stretch",
        topology?.contextOnly && "opacity-65"
      )}
      data-topology-role={topology?.role}
      data-topology-depth={topology?.depth}
    >
      {depth > 0 ? (
        <div aria-hidden="true" className="relative mr-2 min-h-9 shrink-0" style={{ width: depth * 18 }}>
          {Array.from({ length: depth }).map((_, index) => (
            <span
              key={index}
              className="absolute bottom-0 top-0 border-l border-[color-mix(in_srgb,var(--ac)_22%,var(--hl))]"
              style={{ left: index * 18 + 7 }}
            />
          ))}
          <span
            className="absolute top-[10px] h-3 w-3 border-b border-l border-[color-mix(in_srgb,var(--ac)_35%,var(--hl))]"
            style={{ left: (depth - 1) * 18 + 7 }}
          />
        </div>
      ) : null}
      <div className="flex min-w-0 flex-col gap-1.5">
        <div className="flex min-w-0 items-center gap-2">
          {hasChildren ? (
            <button
              type="button"
              className="flex size-5 shrink-0 items-center justify-center rounded border border-[var(--hl)] text-[var(--fa)] hover:border-[var(--ac)] hover:text-[var(--ac)]"
              aria-label={`${expanded ? "Collapse" : "Expand"} ${agent.name} delegates`}
              onClick={(event) => {
                event.stopPropagation()
                onToggle?.()
              }}
            >
              {expanded ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
            </button>
          ) : null}
          <span className="truncate font-hud text-[13px] font-medium text-[var(--tx)]">
            {agent.name}
          </span>
          {topology?.contextOnly ? (
            <span className="font-hud text-[10px] text-[var(--fa)]">context</span>
          ) : null}
        </div>
        <span className="truncate font-hud text-[11px] text-[var(--fa)]">
          {agent.runtime_agent_id}
        </span>
        {topology ? (
          <div className="flex min-w-0 flex-wrap gap-1">
            <TopologyBadge role={topology.role} />
            {visibleOrchestrators.map((orchestrator) => (
              <Badge
                key={orchestrator.id}
                variant="outline"
                title={`Delegated by ${orchestrator.name}`}
                className="max-w-[170px] border-[var(--hl)] bg-[var(--sl)] font-hud text-[10px] text-[var(--mu)]"
              >
                <span className="truncate">Delegated by {orchestrator.name}</span>
              </Badge>
            ))}
            {(topology.delegatedBy.length ?? 0) > visibleOrchestrators.length ? (
              <Badge
                variant="outline"
                className="border-[var(--hl)] bg-[var(--sl)] font-hud text-[10px] text-[var(--fa)]"
              >
                +{topology.delegatedBy.length - visibleOrchestrators.length} orchestrator
                {topology.delegatedBy.length - visibleOrchestrators.length === 1 ? "" : "s"}
              </Badge>
            ) : null}
            {topology.unresolvedDelegations.length ? (
              <Badge
                variant="outline"
                title={topology.unresolvedDelegations
                  .map((item) => item.display_name || item.delegate_ref)
                  .join(", ")}
                className="border-[var(--wnd)] bg-[var(--wnd)] font-hud text-[10px] text-[var(--wn)]"
              >
                {topology.unresolvedDelegations.length} unresolved
              </Badge>
            ) : null}
            {topology.inCycle ? (
              <Badge className="bg-[var(--wnd)] font-hud text-[10px] text-[var(--wn)]">
                cycle
              </Badge>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  )
}

function TopologyBadge({ role }: { role: AgentTopologyRow["role"] }) {
  const className =
    role === "main"
      ? "border-[var(--ac)] bg-[var(--acd)] text-[var(--ac)]"
      : role === "delegated"
        ? "border-[var(--hl)] bg-[var(--sl)] text-[var(--mu)]"
        : "border-[var(--hl)] bg-transparent text-[var(--fa)]"
  return (
    <Badge variant="outline" className={cn("font-hud text-[10px]", className)}>
      {role}
    </Badge>
  )
}

export function AgentSkillCount({ agentId }: { agentId: string }) {
  const skillsQuery = useAgentSkillsQuery(agentId)

  if (skillsQuery.isLoading) {
    return <Skeleton className="h-5 w-10" />
  }

  return (
    <span className="font-hud text-[13px] tabular text-[var(--tx)]">
      {skillsQuery.data?.length ?? 0}
    </span>
  )
}

export function AgentAccessChips({ agentId }: { agentId: string }) {
  const accessQuery = useAgentAccessQuery(agentId)
  const selections =
    accessQuery.data?.selections.filter((selection) => selection.allowed) ?? []
  const visible = selections.slice(0, 3)
  const hiddenCount = Math.max(0, selections.length - visible.length)

  if (accessQuery.isLoading) {
    return (
      <div className="flex flex-wrap gap-1.5">
        <Skeleton className="h-5 w-20" />
        <Skeleton className="h-5 w-16" />
      </div>
    )
  }

  if (!visible.length) {
    return <span className="font-hud text-[11px] text-[var(--fa)]">none resolved</span>
  }

  return (
    <div className="flex flex-wrap gap-1.5">
      {visible.map((selection) => (
        <Badge
          key={`${selection.kind}:${selection.id}`}
          variant="outline"
          title={selection.name || selection.id}
          className="border-[var(--hl)] bg-[var(--sl)] font-hud text-[11px] text-[var(--mu)]"
        >
          {selection.kind}:{selection.name || selection.id}
        </Badge>
      ))}
      {hiddenCount > 0 ? (
        <Badge
          variant="outline"
          className="border-[var(--hl)] bg-[var(--sl)] font-hud text-[11px] text-[var(--fa)]"
        >
          +{hiddenCount}
        </Badge>
      ) : null}
    </div>
  )
}

export function AgentStatusPill({ agent }: { agent: PersistedAgent }) {
  const status = agentStatus(agent)
  const className =
    status === "running"
      ? "bg-[var(--acd)] text-[var(--ac)]"
      : status === "managed"
        ? "bg-[var(--sl)] text-[var(--tx)]"
      : status === "failed"
        ? "bg-[var(--dgd)] text-[var(--dg)]"
        : "bg-[var(--sl)] text-[var(--fa)]"

  return (
    <Badge className={cn("font-hud text-[11px]", className)}>
      {status === "failed" ? "failed" : status}
    </Badge>
  )
}

export function locationForAgent(
  agent: PersistedAgent,
  instances: RuntimeInstance[]
): AgentLocation {
  const instance = instances.find(
    (item) => item.id === agent.runtime_connection_id
  )

  return {
    adapterName: instance
      ? displayRuntimeType(instance.runtime_type)
      : "Unknown",
    instanceName: instance?.display_name || instance?.name || "unknown",
  }
}

function agentStatus(agent: PersistedAgent) {
  if (isLangGraphSystemManaged(agent)) {
    return "managed"
  }
  if (agent.metadata?.runtime_deleted) {
    return "deleted"
  }
  if (agent.freshness === "stale") {
    return "stale"
  }
  const agentValue = (agent.status || "").toLowerCase()
  if (agentValue.includes("disabled")) {
    return "disabled"
  }
  if (agentValue.includes("stale")) {
    return "stale"
  }
  const runtimeValue = (agent.runtime_status || "").toLowerCase()
  if (runtimeValue.includes("fail") || runtimeValue.includes("error")) {
    return "failed"
  }
  if (
    agentValue.includes("enabled") ||
    agentValue.includes("running") ||
    runtimeValue.includes("active") ||
    runtimeValue.includes("healthy")
  ) {
    return "running"
  }
  return "idle"
}

function isLangGraphSystemManaged(agent: PersistedAgent) {
  const metadata = agent.metadata?.assistant_metadata
  return (
    metadata !== null &&
    typeof metadata === "object" &&
    !Array.isArray(metadata) &&
    "created_by" in metadata &&
    metadata.created_by === "system"
  )
}

"use client"

import * as React from "react"

import { AgentDrilldownDrawer } from "@/components/agent-drilldown-drawer"
import {
  AgentTableRow,
  locationForAgent,
} from "@/components/agent-table"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  useFleetAgentDelegationsQuery,
  usePersistedAgentsQuery,
  useRuntimeInstancesQuery,
  useSubagentExecutionsQuery,
} from "@/lib/api-hooks"
import type {
  PersistedAgent,
  SubagentExecution,
} from "@/lib/api-types"
import { relativeTime } from "@/lib/adapters"
import {
  buildAgentTopology,
  filterAgentTopology,
} from "@/lib/agent-topology"
import { cn } from "@/lib/utils"

export function AgentsFleet() {
  const [query, setQuery] = React.useState("")
  const [selectedAgent, setSelectedAgent] = React.useState<PersistedAgent | null>(
    null
  )
  const runtimeInstancesQuery = useRuntimeInstancesQuery()
  const agentsQuery = usePersistedAgentsQuery()
  const executionsQuery = useSubagentExecutionsQuery({})
  const instances = React.useMemo(
    () => runtimeInstancesQuery.data ?? [],
    [runtimeInstancesQuery.data]
  )
  const agents = React.useMemo(
    () => agentsQuery.data ?? [],
    [agentsQuery.data]
  )
  const instanceIDs = React.useMemo(
    () => instances.map((instance) => instance.id),
    [instances]
  )
  const delegationsQuery = useFleetAgentDelegationsQuery(instanceIDs)
  const topology = React.useMemo(
    () =>
      buildAgentTopology(agents, delegationsQuery.data, instances),
    [agents, delegationsQuery.data, instances]
  )
  const filteredTopology = React.useMemo(
    () => filterAgentTopology(topology, instances, query),
    [topology, instances, query]
  )
  const loading =
    runtimeInstancesQuery.isLoading ||
    agentsQuery.isLoading ||
    delegationsQuery.isLoading

  return (
    <section className="flex flex-col gap-5">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h1 className="text-[22px] font-bold leading-tight tracking-[-0.02em] text-[var(--tx)]">
            Agents
          </h1>
          <p className="mt-1 max-w-2xl text-[13px] text-[var(--mu)]">
            Every agent Capcom has imported, across all adapters and instances.
          </p>
        </div>
        <div className="w-full lg:w-[280px]">
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search agents, adapters, instances"
            className="font-hud text-[12px]"
            aria-label="Search agents"
          />
        </div>
      </div>

      {delegationsQuery.isError ? (
        <div
          role="status"
          className="rounded-lg border border-[var(--wnd)] bg-[var(--wnd)] px-4 py-3 text-[12px] text-[var(--wn)]"
        >
          Delegation relationships could not be loaded. Showing role-based agent ordering until the next refresh.
        </div>
      ) : null}

      <div className="overflow-hidden rounded-xl border border-[var(--hl)] bg-[var(--el)] shadow-[var(--chi)]">
        <Table className="table-fixed">
          <colgroup>
            <col className="w-[34%]" />
            <col className="w-[21%]" />
            <col className="w-[10%]" />
            <col className="w-[25%]" />
            <col className="w-[10%]" />
          </colgroup>
          <TableHeader>
            <TableRow className="border-[var(--sl)] hover:bg-transparent">
              <TableHead className="capcom-eyebrow h-10 px-[18px]">Agent</TableHead>
              <TableHead className="capcom-eyebrow h-10 px-[18px]">
                Adapter · Instance
              </TableHead>
              <TableHead className="capcom-eyebrow h-10 px-[18px]">Skills</TableHead>
              <TableHead className="capcom-eyebrow h-10 px-[18px]">
                Can access
              </TableHead>
              <TableHead className="capcom-eyebrow h-10 px-[18px]">Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <FleetSkeletonRows />
            ) : filteredTopology.length ? (
              filteredTopology.map((row) => (
                <AgentTableRow
                  key={row.agent.id}
                  agent={row.agent}
                  topology={row}
                  location={locationForAgent(row.agent, instances)}
                  onAgentClick={setSelectedAgent}
                />
              ))
            ) : (
              <TableRow className="border-[var(--sl)] hover:bg-transparent">
                <TableCell
                  colSpan={5}
                  className="px-[18px] py-10 text-center text-[13px] text-[var(--mu)]"
                >
                  No agents match the current search.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
        <div className="border-t border-[var(--sl)] px-[18px] py-3 font-hud text-[12px] text-[var(--fa)]">
          Showing {filteredTopology.length} of {agents.length} agents - delegated agents follow their orchestrator and search retains ancestor context.
        </div>
      </div>

      <SubagentExecutionsSection
        loading={executionsQuery.isLoading}
        executions={executionsQuery.data ?? []}
        agents={agents}
      />

      <AgentDrilldownDrawer
        agent={selectedAgent}
        open={Boolean(selectedAgent)}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) {
            setSelectedAgent(null)
          }
        }}
      />
    </section>
  )
}

function FleetSkeletonRows() {
  return (
    <>
      {[0, 1, 2, 3, 4].map((row) => (
        <TableRow key={row} className="border-[var(--sl)] hover:bg-transparent">
          {Array.from({ length: 5 }).map((_, column) => (
            <TableCell key={column} className="px-[18px] py-3">
              <Skeleton className="h-5 w-full" />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  )
}

function SubagentExecutionsSection({
  loading,
  executions,
  agents,
}: {
  loading: boolean
  executions: SubagentExecution[]
  agents: PersistedAgent[]
}) {
  const owners = React.useMemo(() => {
    const map = new Map<string, string>()
    for (const agent of agents) {
      map.set(agent.runtime_agent_id, agent.name)
    }
    return map
  }, [agents])

  return (
    <section className="overflow-hidden rounded-xl border border-[var(--hl)] bg-[var(--el)] shadow-[var(--chi)]">
      <div className="flex items-center justify-between gap-3 border-b border-[var(--sl)] px-[18px] py-3">
        <div>
          <div className="capcom-eyebrow">Ephemeral runtime activity</div>
          <h2 className="text-[15px] font-semibold text-[var(--tx)]">
            Subagent executions
          </h2>
        </div>
        <span className="font-hud text-[11px] text-[var(--fa)]">
          {executions.length} execution{executions.length === 1 ? "" : "s"}
        </span>
      </div>
      <Table className="table-fixed">
        <colgroup>
          <col className="w-[20%]" />
          <col className="w-[18%]" />
          <col className="w-[11%]" />
          <col className="w-[17%]" />
          <col className="w-[22%]" />
          <col className="w-[12%]" />
        </colgroup>
        <TableHeader>
          <TableRow className="border-[var(--sl)] hover:bg-transparent">
            <TableHead className="capcom-eyebrow h-10 px-[18px]">Subagent</TableHead>
            <TableHead className="capcom-eyebrow h-10 px-[18px]">Owner</TableHead>
            <TableHead className="capcom-eyebrow h-10 px-[18px]">Status</TableHead>
            <TableHead className="capcom-eyebrow h-10 px-[18px]">Parent run</TableHead>
            <TableHead className="capcom-eyebrow h-10 px-[18px]">Task</TableHead>
            <TableHead className="capcom-eyebrow h-10 px-[18px]">Observed</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? (
            <SubagentSkeletonRows />
          ) : executions.length ? (
            executions.map((execution) => (
              <TableRow
                key={execution.id}
                className="border-[var(--sl)] hover:bg-[var(--sl)]"
              >
                <TableCell className="px-[18px] py-3">
                  <div className="flex min-w-0 flex-col gap-1">
                    <span className="truncate font-hud text-[13px] font-medium text-[var(--tx)]">
                      {execution.subagent_type || "Delegated agent"}
                    </span>
                    <span className="truncate font-hud text-[11px] text-[var(--fa)]">
                      {execution.runtime_execution_id}
                    </span>
                  </div>
                </TableCell>
                <TableCell className="px-[18px] py-3 text-[12px] text-[var(--mu)]">
                  {ownerLabel(execution, owners)}
                </TableCell>
                <TableCell className="px-[18px] py-3">
                  <ExecutionStatusBadge status={execution.status} />
                </TableCell>
                <TableCell className="truncate px-[18px] py-3 font-hud text-[12px] text-[var(--mu)]">
                  {execution.parent_run_id || "none"}
                </TableCell>
                <TableCell
                  className="truncate px-[18px] py-3 text-[12px] text-[var(--mu)]"
                  title={execution.description || execution.summary}
                >
                  {execution.description ||
                    execution.summary ||
                    "No task description"}
                </TableCell>
                <TableCell className="px-[18px] py-3 font-hud text-[12px] text-[var(--fa)]">
                  {relativeTime(execution.observed_at)}
                </TableCell>
              </TableRow>
            ))
          ) : (
            <TableRow className="border-[var(--sl)] hover:bg-transparent">
              <TableCell
                colSpan={6}
                className="px-[18px] py-8 text-center text-[13px] text-[var(--mu)]"
              >
                No delegated subagent executions observed. Registered Gantry agents remain listed above.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </section>
  )
}

function SubagentSkeletonRows() {
  return (
    <>
      {[0, 1, 2].map((row) => (
        <TableRow key={row} className="border-[var(--sl)] hover:bg-transparent">
          {Array.from({ length: 6 }).map((_, column) => (
            <TableCell key={column} className="px-[18px] py-3">
              <Skeleton className="h-5 w-full" />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  )
}

function ExecutionStatusBadge({ status }: { status: string }) {
  const normalized = status.toLowerCase()
  const className =
    normalized.includes("fail") || normalized.includes("error")
      ? "bg-[var(--dgd)] text-[var(--dg)]"
      : normalized.includes("running") || normalized.includes("active")
        ? "bg-[var(--acd)] text-[var(--ac)]"
        : normalized.includes("pending") || normalized.includes("queued")
          ? "bg-[var(--wnd)] text-[var(--wn)]"
          : "bg-[var(--sl)] text-[var(--fa)]"

  return (
    <Badge className={cn("font-hud text-[11px]", className)}>
      {status || "unknown"}
    </Badge>
  )
}

function ownerLabel(
  execution: SubagentExecution,
  owners: Map<string, string>
) {
  if (execution.runtime_agent_id) {
    return owners.get(execution.runtime_agent_id) ?? execution.runtime_agent_id
  }
  return "Runtime job"
}

"use client"

import * as React from "react"
import { useMutation, useQueries, useQueryClient } from "@tanstack/react-query"
import { Ban, ChevronDown, ChevronRight, GitBranch, RefreshCw } from "lucide-react"
import { toast } from "sonner"

import { AddInstanceDialog } from "@/components/add-instance-dialog"
import { AdapterSettingsDialog } from "@/components/adapter-settings-dialog"
import {
  ConsolidateInstanceDialog,
  EndpointTopology,
} from "@/components/consolidate-instance-dialog"
import { AgentDrilldownDrawer } from "@/components/agent-drilldown-drawer"
import { RuntimeCatalogPanel } from "@/components/runtime-catalog-panel"
import {
  AgentTableRow,
} from "@/components/agent-table"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  Collapsible,
  CollapsibleContent,
} from "@/components/ui/collapsible"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  SyncDialog,
  type SyncDialogSubmit,
} from "@/components/sync-dialog"
import {
  buildAdaptersModel,
  relativeTime,
  statusClass,
  type AdapterInstance,
  type AdapterModel,
} from "@/lib/adapters"
import { capcomApi } from "@/lib/api-client"
import {
  queryKeys,
  useCancelExecutionMutation,
  usePersistedAgentsQuery,
  useRuntimeInstanceAgentsQuery,
  useRuntimeInstanceExecutionsQuery,
  useRuntimeInstancesQuery,
  useSyncRuntimeInstanceMutation,
  useTestRuntimeInstanceMutation,
} from "@/lib/api-hooks"
import type {
  PersistedAgent,
  RuntimeCapabilities,
  RuntimeConnectionTestResult,
  RuntimeExecution,
  RuntimeInstance,
  RuntimeType,
  RuntimeSyncRun,
} from "@/lib/api-types"
import { cn } from "@/lib/utils"

const AGENT_PREVIEW_LIMIT = 8

function randomControlKey() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID()
  }
  return `local-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

export function AdapterDetail({ adapterId }: { adapterId: string }) {
  const queryClient = useQueryClient()
  const [syncAllOpen, setSyncAllOpen] = React.useState(false)
  const [addInstanceOpen, setAddInstanceOpen] = React.useState(false)
  const [settingsOpen, setSettingsOpen] = React.useState(false)
  const [consolidateOpen, setConsolidateOpen] = React.useState(false)
  const [selectedAgent, setSelectedAgent] = React.useState<PersistedAgent | null>(
    null
  )
  const runtimeInstancesQuery = useRuntimeInstancesQuery()
  const agentsQuery = usePersistedAgentsQuery()
  const now = React.useMemo(() => {
    const timestamp = Math.max(
      runtimeInstancesQuery.dataUpdatedAt,
      agentsQuery.dataUpdatedAt,
      0
    )
    return timestamp > 0 ? new Date(timestamp) : new Date()
  }, [agentsQuery.dataUpdatedAt, runtimeInstancesQuery.dataUpdatedAt])
  const adaptersModel = React.useMemo(
    () =>
      buildAdaptersModel(
        runtimeInstancesQuery.data ?? [],
        agentsQuery.data ?? [],
        now
      ),
    [agentsQuery.data, now, runtimeInstancesQuery.data]
  )
  const adapter = adaptersModel.adapters.find((item) => item.id === adapterId)
  const loading = runtimeInstancesQuery.isLoading || agentsQuery.isLoading
  const syncAllMutation = useMutation<
    RuntimeSyncRun[],
    Error,
    SyncDialogSubmit
  >({
    mutationFn: async (payload) => {
      if (!adapter) {
        return []
      }
      return Promise.all(
        adapter.instances.map((item) =>
          capcomApi.syncRuntimeInstance(item.instance.id, payload)
        )
      )
    },
    onSuccess: async (runs) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstances }),
        queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgents() }),
        ...runs.map((run) =>
          queryClient.invalidateQueries({
            queryKey: queryKeys.runtimeInstanceAgents(
              run.runtime_connection_id
            ),
          })
        ),
        ...runs.map((run) =>
          queryClient.invalidateQueries({
            queryKey: queryKeys.runtimeInstanceExecutions(
              run.runtime_connection_id
            ),
          })
        ),
        ...runs.flatMap((run) => [
          queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstanceDiagnostics(run.runtime_connection_id) }),
          queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstanceInventory(run.runtime_connection_id) }),
          queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstanceCapabilities(run.runtime_connection_id) }),
        ]),
        ...runs.map((run) =>
          queryClient.invalidateQueries({
            queryKey: queryKeys.runtimeInstanceSyncRuns(
              run.runtime_connection_id
            ),
          })
        ),
      ])
      setSyncAllOpen(false)
      toast.success(
        `${runs.length} instance${runs.length === 1 ? "" : "s"} re-imported`
      )
    },
  })

  if (loading) {
    return <AdapterDetailSkeleton />
  }

  if (!adapter) {
    return (
      <EmptyState
        title="Adapter not found"
        description={`No runtime instances are connected for ${adapterId}.`}
      />
    )
  }

  const styles = statusClass(adapter.status)

  return (
    <section className="flex flex-col gap-5">
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-[22px] font-bold leading-tight tracking-[-0.02em] text-[var(--tx)]">
              {adapter.name}
            </h1>
            <Badge className={cn("font-hud text-[11px]", styles.badge)}>
              {adapter.badge}
            </Badge>
          </div>
          <p className="mt-1 text-[13px] text-[var(--mu)]">
            {adapter.instanceCount} instances connected / {adapter.agentCount} agents running / state re-imported automatically every{" "}
            {syncIntervalLabel(adapter.instances)}
          </p>
        </div>

        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            className="hover:border-[var(--ac)] hover:text-[var(--ac)]"
            onClick={() => setAddInstanceOpen(true)}
          >
            + Add instance
          </Button>
          <Button
            variant="outline"
            className="hover:border-[var(--ac)] hover:text-[var(--ac)]"
            onClick={() => setSettingsOpen(true)}
          >
            Adapter settings
          </Button>
          {adapter.instances.length > 1 ? (
            <Button
              variant="outline"
              className="hover:border-[var(--ac)] hover:text-[var(--ac)]"
              onClick={() => setConsolidateOpen(true)}
            >
              <GitBranch className="size-4" />
              Consolidate connections
            </Button>
          ) : null}
          <Button
            className="shadow-[0_0_0_3px_var(--glow)] hover:brightness-[1.08]"
            onClick={() => setSyncAllOpen(true)}
          >
            <RefreshCw className="size-4" />
            Re-import all instances
          </Button>
        </div>
      </div>

      <div className="flex flex-col gap-3.5">
        {adapter.instances.map((item, index) => (
          <InstanceGroup
            key={item.instance.id}
            item={item}
            defaultOpen={index === 0}
            onAgentClick={setSelectedAgent}
          />
        ))}
      </div>

      <PageFooter adapter={adapter} />

      <AgentDrilldownDrawer
        agent={selectedAgent}
        open={Boolean(selectedAgent)}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) {
            setSelectedAgent(null)
          }
        }}
      />

      <SyncDialog
        open={syncAllOpen}
        targets={adapter.instances.map((item) => ({
          id: item.instance.id,
          name: item.instance.display_name || item.instance.name,
        }))}
        pending={syncAllMutation.isPending}
        defaultReason={`Re-import all ${adapter.name} runtime instances`}
        onOpenChange={setSyncAllOpen}
        onSubmit={(payload) => syncAllMutation.mutate(payload)}
      />

      <AddInstanceDialog
        open={addInstanceOpen}
        defaultAdapterId={runtimeTypeFromRoute(adapterId)}
        onOpenChange={setAddInstanceOpen}
      />

      <AdapterSettingsDialog
        adapterName={adapter.name}
        instances={adapter.instances.map((item) => item.instance)}
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
      />
      <ConsolidateInstanceDialog
        instances={adapter.instances.map((item) => item.instance)}
        open={consolidateOpen}
        onOpenChange={setConsolidateOpen}
      />
    </section>
  )
}

function InstanceGroup({
  item,
  defaultOpen,
  onAgentClick,
}: {
  item: AdapterInstance
  defaultOpen: boolean
  onAgentClick: (agent: PersistedAgent) => void
}) {
  const [open, setOpen] = React.useState(defaultOpen)
  const [syncOpen, setSyncOpen] = React.useState(false)
  const agentsQuery = useRuntimeInstanceAgentsQuery(item.instance.id)
  const executionsQuery = useRuntimeInstanceExecutionsQuery(item.instance.id)
  const syncMutation = useSyncRuntimeInstanceMutation(item.instance.id)
  const agents = agentsQuery.data ?? []
  const executions = executionsQuery.data ?? []
  const shownAgents = agents.slice(0, AGENT_PREVIEW_LIMIT)
  const styles = statusClass(item.status)
  const updateLabel =
    item.status === "failed"
      ? `import failed / ${item.updated}`
      : `updated ${item.updated}`

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <Card className="gap-0 border border-[var(--hl)] bg-[var(--el)] py-0 shadow-[var(--chi)]">
        <div
          className="flex cursor-pointer flex-wrap items-center gap-3 px-[18px] py-3.5 transition hover:bg-[var(--sl)]"
          onClick={() => setOpen((value) => !value)}
        >
          <button
            type="button"
            className="font-hud text-[11px] text-[var(--fa)]"
            aria-label={open ? "Collapse instance" : "Expand instance"}
            onClick={(event) => {
              event.stopPropagation()
              setOpen((value) => !value)
            }}
          >
            {open ? (
              <ChevronDown className="size-4" />
            ) : (
              <ChevronRight className="size-4" />
            )}
          </button>
          <span className={cn("size-2 rounded-full", styles.dot)} />
          <span className="font-hud text-sm font-semibold text-[var(--tx)]">
            {item.instance.display_name || item.instance.name}
          </span>
          <Badge
            variant="outline"
            className="border-[var(--hl)] bg-transparent font-hud text-[11px] text-[var(--mu)]"
          >
            {item.instance.environment || "unspecified"}
          </Badge>
          <span className="font-hud text-[12px] text-[var(--mu)]">
            {agentsQuery.isLoading ? (
              "loading agents"
            ) : (
              <>
                {agents.length} agents / <InstanceSkillTotal agents={agents} /> skills
                {item.instance.runtime_type === "langgraph" || executions.length
                  ? ` / ${executions.length} executions`
                  : null}
              </>
            )}
          </span>
          <div className="ml-auto flex items-center gap-3">
            <span
              className={cn(
                "font-hud text-[12px]",
                item.status === "ok" ? "text-[var(--mu)]" : styles.text
              )}
            >
              {updateLabel}
            </span>
            <Button
              variant="outline"
              size="sm"
              className="font-hud text-xs text-[var(--ac)] hover:border-[var(--ac)] hover:text-[var(--ac)]"
              disabled={syncMutation.isPending}
              onClick={(event) => {
                event.stopPropagation()
                setSyncOpen(true)
              }}
            >
              <RefreshCw className="size-3.5" />
              Re-import
            </Button>
          </div>
        </div>

        <CollapsibleContent>
          <div className="border-t border-[var(--sl)]">
            <EndpointTopology
              endpoints={item.instance.endpoints}
              fallback={item.instance.endpoint}
            />
            <AgentSubTable
              loading={agentsQuery.isLoading}
              agents={shownAgents}
              totalAgents={agents.length}
              onAgentClick={onAgentClick}
            />
            {item.instance.runtime_type === "gantry" ? (
              <RuntimeCatalogPanel runtimeId={item.instance.id} />
            ) : null}
            {item.instance.runtime_type === "langgraph" || executions.length ? (
              <RuntimeExecutionsPanel
                loading={executionsQuery.isLoading}
                executions={executions}
                agents={agents}
                instance={item.instance}
              />
            ) : null}
            <InstanceCapabilityPanel instance={item.instance} />
          </div>
        </CollapsibleContent>
      </Card>

      <SyncDialog
        open={syncOpen}
        targets={[
          {
            id: item.instance.id,
            name: item.instance.display_name || item.instance.name,
          },
        ]}
        pending={syncMutation.isPending}
        defaultReason={`Re-import ${item.instance.display_name || item.instance.name}`}
        onOpenChange={setSyncOpen}
        onSubmit={(payload) => {
          syncMutation.mutate(payload, {
            onSuccess: (run) => {
              setSyncOpen(false)
              toast.success(syncSummary(run))
            },
          })
        }}
      />
    </Collapsible>
  )
}

function AgentSubTable({
  loading,
  agents,
  totalAgents,
  onAgentClick,
}: {
  loading: boolean
  agents: PersistedAgent[]
  totalAgents: number
  onAgentClick: (agent: PersistedAgent) => void
}) {
  return (
    <div>
      <Table className="table-fixed">
        <colgroup>
          <col className="w-[34%]" />
          <col className="w-[13%]" />
          <col className="w-[38%]" />
          <col className="w-[15%]" />
        </colgroup>
        <TableHeader>
          <TableRow className="border-[var(--sl)] hover:bg-transparent">
            <TableHead className="capcom-eyebrow h-9 px-[18px]">Agent</TableHead>
            <TableHead className="capcom-eyebrow h-9 px-[18px]">Skills</TableHead>
            <TableHead className="capcom-eyebrow h-9 px-[18px]">Can access</TableHead>
            <TableHead className="capcom-eyebrow h-9 px-[18px]">Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? (
            <AgentSkeletonRows columns={4} />
          ) : agents.length ? (
            agents.map((agent) => (
              <AgentTableRow
                key={agent.id}
                agent={agent}
                onAgentClick={onAgentClick}
              />
            ))
          ) : (
            <TableRow className="border-[var(--sl)] hover:bg-transparent">
              <TableCell
                colSpan={4}
                className="px-[18px] py-8 text-center text-[13px] text-[var(--mu)]"
              >
                No imported agents. Run a sync for this runtime instance.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
      {totalAgents > agents.length ? (
        <div className="border-t border-[var(--sl)] px-[18px] py-3 font-hud text-[12px] text-[var(--fa)]">
          Showing {agents.length} of {totalAgents} agents
        </div>
      ) : null}
    </div>
  )
}

function RuntimeExecutionsPanel({
  loading,
  executions,
  agents,
  instance,
}: {
  loading: boolean
  executions: RuntimeExecution[]
  agents: PersistedAgent[]
  instance: RuntimeInstance
}) {
  const [collapsedThreads, setCollapsedThreads] = React.useState<Set<string>>(
    () => new Set()
  )
  const [cancelTarget, setCancelTarget] = React.useState<RuntimeExecution | null>(
    null
  )
  const cancelMutation = useCancelExecutionMutation(
    cancelTarget?.id ?? "",
    instance.id
  )
  const agentNames = React.useMemo(
    () => new Map(agents.map((agent) => [agent.runtime_agent_id, agent.name])),
    [agents]
  )
  const rows = React.useMemo(
    () => executionRows(executions, collapsedThreads),
    [collapsedThreads, executions]
  )
  const threadCount = executions.filter((item) => item.kind === "thread").length
  const runCount = executions.filter((item) => item.kind === "run").length

  function toggleThread(id: string) {
    setCollapsedThreads((current) => {
      const next = new Set(current)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  return (
    <section className="border-t border-[var(--sl)]">
      <div className="flex flex-wrap items-center justify-between gap-3 px-[18px] py-3">
        <div>
          <div className="capcom-eyebrow">Runtime activity</div>
          <h3 className="text-[14px] font-semibold text-[var(--tx)]">
            Threads and runs
          </h3>
        </div>
        <span className="font-hud text-[11px] text-[var(--fa)]">
          {threadCount} thread{threadCount === 1 ? "" : "s"} / {runCount} run
          {runCount === 1 ? "" : "s"}
        </span>
      </div>

      <div className="overflow-x-auto">
      <Table className="min-w-[760px] table-fixed">
        <colgroup>
          <col className="w-[35%]" />
          <col className="w-[20%]" />
          <col className="w-[13%]" />
          <col className="w-[17%]" />
          <col className="w-[12%]" />
          <col className="w-[11%]" />
        </colgroup>
        <TableHeader>
          <TableRow className="border-[var(--sl)] hover:bg-transparent">
            <TableHead className="capcom-eyebrow h-9 px-[18px]">Execution</TableHead>
            <TableHead className="capcom-eyebrow h-9 px-[18px]">Agent</TableHead>
            <TableHead className="capcom-eyebrow h-9 px-[18px]">Status</TableHead>
            <TableHead className="capcom-eyebrow h-9 px-[18px]">Started</TableHead>
            <TableHead className="capcom-eyebrow h-9 px-[18px]">Duration</TableHead>
            <TableHead className="capcom-eyebrow h-9 px-[18px] text-right">Action</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? (
            <AgentSkeletonRows columns={5} />
          ) : rows.length ? (
            rows.map(({ execution, depth, childCount }) => {
              const isThread = execution.kind === "thread"
              const collapsed = collapsedThreads.has(
                execution.runtime_execution_id
              )
              return (
                <TableRow
                  key={`${execution.kind}:${execution.runtime_execution_id}`}
                  className="border-[var(--sl)] hover:bg-[var(--sl)]"
                >
                  <TableCell className="px-[18px] py-3">
                    <div
                      className={cn(
                        "flex min-w-0 items-start gap-2",
                        depth && "pl-7"
                      )}
                    >
                      {isThread ? (
                        <button
                          type="button"
                          title={collapsed ? "Expand thread" : "Collapse thread"}
                          aria-label={collapsed ? "Expand thread" : "Collapse thread"}
                          className="mt-0.5 flex size-5 shrink-0 items-center justify-center text-[var(--fa)] hover:text-[var(--ac)]"
                          onClick={() => toggleThread(execution.runtime_execution_id)}
                        >
                          <ChevronRight
                            className={cn(
                              "size-4 transition-transform",
                              !collapsed && "rotate-90"
                            )}
                          />
                        </button>
                      ) : (
                        <GitBranch className="mt-1 size-4 shrink-0 text-[var(--fa)]" />
                      )}
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-hud text-[12px] font-semibold text-[var(--tx)]">
                            {isThread ? "Thread" : "Run"}
                          </span>
                          {isThread ? (
                            <span className="font-hud text-[10px] text-[var(--fa)]">
                              {childCount} run{childCount === 1 ? "" : "s"}
                            </span>
                          ) : null}
                        </div>
                        <div
                          className="truncate font-hud text-[11px] text-[var(--fa)]"
                          title={execution.runtime_execution_id}
                        >
                          {execution.runtime_execution_id}
                        </div>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="truncate px-[18px] py-3 text-[12px] text-[var(--mu)]">
                    {execution.runtime_agent_id
                      ? agentNames.get(execution.runtime_agent_id) ??
                        execution.runtime_agent_id
                      : isThread
                        ? threadAgentName(execution, agentNames)
                        : "Unassigned"}
                  </TableCell>
                  <TableCell className="px-[18px] py-3">
                    <RuntimeExecutionStatus status={execution.status} />
                  </TableCell>
                  <TableCell className="px-[18px] py-3 font-hud text-[12px] text-[var(--mu)]">
                    {relativeTime(execution.started_at ?? execution.observed_at)}
                  </TableCell>
                  <TableCell className="px-[18px] py-3 font-hud text-[12px] text-[var(--fa)]">
                    {executionDuration(execution)}
                  </TableCell>
                  <TableCell className="px-[18px] py-3 text-right">
                    {instance.runtime_type === "langgraph" &&
                    instance.mode === "control_enabled" &&
                    execution.kind === "run" &&
                    ["pending", "running"].includes(execution.status) ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        title="Cancel run"
                        aria-label="Cancel run"
                        className="text-[var(--dg)] hover:text-[var(--dg)]"
                        onClick={() => setCancelTarget(execution)}
                      >
                        <Ban className="size-3.5" />
                        Cancel
                      </Button>
                    ) : null}
                  </TableCell>
                </TableRow>
              )
            })
          ) : (
            <TableRow className="border-[var(--sl)] hover:bg-transparent">
              <TableCell
                colSpan={6}
                className="px-[18px] py-8 text-center text-[13px] text-[var(--mu)]"
              >
                No runtime executions imported yet. Run a sync after invoking an agent.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
      </div>
      <Dialog
        open={Boolean(cancelTarget)}
        onOpenChange={(open) => {
          if (!open && !cancelMutation.isPending) setCancelTarget(null)
        }}
      >
        <DialogContent className="border border-[var(--hl)] bg-[var(--el)] shadow-[var(--shdw)] sm:max-w-[480px]">
          <form
            className="flex flex-col gap-4"
            onSubmit={(event) => {
              event.preventDefault()
              if (!cancelTarget) return
              const formData = new FormData(event.currentTarget)
              const actor = String(formData.get("actor") ?? "").trim()
              const reason = String(formData.get("reason") ?? "").trim()
              if (!actor || !reason) return
              const dryRun = formData.get("dry_run") === "on"
              cancelMutation.mutate(
                {
                  actor,
                  reason,
                  idempotency_key: randomControlKey(),
                  dry_run: dryRun,
                },
                {
                  onSuccess: (action) => {
                    toast.success(
                      `Cancel run ${action.status.replaceAll("_", " ")}${dryRun ? " (validation only)" : ""}`
                    )
                    setCancelTarget(null)
                  },
                }
              )
            }}
          >
            <DialogHeader>
              <DialogTitle>Cancel run</DialogTitle>
              <DialogDescription>
                Interrupt this run and preserve its record and checkpoints.
              </DialogDescription>
            </DialogHeader>
            <label className="flex flex-col gap-2">
              <span className="capcom-eyebrow">Actor</span>
              <Input name="actor" defaultValue="local-operator" required className="font-hud text-[13px]" />
            </label>
            <label className="flex flex-col gap-2">
              <span className="capcom-eyebrow">Reason</span>
              <Textarea name="reason" defaultValue="Stop active execution" required rows={3} className="resize-none font-hud text-[13px]" />
            </label>
            <label className="flex items-center gap-2 font-hud text-[12px] text-[var(--mu)]">
              <input name="dry_run" type="checkbox" className="accent-[var(--ac)]" />
              Validate only
            </label>
            <DialogFooter>
              <Button type="button" variant="outline" disabled={cancelMutation.isPending} onClick={() => setCancelTarget(null)}>Cancel</Button>
              <Button type="submit" variant="destructive" disabled={cancelMutation.isPending}>
                {cancelMutation.isPending ? "Cancelling" : "Cancel run"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </section>
  )
}

function executionRows(
  executions: RuntimeExecution[],
  collapsedThreads: Set<string>
) {
  const children = new Map<string, RuntimeExecution[]>()
  const threads: RuntimeExecution[] = []
  const orphans: RuntimeExecution[] = []

  for (const execution of executions) {
    if (execution.kind === "thread") {
      threads.push(execution)
    } else if (execution.parent_runtime_execution_id) {
      const bucket = children.get(execution.parent_runtime_execution_id) ?? []
      bucket.push(execution)
      children.set(execution.parent_runtime_execution_id, bucket)
    } else {
      orphans.push(execution)
    }
  }

  const byNewest = (left: RuntimeExecution, right: RuntimeExecution) =>
    executionTime(right) - executionTime(left)
  threads.sort(byNewest)
  orphans.sort(byNewest)

  const rows: Array<{
    execution: RuntimeExecution
    depth: number
    childCount: number
  }> = []
  const knownThreads = new Set(threads.map((item) => item.runtime_execution_id))
  for (const thread of threads) {
    const runs = (children.get(thread.runtime_execution_id) ?? []).sort(byNewest)
    rows.push({ execution: thread, depth: 0, childCount: runs.length })
    if (!collapsedThreads.has(thread.runtime_execution_id)) {
      rows.push(
        ...runs.map((execution) => ({ execution, depth: 1, childCount: 0 }))
      )
    }
  }
  for (const [parentID, runs] of children) {
    if (!knownThreads.has(parentID)) {
      orphans.push(...runs)
    }
  }
  rows.push(
    ...orphans.sort(byNewest).map((execution) => ({
      execution,
      depth: 0,
      childCount: 0,
    }))
  )
  return rows
}

function RuntimeExecutionStatus({ status }: { status: string }) {
  const normalized = status.toLowerCase()
  const className =
    normalized === "success" || normalized === "completed"
      ? "bg-[var(--acd)] text-[var(--ac)]"
      : normalized.includes("fail") || normalized.includes("error")
        ? "bg-[var(--dgd)] text-[var(--dg)]"
        : normalized.includes("running") || normalized === "busy"
          ? "bg-[var(--wnd)] text-[var(--wn)]"
          : "bg-[var(--sl)] text-[var(--fa)]"

  return (
    <Badge className={cn("font-hud text-[11px]", className)}>
      {status || "unknown"}
    </Badge>
  )
}

function threadAgentName(
  execution: RuntimeExecution,
  agentNames: Map<string, string>
) {
  const metadata = execution.metadata?.thread_metadata
  if (!metadata || typeof metadata !== "object") {
    return "Runtime thread"
  }
  const assistantID = (metadata as Record<string, unknown>).assistant_id
  return typeof assistantID === "string"
    ? agentNames.get(assistantID) ?? assistantID
    : "Runtime thread"
}

function executionDuration(execution: RuntimeExecution) {
  if (!execution.started_at || !execution.ended_at) {
    return execution.ended_at ? "-" : "in progress"
  }
  const duration = Math.max(
    0,
    new Date(execution.ended_at).getTime() -
      new Date(execution.started_at).getTime()
  )
  if (duration < 1000) {
    return `${duration}ms`
  }
  if (duration < 60_000) {
    return `${(duration / 1000).toFixed(1)}s`
  }
  return `${Math.floor(duration / 60_000)}m ${Math.round(
    (duration % 60_000) / 1000
  )}s`
}

function executionTime(execution: RuntimeExecution) {
  return new Date(execution.started_at ?? execution.observed_at).getTime()
}

function InstanceSkillTotal({ agents }: { agents: PersistedAgent[] }) {
  const skillQueries = useQueries({
    queries: agents.map((agent) => ({
      queryKey: queryKeys.agentSkills(agent.id),
      queryFn: () => capcomApi.listAgentSkills(agent.id),
      staleTime: 30_000,
    })),
  })
  const pending = skillQueries.some((query) => query.isLoading)
  const total = skillQueries.reduce(
    (sum, query) => sum + (query.data?.length ?? 0),
    0
  )

  if (!agents.length) {
    return <span>0</span>
  }

  return <span>{pending ? "..." : total}</span>
}

function InstanceCapabilityPanel({ instance }: { instance: RuntimeInstance }) {
  const testMutation = useTestRuntimeInstanceMutation(instance.id)
  const result = testMutation.data

  return (
    <div className="flex flex-col gap-3 border-t border-[var(--sl)] px-[18px] py-3 md:flex-row md:items-center md:justify-between">
      <div className="min-w-0">
        <div className="capcom-eyebrow">Test connection</div>
        <div className="mt-1 text-[12px] text-[var(--mu)]">
          {result?.message ?? "Capabilities pending"}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <CapabilityChips result={result} />
        <Button
          variant="outline"
          size="sm"
          className="font-hud text-xs hover:border-[var(--ac)] hover:text-[var(--ac)]"
          disabled={testMutation.isPending}
          onClick={() => {
            testMutation.mutate(undefined, {
              onSuccess: (data) => {
                toast.success(`Connection test ${data.status}`)
              },
            })
          }}
        >
          {testMutation.isPending ? "Testing" : "Test connection"}
        </Button>
      </div>
    </div>
  )
}

function CapabilityChips({
  result,
}: {
  result?: RuntimeConnectionTestResult
}) {
  if (!result) {
    return (
      <Badge
        variant="outline"
        className="border-[var(--hl)] bg-transparent font-hud text-[11px] text-[var(--fa)]"
      >
        capabilities pending
      </Badge>
    )
  }

  return (
    <>
      {capabilityEntries(result.capabilities).map(([key, label, enabled]) => (
        <Badge
          key={key}
          variant="outline"
          className={cn(
            "border-[var(--hl)] bg-transparent font-hud text-[11px]",
            enabled
              ? "border-[color-mix(in_srgb,var(--ac)_35%,var(--hl))] bg-[var(--acd)] text-[var(--ac)]"
              : "text-[var(--fa)]"
          )}
        >
          {label}
        </Badge>
      ))}
    </>
  )
}

function capabilityEntries(capabilities: RuntimeCapabilities) {
  return [
    ["read_agents", "Read agents", capabilities.read_agents],
    ["read_agent_hierarchy", "Agent hierarchy", capabilities.read_agent_hierarchy],
    ["read_agent_delegates", "Agent delegates", capabilities.read_agent_delegates],
    ["read_agent_skills", "Read skills", capabilities.read_agent_skills],
    ["read_agent_access", "Read access", capabilities.read_agent_access],
    ["replace_agent_access", "Replace access", capabilities.replace_agent_access],
    [
      "read_subagent_executions",
      "Subagent executions",
      Boolean(capabilities.read_subagent_executions),
    ],
    ["read_executions", "Executions", Boolean(capabilities.read_executions)],
    ["read_diagnostics", "Diagnostics", Boolean(capabilities.read_diagnostics)],
    ["read_inventory", "Inventory", Boolean(capabilities.read_inventory)],
    ["read_capability_catalog", "Capability catalog", Boolean(capabilities.read_capability_catalog)],
    ["set_agent_status", "Agent status control", Boolean(capabilities.set_agent_status)],
    ["delete_agent", "Delete agent", Boolean(capabilities.delete_agent)],
    ["cancel_execution", "Cancel execution", Boolean(capabilities.cancel_execution)],
  ] as const
}

function PageFooter({ adapter }: { adapter: AdapterModel }) {
  const first = adapter.instances[0]?.instance

  return (
    <p className="font-hud text-[12px] text-[var(--fa)]">
      Connection: {first?.endpoint ?? "none"} / adapter v
      {adapterVersion(adapter.instances.map((item) => item.instance))} / freshness budget 5m per instance
    </p>
  )
}

function AdapterDetailSkeleton() {
  return (
    <section className="flex flex-col gap-5">
      <div className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-2">
          <Skeleton className="h-8 w-44" />
          <Skeleton className="h-5 w-[420px]" />
        </div>
        <Skeleton className="h-8 w-72" />
      </div>
      <Skeleton className="h-[260px] rounded-xl" />
      <Skeleton className="h-[74px] rounded-xl" />
    </section>
  )
}

function AgentSkeletonRows({ columns }: { columns: number }) {
  return (
    <>
      {[0, 1, 2].map((row) => (
        <TableRow key={row} className="border-[var(--sl)] hover:bg-transparent">
          {Array.from({ length: columns }).map((_, column) => (
            <TableCell key={column} className="px-[18px] py-3">
              <Skeleton className="h-5 w-full" />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  )
}

function EmptyState({
  title,
  description,
  action,
}: {
  title: string
  description: string
  action?: React.ReactNode
}) {
  return (
    <section className="rounded-xl border border-[var(--hl)] bg-[var(--el)] p-5 shadow-[var(--chi)]">
      <h1 className="text-[22px] font-bold leading-tight text-[var(--tx)]">
        {title}
      </h1>
      <p className="mt-1 text-[13px] text-[var(--mu)]">{description}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </section>
  )
}

function syncIntervalLabel(instances: AdapterInstance[]) {
  const intervals = new Set(
    instances.map((item) => item.instance.sync_interval_seconds)
  )
  if (intervals.size !== 1) {
    return "mixed intervals"
  }
  const seconds = instances[0]?.instance.sync_interval_seconds ?? 0
  if (seconds <= 0) {
    return "manual sync"
  }
  if (seconds < 60) {
    return `${seconds}s`
  }
  const minutes = Math.round(seconds / 60)
  return `${minutes}m`
}

function adapterVersion(instances: RuntimeInstance[]) {
  for (const instance of instances) {
    const version =
      instance.labels.adapter_version ??
      instance.labels.runtime_version ??
      instance.labels.version
    if (version) {
      return version
    }
  }
  return "unknown"
}

function syncSummary(run: RuntimeSyncRun) {
  const agents = run.agents_seen ?? 0
  const skills = run.skills_seen ?? 0
  const executions = run.executions_seen ?? 0
  return `Instance sync completed: ${agents} agents, ${skills} skills, and ${executions} executions imported`
}

function runtimeTypeFromRoute(adapterId: string): RuntimeType | undefined {
  if (
    adapterId === "gantry" ||
    adapterId === "langgraph" ||
    adapterId === "temporal" ||
    adapterId === "letta" ||
    adapterId === "crewai"
  ) {
    return adapterId
  }
  return undefined
}

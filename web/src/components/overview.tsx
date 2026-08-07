"use client"

import * as React from "react"
import Link from "next/link"
import { MoreHorizontal, RefreshCw, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { AddInstanceDialog } from "@/components/add-instance-dialog"
import { RemoveInstanceDialog } from "@/components/remove-instance-dialog"
import {
  OperationalError,
  PageHeader,
  RuntimeSyncStatus,
} from "@/components/operator-ui"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  buildAdaptersModel,
  statusClass,
  type AdapterModel,
  type AttentionItem,
} from "@/lib/adapters"
import {
  usePersistedAgentsQuery,
  useRuntimeInstancesQuery,
  useSyncRuntimeInstanceMutation,
  useSyncRuntimeInstancesMutation,
} from "@/lib/api-hooks"
import { cn } from "@/lib/utils"

export function Overview() {
  const [addInstanceOpen, setAddInstanceOpen] = React.useState(false)
  const runtimeInstancesQuery = useRuntimeInstancesQuery()
  const agentsQuery = usePersistedAgentsQuery()
  const now = React.useMemo(
    () =>
      new Date(
        Math.max(
          runtimeInstancesQuery.dataUpdatedAt,
          agentsQuery.dataUpdatedAt,
          0
        )
      ),
    [agentsQuery.dataUpdatedAt, runtimeInstancesQuery.dataUpdatedAt]
  )
  const { adapters, attention } = React.useMemo(
    () =>
      buildAdaptersModel(
        runtimeInstancesQuery.data ?? [],
        agentsQuery.data ?? [],
        now
      ),
    [agentsQuery.data, now, runtimeInstancesQuery.data]
  )
  const loading = runtimeInstancesQuery.isLoading || agentsQuery.isLoading
  const instanceIDs = React.useMemo(
    () => (runtimeInstancesQuery.data ?? []).map((instance) => instance.id),
    [runtimeInstancesQuery.data]
  )
  const refreshAllMutation = useSyncRuntimeInstancesMutation(instanceIDs)

  function refreshAll() {
    refreshAllMutation.mutate(
      {
        actor: "local-operator",
        reason: "Overview refresh of all runtime adapters",
      },
      {
        onSuccess: (runs) =>
          toast.success(
            `${runs.length} instance${runs.length === 1 ? "" : "s"} refreshed`
          ),
      }
    )
  }

  return (
    <section className="flex flex-col gap-6">
      <PageHeader
        eyebrow="Overview"
        title="Runtime adapters"
        description="Runtime availability and imported-state freshness across the fleet."
        actions={
          <Button
            variant="outline"
            size="sm"
            className="font-hud text-xs hover:border-[var(--ac)] hover:text-[var(--ac)]"
            disabled={loading || !instanceIDs.length || refreshAllMutation.isPending}
            onClick={refreshAll}
          >
            <RefreshCw className={cn("size-3.5", refreshAllMutation.isPending && "animate-spin")} />
            {refreshAllMutation.isPending ? "Refreshing all" : "Refresh all"}
          </Button>
        }
      />

      {loading ? (
        <OverviewSkeleton />
      ) : (
        <div className="grid gap-4 lg:grid-cols-3">
          {adapters.map((adapter) => (
            <AdapterCard key={adapter.id} adapter={adapter} />
          ))}
          <button
            type="button"
            onClick={() => setAddInstanceOpen(true)}
            className="capcom-connect-card min-h-[178px] rounded-xl p-4 text-left transition hover:text-[var(--ac)]"
          >
            <div className="flex h-full flex-col justify-between">
              <div>
                <div className="flex h-8 w-8 items-center justify-center rounded-[var(--radius-control)] border border-[var(--hl)] font-hud text-lg">
                  +
                </div>
                <div className="mt-4 text-sm font-semibold">
                  Connect an adapter
                </div>
                <p className="mt-1 text-[13px] text-[var(--mu)]">
                  Add a runtime instance and Capcom will group it by runtime
                  type.
                </p>
              </div>
              <div className="font-hud text-[11px] text-[var(--fa)]">
                runtime_type becomes adapterId
              </div>
            </div>
          </button>
        </div>
      )}

      <AttentionQueue attention={attention} loading={loading} />

      <AddInstanceDialog
        open={addInstanceOpen}
        onOpenChange={setAddInstanceOpen}
      />
    </section>
  )
}

function AdapterCard({ adapter }: { adapter: AdapterModel }) {
  const styles = statusClass(adapter.status)
  const instanceIDs = React.useMemo(
    () => adapter.instances.map((item) => item.instance.id),
    [adapter.instances]
  )
  const refreshMutation = useSyncRuntimeInstancesMutation(instanceIDs)
  const issue = adapter.instances.find((item) => item.status !== "ok")
  const healthyInstances = adapter.instances.filter((item) => item.status === "ok").length

  function refreshAdapter() {
    refreshMutation.mutate(
      {
        actor: "local-operator",
        reason: `Overview refresh of ${adapter.name}`,
      },
      {
        onSuccess: () => toast.success(`${adapter.name} refreshed`),
      }
    )
  }

  return (
    <Card
      data-status={adapter.status}
      className="capcom-status-surface relative gap-0 overflow-hidden bg-[var(--el)] p-0 transition hover:-translate-y-px hover:border-[var(--ac)]"
    >
      <Link
        href={`/adapters/${adapter.id}`}
        className="flex min-h-[166px] flex-col gap-4 p-5 pb-11"
      >
        <CardHeader className="p-0">
          <CardTitle className="flex items-center gap-2 text-[15px]">
            <span className={cn("h-2 w-2 rounded-full", styles.dot)} />
            {adapter.name}
          </CardTitle>
          <CardAction>
            <Badge className={cn("font-hud text-[11px]", styles.badge)}>
              {adapter.badge}
            </Badge>
          </CardAction>
        </CardHeader>
        <CardContent className="flex flex-1 flex-col justify-between gap-3 p-0">
          <div className="grid grid-cols-2 gap-3">
            <Metric label="instances" value={adapter.instanceCount} />
            <Metric label="agents" value={adapter.agentCount} />
          </div>
          <div className="flex flex-wrap items-end justify-between gap-2 pr-8">
            <div className="font-hud text-[11px] text-[var(--mu)]">
              {healthyInstances}/{adapter.instanceCount} instances ready
              <span className="text-[var(--fa)]"> · {issue ? issue.updated : adapter.instances[0]?.updated ?? "never"}</span>
            </div>
            {adapter.instances[0] ? (
              <RuntimeSyncStatus
                runtime={issue?.runtimeHealth ?? adapter.instances[0].runtimeHealth}
                sync={issue?.syncHealth ?? adapter.instances[0].syncHealth}
              />
            ) : null}
          </div>
        </CardContent>
      </Link>
      {issue?.instance.last_error ? (
        <div className="border-t border-[var(--sl)] px-4 py-2.5 pr-12">
          <OperationalError error={issue.instance.last_error} compact />
        </div>
      ) : null}
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        title={`Refresh ${adapter.name}`}
        aria-label={`Refresh ${adapter.name}`}
        className="absolute bottom-3 right-3 border border-[var(--hl)] bg-[var(--cv)]/60 text-[var(--mu)] shadow-[var(--chi)] hover:border-[var(--ac)] hover:text-[var(--ac)]"
        disabled={refreshMutation.isPending}
        onClick={refreshAdapter}
      >
        <RefreshCw
          className={cn("size-3.5", refreshMutation.isPending && "animate-spin")}
        />
      </Button>
    </Card>
  )
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <div className="font-hud text-[11px] uppercase text-[var(--fa)]">
        {label}
      </div>
      <div className="mt-1 font-hud text-2xl font-semibold tabular text-[var(--tx)]">
        {value}
      </div>
    </div>
  )
}

function AttentionQueue({
  attention,
  loading,
}: {
  attention: AttentionItem[]
  loading: boolean
}) {
  const queueStatus = attention.some((item) => item.status === "failed")
    ? "failed"
    : attention.some((item) => item.status === "stale")
      ? "stale"
      : "ok"

  return (
    <section
      data-status={queueStatus}
      className="capcom-status-surface rounded-xl bg-[var(--el)]"
    >
      <div className="flex items-center justify-between border-b border-[var(--sl)] px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold">Needs your attention</h2>
          <div className="font-hud text-[11px] text-[var(--fa)]">
            {attention.length
              ? `${attention.length} item${attention.length === 1 ? "" : "s"}`
              : "all clear"}
          </div>
        </div>
      </div>

      {loading ? (
        <div className="grid gap-3 p-4">
          <Skeleton className="h-12" />
          <Skeleton className="h-12" />
        </div>
      ) : attention.length ? (
        <div className="divide-y divide-[var(--sl)]">
          {attention.map((item) => (
            <AttentionRow key={item.instanceId} item={item} />
          ))}
        </div>
      ) : (
        <div className="flex items-center gap-3 px-4 py-4 text-[13px] text-[var(--mu)]">
          <span className="h-2 w-2 rounded-full bg-[var(--ac)] shadow-[0_0_0_3px_var(--acd)]" />
          Nothing needs attention right now. All instances are fresh.
        </div>
      )}
    </section>
  )
}

function AttentionRow({ item }: { item: AttentionItem }) {
  const styles = statusClass(item.status)
  const syncMutation = useSyncRuntimeInstanceMutation(item.instanceId)
  const [removeOpen, setRemoveOpen] = React.useState(false)

  return (
    <div className="grid gap-3 px-4 py-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="flex min-w-0 items-start gap-3">
        <span className={cn("mt-1 h-2 w-2 rounded-full", styles.dot)} />
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-[var(--tx)]">
              {item.instanceName}
            </span>
            <Badge className={cn("font-hud text-[11px]", styles.badge)}>
              {item.badge}
            </Badge>
            <span className="font-hud text-[11px] text-[var(--fa)]">
              {item.adapterName}
            </span>
          </div>
          <div className="mt-1">
            <RuntimeSyncStatus runtime={item.runtimeHealth} sync={item.syncHealth} />
          </div>
          {item.error ? (
            <div className="mt-2">
              <OperationalError error={item.error} />
            </div>
          ) : (
            <p className="mt-1 text-[12px] text-[var(--mu)]">{item.message}</p>
          )}
        </div>
      </div>
      <div className="flex justify-self-start gap-2 md:justify-self-end">
        <Button
          variant="outline"
          size="sm"
          className="font-hud text-xs hover:border-[var(--ac)] hover:text-[var(--ac)]"
          disabled={syncMutation.isPending}
          onClick={() => {
            syncMutation.mutate(
              {
                actor: "local-operator",
                reason: `Overview attention action for ${item.instanceName}`,
              },
              {
                onSuccess: () =>
                  toast.success(`${item.instanceName} sync complete`),
              }
            )
          }}
        >
          {syncMutation.isPending ? "Importing" : item.action}
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button variant="outline" size="icon-sm" aria-label={`Actions for ${item.instanceName}`}>
                <MoreHorizontal className="size-4" />
              </Button>
            }
          />
          <DropdownMenuContent align="end" className="w-44 border border-[var(--hl)] bg-[var(--el)]">
            <DropdownMenuItem variant="destructive" onClick={() => setRemoveOpen(true)}>
              <Trash2 /> Remove instance
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <RemoveInstanceDialog
          instance={{
            id: item.instanceId,
            name: item.instanceStableKey,
            display_name: item.instanceName,
          }}
          open={removeOpen}
          onOpenChange={setRemoveOpen}
        />
      </div>
    </div>
  )
}

function OverviewSkeleton() {
  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Skeleton className="h-[178px]" />
      <Skeleton className="h-[178px]" />
      <Skeleton className="h-[178px]" />
    </div>
  )
}

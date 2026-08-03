"use client"

import * as React from "react"
import Link from "next/link"
import { Trash2 } from "lucide-react"
import { toast } from "sonner"

import { AddInstanceDialog } from "@/components/add-instance-dialog"
import { RemoveInstanceDialog } from "@/components/remove-instance-dialog"
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
  buildAdaptersModel,
  statusClass,
  type AdapterModel,
  type AttentionItem,
} from "@/lib/adapters"
import {
  usePersistedAgentsQuery,
  useRuntimeInstancesQuery,
  useSyncRuntimeInstanceMutation,
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

  return (
    <section className="flex flex-col gap-6">
      <div>
        <div className="capcom-eyebrow">Overview</div>
        <h1 className="mt-1 text-[22px] font-bold leading-tight text-[var(--tx)]">
          Runtime adapters
        </h1>
        <p className="mt-1 max-w-2xl text-[13px] text-[var(--mu)]">
          Imported runtime state grouped by adapter, with freshness and sync
          health derived from live Capcom API data.
        </p>
      </div>

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
            className="min-h-[178px] rounded-xl border border-dashed border-[var(--hl)] bg-transparent p-4 text-left transition hover:border-[var(--ac)] hover:text-[var(--ac)]"
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

  return (
    <Link href={`/adapters/${adapter.id}`} className="block">
      <Card className="min-h-[178px] border border-[var(--hl)] bg-[var(--el)] shadow-[var(--chi)] transition hover:border-[var(--ac)]">
        <CardHeader>
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
        <CardContent className="flex flex-1 flex-col justify-between gap-5">
          <div className="grid grid-cols-2 gap-3">
            <Metric label="instances" value={adapter.instanceCount} />
            <Metric label="agents" value={adapter.agentCount} />
          </div>
          <div className={cn("font-hud text-[11px]", adapter.status === "ok" ? "text-[var(--mu)]" : styles.text)}>
            {adapter.footer}
          </div>
        </CardContent>
      </Card>
    </Link>
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
  return (
    <section className="rounded-xl border border-[var(--hl)] bg-[var(--el)] shadow-[var(--chi)]">
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
          <p className="mt-1 text-[13px] text-[var(--mu)]">{item.message}</p>
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
        <Button
          variant="outline"
          size="sm"
          className="font-hud text-xs text-red-300 hover:border-red-500/60 hover:bg-red-500/10 hover:text-red-200"
          onClick={() => setRemoveOpen(true)}
        >
          <Trash2 className="size-3.5" />
          Remove
        </Button>
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

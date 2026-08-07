"use client"

import * as React from "react"
import {
  Activity,
  ArrowUpRight,
  Clock3,
  Coins,
  Gauge,
  Pause,
  Play,
  Radio,
  RefreshCw,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { PageHeader } from "@/components/operator-ui"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Skeleton } from "@/components/ui/skeleton"
import {
  useFleetTelemetryHealthQuery,
  usePersistedAgentsQuery,
  useRuntimeInstancesQuery,
  useScopedMetricsQuery,
} from "@/lib/api-hooks"
import type { MetricSummary, TelemetryHealth } from "@/lib/api-types"
import { cn } from "@/lib/utils"

type MetricsRange = "24h" | "7d" | "30d"
type DetailKind = "tokens" | "requests" | "models"
type MetricsScope = "fleet" | "runtime" | "agent"

export function MetricsDashboard() {
  const [range, setRange] = React.useState<MetricsRange>("24h")
  const [detail, setDetail] = React.useState<DetailKind | null>(null)
  const [live, setLive] = React.useState(true)
  const [scope, setScope] = React.useState<MetricsScope>("fleet")
  const [runtimeID, setRuntimeID] = React.useState("")
  const [agentID, setAgentID] = React.useState("")
  const instancesQuery = useRuntimeInstancesQuery()
  const agentsQuery = usePersistedAgentsQuery()
  const effectiveRuntimeID =
    scope === "fleet" ? "" : runtimeID || instancesQuery.data?.[0]?.id || ""
  const scopedAgents = React.useMemo(
    () =>
      (agentsQuery.data ?? []).filter(
        (agent) =>
          !effectiveRuntimeID || agent.runtime_connection_id === effectiveRuntimeID
      ),
    [agentsQuery.data, effectiveRuntimeID]
  )
  const effectiveAgentID =
    scope === "agent" ? agentID || scopedAgents[0]?.id || "" : ""
  const query = useScopedMetricsQuery(
    range,
    effectiveRuntimeID || undefined,
    effectiveAgentID || undefined,
    live
  )
  const healthQuery = useFleetTelemetryHealthQuery(live)
  const metrics = query.data
  const refreshing = query.isFetching || healthQuery.isFetching
  const refreshedAt = Math.max(query.dataUpdatedAt, healthQuery.dataUpdatedAt)

  function refresh() {
    void Promise.all([query.refetch(), healthQuery.refetch()])
  }

  return (
    <section className="flex flex-col gap-5">
      <PageHeader
        eyebrow="Telemetry"
        title="Metrics"
        description="Live token flow, request volume, model mix, latency, and cost. Open any chart for exact observations."
        actions={
          <>
          <div className="flex items-center gap-2 font-hud text-[10px] text-[var(--fa)]">
            <span className={cn("size-1.5 rounded-full", live ? "bg-[var(--ac)] shadow-[0_0_0_3px_var(--acd)]" : "bg-[var(--fa)]")} />
            {live ? "Live · 30s" : "Paused"}
            <span className="text-[var(--mu)]">
              {refreshedAt ? `updated ${relativeTime(new Date(refreshedAt).toISOString())}` : "connecting"}
            </span>
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="h-9 font-hud text-[11px]"
            onClick={() => setLive((value) => !value)}
          >
            {live ? <Pause className="size-3.5" /> : <Play className="size-3.5" />}
            {live ? "Pause" : "Resume live"}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="h-9 font-hud text-[11px]"
            onClick={refresh}
            disabled={refreshing}
          >
            <RefreshCw className={cn("size-3.5", refreshing && "animate-spin")} />
            Refresh now
          </Button>
          <div className="flex rounded-lg border border-[var(--hl)] bg-[var(--sf)] p-1">
            {(["24h", "7d", "30d"] as const).map((item) => (
              <Button
                key={item}
                type="button"
                size="sm"
                variant="ghost"
                className={cn(
                  "h-7 min-w-12 font-hud text-[11px]",
                  range === item && "bg-[var(--acd)] text-[var(--ac)]"
                )}
                onClick={() => setRange(item)}
              >
                {item}
              </Button>
            ))}
          </div>
          </>
        }
      />

      <div className="capcom-panel-surface flex flex-col gap-3 rounded-xl bg-[var(--el)] p-3 lg:flex-row lg:items-center">
        <div className="flex rounded-lg bg-[var(--sf)] p-1">
          {(["fleet", "runtime", "agent"] as const).map((item) => (
            <Button
              key={item}
              type="button"
              variant="ghost"
              size="sm"
              className={cn("font-hud text-[11px] capitalize", scope === item && "bg-[var(--acd)] text-[var(--ac)]")}
              onClick={() => setScope(item)}
            >
              {item === "runtime" ? "Runtime instance" : item}
            </Button>
          ))}
        </div>
        {scope !== "fleet" ? (
          <select
            value={effectiveRuntimeID}
            onChange={(event) => {
              setRuntimeID(event.target.value)
              setAgentID("")
            }}
            className="h-8 min-w-56 rounded-lg border border-[var(--hl)] bg-[var(--cv)] px-2.5 font-hud text-[11px] text-[var(--tx)] outline-none focus:border-[var(--ac)]"
            aria-label="Runtime instance"
          >
            <option value="">Select a runtime instance</option>
            {(instancesQuery.data ?? []).map((instance) => (
              <option key={instance.id} value={instance.id}>{instance.display_name || instance.name}</option>
            ))}
          </select>
        ) : null}
        {scope === "agent" ? (
          <select
            value={effectiveAgentID}
            onChange={(event) => setAgentID(event.target.value)}
            disabled={!effectiveRuntimeID}
            className="h-8 min-w-56 rounded-lg border border-[var(--hl)] bg-[var(--cv)] px-2.5 font-hud text-[11px] text-[var(--tx)] outline-none focus:border-[var(--ac)] disabled:opacity-50"
            aria-label="Agent"
          >
            <option value="">Select an agent</option>
            {scopedAgents.map((agent) => (
              <option key={agent.id} value={agent.id}>{agent.name}</option>
            ))}
          </select>
        ) : null}
        <span className="ml-auto font-hud text-[10px] text-[var(--fa)]">
          {scope === "fleet" ? "All runtimes" : scope === "runtime" ? "One runtime instance" : "One imported agent"}
        </span>
      </div>

      {query.isLoading ? (
        <MetricsSkeleton />
      ) : (
        <>
          <TelemetryStatus
            metrics={metrics}
            health={healthQuery.data}
          />
          <MetricRail metrics={metrics} />
          <div className="grid gap-4 xl:grid-cols-2">
            <ChartPanel
              title="Token flow"
              eyebrow="Input / output"
              summary={formatTokens(metrics?.usage.total_tokens ?? 0)}
              detail="total tokens"
              onOpen={() => setDetail("tokens")}
            >
              <SignalChart
                first={(metrics?.time_series ?? []).map((item) => item.input_tokens)}
                second={(metrics?.time_series ?? []).map((item) => item.output_tokens)}
                labels={(metrics?.time_series ?? []).map((item) => item.started_at)}
              />
              <ChartLegend
                items={[
                  ["Input", "var(--chart-1)"],
                  ["Output", "var(--chart-4)"],
                ]}
              />
            </ChartPanel>

            <ChartPanel
              title="Request volume"
              eyebrow="Throughput"
              summary={formatCount(metrics?.usage.requests ?? 0)}
              detail="requests"
              onOpen={() => setDetail("requests")}
            >
              <BarChart
                values={(metrics?.time_series ?? []).map(
                  (item) => item.request_count
                )}
                labels={(metrics?.time_series ?? []).map((item) => item.started_at)}
              />
              <ChartLegend items={[["Requests", "var(--chart-2)"]]} />
            </ChartPanel>

            <ChartPanel
              title="Model mix"
              eyebrow="Workload"
              summary={String(visibleModels(metrics).length)}
              detail="active models"
              className="xl:col-span-2"
              onOpen={() => setDetail("models")}
            >
              <ModelBars models={visibleModels(metrics)} />
            </ChartPanel>
          </div>
        </>
      )}

      <MetricDetailDialog
        kind={detail}
        metrics={metrics}
        open={detail !== null}
        onOpenChange={(open) => {
          if (!open) setDetail(null)
        }}
      />
    </section>
  )
}

function TelemetryStatus({
  metrics,
  health,
}: {
  metrics?: MetricSummary
  health?: TelemetryHealth[]
}) {
  if (metrics?.available) {
    return (
      <div className="flex flex-wrap items-center gap-2 font-hud text-[11px] text-[var(--mu)]">
        <span className="size-1.5 rounded-full bg-[var(--ac)] shadow-[0_0_0_3px_var(--acd)]" />
        Live telemetry
        {metrics.source && <Badge variant="outline">{sourceLabel(metrics.source)}</Badge>}
        <span className="text-[var(--fa)]">
          last observed {metrics.last_observed_at ? relativeTime(metrics.last_observed_at) : "never"}
        </span>
      </div>
    )
  }
  const failed = health?.find((item) => item.status === "failed")
  return (
    <div className="flex items-start gap-3 rounded-xl border border-[var(--wnd)] bg-[var(--wnd)]/30 px-4 py-3">
      <Radio className="mt-0.5 size-4 shrink-0 text-[var(--wn)]" />
      <div>
        <p className="text-sm font-medium text-[var(--tx)]">
          Telemetry {metrics?.configured ? "temporarily unavailable" : "not configured"}
        </p>
        <p className="mt-0.5 text-xs text-[var(--mu)]">
          {failed?.message
            ? `${failed.runtime_display_name ?? "Runtime"}: ${failed.message}`
            : metrics?.configured
              ? "Capcom is preserving the last known observations while the collector recovers."
            : "Connect Gantry usage, LangSmith, or OTLP telemetry to populate these charts."}
        </p>
      </div>
    </div>
  )
}

function MetricRail({ metrics }: { metrics?: MetricSummary }) {
  const items = [
    { label: "Requests", value: formatCount(metrics?.usage.requests ?? 0), icon: Activity },
    { label: "Total tokens", value: formatTokens(metrics?.usage.total_tokens ?? 0), icon: Gauge },
    { label: "Estimated cost", value: formatCost(metrics?.usage.estimated_cost_usd), icon: Coins },
    { label: "P95 latency", value: formatDuration(metrics?.performance.p95_duration_ms), icon: Clock3 },
  ]
  return (
    <div className="grid divide-y divide-[var(--sl)] rounded-xl border border-[var(--hl)] bg-[var(--el)] shadow-[var(--chi)] sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-4">
      {items.map((item) => (
        <div key={item.label} className="flex items-center gap-3 px-4 py-4">
          <item.icon className="size-4 text-[var(--fa)]" />
          <div>
            <div className="font-hud text-[10px] uppercase tracking-[0.12em] text-[var(--fa)]">
              {item.label}
            </div>
            <div className="mt-0.5 font-hud text-lg font-semibold tabular text-[var(--tx)]">
              {item.value}
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function ChartPanel({
  title,
  eyebrow,
  summary,
  detail,
  children,
  className,
  onOpen,
}: {
  title: string
  eyebrow: string
  summary: string
  detail: string
  children: React.ReactNode
  className?: string
  onOpen: () => void
}) {
  return (
    <button
      type="button"
      onClick={onOpen}
      className={cn(
        "group min-h-[292px] rounded-xl border border-[var(--hl)] bg-[var(--el)] p-4 text-left shadow-[var(--chi)] transition hover:-translate-y-0.5 hover:border-[var(--ac)] focus-visible:ring-2 focus-visible:ring-[var(--ac)]",
        className
      )}
    >
      <div className="mb-5 flex items-start justify-between gap-4">
        <div>
          <div className="font-hud text-[10px] uppercase tracking-[0.14em] text-[var(--fa)]">
            {eyebrow}
          </div>
          <h2 className="mt-1 text-sm font-semibold text-[var(--tx)]">{title}</h2>
        </div>
        <div className="flex items-start gap-2 text-right">
          <div>
            <div className="font-hud text-xl font-semibold tabular text-[var(--tx)]">{summary}</div>
            <div className="font-hud text-[10px] text-[var(--fa)]">{detail}</div>
          </div>
          <ArrowUpRight className="size-4 text-[var(--fa)] transition group-hover:text-[var(--ac)]" />
        </div>
      </div>
      {children}
    </button>
  )
}

function SignalChart({
  first,
  second,
  labels,
  expanded = false,
}: {
  first: number[]
  second: number[]
  labels: string[]
  expanded?: boolean
}) {
  const width = 760
  const height = expanded ? 260 : 170
  const values = [...first, ...second]
  if (!values.length) return <EmptyChart expanded={expanded} />
  const max = Math.max(...values, 1)
  const points = (series: number[]) =>
    series
      .map((value, index) => {
        const x = series.length === 1 ? width / 2 : (index / (series.length - 1)) * width
        const y = height - 12 - (value / max) * (height - 28)
        return `${x},${y}`
      })
      .join(" ")
  return (
    <div>
      <svg viewBox={`0 0 ${width} ${height}`} className={cn("w-full", expanded ? "h-[260px]" : "h-[170px]")} role="img" aria-label="Input and output token trend">
        {[0.25, 0.5, 0.75].map((fraction) => (
          <line key={fraction} x1="0" x2={width} y1={height * fraction} y2={height * fraction} stroke="var(--sl)" strokeWidth="1" />
        ))}
        <polyline points={points(first)} fill="none" stroke="var(--chart-1)" strokeWidth="3" strokeLinejoin="round" strokeLinecap="round" />
        <polyline points={points(second)} fill="none" stroke="var(--chart-4)" strokeWidth="3" strokeLinejoin="round" strokeLinecap="round" />
      </svg>
      <AxisLabels labels={labels} />
    </div>
  )
}

function BarChart({
  values,
  labels,
  expanded = false,
}: {
  values: number[]
  labels: string[]
  expanded?: boolean
}) {
  if (!values.length) return <EmptyChart expanded={expanded} />
  const max = Math.max(...values, 1)
  return (
    <div>
      <div className={cn("flex items-end gap-1", expanded ? "h-[260px]" : "h-[170px]")}>
        {values.map((value, index) => (
          <div
            key={`${labels[index]}-${index}`}
            className="min-w-0 flex-1 rounded-t-sm bg-[var(--chart-2)] opacity-80 transition hover:opacity-100"
            style={{ height: `${Math.max((value / max) * 100, 2)}%` }}
            title={`${formatDate(labels[index])}: ${formatCount(value)} requests`}
          />
        ))}
      </div>
      <AxisLabels labels={labels} />
    </div>
  )
}

function ModelBars({ models }: { models: NonNullable<MetricSummary["models"]> }) {
  if (!models.length) return <EmptyChart />
  const max = Math.max(...models.map((model) => model.total_tokens), 1)
  return (
    <div className="grid gap-3">
      {models.slice(0, 6).map((model) => (
        <div key={model.model} className="grid items-center gap-2 sm:grid-cols-[180px_1fr_100px]">
          <span className="truncate font-hud text-[11px] text-[var(--mu)]">{model.model}</span>
          <div className="h-2 overflow-hidden rounded-full bg-[var(--sl)]">
            <div className="h-full rounded-full bg-[var(--chart-1)]" style={{ width: `${(model.total_tokens / max) * 100}%` }} />
          </div>
          <span className="text-right font-hud text-[11px] tabular text-[var(--fa)]">{formatTokens(model.total_tokens)}</span>
        </div>
      ))}
    </div>
  )
}

function MetricDetailDialog({
  kind,
  metrics,
  open,
  onOpenChange,
}: {
  kind: DetailKind | null
  metrics?: MetricSummary
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const title = kind === "tokens" ? "Token flow details" : kind === "requests" ? "Request volume details" : "Model usage details"
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto border border-[var(--hl)] bg-[var(--el)] shadow-[var(--shdw)] sm:max-w-[900px]">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {formatDate(metrics?.period.from)} – {formatDate(metrics?.period.to)}
            {metrics?.source ? ` · ${sourceLabel(metrics.source)}` : ""}
          </DialogDescription>
        </DialogHeader>
        {kind === "tokens" && (
          <>
            <SignalChart
              first={(metrics?.time_series ?? []).map((item) => item.input_tokens)}
              second={(metrics?.time_series ?? []).map((item) => item.output_tokens)}
              labels={(metrics?.time_series ?? []).map((item) => item.started_at)}
              expanded
            />
            <DetailTable metrics={metrics} />
          </>
        )}
        {kind === "requests" && (
          <>
            <BarChart
              values={(metrics?.time_series ?? []).map((item) => item.request_count)}
              labels={(metrics?.time_series ?? []).map((item) => item.started_at)}
              expanded
            />
            <DetailTable metrics={metrics} />
          </>
        )}
        {kind === "models" && <ModelDetailTable metrics={metrics} />}
      </DialogContent>
    </Dialog>
  )
}

function DetailTable({ metrics }: { metrics?: MetricSummary }) {
  return (
    <div className="overflow-x-auto rounded-lg border border-[var(--sl)]">
      <table className="w-full text-left font-hud text-[11px]">
        <thead className="bg-[var(--sf)] text-[var(--fa)]">
          <tr>
            {["Bucket", "Requests", "Input", "Output", "Total"].map((label) => <th key={label} className="px-3 py-2 font-medium">{label}</th>)}
          </tr>
        </thead>
        <tbody className="divide-y divide-[var(--sl)] text-[var(--mu)]">
          {(metrics?.time_series ?? []).map((bucket) => (
            <tr key={bucket.started_at}>
              <td className="px-3 py-2 text-[var(--tx)]">{formatDate(bucket.started_at)}</td>
              <td className="px-3 py-2 tabular">{formatCount(bucket.request_count)}</td>
              <td className="px-3 py-2 tabular">{formatTokens(bucket.input_tokens)}</td>
              <td className="px-3 py-2 tabular">{formatTokens(bucket.output_tokens)}</td>
              <td className="px-3 py-2 tabular">{formatTokens(bucket.total_tokens)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function ModelDetailTable({ metrics }: { metrics?: MetricSummary }) {
  return (
    <div className="overflow-x-auto rounded-lg border border-[var(--sl)]">
      <table className="w-full text-left font-hud text-[11px]">
        <thead className="bg-[var(--sf)] text-[var(--fa)]">
          <tr>
            {["Model", "Requests", "Input", "Output", "Total"].map((label) => <th key={label} className="px-3 py-2 font-medium">{label}</th>)}
          </tr>
        </thead>
        <tbody className="divide-y divide-[var(--sl)] text-[var(--mu)]">
          {visibleModels(metrics).map((model) => (
            <tr key={model.model}>
              <td className="px-3 py-2 text-[var(--tx)]">{model.model}</td>
              <td className="px-3 py-2 tabular">{formatCount(model.request_count)}</td>
              <td className="px-3 py-2 tabular">{formatTokens(model.input_tokens)}</td>
              <td className="px-3 py-2 tabular">{formatTokens(model.output_tokens)}</td>
              <td className="px-3 py-2 tabular">{formatTokens(model.total_tokens)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function visibleModels(metrics?: MetricSummary) {
  return (metrics?.models ?? []).filter(
    (model) => model.model.trim() !== "" && model.request_count > 0
  )
}

function ChartLegend({ items }: { items: Array<[string, string]> }) {
  return (
    <div className="mt-3 flex gap-4 font-hud text-[10px] text-[var(--fa)]">
      {items.map(([label, color]) => (
        <span key={label} className="flex items-center gap-1.5">
          <span className="size-1.5 rounded-full" style={{ background: color }} />
          {label}
        </span>
      ))}
    </div>
  )
}

function AxisLabels({ labels }: { labels: string[] }) {
  if (!labels.length) return null
  return (
    <div className="mt-2 flex justify-between font-hud text-[9px] text-[var(--fa)]">
      <span>{formatDate(labels[0])}</span>
      <span>{formatDate(labels[labels.length - 1])}</span>
    </div>
  )
}

function EmptyChart({ expanded = false }: { expanded?: boolean }) {
  return (
    <div className={cn("flex items-center justify-center rounded-lg border border-dashed border-[var(--hl)] bg-[var(--sf)] font-hud text-[11px] text-[var(--fa)]", expanded ? "h-[260px]" : "h-[170px]")}>
      Waiting for telemetry observations
    </div>
  )
}

function MetricsSkeleton() {
  return (
    <div className="grid gap-4">
      <Skeleton className="h-16" />
      <Skeleton className="h-20" />
      <div className="grid gap-4 xl:grid-cols-2">
        <Skeleton className="h-[292px]" />
        <Skeleton className="h-[292px]" />
      </div>
    </div>
  )
}

function formatCount(value: number) {
  return new Intl.NumberFormat("en-US", { notation: value >= 10_000 ? "compact" : "standard", maximumFractionDigits: 1 }).format(value)
}

function formatTokens(value: number) {
  return `${formatCount(value)} tok`
}

function formatCost(value?: number | null) {
  return value == null ? "—" : `$${value.toFixed(value < 1 ? 4 : 2)}`
}

function formatDuration(value?: number | null) {
  if (value == null) return "—"
  return value >= 1000 ? `${(value / 1000).toFixed(1)}s` : `${Math.round(value)}ms`
}

function formatDate(value?: string) {
  if (!value) return "—"
  const date = new Date(value)
  return new Intl.DateTimeFormat("en", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(date)
}

function relativeTime(value: string) {
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000))
  if (seconds < 60) return `${seconds}s ago`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ago`
  return `${Math.floor(seconds / 86400)}d ago`
}

function sourceLabel(source: string) {
  return { gantry_native: "Gantry", langsmith: "LangSmith", otel: "OpenTelemetry" }[source] ?? source
}

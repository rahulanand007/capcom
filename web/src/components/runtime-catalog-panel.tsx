"use client"

import { Activity, Boxes, ShieldCheck } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  useRuntimeInstanceCapabilitiesQuery,
  useRuntimeInstanceDiagnosticsQuery,
  useRuntimeInstanceInventoryQuery,
} from "@/lib/api-hooks"
import type {
  RuntimeCapability,
  RuntimeDiagnostic,
  RuntimeInventoryItem,
} from "@/lib/api-types"
import { relativeTime } from "@/lib/adapters"
import { cn } from "@/lib/utils"

export function RuntimeCatalogPanel({
  runtimeId,
  view = "all",
}: {
  runtimeId: string
  view?: "all" | "health" | "capabilities"
}) {
  const diagnosticsQuery = useRuntimeInstanceDiagnosticsQuery(runtimeId)
  const inventoryQuery = useRuntimeInstanceInventoryQuery(runtimeId)
  const capabilitiesQuery = useRuntimeInstanceCapabilitiesQuery(runtimeId)
  const diagnostics = diagnosticsQuery.data ?? []
  const inventory = inventoryQuery.data ?? []
  const capabilities = capabilitiesQuery.data ?? []

  return (
    <section className="border-t border-[var(--sl)]">
      <div className="flex flex-wrap items-center justify-between gap-3 px-[18px] py-3">
        <div>
          <div className="capcom-eyebrow">Runtime catalog</div>
          <h3 className="text-[14px] font-semibold text-[var(--tx)]">
            {view === "health"
              ? "Runtime health and doctor checks"
              : view === "capabilities"
                ? "Inventory and supported controls"
                : "Diagnostics, inventory, and capabilities"}
          </h3>
        </div>
        <div className="flex flex-wrap items-center gap-2 font-hud text-[11px] text-[var(--fa)]">
          <span>{diagnostics.length} checks</span>
          <span>/</span>
          <span>{inventory.length} inventory</span>
          <span>/</span>
          <span>{capabilities.length} capabilities</span>
        </div>
      </div>

      {view !== "capabilities" ? <div className="border-t border-[var(--sl)] px-[18px] py-3">
        <div className="mb-2 flex items-center gap-2">
          <Activity className="size-4 text-[var(--fa)]" />
          <span className="capcom-eyebrow">Doctor checks</span>
        </div>
        {diagnosticsQuery.isLoading ? (
          <Skeleton className="h-12 w-full" />
        ) : diagnostics.length ? (
          <div className="grid gap-2 md:grid-cols-2">
            {diagnostics.map((item) => (
              <DiagnosticRow key={item.id} item={item} />
            ))}
          </div>
        ) : (
          <p className="text-[12px] text-[var(--mu)]">No doctor checks imported.</p>
        )}
      </div> : null}

      {view !== "health" ? <Tabs defaultValue="inventory" className="gap-0 border-t border-[var(--sl)]">
        <div className="px-[18px] py-2">
          <TabsList variant="line">
            <TabsTrigger value="inventory" className="font-hud text-[12px]">
              <Boxes data-icon="inline-start" /> Inventory
            </TabsTrigger>
            <TabsTrigger value="capabilities" className="font-hud text-[12px]">
              <ShieldCheck data-icon="inline-start" /> Capabilities
            </TabsTrigger>
          </TabsList>
        </div>
        <TabsContent value="inventory">
          <InventoryTable loading={inventoryQuery.isLoading} items={inventory} />
        </TabsContent>
        <TabsContent value="capabilities">
          <CapabilityTable loading={capabilitiesQuery.isLoading} items={capabilities} />
        </TabsContent>
      </Tabs> : null}
    </section>
  )
}

function DiagnosticRow({ item }: { item: RuntimeDiagnostic }) {
  const status = item.status.toLowerCase()
  const failed = status.includes("fail") || status.includes("error")
  const warned = status.includes("warn") || status.includes("degraded")
  return (
    <div className="flex min-w-0 items-center gap-3 border-l-2 border-[var(--sl)] py-1 pl-3">
      <span
        className={cn(
          "size-2 shrink-0 rounded-full",
          failed ? "bg-[var(--dg)]" : warned ? "bg-[var(--wn)]" : "bg-[var(--ac)]"
        )}
      />
      <div className="min-w-0">
        <div className="font-hud text-[12px] font-semibold text-[var(--tx)]">
          {item.check_id}
        </div>
        <div className="truncate text-[12px] text-[var(--mu)]" title={item.message}>
          {item.message || status}
        </div>
      </div>
      <Badge
        className={cn(
          "ml-auto font-hud text-[10px]",
          failed
            ? "bg-[var(--dgd)] text-[var(--dg)]"
            : warned
              ? "bg-[var(--wnd)] text-[var(--wn)]"
              : "bg-[var(--acd)] text-[var(--ac)]"
        )}
      >
        {status}
      </Badge>
    </div>
  )
}

function InventoryTable({ loading, items }: { loading: boolean; items: RuntimeInventoryItem[] }) {
  return (
    <div className="max-h-[320px] overflow-auto">
      <Table className="min-w-[680px] table-fixed">
        <TableHeader>
          <TableRow className="border-[var(--sl)] hover:bg-transparent">
            <TableHead className="capcom-eyebrow w-[34%] px-[18px]">Resource</TableHead>
            <TableHead className="capcom-eyebrow w-[14%] px-[18px]">Kind</TableHead>
            <TableHead className="capcom-eyebrow w-[18%] px-[18px]">Status</TableHead>
            <TableHead className="capcom-eyebrow w-[18%] px-[18px]">Provider</TableHead>
            <TableHead className="capcom-eyebrow w-[16%] px-[18px]">Observed</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? <CatalogSkeleton columns={5} /> : items.length ? items.map((item) => (
            <TableRow key={item.id} className="border-[var(--sl)] hover:bg-[var(--sl)]">
              <TableCell className="px-[18px] py-2.5">
                <div className="truncate font-hud text-[12px] text-[var(--tx)]">{item.name}</div>
                <div className="truncate font-hud text-[10px] text-[var(--fa)]">{item.runtime_item_id}</div>
              </TableCell>
              <TableCell className="px-[18px] py-2.5 font-hud text-[11px] text-[var(--mu)]">{item.kind.replaceAll("_", " ")}</TableCell>
              <TableCell className="px-[18px] py-2.5 font-hud text-[11px] text-[var(--mu)]">{item.status || "unknown"}</TableCell>
              <TableCell className="truncate px-[18px] py-2.5 text-[12px] text-[var(--mu)]">{item.provider || item.source || "Gantry"}</TableCell>
              <TableCell className="px-[18px] py-2.5 font-hud text-[11px] text-[var(--fa)]">{relativeTime(item.observed_at)}</TableCell>
            </TableRow>
          )) : <EmptyCatalogRow columns={5} label="No inventory imported." />}
        </TableBody>
      </Table>
    </div>
  )
}

function CapabilityTable({ loading, items }: { loading: boolean; items: RuntimeCapability[] }) {
  return (
    <div className="max-h-[320px] overflow-auto">
      <Table className="min-w-[760px] table-fixed">
        <TableHeader>
          <TableRow className="border-[var(--sl)] hover:bg-transparent">
            <TableHead className="capcom-eyebrow w-[28%] px-[18px]">Capability</TableHead>
            <TableHead className="capcom-eyebrow w-[14%] px-[18px]">Category</TableHead>
            <TableHead className="capcom-eyebrow w-[12%] px-[18px]">Risk</TableHead>
            <TableHead className="capcom-eyebrow w-[36%] px-[18px]">Can</TableHead>
            <TableHead className="capcom-eyebrow w-[10%] px-[18px]">Version</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? <CatalogSkeleton columns={5} /> : items.length ? items.map((item) => (
            <TableRow key={item.id} className="border-[var(--sl)] hover:bg-[var(--sl)]">
              <TableCell className="px-[18px] py-2.5">
                <div className="truncate font-hud text-[12px] text-[var(--tx)]">{item.name || item.runtime_capability_id}</div>
                <div className="truncate font-hud text-[10px] text-[var(--fa)]">{item.runtime_capability_id}</div>
              </TableCell>
              <TableCell className="px-[18px] py-2.5 text-[12px] text-[var(--mu)]">{item.category || "uncategorized"}</TableCell>
              <TableCell className="px-[18px] py-2.5"><RiskBadge risk={item.risk} /></TableCell>
              <TableCell className="truncate px-[18px] py-2.5 text-[12px] text-[var(--mu)]" title={item.can}>{item.can || "No description"}</TableCell>
              <TableCell className="px-[18px] py-2.5 font-hud text-[11px] text-[var(--fa)]">{item.version}</TableCell>
            </TableRow>
          )) : <EmptyCatalogRow columns={5} label="No approved capabilities imported." />}
        </TableBody>
      </Table>
    </div>
  )
}

function RiskBadge({ risk }: { risk: string }) {
  const value = risk || "unknown"
  const high = /write|high|critical/i.test(value)
  return <Badge className={cn("font-hud text-[10px]", high ? "bg-[var(--wnd)] text-[var(--wn)]" : "bg-[var(--sl)] text-[var(--fa)]")}>{value}</Badge>
}

function CatalogSkeleton({ columns }: { columns: number }) {
  return <>{[0, 1].map((row) => <TableRow key={row}>{Array.from({ length: columns }).map((_, column) => <TableCell key={column} className="px-[18px] py-3"><Skeleton className="h-4 w-full" /></TableCell>)}</TableRow>)}</>
}

function EmptyCatalogRow({ columns, label }: { columns: number; label: string }) {
  return <TableRow><TableCell colSpan={columns} className="px-[18px] py-8 text-center text-[12px] text-[var(--mu)]">{label}</TableCell></TableRow>
}

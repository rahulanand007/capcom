"use client"

import * as React from "react"
import Link from "next/link"
import { usePathname, useRouter } from "next/navigation"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useTheme } from "next-themes"
import { toast } from "sonner"
import {
  Command,
  CommandDialog,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/components/ui/command"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import { Separator } from "@/components/ui/separator"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { buildAdaptersModel, deriveWorstStatus, statusClass, statusLabel } from "@/lib/adapters"
import { capcomApi } from "@/lib/api-client"
import { queryKeys, useHealthQuery, usePersistedAgentsQuery, useRuntimeInstancesQuery } from "@/lib/api-hooks"
import type { RuntimeSyncRun } from "@/lib/api-types"
import { cn } from "@/lib/utils"

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname()
  const router = useRouter()
  const queryClient = useQueryClient()
  const [paletteOpen, setPaletteOpen] = React.useState(false)
  const [adaptersOpen, setAdaptersOpen] = React.useState(false)
  const [lastUpdate, setLastUpdate] = React.useState<Date | null>(null)
  const healthQuery = useHealthQuery()
  const runtimeInstancesQuery = useRuntimeInstancesQuery()
  const agentsQuery = usePersistedAgentsQuery()
  const now = React.useMemo(
    () =>
      new Date(
        Math.max(
          lastUpdate?.getTime() ?? 0,
          healthQuery.dataUpdatedAt,
          runtimeInstancesQuery.dataUpdatedAt,
          agentsQuery.dataUpdatedAt,
          0
        )
      ),
    [
      agentsQuery.dataUpdatedAt,
      healthQuery.dataUpdatedAt,
      lastUpdate,
      runtimeInstancesQuery.dataUpdatedAt,
    ]
  )
  const adaptersModel = React.useMemo(
    () =>
      buildAdaptersModel(
        runtimeInstancesQuery.data ?? [],
        agentsQuery.data ?? [],
        now
      ),
    [agentsQuery.data, now, runtimeInstancesQuery.data]
  )

  const adapterMatch = pathname.match(/^\/adapters\/([^/]+)/)
  const currentAdapterId = adapterMatch?.[1]
    ? decodeURIComponent(adapterMatch[1])
    : undefined
  const isAdapterRoute = Boolean(currentAdapterId)
  const currentAdapterName =
    adaptersModel.adapters.find((adapter) => adapter.id === currentAdapterId)
      ?.name ?? currentAdapterId
  const effectiveAdaptersOpen = isAdapterRoute || adaptersOpen
  const healthFailed = healthQuery.data?.status !== "ok" && !healthQuery.isLoading
  const worstStatus = deriveWorstStatus([
    healthFailed ? "failed" : "ok",
    ...adaptersModel.adapters.map((adapter) => adapter.status),
  ])
  const statusStyles = statusClass(worstStatus)
  const systemText = healthFailed
    ? "API health check failed"
    : adaptersModel.attention.length
      ? `${adaptersModel.attention.length} item${adaptersModel.attention.length === 1 ? "" : "s"} need attention`
      : statusLabel(worstStatus)
  const agentsCount = agentsQuery.data?.length ?? 0
  const needsAttentionInstanceIds = React.useMemo(
    () =>
      Array.from(
        new Set(adaptersModel.attention.map((item) => item.instanceId))
      ),
    [adaptersModel.attention]
  )
  const paletteSyncMutation = useMutation<RuntimeSyncRun[]>({
    mutationFn: async () => {
      if (!needsAttentionInstanceIds.length) {
        return []
      }
      return Promise.all(
        needsAttentionInstanceIds.map((id) =>
          capcomApi.syncRuntimeInstance(id, {
            actor: "local-operator",
            reason: "Command palette re-import of stale or failed instances",
          })
        )
      )
    },
    onSuccess: async (runs) => {
      await invalidateRuntimeState(runs.map((run) => run.runtime_connection_id))
      setLastUpdate(new Date())
      if (runs.length) {
        toast.success(
          `${runs.length} instance${runs.length === 1 ? "" : "s"} queued for re-import`
        )
      } else {
        toast.info("No stale or failed instances to re-import")
      }
    },
  })

  React.useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault()
        setPaletteOpen((open) => !open)
      }

      if (event.key === "Escape") {
        setPaletteOpen(false)
      }
    }

    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])

  async function refreshAll() {
    await invalidateRuntimeState()
    await Promise.all([
      queryClient.refetchQueries({ queryKey: queryKeys.health }),
      queryClient.refetchQueries({ queryKey: queryKeys.runtimeInstances }),
      queryClient.refetchQueries({ queryKey: queryKeys.persistedAgents() }),
      queryClient.refetchQueries({ queryKey: queryKeys.subagentExecutions() }),
    ])
    setLastUpdate(new Date())
    toast.success("Adapter data refreshed")
  }

  async function invalidateRuntimeState(runtimeConnectionIds: string[] = []) {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: queryKeys.health }),
      queryClient.invalidateQueries({ queryKey: queryKeys.runtimeInstances }),
      queryClient.invalidateQueries({ queryKey: queryKeys.persistedAgents() }),
      queryClient.invalidateQueries({ queryKey: queryKeys.subagentExecutions() }),
      ...runtimeConnectionIds.flatMap((id) => [
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstance(id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceAgents(id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceSyncRuns(id),
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.runtimeInstanceSubagentExecutions(id),
        }),
      ]),
    ])
  }

  function openPalettePath(nextPath: string) {
    setPaletteOpen(false)
    router.push(nextPath)
  }

  function reimportNeedsAttention() {
    setPaletteOpen(false)
    paletteSyncMutation.mutate()
  }

  return (
    <div className="capcom-shell">
      <aside className="capcom-sidebar">
        <SidebarHeader />
        <nav className="mt-8 flex flex-col gap-1">
          <SidebarLink href="/" active={pathname === "/"}>
            Overview
          </SidebarLink>

          <Collapsible
            open={effectiveAdaptersOpen}
            onOpenChange={(open) => {
              if (!isAdapterRoute) {
                setAdaptersOpen(open)
              }
            }}
          >
            <CollapsibleTrigger className="capcom-eyebrow flex w-full cursor-pointer items-center gap-1.5 rounded-md px-2.5 pb-1 pt-2.5 hover:text-[var(--mu)]">
              <span className="text-[10px]">
                {effectiveAdaptersOpen ? "v" : ">"}
              </span>
              <span>Adapters</span>
              <span className="ml-auto font-hud tabular">
                {adaptersModel.adapters.length}
              </span>
            </CollapsibleTrigger>
            <CollapsibleContent className="flex flex-col gap-1 pb-1">
              {adaptersModel.adapters.map((adapter) => {
                const styles = statusClass(adapter.status)
                return (
                  <Link
                    key={adapter.id}
                    href={`/adapters/${adapter.id}`}
                    className="capcom-nav-item pl-4"
                    data-active={currentAdapterId === adapter.id}
                  >
                    <span className={cn("h-1.5 w-1.5 rounded-full", styles.dot)} />
                    <span>{adapter.name}</span>
                    <span className="ml-auto font-hud text-[11px] tabular text-[var(--fa)]">
                      {adapter.instanceCount}
                    </span>
                  </Link>
                )
              })}
              {!adaptersModel.adapters.length && (
                <div className="px-4 py-2 font-hud text-[11px] text-[var(--fa)]">
                  none connected
                </div>
              )}
            </CollapsibleContent>
          </Collapsible>

          <SidebarLink href="/agents" active={pathname === "/agents"}>
            <span>Agents</span>
            <span className="ml-auto font-hud text-[11px] tabular text-[var(--fa)]">
              {agentsCount}
            </span>
          </SidebarLink>

          <SidebarLink href="/metrics" active={pathname === "/metrics"}>
            Metrics
          </SidebarLink>
        </nav>

        <div className="mt-auto">
          <Separator className="mb-4 bg-[var(--sl)]" />
          <div className={cn("flex items-center gap-2 font-hud text-[11px]", statusStyles.text)}>
            <span className={cn("h-1.5 w-1.5 rounded-full", statusStyles.dot)} />
            {systemText}
          </div>
          <div className="mt-2 font-hud text-[11px] text-[var(--fa)]">
            v{healthQuery.data?.version ?? "unknown"} / go/capcom
          </div>
        </div>
      </aside>

      <div className="capcom-main">
        <header className="capcom-topbar">
          <div className="min-w-0">
            {isAdapterRoute ? (
              <div className="flex min-w-0 items-center gap-2 text-sm">
                <Link href="/" className="text-[var(--fa)] hover:text-[var(--tx)]">
                  Adapters
                </Link>
                <span className="font-hud text-[12px] text-[var(--fa)]">&gt;</span>
                <span className="truncate font-medium text-[var(--tx)]">
                  {currentAdapterName}
                </span>
              </div>
            ) : (
              <button className="capcom-chip" type="button">
                <span className="text-[var(--fa)]">env</span>
                <span>All environments</span>
                <span>v</span>
              </button>
            )}
          </div>

          <div className="flex shrink-0 items-center gap-3">
            <span className="hidden font-hud text-xs text-[var(--fa)] sm:inline">
              last update {lastUpdate ? "just now" : "pending"}
            </span>
            <Button
              variant="outline"
              size="sm"
              className="font-hud text-xs hover:border-[var(--ac)] hover:text-[var(--ac)]"
              onClick={() => void refreshAll()}
            >
              Refresh now
            </Button>
            <Tooltip>
              <TooltipTrigger
                render={
                  <button
                    type="button"
                    className="capcom-chip px-2.5"
                    onClick={() => setPaletteOpen(true)}
                    aria-label="Open command palette"
                  >
                    CmdK
                  </button>
                }
              />
              <TooltipContent>Open command palette</TooltipContent>
            </Tooltip>
            <ThemeToggle />
          </div>
        </header>

        <main className="capcom-content">
          <div className="capcom-view">{children}</div>
        </main>
      </div>

      <CommandDialog
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        className="top-[110px] w-[min(600px,calc(100vw-32px))] max-w-none translate-y-0 border border-[var(--hl)] bg-[var(--el)] p-0 shadow-[var(--shdw)]"
        title="Command Palette"
        description="Run Capcom console commands"
      >
        <Command className="rounded-xl bg-[var(--el)] p-0">
          <div className="flex items-center gap-2 border-b border-[var(--sl)] px-3 py-2">
            <span className="font-hud text-[var(--fa)]">&gt;</span>
            <CommandInput
              placeholder="Type a command..."
              className="font-sans text-[13px]"
            />
            <button
              type="button"
              className="rounded-[var(--radius-kbd)] border border-[var(--hl)] px-1.5 py-1 font-hud text-[11px] text-[var(--fa)]"
              onClick={() => setPaletteOpen(false)}
            >
              esc
            </button>
          </div>
          <CommandList className="max-h-[320px] p-2">
            <CommandGroup
              heading="Actions"
              className="font-hud text-[11px] uppercase tracking-[0.08em] text-[var(--fa)]"
            >
              <CommandItem
                disabled={paletteSyncMutation.isPending}
                onSelect={reimportNeedsAttention}
                className="font-sans text-[13px]"
              >
                <span className="font-hud text-[var(--fa)]">Refresh</span>
                <span className="font-medium">
                  Re-import all stale or failed instances
                </span>
                <CommandShortcut>
                  {adaptersModel.attention.length
                    ? `${adaptersModel.attention.length} pending`
                    : "none"}
                </CommandShortcut>
              </CommandItem>
              {adaptersModel.adapters.map((adapter) => (
                <CommandItem
                  key={adapter.id}
                  onSelect={() => openPalettePath(`/adapters/${adapter.id}`)}
                  className="font-sans text-[13px]"
                >
                  <span className="font-hud text-[var(--fa)]">Open</span>
                  <span>{adapter.name} adapter</span>
                </CommandItem>
              ))}
              <CommandItem
                onSelect={() => {
                  setPaletteOpen(false)
                  void refreshAll()
                }}
                className="font-sans text-[13px]"
              >
                <span className="font-hud text-[var(--fa)]">Refresh</span>
                <span>Refresh all adapters now</span>
              </CommandItem>
            </CommandGroup>
          </CommandList>
          <div className="border-t border-[var(--sl)] px-3 py-2 font-hud text-[11px] text-[var(--fa)]">
            enter run / esc close / capcom CmdK
          </div>
        </Command>
      </CommandDialog>
    </div>
  )
}

function SidebarHeader() {
  return (
    <div>
      <div className="flex items-center gap-2">
        <span className="capcom-status-dot" />
        <span className="font-hud text-sm font-semibold tracking-[0.14em]">
          CAPCOM
        </span>
      </div>
      <div className="mt-1 font-hud text-[11px] text-[var(--fa)]">
        control plane
      </div>
    </div>
  )
}

function SidebarLink({
  href,
  active,
  children,
}: {
  href: string
  active: boolean
  children: React.ReactNode
}) {
  return (
    <Link href={href} className="capcom-nav-item" data-active={active}>
      {children}
    </Link>
  )
}

function ThemeToggle() {
  const { theme, resolvedTheme, setTheme } = useTheme()
  const isLight = theme === "light" || resolvedTheme === "light"

  return (
    <button
      type="button"
      className="capcom-chip"
      onClick={() => setTheme(isLight ? "dark" : "light")}
      aria-label="Toggle theme"
    >
      {isLight ? "light" : "dark"}
    </button>
  )
}

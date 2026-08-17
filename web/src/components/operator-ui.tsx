"use client"

import * as React from "react"
import { AlertTriangle, ChevronDown, ChevronRight } from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Collapsible,
  CollapsibleContent,
} from "@/components/ui/collapsible"
import type {
  DerivedStatus,
  RuntimeHealth,
  SyncHealth,
} from "@/lib/adapters"
import { cn } from "@/lib/utils"

export function PageHeader({
  eyebrow,
  title,
  description,
  badge,
  actions,
}: {
  eyebrow?: string
  title: string
  description: React.ReactNode
  badge?: React.ReactNode
  actions?: React.ReactNode
}) {
  return (
    <header className="flex flex-col gap-3 border-b border-[var(--sl)] pb-4 lg:flex-row lg:items-end lg:justify-between">
      <div className="min-w-0">
        {eyebrow ? <div className="capcom-eyebrow">{eyebrow}</div> : null}
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <h1 className="text-[22px] font-bold leading-tight tracking-[-0.02em] text-[var(--tx)]">
            {title}
          </h1>
          {badge}
        </div>
        <div className="mt-1 max-w-3xl text-[13px] text-[var(--mu)]">
          {description}
        </div>
      </div>
      {actions ? <div className="flex shrink-0 flex-wrap gap-2">{actions}</div> : null}
    </header>
  )
}

export function RuntimeSyncStatus({
  runtime,
  sync,
}: {
  runtime: RuntimeHealth
  sync: SyncHealth
}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5" aria-label={`Runtime ${runtime}; sync ${sync}`}>
      <StatusBadge label={`runtime ${runtime}`} status={runtimeStatus(runtime)} />
      <StatusBadge label={`sync ${sync}`} status={syncStatus(sync)} />
    </div>
  )
}

function StatusBadge({ label, status }: { label: string; status: DerivedStatus | "unknown" }) {
  const classes =
    status === "ok"
      ? "border-[color-mix(in_srgb,var(--ac)_28%,var(--hl))] bg-[var(--acd)] text-[var(--ac)]"
      : status === "failed"
        ? "border-[color-mix(in_srgb,var(--dg)_30%,var(--hl))] bg-[var(--dgd)] text-[var(--dg)]"
        : status === "stale"
          ? "border-[color-mix(in_srgb,var(--wn)_30%,var(--hl))] bg-[var(--wnd)] text-[var(--wn)]"
          : "border-[var(--hl)] bg-[var(--sl)] text-[var(--fa)]"

  return (
    <Badge variant="outline" className={cn("font-hud text-[10px] font-normal", classes)}>
      {label}
    </Badge>
  )
}

export function OperationalError({
  error,
  compact = false,
}: {
  error?: string | null
  compact?: boolean
}) {
  const [open, setOpen] = React.useState(false)
  const guidance = explainOperationalError(error)

  if (!error) return null

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className={cn("border-l-2 border-[var(--dg)]", compact ? "pl-2" : "rounded-lg bg-[var(--dgd)]/30 px-3 py-2.5")}>
        <div className="flex items-start gap-2">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-[var(--dg)]" />
          <div className="min-w-0 flex-1">
            <p className="text-[12px] font-medium text-[var(--tx)]">{guidance.summary}</p>
            {!compact ? (
              <>
                <p className="mt-0.5 text-[11px] text-[var(--mu)]">
                  Likely cause: {guidance.cause}
                </p>
                <p className="mt-0.5 text-[11px] text-[var(--mu)]">
                  Next step: {guidance.action}
                </p>
              </>
            ) : null}
          </div>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            className="h-6 font-hud text-[10px] text-[var(--fa)]"
            onClick={() => setOpen((value) => !value)}
          >
            {open ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
            Details
          </Button>
        </div>
        <CollapsibleContent>
          <pre className="mt-2 max-h-32 overflow-auto whitespace-pre-wrap break-all rounded-md border border-[var(--sl)] bg-[var(--cv)] px-2.5 py-2 font-hud text-[10px] leading-4 text-[var(--fa)]">
            {error}
          </pre>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}

export function EmptyPanel({
  title,
  description,
  action,
}: {
  title: string
  description: string
  action?: React.ReactNode
}) {
  return (
    <div className="flex min-h-32 flex-col items-center justify-center rounded-lg border border-dashed border-[var(--hl)] bg-[var(--sf)]/40 px-5 py-6 text-center">
      <p className="text-[13px] font-medium text-[var(--tx)]">{title}</p>
      <p className="mt-1 max-w-md text-[12px] text-[var(--mu)]">{description}</p>
      {action ? <div className="mt-3">{action}</div> : null}
    </div>
  )
}

export function explainOperationalError(error?: string | null) {
  const value = (error ?? "").toLowerCase()
  if (value.includes("connection refused")) {
    return {
      summary: "The runtime is not accepting connections.",
      cause: "the service is stopped or the configured host and port do not match its listener",
      action: "start the runtime, then verify the connection endpoint before retrying",
    }
  }
  if (value.includes("timeout") || value.includes("deadline exceeded")) {
    return {
      summary: "The runtime did not respond in time.",
      cause: "the service is overloaded, starting up, or unreachable across the network",
      action: "check runtime health and network routing, then retry",
    }
  }
  if (value.includes("unauthorized") || value.includes("forbidden") || value.includes("401") || value.includes("403")) {
    return {
      summary: "Capcom could not authenticate with the runtime.",
      cause: "the configured credential is missing, expired, or lacks the required scope",
      action: "update the connection credential and test the connection",
    }
  }
  if (value.includes("eof") || value.includes("closed")) {
    return {
      summary: "The runtime closed the connection before responding.",
      cause: "the runtime restarted or its API endpoint terminated the request",
      action: "check the runtime logs and retry when the service is ready",
    }
  }
  return {
    summary: "Capcom could not update this runtime.",
    cause: "the runtime returned an unexpected response",
    action: "test the connection and review the diagnostic details",
  }
}

function runtimeStatus(status: RuntimeHealth): DerivedStatus | "unknown" {
  if (status === "healthy") return "ok"
  if (status === "offline") return "failed"
  if (status === "degraded") return "stale"
  return "unknown"
}

function syncStatus(status: SyncHealth): DerivedStatus | "unknown" {
  if (status === "fresh") return "ok"
  if (status === "failed") return "failed"
  if (status === "stale") return "stale"
  return "unknown"
}

"use client"

import * as React from "react"
import { GitMerge } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { useConsolidateRuntimeEndpointMutation } from "@/lib/api-hooks"
import type { RuntimeEndpoint, RuntimeInstance } from "@/lib/api-types"

const SELECT_CLASS =
  "h-9 w-full rounded-lg border border-input bg-[var(--el)] px-3 font-hud text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"

export function ConsolidateInstanceDialog({
  instances,
  open,
  onOpenChange,
}: {
  instances: RuntimeInstance[]
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [canonicalID, setCanonicalID] = React.useState(instances[0]?.id ?? "")
  const canonical =
    instances.find((item) => item.id === canonicalID) ?? instances[0]
  const effectiveCanonicalID = canonical?.id ?? ""
  const duplicates = instances.filter((item) => item.id !== effectiveCanonicalID)
  const [duplicateID, setDuplicateID] = React.useState(duplicates[0]?.id ?? "")
  const [kind, setKind] = React.useState<"alias" | "relay">("alias")
  const [confirmation, setConfirmation] = React.useState("")
  const [actor, setActor] = React.useState("local-operator")
  const [reason, setReason] = React.useState("")
  const duplicate =
    instances.find((item) => item.id === duplicateID) ?? duplicates[0]
  const mutation = useConsolidateRuntimeEndpointMutation(effectiveCanonicalID)

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setConfirmation("")
      setReason("")
    }
    onOpenChange(nextOpen)
  }

  function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!duplicate) return
    mutation.mutate(
      {
        duplicate_runtime_instance_id: duplicate.id,
        kind,
        confirmation,
        actor,
        reason,
      },
      {
        onSuccess: (instance) => {
          toast.success(
            `${duplicate.display_name || duplicate.name} is now a ${kind} endpoint of ${instance.display_name || instance.name}`
          )
          handleOpenChange(false)
        },
      }
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="border border-[var(--hl)] bg-[var(--el)] shadow-[var(--shdw)] sm:max-w-[620px]">
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <GitMerge className="size-4 text-[var(--ac)]" />
              Consolidate endpoint
            </DialogTitle>
            <DialogDescription>
              Use this when two Capcom connections reach the same runtime. The
              duplicate becomes an endpoint route; its imported history remains
              retained and agents are shown only under the canonical instance.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-5 sm:grid-cols-2">
            <Field label="Canonical runtime" id="canonical-runtime">
              <select
                id="canonical-runtime"
                className={SELECT_CLASS}
                value={effectiveCanonicalID}
                onChange={(event) => {
                  const nextCanonicalID = event.target.value
                  setCanonicalID(nextCanonicalID)
                  setDuplicateID(
                    instances.find((item) => item.id !== nextCanonicalID)?.id ?? ""
                  )
                  setConfirmation("")
                }}
              >
                {instances.map((instance) => (
                  <option key={instance.id} value={instance.id}>
                    {instance.display_name || instance.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Duplicate connection" id="duplicate-runtime">
              <select
                id="duplicate-runtime"
                className={SELECT_CLASS}
                value={duplicate?.id ?? ""}
                onChange={(event) => {
                  setDuplicateID(event.target.value)
                  setConfirmation("")
                }}
              >
                {duplicates.map((instance) => (
                  <option key={instance.id} value={instance.id}>
                    {instance.display_name || instance.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Route type" id="endpoint-kind">
              <select
                id="endpoint-kind"
                className={SELECT_CLASS}
                value={kind}
                onChange={(event) =>
                  setKind(event.target.value as "alias" | "relay")
                }
              >
                <option value="alias">Alias — same runtime address</option>
                <option value="relay">Relay — forwarded address</option>
              </select>
            </Field>
            <Field label="Actor" id="consolidate-actor">
              <Input
                id="consolidate-actor"
                value={actor}
                onChange={(event) => setActor(event.target.value)}
                required
              />
            </Field>
            <div className="sm:col-span-2">
              <Field label="Reason" id="consolidate-reason">
                <Textarea
                  id="consolidate-reason"
                  value={reason}
                  onChange={(event) => setReason(event.target.value)}
                  placeholder="Why these connections represent one runtime"
                  required
                />
              </Field>
            </div>
            <div className="sm:col-span-2">
              <Field
                label={`Type ${duplicate?.name ?? "the duplicate stable key"} to confirm`}
                id="consolidate-confirmation"
              >
                <Input
                  id="consolidate-confirmation"
                  className="font-hud"
                  value={confirmation}
                  onChange={(event) => setConfirmation(event.target.value)}
                  required
                />
              </Field>
            </div>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={
                mutation.isPending ||
                !duplicate ||
                confirmation !== duplicate.name ||
                !reason.trim() ||
                !actor.trim()
              }
            >
              {mutation.isPending ? "Consolidating…" : "Consolidate connection"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function Field({
  label,
  id,
  children,
}: {
  label: string
  id: string
  children: React.ReactNode
}) {
  return (
    <div className="grid gap-1.5">
      <label htmlFor={id} className="capcom-eyebrow">
        {label}
      </label>
      {children}
    </div>
  )
}

export function EndpointTopology({
  endpoints,
  fallback,
}: {
  endpoints?: RuntimeEndpoint[]
  fallback: string
}) {
  const routes =
    endpoints?.length
      ? endpoints
      : [
          {
            id: "canonical-fallback",
            endpoint: fallback,
            kind: "canonical" as const,
          },
        ]
  return (
    <section className="border-b border-[var(--sl)] bg-[color-mix(in_srgb,var(--el)_78%,var(--bg))] px-[18px] py-3">
      <div className="capcom-eyebrow mb-2">Endpoint topology</div>
      <div className="flex flex-wrap gap-2">
        {routes.map((route) => (
          <div
            key={route.id}
            className="flex min-w-0 items-center gap-2 rounded-md border border-[var(--hl)] bg-[var(--bg)] px-2.5 py-1.5"
          >
            <span
              className={
                route.kind === "canonical"
                  ? "size-1.5 rounded-full bg-[var(--ok)]"
                  : route.kind === "relay"
                    ? "size-1.5 rounded-full bg-[var(--wa)]"
                    : "size-1.5 rounded-full bg-[var(--ac)]"
              }
            />
            <span className="font-hud text-[10px] uppercase text-[var(--fa)]">
              {route.kind}
            </span>
            <span className="truncate font-hud text-[11px] text-[var(--mu)]">
              {route.endpoint}
            </span>
          </div>
        ))}
      </div>
    </section>
  )
}

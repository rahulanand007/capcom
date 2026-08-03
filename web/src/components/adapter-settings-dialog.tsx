"use client"

import * as React from "react"
import { Plus, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { Badge } from "@/components/ui/badge"
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
import {
  useRemoveRuntimeInstanceMutation,
  useUpdateRuntimeInstanceSettingsMutation,
} from "@/lib/api-hooks"
import type { RuntimeInstance, RuntimeMode } from "@/lib/api-types"

type LabelRow = { id: string; key: string; value: string }

type AdapterSettingsDialogProps = {
  adapterName: string
  instances: RuntimeInstance[]
  open: boolean
  onOpenChange: (open: boolean) => void
}

const SELECT_CLASS =
  "h-8 w-full rounded-lg border border-input bg-[var(--el)] px-2.5 py-1 font-hud text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"

export function AdapterSettingsDialog({
  adapterName,
  instances,
  open,
  onOpenChange,
}: AdapterSettingsDialogProps) {
  const [selectedID, setSelectedID] = React.useState(instances[0]?.id ?? "")
  const selected =
    instances.find((instance) => instance.id === selectedID) ?? instances[0]

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto border border-[var(--hl)] bg-[var(--el)] shadow-[var(--shdw)] sm:max-w-[720px]">
        <DialogHeader>
          <DialogTitle>{adapterName} adapter settings</DialogTitle>
          <DialogDescription>
            Configure each connected runtime instance independently.
          </DialogDescription>
        </DialogHeader>

        {selected ? (
          <>
            <Field label="Runtime instance" id="settings-instance">
              <select
                id="settings-instance"
                className={SELECT_CLASS}
                value={selected.id}
                onChange={(event) => setSelectedID(event.target.value)}
              >
                {instances.map((instance) => (
                  <option key={instance.id} value={instance.id}>
                    {instance.display_name || instance.name} ({instance.environment})
                  </option>
                ))}
              </select>
            </Field>
            <InstanceSettingsForm
              key={selected.id}
              instance={selected}
              onClose={() => onOpenChange(false)}
            />
          </>
        ) : (
          <p className="py-6 text-sm text-[var(--mu)]">
            Add an instance before configuring adapter settings.
          </p>
        )}
      </DialogContent>
    </Dialog>
  )
}

function InstanceSettingsForm({
  instance,
  onClose,
}: {
  instance: RuntimeInstance
  onClose: () => void
}) {
  const mutation = useUpdateRuntimeInstanceSettingsMutation(instance.id)
  const [displayName, setDisplayName] = React.useState(instance.display_name)
  const [environment, setEnvironment] = React.useState(instance.environment)
  const [endpoint, setEndpoint] = React.useState(instance.endpoint)
  const [mode, setMode] = React.useState<RuntimeMode>(instance.mode)
  const [authRef, setAuthRef] = React.useState(instance.auth_ref)
  const [description, setDescription] = React.useState(instance.description ?? "")
  const [syncEnabled, setSyncEnabled] = React.useState(instance.sync_enabled)
  const [syncInterval, setSyncInterval] = React.useState(
    instance.sync_interval_seconds || 60
  )
  const [labels, setLabels] = React.useState<LabelRow[]>(() =>
    Object.entries(instance.labels ?? {}).map(([key, value], index) => ({
      id: `${index}-${key}`,
      key,
      value,
    }))
  )
  const [actor, setActor] = React.useState("local-operator")
  const [reason, setReason] = React.useState("")

  const canSubmit =
    displayName.trim() &&
    environment.trim() &&
    endpoint.trim() &&
    authRef.trim() &&
    actor.trim() &&
    reason.trim() &&
    syncInterval >= 15 &&
    syncInterval <= 86400 &&
    !mutation.isPending

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSubmit) return

    mutation.mutate(
      {
        display_name: displayName.trim(),
        environment: environment.trim().toLowerCase(),
        labels: Object.fromEntries(
          labels
            .map((row) => [row.key.trim(), row.value.trim()])
            .filter(([key, value]) => key && value)
        ),
        mode,
        endpoint: endpoint.trim(),
        auth_ref: authRef.trim(),
        description: description.trim(),
        sync_enabled: syncEnabled,
        sync_interval_seconds: syncInterval,
        actor: actor.trim(),
        reason: reason.trim(),
      },
      {
        onSuccess: () => {
          toast.success(`${displayName.trim()} settings updated`)
          onClose()
        },
      }
    )
  }

  return (
    <>
    <form onSubmit={submit} className="flex flex-col gap-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <ReadOnlyField label="Stable key" value={instance.name} />
        <ReadOnlyField label="Adapter type" value={instance.runtime_type} />
        <Field label="Display name" id="settings-display-name">
          <Input
            id="settings-display-name"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            required
          />
        </Field>
        <Field label="Environment" id="settings-environment">
          <Input
            id="settings-environment"
            value={environment}
            onChange={(event) => setEnvironment(event.target.value)}
            pattern="[a-z][a-z0-9_-]{0,31}"
            required
          />
        </Field>
      </div>

      <Field label="Endpoint URL" id="settings-endpoint">
        <Input
          id="settings-endpoint"
          type="url"
          value={endpoint}
          onChange={(event) => setEndpoint(event.target.value)}
          required
        />
      </Field>

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Control mode" id="settings-mode">
          <select
            id="settings-mode"
            className={SELECT_CLASS}
            value={mode}
            onChange={(event) => setMode(event.target.value as RuntimeMode)}
          >
            <option value="read_only">Read only</option>
            <option value="control_enabled">Control enabled</option>
          </select>
        </Field>
        <Field label="Credential reference" id="settings-auth-ref">
          <Input
            id="settings-auth-ref"
            value={authRef}
            onChange={(event) => setAuthRef(event.target.value)}
            required
          />
        </Field>
      </div>

      <Field label="Description" id="settings-description">
        <Textarea
          id="settings-description"
          value={description}
          onChange={(event) => setDescription(event.target.value)}
          rows={2}
        />
      </Field>

      <div className="border-y border-[var(--sl)] py-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <p className="font-hud text-sm font-semibold text-[var(--tx)]">
              Automatic re-import
            </p>
            <p className="mt-0.5 text-xs text-[var(--mu)]">
              Keep the cached agent inventory synchronized.
            </p>
          </div>
          <label className="flex cursor-pointer items-center gap-2 font-hud text-xs">
            <input
              type="checkbox"
              checked={syncEnabled}
              onChange={(event) => setSyncEnabled(event.target.checked)}
              className="size-4 accent-[var(--ac)]"
            />
            Enabled
          </label>
        </div>
        <div className="mt-3 max-w-[240px]">
          <Field label="Interval (seconds)" id="settings-sync-interval">
            <Input
              id="settings-sync-interval"
              type="number"
              min={15}
              max={86400}
              value={syncInterval}
              onChange={(event) => setSyncInterval(Number(event.target.value))}
            />
          </Field>
        </div>
      </div>

      <div className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <span className="capcom-eyebrow normal-case tracking-normal">Labels</span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              setLabels((current) => [
                ...current,
                { id: crypto.randomUUID(), key: "", value: "" },
              ])
            }
          >
            <Plus className="size-3.5" />
            Add label
          </Button>
        </div>
        {labels.map((row) => (
          <div key={row.id} className="grid grid-cols-[1fr_1fr_32px] gap-2">
            <Input
              aria-label="Label key"
              placeholder="team"
              value={row.key}
              onChange={(event) =>
                setLabels((current) =>
                  current.map((item) =>
                    item.id === row.id ? { ...item, key: event.target.value } : item
                  )
                )
              }
            />
            <Input
              aria-label="Label value"
              placeholder="platform"
              value={row.value}
              onChange={(event) =>
                setLabels((current) =>
                  current.map((item) =>
                    item.id === row.id ? { ...item, value: event.target.value } : item
                  )
                )
              }
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Remove label"
              onClick={() =>
                setLabels((current) => current.filter((item) => item.id !== row.id))
              }
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        ))}
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Actor" id="settings-actor">
          <Input
            id="settings-actor"
            value={actor}
            onChange={(event) => setActor(event.target.value)}
            required
          />
        </Field>
        <Field label="Reason" id="settings-reason">
          <Input
            id="settings-reason"
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder="Why this setting is changing"
            required
          />
        </Field>
      </div>

      <DialogFooter className="bg-transparent">
        <Badge variant="outline" className="mr-auto font-hud text-[11px]">
          {mode === "control_enabled" ? "Control actions enabled" : "Read only"}
        </Badge>
        <Button type="button" variant="outline" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={!canSubmit}>
          {mutation.isPending ? "Saving" : "Save settings"}
        </Button>
      </DialogFooter>
    </form>
    <RemoveInstancePanel instance={instance} onRemoved={onClose} />
    </>
  )
}

function RemoveInstancePanel({
  instance,
  onRemoved,
}: {
  instance: RuntimeInstance
  onRemoved: () => void
}) {
  const mutation = useRemoveRuntimeInstanceMutation(instance.id)
  const [confirmation, setConfirmation] = React.useState("")
  const [actor, setActor] = React.useState("local-operator")
  const [reason, setReason] = React.useState("")
  const canRemove =
    confirmation === instance.name &&
    Boolean(actor.trim()) &&
    Boolean(reason.trim()) &&
    !mutation.isPending

  return (
    <section className="mt-2 rounded-lg border border-red-500/30 bg-red-500/5 p-4">
      <div className="flex items-start gap-3">
        <Trash2 className="mt-0.5 size-4 shrink-0 text-red-400" />
        <div>
          <p className="font-hud text-sm font-semibold text-red-300">
            Remove runtime instance
          </p>
          <p className="mt-1 text-xs leading-5 text-[var(--mu)]">
            Stops imports and hides this connection. Imported state and audit
            history are retained. Type the stable key{" "}
            <code className="text-[var(--tx)]">{instance.name}</code> to confirm.
          </p>
        </div>
      </div>
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <Field label="Stable key confirmation" id={`remove-confirm-${instance.id}`}>
          <Input
            id={`remove-confirm-${instance.id}`}
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            autoComplete="off"
          />
        </Field>
        <Field label="Actor" id={`remove-actor-${instance.id}`}>
          <Input
            id={`remove-actor-${instance.id}`}
            value={actor}
            onChange={(event) => setActor(event.target.value)}
          />
        </Field>
      </div>
      <div className="mt-3">
        <Field label="Reason" id={`remove-reason-${instance.id}`}>
          <Input
            id={`remove-reason-${instance.id}`}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder="Why this instance is no longer needed"
          />
        </Field>
      </div>
      <div className="mt-3 flex justify-end">
        <Button
          type="button"
          variant="destructive"
          disabled={!canRemove}
          onClick={() =>
            mutation.mutate(
              {
                confirmation,
                actor: actor.trim(),
                reason: reason.trim(),
              },
              {
                onSuccess: () => {
                  toast.success(`${instance.display_name || instance.name} removed`)
                  onRemoved()
                },
              }
            )
          }
        >
          {mutation.isPending ? "Removing" : "Remove instance"}
        </Button>
      </div>
    </section>
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
    <div className="flex flex-col gap-1.5">
      <label className="capcom-eyebrow normal-case tracking-normal" htmlFor={id}>
        {label}
      </label>
      {children}
    </div>
  )
}

function ReadOnlyField({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="capcom-eyebrow normal-case tracking-normal">{label}</span>
      <div className="h-8 rounded-lg border border-[var(--sl)] bg-[var(--bg)] px-2.5 py-1.5 font-hud text-xs text-[var(--mu)]">
        {value}
      </div>
    </div>
  )
}

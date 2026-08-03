"use client"

import * as React from "react"
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
import { useAddRuntimeInstanceMutation } from "@/lib/api-hooks"
import type { RuntimeMode, RuntimeType } from "@/lib/api-types"
import { cn } from "@/lib/utils"

type AdapterOption = {
  id: RuntimeType
  name: string
  tokenLabel: string
  readOnly?: boolean
}

type AddInstanceDialogProps = {
  open: boolean
  defaultAdapterId?: RuntimeType
  onOpenChange: (open: boolean) => void
}

const ADAPTERS: AdapterOption[] = [
  {
    id: "gantry",
    name: "Gantry",
    tokenLabel: "Gantry Control API token",
  },
  {
    id: "langgraph",
    name: "LangGraph",
    tokenLabel: "LangSmith API key",
  },
  {
    id: "temporal",
    name: "Temporal",
    tokenLabel: "Temporal Control API token",
  },
  {
    id: "letta",
    name: "Letta",
    tokenLabel: "Letta Control API token",
  },
  {
    id: "crewai",
    name: "CrewAI",
    tokenLabel: "CrewAI Control API token",
  },
]

const ENVIRONMENTS = ["production", "staging", "development"] as const
const MODES: RuntimeMode[] = ["read_only", "control_enabled"]
const SELECT_CLASS =
  "h-8 w-full rounded-lg border border-input bg-transparent px-2.5 py-1 font-hud text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50"

export function AddInstanceDialog({
  open,
  defaultAdapterId,
  onOpenChange,
}: AddInstanceDialogProps) {
  const addInstanceMutation = useAddRuntimeInstanceMutation()
  const [step, setStep] = React.useState<1 | 2>(1)
  const [adapterId, setAdapterId] = React.useState<RuntimeType>(
    defaultAdapterId ?? "gantry"
  )
  const [displayName, setDisplayName] = React.useState("")
  const [environment, setEnvironment] =
    React.useState<(typeof ENVIRONMENTS)[number]>("development")
  const [endpoint, setEndpoint] = React.useState("")
  const [mode, setMode] = React.useState<RuntimeMode>("read_only")
  const [token, setToken] = React.useState("")

  function resetForm() {
    setStep(1)
    setAdapterId(defaultAdapterId ?? "gantry")
    setDisplayName("")
    setEnvironment("development")
    setEndpoint("")
    setMode("read_only")
    setToken("")
  }

  const selectedAdapter =
    ADAPTERS.find((adapter) => adapter.id === adapterId) ?? ADAPTERS[0]

  const canSubmit =
    displayName.trim() &&
    endpoint.trim() &&
    token.trim() &&
    !addInstanceMutation.isPending

  function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!canSubmit) {
      return
    }

    const trimmedDisplayName = displayName.trim()
    const slug = kebabCase(trimmedDisplayName)
    const authRef = `${slug}-key`
    const reason = `Add ${trimmedDisplayName} via console`

    addInstanceMutation.mutate(
      {
        secret: {
          name: authRef,
          value: token,
          actor: "local-operator",
          reason,
        },
        runtimeInstance: {
          name: slug,
          display_name: trimmedDisplayName,
          environment,
          runtime_type: adapterId,
          mode: selectedAdapter.readOnly ? "read_only" : mode,
          endpoint: endpoint.trim(),
          auth_ref: authRef,
          actor: "local-operator",
          reason,
        },
      },
      {
        onSuccess: () => {
          toast.success(`${trimmedDisplayName} added`)
          resetForm()
          onOpenChange(false)
        },
      }
    )
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (!addInstanceMutation.isPending) {
          if (!nextOpen) {
            resetForm()
          }
          onOpenChange(nextOpen)
        }
      }}
    >
      <DialogContent className="border border-[var(--hl)] bg-[var(--el)] shadow-[var(--shdw)] sm:max-w-[520px]">
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>Add runtime instance</DialogTitle>
            <DialogDescription>
              {step === 1
                ? "Choose the adapter type for this runtime instance."
                : `Configure ${selectedAdapter.name} connection details.`}
            </DialogDescription>
          </DialogHeader>

          {step === 1 ? (
            <AdapterStep
              adapterId={adapterId}
              onAdapterChange={(nextAdapterId) => {
                setAdapterId(nextAdapterId)
                const nextAdapter = ADAPTERS.find(
                  (adapter) => adapter.id === nextAdapterId
                )
                if (nextAdapter?.readOnly) {
                  setMode("read_only")
                }
              }}
            />
          ) : (
            <DetailsStep
              selectedAdapter={selectedAdapter}
              displayName={displayName}
              environment={environment}
              endpoint={endpoint}
              mode={mode}
              token={token}
              onDisplayNameChange={setDisplayName}
              onEnvironmentChange={setEnvironment}
              onEndpointChange={setEndpoint}
              onModeChange={setMode}
              onTokenChange={setToken}
            />
          )}

          <DialogFooter className="bg-transparent">
            {step === 2 ? (
              <Button
                type="button"
                variant="outline"
                disabled={addInstanceMutation.isPending}
                onClick={() => setStep(1)}
              >
                Back
              </Button>
            ) : null}
            {step === 1 ? (
              <Button type="button" onClick={() => setStep(2)}>
                Continue
              </Button>
            ) : (
              <Button type="submit" disabled={!canSubmit}>
                {addInstanceMutation.isPending ? "Adding" : "Add instance"}
              </Button>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function AdapterStep({
  adapterId,
  onAdapterChange,
}: {
  adapterId: RuntimeType
  onAdapterChange: (adapterId: RuntimeType) => void
}) {
  return (
    <div className="grid gap-2 sm:grid-cols-2">
      {ADAPTERS.map((adapter) => (
        <button
          key={adapter.id}
          type="button"
          className={cn(
            "rounded-lg border border-[var(--hl)] bg-transparent p-3 text-left transition hover:border-[var(--ac)]",
            adapterId === adapter.id && "border-[var(--ac)] bg-[var(--acd)]"
          )}
          onClick={() => onAdapterChange(adapter.id)}
        >
          <div className="flex items-center justify-between gap-2">
            <span className="font-hud text-sm font-semibold">
              {adapter.name}
            </span>
            {adapterId === adapter.id ? (
              <Badge className="bg-[var(--acd)] font-hud text-[11px] text-[var(--ac)]">
                selected
              </Badge>
            ) : null}
          </div>
          <div className="mt-2 font-hud text-[11px] text-[var(--fa)]">
            runtime_type: {adapter.id}
          </div>
        </button>
      ))}
    </div>
  )
}

function DetailsStep({
  selectedAdapter,
  displayName,
  environment,
  endpoint,
  mode,
  token,
  onDisplayNameChange,
  onEnvironmentChange,
  onEndpointChange,
  onModeChange,
  onTokenChange,
}: {
  selectedAdapter: AdapterOption
  displayName: string
  environment: (typeof ENVIRONMENTS)[number]
  endpoint: string
  mode: RuntimeMode
  token: string
  onDisplayNameChange: (value: string) => void
  onEnvironmentChange: (value: (typeof ENVIRONMENTS)[number]) => void
  onEndpointChange: (value: string) => void
  onModeChange: (value: RuntimeMode) => void
  onTokenChange: (value: string) => void
}) {
  return (
    <div className="grid gap-3">
      <Field label="Display name" id="runtime-display-name">
        <Input
          id="runtime-display-name"
          value={displayName}
          onChange={(event) => onDisplayNameChange(event.target.value)}
          placeholder={`${selectedAdapter.name} Production`}
          className="font-hud"
          required
        />
      </Field>
      <Field label="Environment" id="runtime-environment">
        <select
          id="runtime-environment"
          className={SELECT_CLASS}
          value={environment}
          onChange={(event) =>
            onEnvironmentChange(
              event.target.value as (typeof ENVIRONMENTS)[number]
            )
          }
        >
          {ENVIRONMENTS.map((item) => (
            <option key={item} value={item}>
              {item}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Endpoint URL" id="runtime-endpoint">
        <Input
          id="runtime-endpoint"
          type="url"
          value={endpoint}
          onChange={(event) => onEndpointChange(event.target.value)}
          placeholder="http://127.0.0.1:8787"
          className="font-hud"
          required
        />
      </Field>
      <Field label="Mode" id="runtime-mode">
        <select
          id="runtime-mode"
          className={SELECT_CLASS}
          value={mode}
          onChange={(event) => onModeChange(event.target.value as RuntimeMode)}
          disabled={selectedAdapter.readOnly}
        >
          {MODES.map((item) => (
            <option key={item} value={item}>
              {item}
            </option>
          ))}
        </select>
      </Field>
      <Field label={selectedAdapter.tokenLabel} id="runtime-token">
        <Input
          id="runtime-token"
          type="password"
          autoComplete="off"
          value={token}
          onChange={(event) => onTokenChange(event.target.value)}
          placeholder={selectedAdapter.id === "langgraph" ? "Local development may use any non-empty placeholder" : "Token"}
          className="font-hud"
          required
        />
      </Field>
    </div>
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

function kebabCase(value: string) {
  const slug = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
  return slug || "runtime-instance"
}

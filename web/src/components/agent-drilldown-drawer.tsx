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
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { Textarea } from "@/components/ui/textarea"
import {
  useAgentAccessQuery,
  useAgentDelegationsQuery,
  useAgentSkillsQuery,
  useDeleteAgentMutation,
  usePersistedAgentQuery,
  useReconcileAgentAccessMutation,
  useSetAgentStatusMutation,
  useRuntimeInstancesQuery,
  useRuntimeInstanceAgentsQuery,
} from "@/lib/api-hooks"
import type {
  AgentDelegation,
  ControlAction,
  JsonObject,
  PersistedAgent,
  RuntimeAccessSelection,
  RuntimeAgentSkill,
} from "@/lib/api-types"
import { displayRuntimeType, relativeTime } from "@/lib/adapters"
import { cn } from "@/lib/utils"

type AgentDrilldownDrawerProps = {
  agent: PersistedAgent | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AgentDrilldownDrawer({
  agent,
  open,
  onOpenChange,
}: AgentDrilldownDrawerProps) {
  const agentId = agent?.id
  const agentQuery = usePersistedAgentQuery(open ? agentId : undefined)
  const skillsQuery = useAgentSkillsQuery(open ? agentId : undefined)
  const accessQuery = useAgentAccessQuery(open ? agentId : undefined)
  const delegationsQuery = useAgentDelegationsQuery(open ? agentId : undefined)
  const runtimeInstancesQuery = useRuntimeInstancesQuery(open)
  const detail = agentQuery.data ?? agent
  const instance = runtimeInstancesQuery.data?.find(
    (item) => item.id === detail?.runtime_connection_id
  )
  const instanceAgentsQuery = useRuntimeInstanceAgentsQuery(
    open ? detail?.runtime_connection_id : undefined
  )
  const skills = skillsQuery.data ?? []
  const selections = accessQuery.data?.selections ?? []
  const canControl = instance?.mode === "control_enabled"

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="w-[min(720px,calc(100vw-18px))] gap-0 border-[var(--hl)] bg-[var(--el)] p-0 sm:max-w-[720px]"
      >
        <SheetHeader className="border-b border-[var(--sl)] px-6 py-5 pr-12">
          <SheetDescription className="capcom-eyebrow">
            runtime state
          </SheetDescription>
          <SheetTitle className="truncate font-hud text-[18px] font-semibold text-[var(--tx)]">
            {detail?.name ?? "Agent"}
          </SheetTitle>
          <div className="font-hud text-[11px] text-[var(--fa)]">
            {detail ? (
              <AgentMetaLine
                agent={detail}
                instanceName={instance?.display_name || instance?.name}
                environment={instance?.environment}
                skillCount={skills.length}
                accessCount={selections.length}
              />
            ) : (
              "Loading runtime metadata"
            )}
          </div>
        </SheetHeader>

        <ScrollArea className="min-h-0 flex-1">
          <div className="flex flex-col gap-5 px-6 py-5">
            {!detail ? (
              <DrawerSkeleton />
            ) : (
              <>
                <AgentOverview
                  agent={detail}
                  instanceLabel={
                    instance
                      ? `${displayRuntimeType(instance.runtime_type)} / ${
                          instance.display_name || instance.name
                        }`
                      : "Unknown runtime"
                  }
                />
                <DelegationsSection
                  agent={detail}
                  agents={instanceAgentsQuery.data ?? []}
                  delegations={delegationsQuery.data ?? []}
                  loading={delegationsQuery.isLoading || instanceAgentsQuery.isLoading}
                />
                <SkillsSection
                  loading={skillsQuery.isLoading}
                  skills={skills}
                />
                <EffectiveAccessSection
                  loading={accessQuery.isLoading}
                  selections={selections}
                />
              </>
            )}
          </div>
        </ScrollArea>

        {detail && instance?.runtime_type === "gantry" ? (
          <div className="flex flex-wrap justify-end gap-2 border-t border-[var(--sl)] px-6 py-4">
            <AgentStatusEditor agent={detail} enabled={canControl} />
            {canControl ? (
              <ReconcileAccessEditor
                agent={detail}
                selections={selections}
                pendingAccess={accessQuery.isLoading}
              />
            ) : null}
          </div>
        ) : null}
        {detail && instance?.runtime_type === "langgraph" ? (
          <div className="flex flex-wrap justify-end gap-2 border-t border-[var(--sl)] px-6 py-4">
            <DeleteAssistantEditor
              agent={detail}
              enabled={canControl && !Boolean(detail.metadata?.runtime_deleted)}
              onDeleted={() => onOpenChange(false)}
            />
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}

function AgentMetaLine({
  agent,
  instanceName,
  environment,
  skillCount,
  accessCount,
}: {
  agent: PersistedAgent
  instanceName?: string
  environment?: string
  skillCount: number
  accessCount: number
}) {
  return (
    <>
      {instanceName ?? "unknown instance"} / {environment || "unspecified"} /{" "}
      {agent.freshness} / observed {relativeTime(agent.observed_at)} /{" "}
      {skillCount} skills / {accessCount} access
    </>
  )
}

function AgentOverview({
  agent,
  instanceLabel,
}: {
  agent: PersistedAgent
  instanceLabel: string
}) {
  const metadata = agent.metadata ?? {}
  const configuration =
    pickMetadata(metadata, ["configuration", "config", "settings"]) ?? "none"
  const harness = pickMetadata(metadata, [
    "agent_harness",
    "harness",
    "harness_id",
    "harness_name",
  ])

  return (
    <DrawerSection title="Agent overview">
      <div className="grid gap-3 sm:grid-cols-2">
        <OverviewField label="Type" value={agent.kind} />
        <OverviewField label="Status" value={agent.runtime_status || agent.status} />
        <OverviewField label="Freshness" value={agent.freshness} />
        <OverviewField label="Runtime ID" value={agent.runtime_agent_id} mono />
        <OverviewField
          label="Parent agent"
          value={agent.parent_runtime_agent_id || "none"}
          mono={Boolean(agent.parent_runtime_agent_id)}
        />
        <OverviewField
          label="Harness"
          value={harness === undefined ? "not reported" : formatMetadataValue(harness)}
        />
        <OverviewField label="Runtime instance" value={instanceLabel} />
        <OverviewField
          label="Configuration"
          value={formatMetadataValue(configuration)}
          wide
        />
      </div>
    </DrawerSection>
  )
}

function DelegationsSection({
  agent,
  agents,
  delegations,
  loading,
}: {
  agent: PersistedAgent
  agents: PersistedAgent[]
  delegations: AgentDelegation[]
  loading: boolean
}) {
  const names = new Map(agents.map((item) => [item.runtime_agent_id, item.name]))
  const outgoing = delegations.filter(
    (item) => item.orchestrator_runtime_agent_id === agent.runtime_agent_id
  )
  const incoming = delegations.filter(
    (item) => item.delegate_runtime_agent_id === agent.runtime_agent_id
  )

  return (
    <DrawerSection title="Agent relationships">
      {loading ? (
        <StackSkeleton />
      ) : outgoing.length || incoming.length ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <DelegationList
            empty="No callable delegates."
            items={outgoing}
            label="Delegates"
            nameFor={(item) =>
              item.display_name ||
              names.get(item.delegate_runtime_agent_id || "") ||
              item.delegate_ref
            }
            runtimeIDFor={(item) => item.delegate_runtime_agent_id || item.delegate_ref}
          />
          <DelegationList
            empty="Not delegated by another agent."
            items={incoming}
            label="Delegated by"
            nameFor={(item) =>
              names.get(item.orchestrator_runtime_agent_id) ||
              item.orchestrator_runtime_agent_id
            }
            runtimeIDFor={(item) => item.orchestrator_runtime_agent_id}
          />
        </div>
      ) : (
        <EmptyPanel>No durable delegation relationships imported.</EmptyPanel>
      )}
    </DrawerSection>
  )
}

function DelegationList({
  label,
  items,
  empty,
  nameFor,
  runtimeIDFor,
}: {
  label: string
  items: AgentDelegation[]
  empty: string
  nameFor: (item: AgentDelegation) => string
  runtimeIDFor: (item: AgentDelegation) => string
}) {
  return (
    <div>
      <div className="capcom-eyebrow mb-2">{label}</div>
      {items.length ? (
        <div className="flex flex-col gap-2">
          {items.map((item) => (
            <div
              key={item.id}
              className="border border-[var(--sl)] bg-[var(--sf)] px-3 py-2"
            >
              <div className="font-hud text-[13px] text-[var(--tx)]">
                {nameFor(item)}
              </div>
              <div className="mt-1 break-all font-hud text-[10px] text-[var(--fa)]">
                {runtimeIDFor(item)}
              </div>
              <div className="mt-2 flex flex-wrap gap-1.5">
                <SkillBadge>
                  {item.configured ? "configured" : "conversation-bound"}
                </SkillBadge>
                <SkillBadge>{item.resolved ? "resolved" : "unresolved"}</SkillBadge>
                {item.persona ? <SkillBadge>{item.persona}</SkillBadge> : null}
                {item.status === "stale" ? <SkillBadge>stale</SkillBadge> : null}
              </div>
              {item.tool_name ? (
                <div className="mt-2 font-hud text-[10px] text-[var(--mu)]">
                  {item.tool_name}
                </div>
              ) : null}
            </div>
          ))}
        </div>
      ) : (
        <div className="border border-dashed border-[var(--sl)] px-3 py-4 text-[11px] text-[var(--fa)]">
          {empty}
        </div>
      )}
    </div>
  )
}

function OverviewField({
  label,
  value,
  mono = false,
  wide = false,
}: {
  label: string
  value: React.ReactNode
  mono?: boolean
  wide?: boolean
}) {
  return (
    <div
      className={cn(
        "rounded-lg border border-[var(--sl)] bg-[var(--sf)] px-3 py-2",
        wide && "sm:col-span-2"
      )}
    >
      <div className="capcom-eyebrow">{label}</div>
      <div
        className={cn(
          "mt-1 break-words text-[13px] text-[var(--tx)]",
          mono && "font-hud"
        )}
      >
        {value}
      </div>
    </div>
  )
}

function SkillsSection({
  loading,
  skills,
}: {
  loading: boolean
  skills: RuntimeAgentSkill[]
}) {
  return (
    <DrawerSection title="Skills">
      {loading ? (
        <StackSkeleton />
      ) : skills.length ? (
        <div className="flex flex-col gap-3">
          {skills.map((skill) => (
            <div
              key={skill.runtime_skill_id}
              className="rounded-lg border border-[var(--sl)] bg-[var(--sf)] px-3 py-3"
            >
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                  <div className="truncate font-hud text-[13px] font-medium text-[var(--tx)]">
                    {skill.name}
                  </div>
                  <div className="mt-1 flex flex-wrap gap-1.5">
                    <SkillBadge>{skill.source || "runtime"}</SkillBadge>
                    <SkillBadge>{skill.status || "unknown"}</SkillBadge>
                    {skill.version ? <SkillBadge>v{skill.version}</SkillBadge> : null}
                  </div>
                </div>
                <span className="font-hud text-[11px] text-[var(--fa)]">
                  {skill.runtime_skill_id}
                </span>
              </div>
              {skill.description ? (
                <p className="mt-2 text-[12px] text-[var(--mu)]">
                  {skill.description}
                </p>
              ) : null}
              <ToolWorkflowList label="Tools" values={skill.tool_ids} />
              <ToolWorkflowList label="Workflows" values={skill.workflow_refs} />
            </div>
          ))}
        </div>
      ) : (
        <EmptyPanel>No skills imported for this agent.</EmptyPanel>
      )}
    </DrawerSection>
  )
}

function ToolWorkflowList({
  label,
  values,
}: {
  label: string
  values: string[]
}) {
  if (!values.length) {
    return null
  }

  return (
    <div className="mt-2">
      <div className="capcom-eyebrow">{label}</div>
      <div className="mt-1 flex flex-wrap gap-1.5">
        {values.map((value) => (
          <Badge
            key={value}
            variant="outline"
            className="border-[var(--hl)] bg-[var(--sl)] font-hud text-[11px] text-[var(--mu)]"
          >
            {value}
          </Badge>
        ))}
      </div>
    </div>
  )
}

function EffectiveAccessSection({
  loading,
  selections,
}: {
  loading: boolean
  selections: RuntimeAccessSelection[]
}) {
  return (
    <DrawerSection title="Effective access">
      {loading ? (
        <StackSkeleton />
      ) : selections.length ? (
        <div className="flex flex-col gap-2">
          {selections.map((selection) => (
            <div
              key={accessSelectionKey(selection)}
              className="grid gap-2 rounded-lg border border-[var(--sl)] bg-[var(--sf)] px-3 py-2 sm:grid-cols-[1fr_auto]"
            >
              <div className="min-w-0">
                <div className="font-hud text-[13px] text-[var(--tx)]">
                  {selection.name || selection.id}
                </div>
                <div className="mt-0.5 font-hud text-[11px] text-[var(--fa)]">
                  {selection.kind} / {selection.id}
                </div>
              </div>
              <div className="flex flex-wrap items-center gap-2 sm:justify-end">
                <AccessBadge allowed={selection.allowed} />
                <Badge
                  variant="outline"
                  className="border-[var(--hl)] bg-transparent font-hud text-[11px] text-[var(--fa)]"
                >
                  version {attributeVersion(selection.attributes)}
                </Badge>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <EmptyPanel>No effective access selections resolved.</EmptyPanel>
      )}
    </DrawerSection>
  )
}

function ReconcileAccessEditor({
  agent,
  selections,
  pendingAccess,
}: {
  agent: PersistedAgent
  selections: RuntimeAccessSelection[]
  pendingAccess: boolean
}) {
  const [open, setOpen] = React.useState(false)
  const mutation = useReconcileAgentAccessMutation(agent.id)

  return (
    <>
      <Button
        className="shadow-[0_0_0_3px_var(--glow)] hover:brightness-[1.08]"
        disabled={pendingAccess}
        onClick={() => setOpen(true)}
      >
        Reconcile access
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="border border-[var(--hl)] bg-[var(--el)] shadow-[var(--shdw)] sm:max-w-[520px]">
          <ReconcileAccessForm
            key={open ? `${agent.id}:${selections.length}` : "closed"}
            pending={mutation.isPending}
            selections={selections}
            onCancel={() => setOpen(false)}
            onSubmit={(payload) => {
              mutation.mutate(payload, {
                onSuccess: (action) => {
                  const suffix = payload.dry_run ? " (validation only)" : ""
                  toast.success(
                    `Reconcile access ${actionStatus(action)}${suffix}`
                  )
                  setOpen(false)
                },
              })
            }}
          />
        </DialogContent>
      </Dialog>
    </>
  )
}

function AgentStatusEditor({
  agent,
  enabled,
}: {
  agent: PersistedAgent
  enabled: boolean
}) {
  const [open, setOpen] = React.useState(false)
  const mutation = useSetAgentStatusMutation(agent.id)
  const targetStatus = agent.status === "disabled" ? "enabled" : "disabled"
  const command = targetStatus === "enabled" ? "Enable" : "Disable"

  return (
    <>
      <Button
        variant={targetStatus === "disabled" ? "outline" : "default"}
        disabled={!enabled}
        title={enabled ? `${command} agent` : "Runtime instance is read-only"}
        className={cn(
          "font-hud text-xs",
          targetStatus === "disabled" && "hover:border-[var(--dg)] hover:text-[var(--dg)]"
        )}
        onClick={() => setOpen(true)}
      >
        {command} agent
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="border border-[var(--hl)] bg-[var(--el)] shadow-[var(--shdw)] sm:max-w-[480px]">
          <form
            className="flex flex-col gap-4"
            onSubmit={(event) => {
              event.preventDefault()
              const formData = new FormData(event.currentTarget)
              const actor = String(formData.get("actor") ?? "").trim()
              const reason = String(formData.get("reason") ?? "").trim()
              if (!actor || !reason) return
              const dryRun = formData.get("dry_run") === "on"
              mutation.mutate(
                {
                  status: targetStatus,
                  actor,
                  reason,
                  idempotency_key: randomIdempotencyKey(),
                  dry_run: dryRun,
                },
                {
                  onSuccess: (action) => {
                    toast.success(`${command} agent ${actionStatus(action)}${dryRun ? " (validation only)" : ""}`)
                    setOpen(false)
                  },
                  onError: (error) => toast.error(error.message),
                }
              )
            }}
          >
            <DialogHeader>
              <DialogTitle>{command} {agent.name}</DialogTitle>
              <DialogDescription>
                {targetStatus === "disabled"
                  ? "Stops this agent from accepting new Gantry work."
                  : "Returns this Gantry agent to active service."}
              </DialogDescription>
            </DialogHeader>
            <label className="flex flex-col gap-2">
              <span className="capcom-eyebrow">Actor</span>
              <Input name="actor" defaultValue="local-operator" required className="font-hud text-[13px]" />
            </label>
            <label className="flex flex-col gap-2">
              <span className="capcom-eyebrow">Reason</span>
              <Textarea name="reason" defaultValue={`${command} ${agent.name}`} required rows={3} className="resize-none font-hud text-[13px]" />
            </label>
            <label className="flex items-center gap-2 font-hud text-[12px] text-[var(--mu)]">
              <input name="dry_run" type="checkbox" defaultChecked className="accent-[var(--ac)]" />
              Validate only
            </label>
            <DialogFooter>
              <Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => setOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={mutation.isPending}>{mutation.isPending ? "Submitting" : command}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}

function DeleteAssistantEditor({
  agent,
  enabled,
  onDeleted,
}: {
  agent: PersistedAgent
  enabled: boolean
  onDeleted: () => void
}) {
  const [open, setOpen] = React.useState(false)
  const mutation = useDeleteAgentMutation(agent.id)

  return (
    <>
      <Button
        variant="outline"
        disabled={!enabled}
        title={enabled ? "Delete assistant" : "Runtime instance is read-only"}
        className="font-hud text-xs hover:border-[var(--dg)] hover:text-[var(--dg)]"
        onClick={() => setOpen(true)}
      >
        Delete assistant
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="border border-[var(--hl)] bg-[var(--el)] shadow-[var(--shdw)] sm:max-w-[480px]">
          <form
            className="flex flex-col gap-4"
            onSubmit={(event) => {
              event.preventDefault()
              const formData = new FormData(event.currentTarget)
              const actor = String(formData.get("actor") ?? "").trim()
              const reason = String(formData.get("reason") ?? "").trim()
              const confirmation = String(formData.get("confirmation") ?? "").trim()
              if (!actor || !reason || confirmation !== agent.runtime_agent_id) return
              const dryRun = formData.get("dry_run") === "on"
              mutation.mutate(
                {
                  actor,
                  reason,
                  confirmation,
                  idempotency_key: randomIdempotencyKey(),
                  dry_run: dryRun,
                },
                {
                  onSuccess: (action) => {
                    toast.success(
                      `Delete assistant ${actionStatus(action)}${dryRun ? " (validation only)" : ""}`
                    )
                    setOpen(false)
                    if (!dryRun) onDeleted()
                  },
                  onError: (error) => toast.error(error.message),
                }
              )
            }}
          >
            <DialogHeader>
              <DialogTitle>Delete {agent.name}</DialogTitle>
              <DialogDescription>
                This removes the assistant and every one of its versions from LangGraph.
              </DialogDescription>
            </DialogHeader>
            <label className="flex flex-col gap-2">
              <span className="capcom-eyebrow">Actor</span>
              <Input name="actor" defaultValue="local-operator" required className="font-hud text-[13px]" />
            </label>
            <label className="flex flex-col gap-2">
              <span className="capcom-eyebrow">Reason</span>
              <Textarea name="reason" defaultValue={`Delete ${agent.name}`} required rows={3} className="resize-none font-hud text-[13px]" />
            </label>
            <label className="flex flex-col gap-2">
              <span className="capcom-eyebrow">Runtime agent ID</span>
              <Input
                name="confirmation"
                placeholder={agent.runtime_agent_id}
                required
                autoComplete="off"
                className="font-hud text-[13px]"
              />
            </label>
            <label className="flex items-center gap-2 font-hud text-[12px] text-[var(--mu)]">
              <input name="dry_run" type="checkbox" defaultChecked className="accent-[var(--ac)]" />
              Validate only
            </label>
            <DialogFooter>
              <Button type="button" variant="outline" disabled={mutation.isPending} onClick={() => setOpen(false)}>Cancel</Button>
              <Button type="submit" variant="destructive" disabled={mutation.isPending}>
                {mutation.isPending ? "Submitting" : "Delete assistant"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  )
}

function ReconcileAccessForm({
  selections,
  pending,
  onCancel,
  onSubmit,
}: {
  selections: RuntimeAccessSelection[]
  pending: boolean
  onCancel: () => void
  onSubmit: (payload: {
    selections: RuntimeAccessSelection[]
    actor: string
    reason: string
    idempotency_key: string
    dry_run: boolean
  }) => void
}) {
  const [checked, setChecked] = React.useState<Set<string>>(
    () => new Set(selections.map(accessSelectionKey))
  )

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault()
        const formData = new FormData(event.currentTarget)
        const actor = String(formData.get("actor") ?? "").trim()
        const reason = String(formData.get("reason") ?? "").trim()
        const dryRun = formData.get("dry_run") === "on"

        if (!actor || !reason) {
          return
        }

        onSubmit({
          selections: selections.filter((selection) =>
            checked.has(accessSelectionKey(selection))
          ),
          actor,
          reason,
          idempotency_key: randomIdempotencyKey(),
          dry_run: dryRun,
        })
      }}
    >
      <DialogHeader>
        <DialogTitle>Reconcile access</DialogTitle>
        <DialogDescription>
          Choose the current selections to send to the runtime control action.
        </DialogDescription>
      </DialogHeader>

      <div className="max-h-56 overflow-auto rounded-lg border border-[var(--sl)] bg-[var(--sf)]">
        {selections.length ? (
          selections.map((selection) => {
            const key = accessSelectionKey(selection)
            return (
              <label
                key={key}
                className="flex cursor-pointer items-start gap-3 border-b border-[var(--sl)] px-3 py-2 last:border-b-0 hover:bg-[var(--sl)]"
              >
                <input
                  type="checkbox"
                  checked={checked.has(key)}
                  onChange={(event) => {
                    setChecked((current) => {
                      const next = new Set(current)
                      if (event.target.checked) {
                        next.add(key)
                      } else {
                        next.delete(key)
                      }
                      return next
                    })
                  }}
                  className="mt-1 accent-[var(--ac)]"
                />
                <span className="min-w-0">
                  <span className="block truncate font-hud text-[13px] text-[var(--tx)]">
                    {selection.name || selection.id}
                  </span>
                  <span className="block font-hud text-[11px] text-[var(--fa)]">
                    {selection.kind} / {selection.id}
                  </span>
                </span>
              </label>
            )
          })
        ) : (
          <div className="px-3 py-4 text-[12px] text-[var(--mu)]">
            No current selections. Submitting will preserve an empty access
            document.
          </div>
        )}
      </div>

      <label className="flex flex-col gap-2">
        <span className="capcom-eyebrow">Actor</span>
        <Input
          name="actor"
          defaultValue="local-operator"
          required
          className="font-hud text-[13px]"
        />
      </label>

      <label className="flex flex-col gap-2">
        <span className="capcom-eyebrow">Reason</span>
        <Textarea
          name="reason"
          defaultValue="Reconcile agent access"
          required
          rows={3}
          className="resize-none font-hud text-[13px]"
        />
      </label>

      <label className="flex items-center gap-2 font-hud text-[12px] text-[var(--mu)]">
        <input
          name="dry_run"
          type="checkbox"
          defaultChecked
          className="accent-[var(--ac)]"
        />
        Validate only
      </label>

      <DialogFooter>
        <Button
          type="button"
          variant="outline"
          disabled={pending}
          onClick={onCancel}
        >
          Cancel
        </Button>
        <Button
          type="submit"
          disabled={pending}
          className="shadow-[0_0_0_3px_var(--glow)] hover:brightness-[1.08]"
        >
          {pending ? "Submitting" : "Submit"}
        </Button>
      </DialogFooter>
    </form>
  )
}

function DrawerSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section>
      <h2 className="capcom-eyebrow mb-2">{title}</h2>
      {children}
    </section>
  )
}

function SkillBadge({ children }: { children: React.ReactNode }) {
  return (
    <Badge
      variant="outline"
      className="border-[var(--hl)] bg-transparent font-hud text-[11px] text-[var(--fa)]"
    >
      {children}
    </Badge>
  )
}

function AccessBadge({ allowed }: { allowed: boolean }) {
  return (
    <Badge
      className={cn(
        "font-hud text-[11px]",
        allowed
          ? "bg-[var(--acd)] text-[var(--ac)]"
          : "bg-[var(--dgd)] text-[var(--dg)]"
      )}
    >
      {allowed ? "allowed" : "blocked"}
    </Badge>
  )
}

function EmptyPanel({ children }: { children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-[var(--sl)] bg-[var(--sf)] px-3 py-4 text-[12px] text-[var(--mu)]">
      {children}
    </div>
  )
}

function DrawerSkeleton() {
  return (
    <>
      <Skeleton className="h-32 rounded-lg" />
      <Skeleton className="h-40 rounded-lg" />
      <Skeleton className="h-32 rounded-lg" />
    </>
  )
}

function StackSkeleton() {
  return (
    <div className="flex flex-col gap-2">
      <Skeleton className="h-16 rounded-lg" />
      <Skeleton className="h-16 rounded-lg" />
    </div>
  )
}

function accessSelectionKey(selection: RuntimeAccessSelection) {
  return `${selection.kind}:${selection.id}:${selection.name}`
}

function attributeVersion(attributes?: JsonObject) {
  const version = attributes?.version
  if (typeof version === "string" || typeof version === "number") {
    return String(version)
  }
  return "none"
}

function pickMetadata(metadata: JsonObject, keys: string[]) {
  for (const key of keys) {
    const value = metadata[key]
    if (value !== undefined && value !== null && value !== "") {
      return value
    }
  }
  return undefined
}

function formatMetadataValue(value: unknown) {
  if (value === null || value === undefined || value === "") {
    return "none"
  }
  if (typeof value === "string" || typeof value === "number") {
    return String(value)
  }
  if (typeof value === "boolean") {
    return value ? "true" : "false"
  }
  return JSON.stringify(value)
}

function randomIdempotencyKey() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID()
  }
  return `local-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function actionStatus(action: ControlAction) {
  return action.status.replaceAll("_", " ")
}

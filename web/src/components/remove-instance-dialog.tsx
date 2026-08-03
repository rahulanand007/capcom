"use client"

import * as React from "react"
import { Trash2 } from "lucide-react"
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
import { useRemoveRuntimeInstanceMutation } from "@/lib/api-hooks"

type RemoveInstanceDialogProps = {
  instance: {
    id: string
    name: string
    display_name: string
  }
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function RemoveInstanceDialog({
  instance,
  open,
  onOpenChange,
}: RemoveInstanceDialogProps) {
  const mutation = useRemoveRuntimeInstanceMutation(instance.id)
  const [confirmation, setConfirmation] = React.useState("")
  const [actor, setActor] = React.useState("local-operator")
  const [reason, setReason] = React.useState("")
  const canRemove =
    confirmation === instance.name &&
    Boolean(actor.trim()) &&
    Boolean(reason.trim()) &&
    !mutation.isPending

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setConfirmation("")
      setReason("")
    }
    onOpenChange(nextOpen)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="border border-red-500/30 bg-[var(--el)] shadow-[var(--shdw)] sm:max-w-[520px]">
        <DialogHeader>
          <div className="mb-1 flex size-9 items-center justify-center rounded-lg border border-red-500/30 bg-red-500/10">
            <Trash2 className="size-4 text-red-400" />
          </div>
          <DialogTitle>Remove {instance.display_name || instance.name}</DialogTitle>
          <DialogDescription>
            This stops imports and removes the instance and its agents from active
            views. Imported state and audit history remain retained.
          </DialogDescription>
        </DialogHeader>

        <div className="rounded-lg border border-[var(--sl)] bg-[var(--bg)] p-3 font-hud text-xs text-[var(--mu)]">
          Type <span className="text-[var(--tx)]">{instance.name}</span> to
          confirm.
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
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
        <Field label="Reason" id={`remove-reason-${instance.id}`}>
          <Input
            id={`remove-reason-${instance.id}`}
            value={reason}
            onChange={(event) => setReason(event.target.value)}
            placeholder="Why this instance is no longer needed"
          />
        </Field>

        <DialogFooter className="bg-transparent">
          <Button
            type="button"
            variant="outline"
            onClick={() => handleOpenChange(false)}
          >
            Cancel
          </Button>
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
                    toast.success(
                      `${instance.display_name || instance.name} removed`
                    )
                    handleOpenChange(false)
                  },
                }
              )
            }
          >
            {mutation.isPending ? "Removing" : "Remove instance"}
          </Button>
        </DialogFooter>
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
    <div className="flex flex-col gap-1.5">
      <label className="capcom-eyebrow normal-case tracking-normal" htmlFor={id}>
        {label}
      </label>
      {children}
    </div>
  )
}

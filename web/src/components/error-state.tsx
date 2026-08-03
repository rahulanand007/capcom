"use client"

import Link from "next/link"
import { AlertTriangle, RefreshCw } from "lucide-react"

import { Button } from "@/components/ui/button"

export function ErrorState({
  title = "This view could not be loaded",
  message = "Refresh the view to try again. If the problem continues, check that Capcom and the connected runtime are available.",
  onRetry,
}: {
  title?: string
  message?: string
  onRetry?: () => void
}) {
  return (
    <section className="mx-auto flex min-h-[420px] max-w-xl flex-col items-center justify-center text-center">
      <div className="flex size-11 items-center justify-center rounded-xl border border-[var(--wnd)] bg-[var(--wnd)]/30">
        <AlertTriangle className="size-5 text-[var(--wn)]" />
      </div>
      <div className="capcom-eyebrow mt-5">Console recovery</div>
      <h1 className="mt-2 text-xl font-semibold text-[var(--tx)]">{title}</h1>
      <p className="mt-2 max-w-md text-[13px] leading-6 text-[var(--mu)]">
        {message}
      </p>
      <div className="mt-5 flex gap-2">
        {onRetry && (
          <Button onClick={onRetry}>
            <RefreshCw className="size-3.5" />
            Try again
          </Button>
        )}
        <Button variant="outline" render={<Link href="/" />}>
          Return to overview
        </Button>
      </div>
    </section>
  )
}

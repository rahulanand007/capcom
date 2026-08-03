"use client"

import { useEffect } from "react"

import { ErrorState } from "@/components/error-state"

export default function Error({
  error,
  reset,
}: {
  error: Error & { digest?: string }
  reset: () => void
}) {
  useEffect(() => {
    console.error("Capcom route error", error)
  }, [error])

  return <ErrorState onRetry={reset} />
}

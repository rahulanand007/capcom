"use client"

import * as React from "react"
import {
  MutationCache,
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query"
import { ThemeProvider } from "next-themes"
import { toast } from "sonner"

import { AppShell } from "@/components/app-shell"
import { Toaster } from "@/components/ui/sonner"
import { TooltipProvider } from "@/components/ui/tooltip"
import { toUserFacingError } from "@/lib/errors"

export function AppProviders({ children }: { children: React.ReactNode }) {
  const [queryClient] = React.useState(
    () =>
      new QueryClient({
        queryCache: new QueryCache({
          onError: (error, query) => {
            if (query.state.data === undefined) {
              showRequestError(error)
            }
          },
        }),
        mutationCache: new MutationCache({
          onError: showRequestError,
        }),
        defaultOptions: {
          queries: {
            staleTime: 30_000,
            refetchOnWindowFocus: false,
          },
        },
      })
  )

  React.useEffect(() => {
    const onWindowError = () =>
      showRequestError(new Error("unexpected client error"))
    const onUnhandledRejection = (event: PromiseRejectionEvent) =>
      showRequestError(event.reason)
    window.addEventListener("error", onWindowError)
    window.addEventListener("unhandledrejection", onUnhandledRejection)
    return () => {
      window.removeEventListener("error", onWindowError)
      window.removeEventListener("unhandledrejection", onUnhandledRejection)
    }
  }, [])

  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="dark"
      enableSystem={false}
      disableTransitionOnChange
    >
      <QueryClientProvider client={queryClient}>
        <TooltipProvider delay={250}>
          <AppShell>{children}</AppShell>
          <Toaster position="bottom-right" richColors={false} />
        </TooltipProvider>
      </QueryClientProvider>
    </ThemeProvider>
  )
}

function showRequestError(error: unknown) {
  const failure = toUserFacingError(error)
  toast.error(failure.title, {
    description: failure.message,
  })
}

import { ApiError } from "@/lib/api-client"

export type UserFacingError = {
  title: string
  message: string
  reference?: string
}

export function toUserFacingError(error: unknown): UserFacingError {
  if (error instanceof ApiError) {
    return apiErrorMessage(error)
  }

  if (isNetworkError(error)) {
    return {
      title: "Capcom is unreachable",
      message:
        "Check your connection and confirm the local Capcom services are running, then try again.",
    }
  }

  return {
    title: "Something went wrong",
    message:
      "The console could not complete that action. Refresh the page and try again.",
  }
}

function apiErrorMessage(error: ApiError): UserFacingError {
  const publicError = extractPublicError(error.data)
  if (publicError) {
    return {
      title: titleForCode(publicError.code),
      message: publicError.message,
      reference: extractReference(error.data),
    }
  }
  const raw = extractRawMessage(error.data)
  const reference = extractReference(error.data)

  if (raw.includes("usage:read") || raw.includes("telemetry_not_configured")) {
    return {
      title: "Metrics are unavailable",
      message:
        "Telemetry access is not configured for this runtime. Add usage-read access and try again.",
      reference,
    }
  }

  if (
    raw.includes("connection refused") ||
    raw.includes("runtime_unavailable") ||
    raw.includes("upstream_unavailable")
  ) {
    return {
      title: "Runtime is unreachable",
      message:
        "Confirm the runtime is running and that its endpoint is reachable from Capcom.",
      reference,
    }
  }

  if (raw.includes("confirmation must exactly match")) {
    return {
      title: "Confirmation does not match",
      message: "Enter the exact stable key shown in the dialog, then try again.",
      reference,
    }
  }

  switch (error.status) {
    case 400:
    case 422:
      return {
        title: "Check the information entered",
        message:
          "One or more values are missing or invalid. Review the highlighted fields and try again.",
        reference,
      }
    case 401:
      return {
        title: "Session is not authorized",
        message: "Refresh the console. If this continues, check the Capcom admin token.",
        reference,
      }
    case 403:
      return {
        title: "Action is not allowed",
        message:
          "The current connection does not have permission to perform this action.",
        reference,
      }
    case 404:
      return {
        title: "Item is no longer available",
        message:
          "It may have been removed or changed in another session. Refresh to load the latest state.",
        reference,
      }
    case 409:
      return {
        title: "Action is already in progress",
        message: "Wait for the current operation to finish, then try again.",
        reference,
      }
    case 429:
      return {
        title: "Too many requests",
        message: "Wait a moment before trying again.",
        reference,
      }
    default:
      if (error.status >= 500) {
        return {
          title: "Capcom could not complete the request",
          message:
            "No further action is needed yet. Refresh the page to confirm the latest state, then try again if necessary.",
          reference,
        }
      }
  }

  return {
    title: "Request could not be completed",
    message: "Refresh the page and try again.",
    reference,
  }
}

function extractRawMessage(data: unknown) {
  if (!data || typeof data !== "object") return ""
  if ("error" in data && typeof data.error === "string") {
    return data.error.toLowerCase()
  }
  if (
    "error" in data &&
    data.error &&
    typeof data.error === "object" &&
    "code" in data.error &&
    typeof data.error.code === "string"
  ) {
    return data.error.code.toLowerCase()
  }
  return ""
}

function extractPublicError(data: unknown) {
  if (!data || typeof data !== "object" || !("error" in data)) return undefined
  const detail = data.error
  if (
    !detail ||
    typeof detail !== "object" ||
    !("code" in detail) ||
    !("message" in detail) ||
    typeof detail.code !== "string" ||
    typeof detail.message !== "string"
  ) {
    return undefined
  }
  return { code: detail.code, message: detail.message }
}

function titleForCode(code: string) {
  const titles: Record<string, string> = {
    INVALID_REQUEST: "Check the information entered",
    UNAUTHORIZED: "Session is not authorized",
    FORBIDDEN: "Action is not allowed",
    NOT_FOUND: "Item is no longer available",
    CONFLICT: "Action is already in progress",
    RATE_LIMITED: "Too many requests",
    RUNTIME_UNAVAILABLE: "Runtime is unreachable",
    SERVICE_UNAVAILABLE: "Capcom is temporarily unavailable",
    STORAGE_UNAVAILABLE: "Capcom storage is unavailable",
    TELEMETRY_NOT_CONFIGURED: "Metrics are not configured",
    TELEMETRY_PERMISSION_REQUIRED: "Metrics need additional access",
    CONFIRMATION_MISMATCH: "Confirmation does not match",
    INTERNAL: "Capcom could not complete the request",
  }
  return titles[code] ?? "Request could not be completed"
}

function extractReference(data: unknown) {
  if (!data || typeof data !== "object") return undefined
  for (const key of ["request_id", "requestId"]) {
    if (key in data && typeof data[key as keyof typeof data] === "string") {
      return String(data[key as keyof typeof data])
    }
  }
  return undefined
}

function isNetworkError(error: unknown) {
  if (!(error instanceof Error)) return false
  const message = error.message.toLowerCase()
  return (
    error.name === "TypeError" ||
    message.includes("failed to fetch") ||
    message.includes("network")
  )
}

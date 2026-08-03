package api

import (
	"net/http"
	"strings"
)

type publicErrorResponse struct {
	Error publicErrorDetail `json:"error"`
}

type publicErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func publicError(status int, technical string) publicErrorResponse {
	raw := strings.ToLower(strings.TrimSpace(technical))
	detail := publicErrorDetail{}

	switch {
	case strings.Contains(raw, "confirmation must exactly match"):
		detail = publicErrorDetail{
			Code:    "CONFIRMATION_MISMATCH",
			Message: "Enter the exact stable key shown in the confirmation dialog.",
		}
	case strings.Contains(raw, "usage:read"):
		detail = publicErrorDetail{
			Code:    "TELEMETRY_PERMISSION_REQUIRED",
			Message: "The runtime credential needs the usage:read permission.",
		}
	case strings.Contains(raw, "connection refused"),
		strings.Contains(raw, "runtime_unavailable"),
		strings.Contains(raw, "check gantry"),
		strings.Contains(raw, "call gantry"):
		detail = publicErrorDetail{
			Code:      "RUNTIME_UNAVAILABLE",
			Message:   "The runtime is unreachable. Confirm it is running and its endpoint is correct.",
			Retryable: true,
		}
	case strings.Contains(raw, "database_not_configured"):
		detail = publicErrorDetail{
			Code:      "STORAGE_UNAVAILABLE",
			Message:   "Capcom storage is not ready. Try again after the service finishes starting.",
			Retryable: true,
		}
	case strings.Contains(raw, "telemetry_not_configured"):
		detail = publicErrorDetail{
			Code:    "TELEMETRY_NOT_CONFIGURED",
			Message: "Connect a telemetry source before opening metrics.",
		}
	default:
		detail = publicErrorForStatus(status)
	}

	return publicErrorResponse{Error: detail}
}

func publicErrorForStatus(status int) publicErrorDetail {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return publicErrorDetail{
			Code:    "INVALID_REQUEST",
			Message: "Check the information entered and try again.",
		}
	case http.StatusUnauthorized:
		return publicErrorDetail{
			Code:    "UNAUTHORIZED",
			Message: "The console is not authorized. Refresh the page or check the admin token.",
		}
	case http.StatusForbidden:
		return publicErrorDetail{
			Code:    "FORBIDDEN",
			Message: "The current connection does not allow this action.",
		}
	case http.StatusNotFound:
		return publicErrorDetail{
			Code:    "NOT_FOUND",
			Message: "This item is no longer available. Refresh to load the latest state.",
		}
	case http.StatusConflict:
		return publicErrorDetail{
			Code:      "CONFLICT",
			Message:   "Another operation is already in progress. Wait for it to finish and try again.",
			Retryable: true,
		}
	case http.StatusTooManyRequests:
		return publicErrorDetail{
			Code:      "RATE_LIMITED",
			Message:   "Too many requests were sent. Wait a moment and try again.",
			Retryable: true,
		}
	case http.StatusBadGateway:
		return publicErrorDetail{
			Code:      "RUNTIME_UNAVAILABLE",
			Message:   "The connected runtime did not respond. Confirm it is running and try again.",
			Retryable: true,
		}
	case http.StatusServiceUnavailable:
		return publicErrorDetail{
			Code:      "SERVICE_UNAVAILABLE",
			Message:   "Capcom is temporarily unavailable. Try again shortly.",
			Retryable: true,
		}
	default:
		return publicErrorDetail{
			Code:      "INTERNAL",
			Message:   "Capcom could not complete the request. Refresh to confirm the latest state before trying again.",
			Retryable: true,
		}
	}
}

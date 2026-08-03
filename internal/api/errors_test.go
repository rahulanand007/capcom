package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSONSanitizesTechnicalErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusInternalServerError, errorResponse{
		Error: "database exploded with private implementation detail",
	})

	if strings.Contains(rec.Body.String(), "database exploded") {
		t.Fatalf("response leaked technical error: %s", rec.Body.String())
	}
	var response publicErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error.Code != "INTERNAL" || response.Error.Message == "" {
		t.Fatalf("unexpected public error: %#v", response)
	}
}

func TestPublicErrorMakesRuntimeFailureActionable(t *testing.T) {
	response := publicError(http.StatusBadGateway, "dial tcp 127.0.0.1:8788: connect: connection refused")
	if response.Error.Code != "RUNTIME_UNAVAILABLE" || !response.Error.Retryable {
		t.Fatalf("unexpected runtime error: %#v", response)
	}
	if strings.Contains(response.Error.Message, "127.0.0.1") {
		t.Fatalf("public message leaked endpoint: %q", response.Error.Message)
	}
}

func TestRecoverMiddlewareReturnsSafeError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := recoverMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("private panic detail")
	}), logger)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.Contains(rec.Body.String(), "private panic detail") {
		t.Fatalf("response leaked panic detail: %s", rec.Body.String())
	}
}

package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"capcom/internal/domain"
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

func TestWriteAPIErrorWithPreservesSafeFieldsAndSanitizesError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAPIErrorWith(rec, http.StatusBadGateway, errors.New("GET http://internal.example/v1/health: token secret-value rejected"), map[string]any{"sync_run": map[string]string{"id": "run-1"}})

	if strings.Contains(rec.Body.String(), "internal.example") || strings.Contains(rec.Body.String(), "secret-value") {
		t.Fatalf("response leaked technical error: %s", rec.Body.String())
	}
	var response struct {
		Error   publicErrorDetail `json:"error"`
		SyncRun map[string]string `json:"sync_run"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error.Code != "RUNTIME_UNAVAILABLE" || response.SyncRun["id"] != "run-1" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestRuntimeConnectionResponseSanitizesStoredError(t *testing.T) {
	response := runtimeConnectionResponseFromDomain(domain.RuntimeConnection{
		ID: "runtime-1", Name: "runtime", Kind: domain.RuntimeKindGantry,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		LastError: "GET http://private.internal/v1/health returned token=secret-value",
	})

	if strings.Contains(response.LastError, "private.internal") || strings.Contains(response.LastError, "secret-value") {
		t.Fatalf("stored runtime error leaked: %q", response.LastError)
	}
	if response.LastError == "" {
		t.Fatal("stored runtime error did not produce a public message")
	}
}

func TestSyncResponseSanitizesStoredError(t *testing.T) {
	response := syncRunResponse(domain.RuntimeSyncRun{
		ID: "run-1", ErrorCode: "adapter_failed",
		ErrorMessage: "dial private.internal:8787 with token secret-value",
	})

	message, _ := response["error_message"].(string)
	if strings.Contains(message, "private.internal") || strings.Contains(message, "secret-value") {
		t.Fatalf("stored sync error leaked: %q", message)
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

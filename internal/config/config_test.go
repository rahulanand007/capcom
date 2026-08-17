package config

import (
	"bytes"
	"encoding/base64"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadFromLookupDefaults(t *testing.T) {
	cfg, err := LoadFromLookup(func(string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatalf("LoadFromLookup returned error: %v", err)
	}

	if cfg.HTTP.Addr != defaultHTTPAddr {
		t.Fatalf("HTTP addr = %q, want %q", cfg.HTTP.Addr, defaultHTTPAddr)
	}
	if cfg.HTTP.ReadHeaderTimeout != defaultReadHeaderTimeout {
		t.Fatalf("read header timeout = %s, want %s", cfg.HTTP.ReadHeaderTimeout, defaultReadHeaderTimeout)
	}
	if cfg.HTTP.ShutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("shutdown timeout = %s, want %s", cfg.HTTP.ShutdownTimeout, defaultShutdownTimeout)
	}
	if cfg.Service.Version != defaultServiceVersion {
		t.Fatalf("service version = %q, want %q", cfg.Service.Version, defaultServiceVersion)
	}
	if cfg.Database.URL != "" {
		t.Fatalf("database URL = %q, want empty", cfg.Database.URL)
	}
	if cfg.Database.MaxOpenConns != 10 {
		t.Fatalf("database max open conns = %d, want 10", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 5 {
		t.Fatalf("database max idle conns = %d, want 5", cfg.Database.MaxIdleConns)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("log level = %s, want info", cfg.LogLevel)
	}
}

func TestLoadFromLookupOverrides(t *testing.T) {
	values := map[string]string{
		"CAPCOM_HTTP_ADDR":                "127.0.0.1:9090",
		"CAPCOM_HTTP_READ_HEADER_TIMEOUT": "2s",
		"CAPCOM_HTTP_SHUTDOWN_TIMEOUT":    "3s",
		"CAPCOM_SERVICE_VERSION":          "test-version",
		"CAPCOM_LOG_LEVEL":                "debug",
		"CAPCOM_DATABASE_URL":             "postgres://capcom:capcom@localhost:5432/capcom?sslmode=disable",
		"CAPCOM_DATABASE_MAX_OPEN_CONNS":  "12",
		"CAPCOM_DATABASE_MAX_IDLE_CONNS":  "6",
		"CAPCOM_SECRET_KEY":               base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)),
		"CAPCOM_ADMIN_TOKEN":              "test-admin-token",
	}

	cfg, err := LoadFromLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("LoadFromLookup returned error: %v", err)
	}

	if cfg.HTTP.Addr != "127.0.0.1:9090" {
		t.Fatalf("HTTP addr = %q", cfg.HTTP.Addr)
	}
	if cfg.HTTP.ReadHeaderTimeout != 2*time.Second {
		t.Fatalf("read header timeout = %s", cfg.HTTP.ReadHeaderTimeout)
	}
	if cfg.HTTP.ShutdownTimeout != 3*time.Second {
		t.Fatalf("shutdown timeout = %s", cfg.HTTP.ShutdownTimeout)
	}
	if cfg.Service.Version != "test-version" {
		t.Fatalf("service version = %q", cfg.Service.Version)
	}
	if cfg.Database.URL != "postgres://capcom:capcom@localhost:5432/capcom?sslmode=disable" {
		t.Fatalf("database URL = %q", cfg.Database.URL)
	}
	if cfg.Database.MaxOpenConns != 12 {
		t.Fatalf("database max open conns = %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 6 {
		t.Fatalf("database max idle conns = %d", cfg.Database.MaxIdleConns)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("log level = %s", cfg.LogLevel)
	}
	if len(cfg.Secrets.Key) != 32 {
		t.Fatalf("secret key length = %d", len(cfg.Secrets.Key))
	}
	if cfg.Security.AdminToken != "test-admin-token" {
		t.Fatal("admin token was not loaded")
	}
}

func TestLoadFromLookupRejectsInvalidSecretKey(t *testing.T) {
	_, err := LoadFromLookup(func(key string) (string, bool) {
		if key == "CAPCOM_SECRET_KEY" {
			return base64.StdEncoding.EncodeToString([]byte("too-short")), true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("LoadFromLookup returned nil error")
	}
}

func TestLoadFromLookupRejectsInvalidDuration(t *testing.T) {
	_, err := LoadFromLookup(func(key string) (string, bool) {
		if key == "CAPCOM_HTTP_SHUTDOWN_TIMEOUT" {
			return "nope", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("LoadFromLookup returned nil error")
	}
}

func TestLoadFromLookupRejectsAdminTokenInHostedMode(t *testing.T) {
	values := map[string]string{
		"CAPCOM_DEPLOYMENT_MODE": "hosted",
		"CAPCOM_ADMIN_TOKEN":     "must-not-be-accepted",
	}
	_, err := LoadFromLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err == nil || !strings.Contains(err.Error(), "CAPCOM_ADMIN_TOKEN must be unset") {
		t.Fatalf("error = %v, want hosted admin-token rejection", err)
	}
}

func TestLoadFromLookupRejectsInsecureCookiesInHostedMode(t *testing.T) {
	values := map[string]string{
		"CAPCOM_DEPLOYMENT_MODE": "hosted",
		"CAPCOM_SECURE_COOKIES":  "false",
	}
	_, err := LoadFromLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	})
	if err == nil || !strings.Contains(err.Error(), "CAPCOM_SECURE_COOKIES must be true") {
		t.Fatalf("error = %v, want hosted secure-cookie rejection", err)
	}
}

func TestLoadFromLookupParsesModelCatalog(t *testing.T) {
	cfg, err := LoadFromLookup(func(key string) (string, bool) {
		if key == "CAPCOM_MODEL_CATALOG_JSON" {
			return `[{"provider":"test","model":"model-1","context_window_tokens":128000,"input_cost_per_1m_tokens_usd":"2.5","output_cost_per_1m_tokens_usd":"10","version":"2026-07-29"}]`, true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Telemetry.ModelCatalog) != 1 ||
		cfg.Telemetry.ModelCatalog[0].ContextWindowTokens != 128000 ||
		cfg.Telemetry.ModelCatalog[0].InputCostPer1MTokensUSD != "2.5" {
		t.Fatalf("unexpected model catalog: %#v", cfg.Telemetry.ModelCatalog)
	}
}

func TestLoadUsesDotEnvAndAllowsOSEnvOverride(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	dotEnv := []byte("CAPCOM_HTTP_ADDR=:9090\nCAPCOM_SERVICE_VERSION=from-dotenv\nCAPCOM_LOG_LEVEL=warn\n")
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), dotEnv, 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Setenv("CAPCOM_SERVICE_VERSION", "from-os-env")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTP.Addr != ":9090" {
		t.Fatalf("HTTP addr = %q, want :9090", cfg.HTTP.Addr)
	}
	if cfg.Service.Version != "from-os-env" {
		t.Fatalf("service version = %q, want OS env override", cfg.Service.Version)
	}
	if cfg.LogLevel != slog.LevelWarn {
		t.Fatalf("log level = %s, want warn", cfg.LogLevel)
	}
}

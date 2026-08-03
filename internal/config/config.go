package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddr          = ":8080"
	defaultReadHeaderTimeout = 5 * time.Second
	defaultShutdownTimeout   = 10 * time.Second
	defaultServiceVersion    = "dev"
)

// defaultCORSAllowedOrigins are the browser origins permitted to call the API
// when CAPCOM_CORS_ALLOWED_ORIGINS is unset (the local Next.js console).
var defaultCORSAllowedOrigins = []string{
	"http://localhost:3000",
	"http://127.0.0.1:3000",
}

type Config struct {
	HTTP      HTTPConfig
	Database  DatabaseConfig
	Secrets   SecretConfig
	Security  SecurityConfig
	Service   ServiceConfig
	Sync      SyncConfig
	Telemetry TelemetryConfig
	LogLevel  slog.Level
}

type TelemetryConfig struct {
	WorkerEnabled  bool
	WorkerTick     time.Duration
	RequestTimeout time.Duration
	ModelCatalog   []ModelMetadataConfig
}

type ModelMetadataConfig struct {
	Provider                 string `json:"provider"`
	Model                    string `json:"model"`
	ContextWindowTokens      int64  `json:"context_window_tokens"`
	InputCostPer1MTokensUSD  string `json:"input_cost_per_1m_tokens_usd"`
	OutputCostPer1MTokensUSD string `json:"output_cost_per_1m_tokens_usd"`
	Version                  string `json:"version"`
}

type SyncConfig struct {
	WorkerEnabled    bool
	WorkerTick       time.Duration
	MaxConcurrency   int
	RequestTimeout   time.Duration
	MissingThreshold int
}

type SecretConfig struct {
	Key []byte
}

type SecurityConfig struct {
	AdminToken     string
	AllowedOrigins []string
}

type HTTPConfig struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
}

type ServiceConfig struct {
	Version string
}

type DatabaseConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func Load() (Config, error) {
	dotEnv, err := loadDotEnv(".env")
	if err != nil {
		return Config{}, err
	}
	return LoadFromLookup(mergedLookup(dotEnv, os.LookupEnv))
}

func LoadFromLookup(lookup func(string) (string, bool)) (Config, error) {
	secretKey, err := secretKeyEnv(lookup, "CAPCOM_SECRET_KEY")
	if err != nil {
		return Config{}, err
	}

	readHeaderTimeout, err := durationEnv(lookup, "CAPCOM_HTTP_READ_HEADER_TIMEOUT", defaultReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := durationEnv(lookup, "CAPCOM_HTTP_SHUTDOWN_TIMEOUT", defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}

	logLevel, err := logLevelEnv(lookup, "CAPCOM_LOG_LEVEL", slog.LevelInfo)
	if err != nil {
		return Config{}, err
	}

	connMaxLifetime, err := durationEnv(lookup, "CAPCOM_DATABASE_CONN_MAX_LIFETIME", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}

	maxOpenConns, err := intEnv(lookup, "CAPCOM_DATABASE_MAX_OPEN_CONNS", 10)
	if err != nil {
		return Config{}, err
	}

	maxIdleConns, err := intEnv(lookup, "CAPCOM_DATABASE_MAX_IDLE_CONNS", 5)
	if err != nil {
		return Config{}, err
	}
	workerTick, err := durationEnv(lookup, "CAPCOM_SYNC_WORKER_TICK", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	requestTimeout, err := durationEnv(lookup, "CAPCOM_SYNC_REQUEST_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	maxConcurrency, err := positiveIntEnv(lookup, "CAPCOM_SYNC_MAX_CONCURRENCY", 4)
	if err != nil {
		return Config{}, err
	}
	missingThreshold, err := positiveIntEnv(lookup, "CAPCOM_SYNC_MISSING_THRESHOLD", 3)
	if err != nil {
		return Config{}, err
	}
	workerEnabled, err := boolEnv(lookup, "CAPCOM_SYNC_WORKER_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	telemetryWorkerEnabled, err := boolEnv(lookup, "CAPCOM_TELEMETRY_WORKER_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	telemetryWorkerTick, err := durationEnv(lookup, "CAPCOM_TELEMETRY_WORKER_TICK", time.Minute)
	if err != nil {
		return Config{}, err
	}
	telemetryRequestTimeout, err := durationEnv(lookup, "CAPCOM_TELEMETRY_REQUEST_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	modelCatalog, err := modelCatalogEnv(lookup, "CAPCOM_MODEL_CATALOG_JSON")
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTP: HTTPConfig{
			Addr:              stringEnv(lookup, "CAPCOM_HTTP_ADDR", defaultHTTPAddr),
			ReadHeaderTimeout: readHeaderTimeout,
			ShutdownTimeout:   shutdownTimeout,
		},
		Database: DatabaseConfig{
			URL:             stringEnv(lookup, "CAPCOM_DATABASE_URL", ""),
			MaxOpenConns:    maxOpenConns,
			MaxIdleConns:    maxIdleConns,
			ConnMaxLifetime: connMaxLifetime,
		},
		Secrets: SecretConfig{Key: secretKey},
		Security: SecurityConfig{
			AdminToken:     stringEnv(lookup, "CAPCOM_ADMIN_TOKEN", ""),
			AllowedOrigins: stringListEnv(lookup, "CAPCOM_CORS_ALLOWED_ORIGINS", defaultCORSAllowedOrigins),
		},
		Service: ServiceConfig{
			Version: stringEnv(lookup, "CAPCOM_SERVICE_VERSION", defaultServiceVersion),
		},
		Sync: SyncConfig{
			WorkerEnabled:    workerEnabled,
			WorkerTick:       workerTick,
			MaxConcurrency:   maxConcurrency,
			RequestTimeout:   requestTimeout,
			MissingThreshold: missingThreshold,
		},
		Telemetry: TelemetryConfig{
			WorkerEnabled:  telemetryWorkerEnabled,
			WorkerTick:     telemetryWorkerTick,
			RequestTimeout: telemetryRequestTimeout,
			ModelCatalog:   modelCatalog,
		},
		LogLevel: logLevel,
	}, nil
}

func modelCatalogEnv(lookup func(string) (string, bool), key string) ([]ModelMetadataConfig, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var catalog []ModelMetadataConfig
	if err := json.Unmarshal([]byte(value), &catalog); err != nil {
		return nil, fmt.Errorf("parse %s: %w", key, err)
	}
	for index, item := range catalog {
		if strings.TrimSpace(item.Model) == "" || item.ContextWindowTokens < 0 {
			return nil, fmt.Errorf("%s item %d requires model and non-negative context_window_tokens", key, index)
		}
		for _, price := range []string{item.InputCostPer1MTokensUSD, item.OutputCostPer1MTokensUSD} {
			if price != "" {
				value, valid := new(big.Rat).SetString(price)
				if !valid || value.Sign() < 0 {
					return nil, fmt.Errorf("%s item %d has invalid non-negative pricing", key, index)
				}
			}
		}
	}
	return catalog, nil
}

func boolEnv(lookup func(string) (string, bool), key string, fallback bool) (bool, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func positiveIntEnv(lookup func(string) (string, bool), key string, fallback int) (int, error) {
	value, err := intEnv(lookup, key, fallback)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return value, nil
}

func secretKeyEnv(lookup func(string) (string, bool), key string) ([]byte, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return nil, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("parse %s as base64: %w", key, err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("%s must decode to exactly 32 bytes", key)
	}
	return decoded, nil
}

func stringEnv(lookup func(string) (string, bool), key string, fallback string) string {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func stringListEnv(lookup func(string) (string, bool), key string, fallback []string) []string {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func durationEnv(lookup func(string) (string, bool), key string, fallback time.Duration) (time.Duration, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return duration, nil
}

func intEnv(lookup func(string) (string, bool), key string, fallback int) (int, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}

	var parsed int
	if _, err := fmt.Sscanf(value, "%d", &parsed); err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if parsed < 0 {
		return 0, fmt.Errorf("%s must be zero or positive", key)
	}
	return parsed, nil
}

func logLevelEnv(lookup func(string) (string, bool), key string, fallback slog.Level) (slog.Level, error) {
	value, ok := lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("%s must be one of debug, info, warn, error", key)
	}
}

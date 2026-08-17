package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"capcom/internal/adapters/gantry"
	"capcom/internal/adapters/langgraph"
	"capcom/internal/api"
	"capcom/internal/config"
	"capcom/internal/domain"
	secretcipher "capcom/internal/secrets"
	"capcom/internal/services"
	"capcom/internal/store"
	langsmithtelemetry "capcom/internal/telemetry/langsmith"
	"capcom/internal/workers"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: cfg.LogLevel,
	}))
	slog.SetDefault(logger)

	if err := run(context.Background(), cfg, logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	routerConfig := api.RouterConfig{
		Version:        cfg.Service.Version,
		AdminToken:     cfg.Security.AdminToken,
		AllowedOrigins: cfg.Security.AllowedOrigins,
		SecureCookies:  cfg.Security.SecureCookies,
	}
	var syncWorker *workers.RuntimeSyncWorker
	var telemetryWorker *workers.TelemetryWorker
	if cfg.Database.URL != "" {
		if len(cfg.Secrets.Key) != 32 {
			return fmt.Errorf("CAPCOM_SECRET_KEY is required when CAPCOM_DATABASE_URL is configured")
		}
		db, err := store.OpenPostgres(cfg.Database)
		if err != nil {
			return err
		}
		defer db.Close()

		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			return err
		}

		cipher, err := secretcipher.NewCipher(cfg.Secrets.Key)
		if err != nil {
			return err
		}
		auditRepository := store.NewAuditRepository(db)
		routerConfig.Auth = services.NewAuthService(store.NewAuthRepository(db))
		secretService := services.NewSecretService(store.NewSecretRepository(db), auditRepository, cipher)
		runtimeRepository := store.NewRuntimeConnectionRepository(db)
		gantryAdapter := gantry.NewClient(nil, secretService)
		langGraphAdapter := langgraph.NewClient(nil, secretService)
		langSmithConnector := langsmithtelemetry.NewClient(nil, secretService)
		runtimeService := services.NewRuntimeConnectionService(runtimeRepository, auditRepository).
			WithCredentialResolver(secretService).WithAdapter(gantryAdapter).WithAdapter(langGraphAdapter)
		syncService := services.NewRuntimeSyncService(runtimeRepository, store.NewSyncRepository(db), auditRepository, cfg.Sync.MissingThreshold).
			WithAdapter(gantryAdapter).WithAdapter(langGraphAdapter)
		controlService := services.NewControlActionService(runtimeRepository, store.NewSyncRepository(db), store.NewControlActionRepository(db), auditRepository, syncService).
			WithAdapter(gantryAdapter).WithAdapter(langGraphAdapter)
		telemetryRepository := store.NewTelemetryRepository(db)
		modelMetadata := make([]services.ModelMetadata, 0, len(cfg.Telemetry.ModelCatalog))
		for _, item := range cfg.Telemetry.ModelCatalog {
			modelMetadata = append(modelMetadata, services.ModelMetadata{
				Provider: item.Provider, Model: item.Model,
				ContextWindowTokens:   item.ContextWindowTokens,
				InputCostPer1MTokens:  decimalFromConfig(item.InputCostPer1MTokensUSD),
				OutputCostPer1MTokens: decimalFromConfig(item.OutputCostPer1MTokensUSD),
				Version:               item.Version,
			})
		}
		telemetryService := services.NewTelemetryService(telemetryRepository).
			WithModelMetadataResolver(services.NewModelCatalog(modelMetadata))
		routerConfig.Secrets = secretService
		routerConfig.RuntimeConnections = runtimeService
		routerConfig.RuntimeSync = syncService
		routerConfig.ControlActions = controlService
		routerConfig.Telemetry = telemetryService
		if cfg.Sync.WorkerEnabled {
			syncWorker = workers.NewRuntimeSyncWorker(runtimeService, syncService, cfg.Sync.WorkerTick, cfg.Sync.MaxConcurrency, cfg.Sync.RequestTimeout, logger)
		}
		if cfg.Telemetry.WorkerEnabled {
			telemetryWorker = workers.NewTelemetryWorker(
				runtimeService, telemetryService, telemetryRepository,
				cfg.Telemetry.WorkerTick, cfg.Telemetry.RequestTimeout, logger,
			).WithReader(domain.RuntimeKindGantry, gantryAdapter)
			telemetryWorker.WithReader(domain.RuntimeKindLangGraph, langSmithConnector)
		}
		logger.Info("postgres connected")
	} else {
		logger.Warn("database not configured; runtime connection APIs will return service unavailable")
	}
	if syncWorker != nil {
		go syncWorker.Run(ctx)
	}
	if telemetryWorker != nil {
		go telemetryWorker.Run(ctx)
	}

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           api.NewRouter(routerConfig, logger),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting capcom server", "addr", cfg.HTTP.Addr, "version", cfg.Service.Version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down capcom server")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	select {
	case err := <-errCh:
		return err
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

func decimalFromConfig(value string) *domain.Decimal {
	if value == "" {
		return nil
	}
	decimal := domain.Decimal(value)
	return &decimal
}

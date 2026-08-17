package workers

import (
	"context"
	"log/slog"
	"sync"
	"time"

	runtimeadapter "capcom/internal/adapters/runtime"
	"capcom/internal/domain"
	"capcom/internal/tenant"
)

const telemetrySchemaVersion = "usage-v1"

type TelemetryRuntimeRepository interface {
	List(ctx context.Context) ([]domain.RuntimeConnection, error)
}

type TelemetryIngestor interface {
	Ingest(ctx context.Context, observations []domain.UsageObservation) (accepted, rejected, deduplicated int64, err error)
}

type TelemetryRunRepository interface {
	CreateIngestionRun(ctx context.Context, run domain.TelemetryIngestionRun) (domain.TelemetryIngestionRun, error)
	FinishIngestionRun(ctx context.Context, run domain.TelemetryIngestionRun, runErr error) (domain.TelemetryIngestionRun, error)
}

// TelemetryWorker polls closed, overlapping windows independently from runtime
// inventory synchronization.
type TelemetryWorker struct {
	runtimes       TelemetryRuntimeRepository
	ingestor       TelemetryIngestor
	runs           TelemetryRunRepository
	readers        map[domain.RuntimeKind]runtimeadapter.UsageReader
	tick           time.Duration
	requestTimeout time.Duration
	window         time.Duration
	overlap        time.Duration
	logger         *slog.Logger
}

func NewTelemetryWorker(
	runtimes TelemetryRuntimeRepository,
	ingestor TelemetryIngestor,
	runs TelemetryRunRepository,
	tick, requestTimeout time.Duration,
	logger *slog.Logger,
) *TelemetryWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &TelemetryWorker{
		runtimes: runtimes, ingestor: ingestor, runs: runs,
		readers: make(map[domain.RuntimeKind]runtimeadapter.UsageReader),
		tick:    tick, requestTimeout: requestTimeout,
		window: 5 * time.Minute, overlap: time.Minute, logger: logger,
	}
}

func (w *TelemetryWorker) WithReader(kind domain.RuntimeKind, reader runtimeadapter.UsageReader) *TelemetryWorker {
	w.readers[kind] = reader
	return w
}

func (w *TelemetryWorker) Run(ctx context.Context) {
	w.collectAll(ctx)
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.collectAll(ctx)
		}
	}
}

func (w *TelemetryWorker) collectAll(ctx context.Context) {
	connections, err := w.runtimes.List(ctx)
	if err != nil {
		w.logger.Error("list telemetry runtime connections", "error", err)
		return
	}
	var wg sync.WaitGroup
	for _, connection := range connections {
		reader := w.readers[connection.Kind]
		if reader == nil || !connection.SyncEnabled || connection.Status == domain.RuntimeStatusDisabled {
			continue
		}
		if configurable, ok := reader.(runtimeadapter.UsageReaderConfigurator); ok &&
			!configurable.TelemetryConfigured(connection) {
			continue
		}
		wg.Add(1)
		go func(conn domain.RuntimeConnection, usageReader runtimeadapter.UsageReader) {
			defer wg.Done()
			w.collectOne(ctx, conn, usageReader)
		}(connection, reader)
	}
	wg.Wait()
}

func (w *TelemetryWorker) collectOne(ctx context.Context, conn domain.RuntimeConnection, reader runtimeadapter.UsageReader) {
	ctx = tenant.WithPrincipal(ctx, domain.Principal{OrganizationID: conn.OrganizationID, Organization: domain.Organization{ID: conn.OrganizationID}, Role: "system"})
	now := time.Now().UTC()
	to := now.Truncate(w.window)
	from := to.Add(-w.window - w.overlap)
	source := sourceForRuntime(conn.Kind)
	run, err := w.runs.CreateIngestionRun(ctx, domain.TelemetryIngestionRun{
		RuntimeConnectionID: conn.ID, Source: source, SchemaVersion: telemetrySchemaVersion,
		Cursor: to.Format(time.RFC3339Nano), StartedAt: now,
	})
	if err != nil {
		w.logger.Error("create telemetry ingestion run", "runtime_id", conn.ID, "error", err)
		return
	}

	requestCtx, cancel := context.WithTimeout(ctx, w.requestTimeout)
	observations, collectErr := reader.QueryUsage(requestCtx, conn, domain.UsageQuery{
		From: from, To: to, Interval: w.window, RuntimeConnectionID: conn.ID,
	})
	cancel()
	if collectErr == nil {
		run.Accepted, run.Rejected, run.Deduplicated, collectErr = w.ingestor.Ingest(ctx, observations)
	}
	if _, finishErr := w.runs.FinishIngestionRun(ctx, run, collectErr); finishErr != nil {
		w.logger.Error("finish telemetry ingestion run", "runtime_id", conn.ID, "error", finishErr)
	}
	if collectErr != nil {
		w.logger.Warn("telemetry collection failed; preserving last known observations",
			"runtime_id", conn.ID, "source", source, "error", collectErr)
	}
}

func sourceForRuntime(kind domain.RuntimeKind) domain.UsageSource {
	if kind == domain.RuntimeKindGantry {
		return domain.UsageSourceGantryNative
	}
	return domain.UsageSourceLangSmith
}

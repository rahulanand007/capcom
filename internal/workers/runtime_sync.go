package workers

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"capcom/internal/domain"
	"capcom/internal/services"
	"capcom/internal/tenant"
)

type RuntimeLister interface {
	List(ctx context.Context) ([]domain.RuntimeConnection, error)
}

type RuntimeSyncer interface {
	Sync(ctx context.Context, input services.SyncRuntimeInput) (domain.RuntimeSyncRun, error)
}

type RuntimeSyncWorker struct {
	runtimes       RuntimeLister
	syncer         RuntimeSyncer
	tick           time.Duration
	requestTimeout time.Duration
	semaphore      chan struct{}
	logger         *slog.Logger
	wg             sync.WaitGroup
}

func NewRuntimeSyncWorker(runtimes RuntimeLister, syncer RuntimeSyncer, tick time.Duration, maxConcurrency int, requestTimeout time.Duration, logger *slog.Logger) *RuntimeSyncWorker {
	if tick <= 0 {
		tick = 5 * time.Second
	}
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}
	if requestTimeout <= 0 {
		requestTimeout = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RuntimeSyncWorker{runtimes: runtimes, syncer: syncer, tick: tick, requestTimeout: requestTimeout, semaphore: make(chan struct{}, maxConcurrency), logger: logger}
}

func (w *RuntimeSyncWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.tick)
	defer ticker.Stop()
	w.schedule(ctx)
	for {
		select {
		case <-ctx.Done():
			w.wg.Wait()
			return
		case <-ticker.C:
			w.schedule(ctx)
		}
	}
}

func (w *RuntimeSyncWorker) schedule(ctx context.Context) {
	runtimes, err := w.runtimes.List(ctx)
	if err != nil {
		w.logger.Error("list runtimes for sync worker", "error", err)
		return
	}
	now := time.Now().UTC()
	for _, conn := range runtimes {
		if !conn.SyncEnabled || conn.Status == domain.RuntimeStatusDisabled || conn.AuthRef == "" {
			continue
		}
		interval := time.Duration(conn.SyncIntervalSeconds) * time.Second
		if interval <= 0 {
			interval = 60 * time.Second
		}
		if conn.LastSyncStartedAt != nil && now.Sub(*conn.LastSyncStartedAt) < interval {
			continue
		}
		select {
		case w.semaphore <- struct{}{}:
		default:
			return
		}
		conn := conn
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			defer func() { <-w.semaphore }()
			principal := domain.Principal{OrganizationID: conn.OrganizationID, Organization: domain.Organization{ID: conn.OrganizationID}, Role: "system"}
			runCtx, cancel := context.WithTimeout(tenant.WithPrincipal(ctx, principal), w.requestTimeout)
			defer cancel()
			_, err := w.syncer.Sync(runCtx, services.SyncRuntimeInput{RuntimeConnectionID: conn.ID, Trigger: domain.SyncTriggerScheduled, Actor: "capcom-sync-worker", Reason: "scheduled runtime synchronization"})
			if err != nil && !errors.Is(err, services.ErrSyncConflict) {
				w.logger.Warn("scheduled runtime sync failed", "runtime_connection_id", conn.ID, "error", err)
			}
		}()
	}
}

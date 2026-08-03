package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"capcom/internal/domain"

	"github.com/jackc/pgx/v5/pgconn"
)

// TelemetryRepository persists normalized usage independently of runtime state.
type TelemetryRepository struct {
	db *sql.DB
}

func NewTelemetryRepository(db *sql.DB) TelemetryRepository {
	return TelemetryRepository{db: db}
}

// UpsertObservation replaces a source observation with its latest normalized
// representation. The returned boolean is false when the key already existed.
func (r TelemetryRepository) UpsertObservation(ctx context.Context, observation domain.UsageObservation) (bool, error) {
	attributes, err := json.Marshal(observation.Attributes)
	if err != nil {
		return false, fmt.Errorf("marshal usage attributes: %w", err)
	}
	var inserted bool
	err = r.db.QueryRowContext(ctx, `
INSERT INTO usage_observations (
    source, source_observation_id, runtime_connection_id, runtime_agent_id,
    runtime_execution_id, parent_execution_id, thread_id, model, provider,
    request_count, input_tokens, output_tokens, cached_input_tokens,
    cache_creation_tokens, reasoning_tokens, total_tokens, context_window_tokens,
    context_utilization, input_cost_usd, output_cost_usd, total_cost_usd,
    duration_ms, time_to_first_token_ms, error_count, started_at, ended_at,
    observed_at, model_metadata_version, attributes_json
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21,
    $22, $23, $24, $25, $26, $27, $28, $29
)
ON CONFLICT (source, runtime_connection_id, source_observation_id) DO UPDATE SET
    runtime_agent_id = EXCLUDED.runtime_agent_id,
    runtime_execution_id = EXCLUDED.runtime_execution_id,
    parent_execution_id = EXCLUDED.parent_execution_id,
    thread_id = EXCLUDED.thread_id,
    model = EXCLUDED.model,
    provider = EXCLUDED.provider,
    request_count = EXCLUDED.request_count,
    input_tokens = EXCLUDED.input_tokens,
    output_tokens = EXCLUDED.output_tokens,
    cached_input_tokens = EXCLUDED.cached_input_tokens,
    cache_creation_tokens = EXCLUDED.cache_creation_tokens,
    reasoning_tokens = EXCLUDED.reasoning_tokens,
    total_tokens = EXCLUDED.total_tokens,
    context_window_tokens = EXCLUDED.context_window_tokens,
    context_utilization = EXCLUDED.context_utilization,
    input_cost_usd = EXCLUDED.input_cost_usd,
    output_cost_usd = EXCLUDED.output_cost_usd,
    total_cost_usd = EXCLUDED.total_cost_usd,
    duration_ms = EXCLUDED.duration_ms,
    time_to_first_token_ms = EXCLUDED.time_to_first_token_ms,
    error_count = EXCLUDED.error_count,
    started_at = EXCLUDED.started_at,
    ended_at = EXCLUDED.ended_at,
    observed_at = EXCLUDED.observed_at,
    model_metadata_version = EXCLUDED.model_metadata_version,
    attributes_json = EXCLUDED.attributes_json,
    updated_at = now()
RETURNING (xmax = 0)`,
		observation.Source, observation.SourceObservationID, observation.RuntimeConnectionID,
		observation.RuntimeAgentID, observation.RuntimeExecutionID, observation.ParentExecutionID,
		observation.ThreadID, observation.Model, observation.Provider, observation.RequestCount,
		observation.InputTokens, observation.OutputTokens, observation.CachedInputTokens,
		observation.CacheCreationTokens, observation.ReasoningTokens, observation.TotalTokens,
		observation.ContextWindowTokens, observation.ContextUtilization, observation.InputCostUSD,
		observation.OutputCostUSD, observation.TotalCostUSD, observation.DurationMS,
		observation.TimeToFirstTokenMS, observation.ErrorCount, observation.StartedAt,
		observation.EndedAt, observation.ObservedAt, observation.ModelMetadataVersion, attributes,
	).Scan(&inserted)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23503" {
			return false, fmt.Errorf("%w: %s", domain.ErrUnknownRuntimeConnection, observation.RuntimeConnectionID)
		}
		return false, fmt.Errorf("upsert usage observation: %w", err)
	}
	return inserted, nil
}

func (r TelemetryRepository) CreateIngestionRun(ctx context.Context, run domain.TelemetryIngestionRun) (domain.TelemetryIngestionRun, error) {
	if run.ID == "" {
		id, err := newID()
		if err != nil {
			return run, err
		}
		run.ID = id
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	run.Status = domain.TelemetryRunRunning
	_, err := r.db.ExecContext(ctx, `
INSERT INTO telemetry_ingestion_runs (
    id, runtime_connection_id, source, status, cursor_value, schema_version, started_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		run.ID, run.RuntimeConnectionID, run.Source, run.Status, run.Cursor, run.SchemaVersion, run.StartedAt)
	if err != nil {
		return run, fmt.Errorf("create telemetry ingestion run: %w", err)
	}
	return run, nil
}

func (r TelemetryRepository) FinishIngestionRun(ctx context.Context, run domain.TelemetryIngestionRun, runErr error) (domain.TelemetryIngestionRun, error) {
	finished := time.Now().UTC()
	run.FinishedAt = &finished
	run.Status = domain.TelemetryRunSucceeded
	if runErr != nil {
		run.Status = domain.TelemetryRunFailed
		run.LastError = runErr.Error()
	} else {
		run.LastSuccessfulAt = &finished
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE telemetry_ingestion_runs SET
    status = $2, cursor_value = $3, finished_at = $4,
    last_successful_at = CASE WHEN $2 = 'succeeded' THEN $4 ELSE last_successful_at END,
    records_accepted = $5, records_rejected = $6, records_deduplicated = $7,
    last_error = $8
WHERE id = $1`, run.ID, run.Status, run.Cursor, finished, run.Accepted, run.Rejected, run.Deduplicated, run.LastError)
	if err != nil {
		return run, fmt.Errorf("finish telemetry ingestion run: %w", err)
	}
	return run, nil
}

func (r TelemetryRepository) LatestIngestionRun(ctx context.Context, runtimeID string) (domain.TelemetryIngestionRun, error) {
	var run domain.TelemetryIngestionRun
	err := r.db.QueryRowContext(ctx, `
SELECT id, runtime_connection_id, source, status, cursor_value, schema_version,
       started_at, finished_at, last_successful_at, records_accepted,
       records_rejected, records_deduplicated, last_error
FROM telemetry_ingestion_runs
WHERE runtime_connection_id = $1
ORDER BY started_at DESC LIMIT 1`, runtimeID).Scan(
		&run.ID, &run.RuntimeConnectionID, &run.Source, &run.Status, &run.Cursor,
		&run.SchemaVersion, &run.StartedAt, &run.FinishedAt, &run.LastSuccessfulAt,
		&run.Accepted, &run.Rejected, &run.Deduplicated, &run.LastError,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			var observedAt time.Time
			fallbackErr := r.db.QueryRowContext(ctx, `
SELECT runtime_connection_id, source, MAX(observed_at)
FROM usage_observations
WHERE runtime_connection_id = $1
GROUP BY runtime_connection_id, source
ORDER BY CASE source
    WHEN 'gantry_native' THEN 1 WHEN 'langsmith' THEN 2 WHEN 'otel' THEN 3 ELSE 99 END
LIMIT 1`, runtimeID).Scan(&run.RuntimeConnectionID, &run.Source, &observedAt)
			if errors.Is(fallbackErr, sql.ErrNoRows) {
				return run, nil
			}
			if fallbackErr != nil {
				return run, fmt.Errorf("get observation-derived telemetry health: %w", fallbackErr)
			}
			run.ID = "observation-derived"
			run.Status = domain.TelemetryRunSucceeded
			run.SchemaVersion = "usage-v1"
			run.StartedAt = observedAt
			run.FinishedAt = &observedAt
			run.LastSuccessfulAt = &observedAt
			return run, nil
		}
		return run, fmt.Errorf("get telemetry health: %w", err)
	}
	return run, nil
}

func (r TelemetryRepository) Summary(ctx context.Context, query domain.UsageQuery) (domain.UsageSummary, error) {
	summary := domain.UsageSummary{
		RuntimeConnectionID: query.RuntimeConnectionID,
		AgentID:             query.AgentID,
		From:                query.From,
		To:                  query.To,
		Source:              query.Source,
	}
	var source sql.NullString
	var cost sql.NullString
	var average, p95, errorRate, contextUtilization sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `
WITH scoped AS (
    SELECT u.*
    FROM usage_observations u
    LEFT JOIN agent_runtime_bindings b
      ON b.runtime_connection_id = u.runtime_connection_id
     AND b.runtime_agent_id = u.runtime_agent_id
    WHERE u.observed_at >= $1 AND u.observed_at < $2
      AND ($3 = '' OR u.runtime_connection_id::text = $3)
      AND ($4 = '' OR b.agent_id::text = $4)
      AND ($5 = '' OR u.model = $5)
      AND ($6 = '' OR u.runtime_execution_id = $6)
), selected_sources AS (
    SELECT runtime_connection_id, COALESCE(NULLIF($7, ''), (
        array_agg(source ORDER BY CASE source
            WHEN 'gantry_native' THEN 1
            WHEN 'langsmith' THEN 2
            WHEN 'otel' THEN 3
            ELSE 99 END))[1]
    ) AS source
    FROM scoped
    GROUP BY runtime_connection_id
)
SELECT
    CASE WHEN COUNT(DISTINCT s.source) = 1 THEN MAX(s.source) END,
    COALESCE(SUM(s.request_count), 0),
    COALESCE(SUM(s.input_tokens), 0),
    COALESCE(SUM(s.output_tokens), 0),
    COALESCE(SUM(s.cached_input_tokens), 0),
    COALESCE(SUM(s.cache_creation_tokens), 0),
    COALESCE(SUM(s.reasoning_tokens), 0),
    COALESCE(SUM(s.total_tokens), 0),
    SUM(s.total_cost_usd)::text,
    AVG(s.duration_ms),
    percentile_cont(0.95) WITHIN GROUP (ORDER BY s.duration_ms),
    CASE WHEN SUM(s.request_count) > 0
         THEN SUM(s.error_count)::double precision / SUM(s.request_count) END,
    AVG(s.context_utilization),
    MAX(s.observed_at)
FROM scoped s
JOIN selected_sources chosen
  ON chosen.runtime_connection_id = s.runtime_connection_id
 AND chosen.source = s.source`,
		query.From, query.To, query.RuntimeConnectionID, query.AgentID, query.Model,
		query.RuntimeExecutionID, query.Source,
	).Scan(
		&source, &summary.RequestCount, &summary.InputTokens, &summary.OutputTokens,
		&summary.CachedInputTokens, &summary.CacheCreationTokens, &summary.ReasoningTokens,
		&summary.TotalTokens, &cost, &average, &p95, &errorRate, &contextUtilization,
		&summary.LastObservedAt,
	)
	if err != nil {
		return summary, fmt.Errorf("summarize telemetry: %w", err)
	}
	if source.Valid {
		summary.Source = domain.UsageSource(source.String)
		summary.Available = true
		summary.Configured = true
	}
	configured, err := r.telemetryConfigured(ctx, query)
	if err != nil {
		return summary, err
	}
	summary.Configured = configured || summary.Available
	if cost.Valid {
		value := domain.Decimal(cost.String)
		summary.EstimatedCostUSD = &value
	}
	if average.Valid {
		summary.AverageDurationMS = &average.Float64
	}
	if p95.Valid {
		summary.P95DurationMS = &p95.Float64
	}
	if errorRate.Valid {
		summary.ErrorRate = &errorRate.Float64
	}
	if contextUtilization.Valid {
		summary.ContextUtilization = &contextUtilization.Float64
	}
	if summary.Available {
		models, err := r.modelBreakdown(ctx, query, summary.Source)
		if err != nil {
			return summary, err
		}
		summary.ModelBreakdown = models
		series, err := r.timeSeries(ctx, query, summary.Source)
		if err != nil {
			return summary, err
		}
		summary.TimeSeries = series
	}
	return summary, nil
}

func (r TelemetryRepository) telemetryConfigured(ctx context.Context, query domain.UsageQuery) (bool, error) {
	var configured bool
	err := r.db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM telemetry_ingestion_runs t
    WHERE ($1 = '' OR t.runtime_connection_id::text = $1)
      AND ($2 = '' OR t.runtime_connection_id = (
          SELECT runtime_connection_id FROM agent_runtime_bindings WHERE agent_id::text = $2
      ))
      AND ($3 = '' OR t.source = $3)
)`, query.RuntimeConnectionID, query.AgentID, query.Source).Scan(&configured)
	if err != nil {
		return false, fmt.Errorf("check telemetry configuration: %w", err)
	}
	return configured, nil
}

func (r TelemetryRepository) modelBreakdown(ctx context.Context, query domain.UsageQuery, _ domain.UsageSource) ([]domain.ModelUsageSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
WITH scoped AS (
    SELECT u.* FROM usage_observations u
    LEFT JOIN agent_runtime_bindings b
      ON b.runtime_connection_id = u.runtime_connection_id AND b.runtime_agent_id = u.runtime_agent_id
    WHERE u.observed_at >= $1 AND u.observed_at < $2
      AND ($3 = '' OR u.runtime_connection_id::text = $3)
      AND ($4 = '' OR b.agent_id::text = $4)
      AND ($5 = '' OR u.model = $5)
      AND ($6 = '' OR u.runtime_execution_id = $6)
), selected_sources AS (
    SELECT runtime_connection_id, COALESCE(NULLIF($7, ''), (
        array_agg(source ORDER BY CASE source
            WHEN 'gantry_native' THEN 1 WHEN 'langsmith' THEN 2
            WHEN 'otel' THEN 3 ELSE 99 END))[1]) AS source
    FROM scoped GROUP BY runtime_connection_id
)
SELECT s.model, SUM(s.request_count), SUM(s.input_tokens), SUM(s.output_tokens), SUM(s.total_tokens)
FROM scoped s JOIN selected_sources chosen
  ON chosen.runtime_connection_id = s.runtime_connection_id AND chosen.source = s.source
GROUP BY s.model
ORDER BY SUM(s.total_tokens) DESC, s.model
LIMIT 50`, query.From, query.To, query.RuntimeConnectionID, query.AgentID, query.Model,
		query.RuntimeExecutionID, query.Source)
	if err != nil {
		return nil, fmt.Errorf("summarize telemetry by model: %w", err)
	}
	defer rows.Close()
	items := make([]domain.ModelUsageSummary, 0)
	for rows.Next() {
		var item domain.ModelUsageSummary
		if err := rows.Scan(&item.Model, &item.RequestCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan telemetry model summary: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r TelemetryRepository) timeSeries(ctx context.Context, query domain.UsageQuery, _ domain.UsageSource) ([]domain.UsageTimeBucket, error) {
	interval := telemetryInterval(query.Interval)
	rows, err := r.db.QueryContext(ctx, `
WITH scoped AS (
    SELECT u.* FROM usage_observations u
    LEFT JOIN agent_runtime_bindings b
      ON b.runtime_connection_id = u.runtime_connection_id AND b.runtime_agent_id = u.runtime_agent_id
    WHERE u.observed_at >= $1 AND u.observed_at < $2
      AND ($3 = '' OR u.runtime_connection_id::text = $3)
      AND ($4 = '' OR b.agent_id::text = $4)
      AND ($5 = '' OR u.model = $5)
      AND ($6 = '' OR u.runtime_execution_id = $6)
), selected_sources AS (
    SELECT runtime_connection_id, COALESCE(NULLIF($7, ''), (
        array_agg(source ORDER BY CASE source
            WHEN 'gantry_native' THEN 1 WHEN 'langsmith' THEN 2
            WHEN 'otel' THEN 3 ELSE 99 END))[1]) AS source
    FROM scoped GROUP BY runtime_connection_id
)
SELECT date_bin($8::interval, s.observed_at, '2000-01-01T00:00:00Z'::timestamptz),
       SUM(s.request_count), SUM(s.input_tokens), SUM(s.output_tokens), SUM(s.total_tokens)
FROM scoped s JOIN selected_sources chosen
  ON chosen.runtime_connection_id = s.runtime_connection_id AND chosen.source = s.source
GROUP BY 1
ORDER BY 1`, query.From, query.To, query.RuntimeConnectionID, query.AgentID, query.Model,
		query.RuntimeExecutionID, query.Source, interval)
	if err != nil {
		return nil, fmt.Errorf("summarize telemetry time series: %w", err)
	}
	defer rows.Close()
	items := make([]domain.UsageTimeBucket, 0)
	for rows.Next() {
		var item domain.UsageTimeBucket
		if err := rows.Scan(&item.StartedAt, &item.RequestCount, &item.InputTokens, &item.OutputTokens, &item.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan telemetry time bucket: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func telemetryInterval(interval time.Duration) string {
	switch interval {
	case 5 * time.Minute:
		return "5 minutes"
	case 15 * time.Minute:
		return "15 minutes"
	case 6 * time.Hour:
		return "6 hours"
	case 24 * time.Hour:
		return "1 day"
	default:
		return "1 hour"
	}
}

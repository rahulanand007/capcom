CREATE TABLE IF NOT EXISTS usage_observations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source text NOT NULL,
    source_observation_id text NOT NULL,
    runtime_connection_id uuid NOT NULL REFERENCES runtime_connections (id) ON DELETE CASCADE,
    runtime_agent_id text NOT NULL DEFAULT '',
    runtime_execution_id text NOT NULL DEFAULT '',
    parent_execution_id text NOT NULL DEFAULT '',
    thread_id text NOT NULL DEFAULT '',
    model text NOT NULL DEFAULT '',
    provider text NOT NULL DEFAULT '',
    request_count bigint NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    input_tokens bigint NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens bigint NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    cached_input_tokens bigint NOT NULL DEFAULT 0 CHECK (cached_input_tokens >= 0),
    cache_creation_tokens bigint NOT NULL DEFAULT 0 CHECK (cache_creation_tokens >= 0),
    reasoning_tokens bigint NOT NULL DEFAULT 0 CHECK (reasoning_tokens >= 0),
    total_tokens bigint NOT NULL DEFAULT 0 CHECK (total_tokens >= 0),
    context_window_tokens bigint,
    context_utilization double precision,
    input_cost_usd numeric(20, 10),
    output_cost_usd numeric(20, 10),
    total_cost_usd numeric(20, 10),
    duration_ms bigint,
    time_to_first_token_ms bigint,
    error_count bigint NOT NULL DEFAULT 0 CHECK (error_count >= 0),
    started_at timestamptz,
    ended_at timestamptz,
    observed_at timestamptz NOT NULL,
    model_metadata_version text NOT NULL DEFAULT '',
    attributes_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source, runtime_connection_id, source_observation_id)
);

CREATE INDEX IF NOT EXISTS idx_usage_observations_agent_time
    ON usage_observations (runtime_connection_id, runtime_agent_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_observations_execution
    ON usage_observations (runtime_connection_id, runtime_execution_id);
CREATE INDEX IF NOT EXISTS idx_usage_observations_model_time
    ON usage_observations (runtime_connection_id, model, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_usage_observations_time
    ON usage_observations (observed_at DESC);

CREATE TABLE IF NOT EXISTS telemetry_ingestion_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    runtime_connection_id uuid NOT NULL REFERENCES runtime_connections (id) ON DELETE CASCADE,
    source text NOT NULL,
    status text NOT NULL,
    cursor_value text NOT NULL DEFAULT '',
    schema_version text NOT NULL,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    last_successful_at timestamptz,
    records_accepted bigint NOT NULL DEFAULT 0,
    records_rejected bigint NOT NULL DEFAULT 0,
    records_deduplicated bigint NOT NULL DEFAULT 0,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_telemetry_ingestion_runtime_source
    ON telemetry_ingestion_runs (runtime_connection_id, source, started_at DESC);

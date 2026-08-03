# 18 - Telemetry And Metrics

## Status

This document is the V1 implementation contract for usage telemetry. Telemetry
is non-authoritative runtime data: it measures runtime behavior but never
defines agents, access, desired state, or control capabilities.

LangSmith and OpenTelemetry are telemetry connectors, not runtime kinds.
Runtime adapters remain responsible for inventory, access, and control.

## Normalized Contract

`domain.UsageObservation` is the stored unit. Each observation contains:

- source and source observation ID;
- runtime connection, runtime agent, execution, parent execution, and thread IDs;
- model and provider;
- request, input, output, cached-input, cache-creation, reasoning, and total tokens;
- optional exact costs, duration, time to first token, and error count;
- optional model context window and estimated context utilization;
- start, end, and observation timestamps;
- allowlisted source attributes and model metadata version.

`total_tokens` is the source total when supplied, otherwise
`input_tokens + output_tokens`. `context_utilization` is
`input_tokens / context_window_tokens` and is always labeled estimated.
Unknown context windows produce `null`.

Prompt, completion, input, and output bodies are not part of the domain
contract and are removed at ingestion.

## Storage

Migration `010_telemetry.sql` adds:

- `usage_observations`, with typed query columns, JSONB allowlisted attributes,
  and uniqueness on `(source, runtime_connection_id, source_observation_id)`;
- `telemetry_ingestion_runs`, with connector cursor, schema version, counts,
  latest success, and error state.

Observations are upserted idempotently. Connector failure records health but
does not delete or zero previously accepted observations. Usage is not stored
on `runtime_executions`, because one execution can contain many model calls.

## Sources And Precedence

One source is selected per query scope:

1. `gantry_native`
2. `langsmith`
3. `otel`

An explicit `source` query parameter overrides selection. Values from multiple
sources are never added together. Gantry native `/v1/usage` is preferred for
Gantry. LangSmith is preferred for LangGraph when configured, with direct OTEL
as the alternative.

No observations means unavailable/not configured, not measured zero.

## Collection

Gantry implements the optional runtime-neutral `UsageReader` interface and
queries `GET /v1/usage`. The worker polls a closed five-minute interval with a
one-minute overlap. Gantry credentials require `usage:read` in addition to
inventory scopes.

LangSmith configuration is read from runtime metadata or labels:

- `langsmith_project`
- `langsmith_auth_ref`
- `langsmith_api_url` (optional)

The connector queries `POST /runs/query`, imports LLM child runs, reads
`usage_metadata`, and correlates `langgraph_assistant_id`,
`langgraph_thread_id`, and `langgraph_run_id`. The application must emit those
metadata fields. If the project and auth reference are absent, no collection
run is created and telemetry remains `not_configured`.

OTLP/HTTP protobuf (the collector default) and JSON traces are accepted at
`POST /v1/telemetry/otlp/v1/traces`. The V1 receiver:

- uses the API's bearer authentication;
- limits requests to 4 MiB;
- requires `capcom.runtime_connection.id` and a span ID;
- maps standard `gen_ai.usage.*`, model, conversation, and Capcom correlation
  attributes;
- persists only a small non-content attribute allowlist;
- uses normalization schema `otel-genai-v1`.

Production deployments should place an OpenTelemetry Collector in front of
Capcom for batching, retry, redaction, authentication, and backpressure.
Direct gRPC is post-V1.

## Read API

- `GET /v1/metrics/summary`
- `GET /v1/agents/{id}/metrics`
- `GET /v1/runtime-instances/{id}/metrics`
- `GET /v1/runtime-instances/{id}/telemetry-health`

Metrics queries accept `from`, `to`, `interval`, `model`, `source`, and
`runtime_execution_id`. Times must be RFC3339, ranges are limited to 90 days,
and intervals are `5m`, `15m`, `1h`, `6h`, or `1d`.

Responses expose source, availability, last observation, usage, performance,
context quality, model breakdown, and time series.

Telemetry-health responses expose a stable error code and safe operator message.
Raw connector errors, upstream response bodies, credentials, and internal URLs
are retained for server logs only and are never returned to the console.

## Console Contract

The primary navigation includes a dedicated **Metrics** route. It presents
fleet-wide request, token, model, latency, and estimated-cost summaries for
24-hour, 7-day, and 30-day ranges. Token, request, and model charts are
interactive: opening a chart shows an expanded visualization and the exact
bucket or model breakdown behind it.

The Metrics route refreshes the summary and connector health every 30 seconds,
continues refreshing in a background tab, displays the last successful UI
refresh time, and provides an explicit **Refresh now** action. This live view
does not change the collection grain: Gantry observations still arrive from
closed five-minute windows polled by the backend worker.

Agent detail retains its scoped Observability summary for in-context diagnosis.
When telemetry is unavailable, both surfaces explain whether collection is not
configured or temporarily unavailable and preserve the last known data.

## Model Metadata

`CAPCOM_MODEL_CATALOG_JSON` accepts an array of entries containing `provider`,
`model`, `context_window_tokens`, `input_cost_per_1m_tokens_usd`,
`output_cost_per_1m_tokens_usd`, and `version`. Runtime-provided metadata takes
precedence when a connector supplies it; otherwise this configured catalog is
used. Missing context or pricing remains `null`.

Costs derived from the catalog are stored on the observation with
`cost_quality=estimated` and the catalog version. Existing observations are not
rewritten when configuration changes.

## Privacy And Retention

- Content is never stored by default.
- Source attributes are allowlisted.
- Raw telemetry payloads are not persisted.
- Runtime keys remain secret references and are resolved only for requests.
- V1 retains normalized observations until an operator/database retention
  policy removes them. Automated retention is post-V1.
- Historical estimated costs retain the model metadata version used; catalog
  changes do not silently rewrite observations.

## Security And Isolation

Read-only runtime mode has no effect on telemetry reads and still rejects all
runtime control actions. Telemetry failures do not fail runtime inventory
synchronization. Runtime connection IDs are database foreign keys, so unknown
IDs are rejected on persistence.

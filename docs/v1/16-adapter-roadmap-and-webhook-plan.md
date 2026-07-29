# 16 - Adapter Roadmap And Webhook Plan

## Status And Scope

This document records the post-V1 integration direction as of 2026-07-21. It
does not move webhooks or additional adapters into the V1 definition of done.
Polling remains the authoritative V1 synchronization mechanism.

## Gantry Completion Assessment

The Gantry adapter has completed the central V1 integration path, but the full
Gantry contract is not complete.

| Area | Status | Current behavior or remaining work |
|---|---|---|
| Authentication and secret handling | Implemented | Encrypted secret references are resolved into Bearer tokens per request. |
| Connection verification | Implemented | `GET /v1/health` and normalized `/v1/doctor` checks drive active/degraded/failed state. |
| Durable agent inventory | Implemented | Agents are imported and isolated by runtime connection. |
| Skills and effective access | Implemented | Bindings are joined with the skill catalog; `/access` is normalized. |
| Main, registered, and delegated execution modeling | Implemented | Durable agents remain distinct from ephemeral delegated task executions. |
| Snapshot synchronization | Implemented | Manual and periodic sync preserve last-known state and prevent overlap. |
| Access control mutation | Implemented | Read-only rejection, dry run, audit, and `PUT /access` are supported. |
| Agent enable/disable | Implemented | Audited generic status control maps to `PATCH /v1/agents/{agentId}` with dry-run and verification sync. |
| Runtime doctor, inventory, and capabilities | Implemented | Gantry diagnostics, tools, skills, MCP servers, and capability manifests are normalized and persisted. |
| Callable-agent delegation | Implemented | `/v1/agents/{id}/delegates` is normalized as directed many-to-many edges with revision and resolution provenance. |
| Incremental run/event collection | Partial | Current collection reads recent runs and events without a durable cursor; add pagination and checkpoints. |
| Stable agent hierarchy | Blocked by source contract | Keep capability false until Gantry exposes stable parent identity. |
| Session approval interactions | Post-V1 | Gantry exposes pending interactions and decisions, but Capcom V1 reserves approval workflows for Phase 2. |
| Signed inbound webhooks | Planned | Implement the durable inbox and targeted-sync design below. |
| Contract regression suite | Partial | Expand HTTP tests with versioned recorded fixtures and an optional live compatibility test. |

Therefore, the next Gantry work should be a bounded hardening slice, not a new
adapter rewrite. Product-level V1 also still needs desired-state manifest apply
and drift detection, as tracked by the build plan.

## Planned Gantry Webhook Receiver

Gantry webhooks are an acceleration and notification path. They do not replace
periodic reconciliation, because delivery can be delayed, duplicated, filtered,
or exhausted after retries.

### Delivery Flow

1. An operator creates a per-runtime webhook signing secret in Capcom.
2. Capcom exposes `POST /v1/runtime-instances/{id}/webhooks/gantry`.
3. The operator registers that URL and event selection through Gantry's
   `POST /v1/webhooks` API and supplies the same secret. Registration requires
   a Gantry credential with `webhooks:write`; delivery to Capcom uses the HMAC
   signature, not that Control API credential.
4. Gantry signs the raw request body and sends its webhook ID, event type,
   timestamp, signature, and correlation ID headers.
5. Capcom verifies the signature against the raw body before JSON decoding,
   rejects stale timestamps, applies a body-size limit, and identifies the
   runtime solely from the route and stored configuration.
6. Capcom durably inserts the event into a webhook inbox with a unique key on
   `(runtime_connection_id, delivery_key)`. Use a source event/delivery ID when
   available; otherwise derive a stable payload fingerprint.
7. Capcom returns `202 Accepted` after the insert commits. Duplicate deliveries
   return a successful idempotent response.
8. A worker processes the inbox record and requests a coalesced targeted sync
   for the affected runtime, agent, run, or job.
9. The normal adapter reads authoritative state from Gantry and persists a new
   snapshot. Periodic full sync remains the repair path for missed events.

### Security And Reliability Rules

- Use a distinct webhook signing secret; never reuse a Gantry Control API key.
- Verify HMAC-SHA-256 over the exact raw bytes and use constant-time comparison.
- Follow Gantry's signing contract for `x-gantry-webhook-id`,
  `x-gantry-webhook-timestamp`, `x-gantry-webhook-event`, and
  `x-gantry-webhook-signature`; use `x-gantry-correlation-id` for tracing when
  present.
- Reject timestamps outside a configured replay window.
- Do not trust runtime IDs, agent IDs, or callback URLs from the payload to
  select credentials or destinations.
- Store event metadata and the minimum payload needed for audit/debugging;
  redact known credential fields and apply retention limits.
- Treat webhook input as a read/sync trigger only. A webhook must never execute
  a runtime mutation directly.
- Coalesce bursts so many events for one runtime produce one targeted sync.
- Track `received`, `processing`, `processed`, and `failed` inbox states with
  bounded retries and operator-visible failure counts.
- Preserve periodic sync for eventual convergence and recovery from Gantry's
  delivery dead-letter state.

### Planned Data And API Surface

- `runtime_webhook_secrets`: encrypted signing-secret reference and rotation metadata.
- `runtime_webhook_events`: delivery key, runtime connection, event type,
  correlation ID, timestamps, payload, processing state, attempts, and error.
- `POST /v1/runtime-instances/{id}/webhooks/gantry`: authenticated Gantry delivery endpoint.
- `GET /v1/runtime-instances/{id}/webhook-events`: operator diagnostics.
- Secret create/rotate endpoint using the same actor, reason, encryption, and
  audit rules as runtime credentials.

## Adapter Selection Model

Capcom should distinguish three integration classes:

| Class | Meaning | Capcom integration |
|---|---|---|
| Managed runtime | Vendor exposes inventory, lifecycle, execution, and identity APIs | Full runtime adapter |
| Agent server/platform | A stable server API represents assistants, threads, runs, and tools | Full or read/control-limited runtime adapter |
| Embedded framework | Agent code runs inside an application with no fleet management API | Registration SDK/sidecar plus OpenTelemetry ingestion |

MCP and A2A are interoperability protocols, not fleet-management APIs. They can
enrich tool and relationship data but should not be presented as runtime
adapters by themselves. Langfuse, Phoenix, and observability-only LangSmith
connections are telemetry connectors rather than authoritative runtime adapters.

## Recommended Adapter Sequence

### 1. LangGraph Agent Server

**Decision: selected and implemented as a read/control adapter.**

Build this next after the Gantry hardening slice. Agent Server has a documented
HTTP API for assistants, threads, runs, cron jobs, store, A2A, MCP, and system
health. It can run as LangSmith cloud, hybrid Kubernetes, a fully self-hosted
platform, or a standalone server. That makes it the fastest way to prove that
Capcom's adapter boundary works beyond Gantry without first building cloud IAM
machinery.

Initial support level:

- inventory: assistants/graphs and deployment metadata
- executions: threads, runs, and run status
- health: system endpoints
- tools: MCP metadata where available
- control: assistant deletion and active-run interruption are implemented;
  invocation remains deferred
- access governance: report unsupported unless the deployment exposes a stable equivalent

The completed implementation includes inventory and selected controls:

1. Add a `langgraph` runtime kind and adapter registration.
2. Support standalone/self-hosted Agent Server base URLs and `X-Api-Key`
   authentication without introducing LangGraph types into the domain.
3. Check system health and identify server/deployment metadata.
4. Import assistants as durable Capcom agents, preserving graph and version
   identifiers in runtime metadata.
5. Import recent threads and runs as execution records with pagination.
6. Declare unsupported capabilities explicitly, especially hierarchy, skills,
   effective access replacement, and status mutation.
7. Add recorded JSON fixtures, normalization tests, auth/error tests, and an
   optional live Agent Server contract test.

Assistant deletion and run cancellation use the common audited control-action
service. Invocation, cron mutation, thread deletion, rollback cancellation, and
deployment lifecycle controls remain deferred.

The implemented contract and local verification runbook are in
`17-langgraph-agent-server-adapter.md`.

Primary references:

- [Agent Server API](https://docs.langchain.com/langsmith/server-api-ref)
- [Agent Server architecture](https://docs.langchain.com/langsmith/agent-server)
- [Self-hosted deployment options](https://docs.langchain.com/langsmith/self-hosted)

### 2. Amazon Bedrock AgentCore

Prioritize this first instead when the initial enterprise customers are AWS
accounts. AgentCore Runtime is framework-neutral managed hosting, and its
control plane exposes list/get/version/endpoint operations while the data plane
supports invocation. Gateway, Identity, Memory, policy, and CloudWatch
observability are useful Capcom inventory and governance inputs.

The adapter should use AWS SDK for Go v2 and normal credential-provider chains;
Capcom must not ask users to paste long-lived AWS access keys into a runtime
token field.

Primary references:

- [AgentCore Runtime model](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/harness-vs-runtime.html)
- [AgentCore control-plane actions](https://docs.aws.amazon.com/bedrock-agentcore-control/latest/APIReference/API_Operations.html)
- [AgentCore observability](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/observability-configure.html)

### 3. Microsoft Foundry Agent Service

Foundry is a strong enterprise adapter for Azure-heavy customers. It supports
prompt agents and containerized hosted agents, immutable versions, dedicated
endpoints and identities, sessions, tools, private networking, and a REST
lifecycle surface. Authentication should prefer Microsoft Entra workload
identity or managed identity over stored API keys.

Primary references:

- [Foundry REST API](https://learn.microsoft.com/en-us/rest/api/microsoft-foundry/aiproject)
- [Hosted agents](https://learn.microsoft.com/en-us/azure/foundry/agents/concepts/hosted-agents)
- [Private networking](https://learn.microsoft.com/en-us/azure/foundry/agents/how-to/virtual-networks)

### 4. Vertex AI Agent Engine

Agent Engine is the corresponding GCP managed-runtime target. It exposes
project/location-scoped reasoning engine inventory, sessions, pagination, and
managed scaling, and integrates with Cloud Trace, Monitoring, and Logging.
Authentication should use Application Default Credentials and workload
identity federation.

Primary references:

- [Agent Engine overview](https://cloud.google.com/vertex-ai/generative-ai/docs/reasoning-engine/overview)
- [Reasoning engines list API](https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/v1beta1/projects.locations.reasoningEngines/list)
- [Session management API](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/agent-engine/sessions/manage-sessions-api)

### Horizontal Connector: OpenTelemetry

Implement an OTLP/collector integration alongside the second external runtime
adapter. It will cover embedded agents running in Kubernetes, ECS, VMs, or
ordinary application services where there is no discoverable agent control
API. OpenTelemetry GenAI conventions include agent, conversation, workflow,
tool, model, and usage attributes, but the conventions are still evolving, so
Capcom must version its normalization and avoid making telemetry the source of
truth for access controls.

Primary reference:

- [OpenTelemetry GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/registry/attributes/gen-ai/)

## Enterprise Deployment Pattern

There is no reliable public evidence that one agent runtime or adapter has a
majority of enterprise deployments. The current vendor documentation instead
shows a heterogeneous market with three recurring patterns:

1. Cloud-managed runtime: teams deploy agent code or definitions to AgentCore,
   Foundry Agent Service, or Vertex AI Agent Engine and use cloud IAM, private
   networking, managed scaling, and cloud observability.
2. Hybrid or self-hosted agent platform: teams run Agent Server/data-plane
   containers on Kubernetes while retaining a vendor or self-hosted control
   plane. LangSmith recommends Kubernetes/Helm for production self-hosting.
3. Application-embedded agents: teams package framework code into normal
   containerized services on Kubernetes, ECS, VMs, or serverless platforms.
   These deployments often have telemetry but no standard fleet API, so Capcom
   needs a registration contract and OpenTelemetry connector rather than a
   vendor runtime adapter.

In Capcom terms, enterprises do not usually "use an adapter." They use a cloud
or agent platform API, while Capcom supplies the adapter that translates that
API into runtime-neutral inventory, health, execution, access, and control
capabilities.

## Decision Gate Before Each Adapter

An adapter enters implementation only when all of these are answered:

- Can Capcom list stable agent/runtime identities without reading vendor databases?
- Can it distinguish account/project/region/deployment/agent/version?
- Which capabilities are authoritative, inferred, or unavailable?
- Does authentication support workload identity and least privilege?
- Are list APIs paginated and are event/run reads incremental?
- Which controls are reversible and auditable?
- Can fixture-based tests run without live cloud credentials?
- What is the last-known-state behavior during throttling or outage?

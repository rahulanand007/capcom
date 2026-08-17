# Capcom GStack Brief

## One-Line Thesis

Capcom is a declarative AgentOps governance and reconciliation layer for production agents, starting with Gantry as the first runtime adapter.

## Problem

Enterprises will not run all agents in one framework. They will have Gantry, LangGraph, CrewAI, OpenAI Agents, internal services, Kubernetes jobs, and SaaS agents. Existing observability tools can show signals, but they do not own approved agent desired state or reconcile live runtime access against that state.

The core unanswered questions are:

- Which agents exist?
- Who owns them?
- What tools, MCP servers, skills, conversations, and systems can they access?
- Is live runtime state different from approved state?
- What action did an operator take, why, and what changed?

## MVP Wedge

The MVP should prove one workflow:

1. Connect Capcom to a Gantry runtime.
2. Import agents and runtime access.
3. Apply or create desired state.
4. Detect basic capability drift.
5. Execute a safe control action.
6. Record an audit trail.

This keeps Capcom sharper than a dashboard and smaller than a full orchestration platform.

## MVP Scope

In scope:

- Gantry runtime connection.
- Agent registry.
- Gantry agent import.
- Access/capability import.
- Desired state model.
- Basic capability drift detection.
- Runtime event ingestion by SDK/control API polling or streaming.
- Control actions for enable/disable and capability restriction where Gantry exposes support.
- Audit log.
- Minimal dashboard, API, CLI, and YAML path over the same state model.

Out of scope:

- New agent runtime.
- Full observability replacement.
- Autonomous remediation by default.
- Full Kubernetes operator.
- A2A/AGNTCY/OpenTelemetry export.
- Business value metadata.
- Topology metadata.
- Advanced policy drift.
- Enforce mode.

## Key Product Decisions

- Use Gantry consistently throughout Capcom documentation.
- Keep Capcom runtime-agnostic even though Gantry is the first adapter.
- Use SDK/control API first, not webhook-first, for the MVP.
- Keep webhooks Phase 2 for cloud push delivery.
- Keep minimal drift in MVP. Move advanced drift and reconciliation to Phase 2.
- Use Postgres for Capcom state because Capcom needs relational queries, audit history, and product workflows.
- Use Go for the core control plane, reconciler, adapter interface, workers, and CLI.
- Use React/Next.js for the dashboard.

## Success Criteria

- A valid Gantry connection passes health and doctor checks.
- Invalid credentials are rejected without activating the connection.
- Capcom imports Gantry agents, access selections, sources, conversations, and approvers.
- Capcom shows an agent timeline from Gantry events.
- Capcom detects one basic capability drift case.
- Capcom executes one safe control action through Gantry.
- Every mutation writes an audit entry with actor, reason, target, before/after, result, and timestamp.


# Capcom GStack Document Pack

## Purpose

This pack turns the Capcom GStack idea into implementation-ready documentation. It should be used as the working source set for planning, building, and demoing the Gantry-first MVP.

## Canonical Documents

| Document | Use It For |
|---|---|
| `gstack-capcom-brief.md` | One-page product thesis, MVP wedge, and success criteria |
| `gstack-capcom-implementation-plan.md` | Phase-by-phase engineering plan and demo script |
| `gstack-gantry-integration-contract.md` | Current Gantry runtime facts, API surface, and integration rules |
| `gstack-capcom-api-schema-contract.md` | Capcom-owned API resources, manifests, state models, and validation rules |
| `gstack-capcom-local-runbook.md` | Local setup, runtime verification, blockers, and operating procedures |
| `gstack-capcom-demo-uat-checklist.md` | Demo acceptance checklist and UAT evidence capture |
| `agentops-control-plane-mvp.md` | Full product and architecture narrative |
| `capcom-go-server-lld.md` | Low-level Go server design |
| `agentops-strategy-stress-test.md` | Strategic risks, market positioning, and wedge validation |

## Document Rules

- Use **Capcom** for the product name.
- Use **Gantry** for the first runtime adapter.
- Use Gantry consistently in Capcom-facing documentation.
- Treat Gantry as an integration proof, not the long-term product boundary.
- Keep the MVP focused on desired state, runtime import, basic drift, safe control action, and audit.
- Keep SDK/control API polling or streaming as the MVP event path.
- Keep signed webhooks as Phase 2 unless a stable callback URL is available.
- Do not document direct Gantry database reads as a supported integration path.

## Handoff Readiness

The MVP is ready to implement when these are true:

- Gantry local runtime can pass health and doctor checks, or the remaining blocker is explicit.
- The Gantry OpenAPI snapshot has been captured from the running runtime.
- The Capcom `RuntimeConnection` and `Agent` manifests are frozen for MVP.
- Control actions have exact runtime mappings and audit requirements.
- Demo/UAT acceptance criteria have owners and evidence fields.


# 07 - Control Action Safety Design

## Goal

Allow operators to take safe, explicit, audited runtime actions without making Capcom an autonomous remediation system in V1.

## V1 Actions

| Action | Runtime Mutation |
|---|---|
| disable_agent | Gantry `PATCH /v1/agents/{agentId}` status disabled |
| enable_agent | Gantry `PATCH /v1/agents/{agentId}` status active |
| replace_access | Gantry `PUT /v1/agents/{agentId}/access` |
| restrict_capability | Read access, remove one selection, PUT access |
| delete_agent | LangGraph `DELETE /assistants/{assistantId}` |
| cancel_execution | LangGraph interrupt cancellation with `wait=true` |

## Required Fields

Every control action requires:

- `requestedBy`
- `reason`
- `targetType`
- `targetId`
- `actionType`
- `parameters`
- `idempotency_key`

Reject vague reasons shorter than 10 characters.

Agent deletion additionally requires an exact runtime-agent-ID confirmation.
Execution cancellation is limited to `pending` or `running` runs and uses
interrupt semantics so records and checkpoints remain available. Rollback
cancellation is outside V1.

## Safety Checks

Before executing:

1. Runtime connection must be `control_enabled`.
2. Runtime status must not be `disabled`.
3. Actor must be present.
4. Reason must be present.
5. Target agent must exist.
6. Action must be supported by adapter.
7. Current actual state must be loaded.
8. For `restrict_capability`, capability must exist in actual selections.

## Audit Sequence

Each action writes at least two audit entries:

1. `control_action_requested`
2. `control_action_completed` or `control_action_failed`

Audit must include:

- actor
- reason
- target
- before state
- intended after state
- runtime request
- runtime response or error
- timestamps

## Idempotency

Recommended V1 behavior:

- Accept optional `Idempotency-Key` header.
- Store request hash on `control_actions`.
- Return previous result for repeated identical key.

If not implemented in first slice, CLI/dashboard should disable duplicate submissions while action is running.

## Failure Behavior

| Failure | Behavior |
|---|---|
| Runtime 4xx | Mark failed, audit, do not retry automatically |
| Runtime timeout | Mark failed or unknown, audit, force sync before another action |
| Runtime unavailable | Mark runtime degraded, action failed |
| Post-action sync fails | Action can succeed, but actual state becomes stale |

## Rollback

V1 does not promise rollback.

For access changes, Capcom can show previous access state in audit logs and allow an operator to apply it manually as a new `replace_access` action.

## UI Confirmation

Dashboard should require confirmation for:

- disable agent
- replace access
- restrict capability on high/critical agent

Confirmation should show:

- target agent
- current runtime
- capability/source being removed
- reason input

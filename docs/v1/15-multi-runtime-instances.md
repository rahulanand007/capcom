# 15 - Multi-Runtime Instances

## Hierarchy

```text
adapter kind -> runtime instance -> endpoint routes
                                 -> durable agents -> ephemeral subagent executions
```

A Gantry installation is one runtime instance. The Gantry adapter is stateless
and reusable; it is not itself an installation or an agent.

One installation may be reachable through several URLs. Capcom stores exactly
one canonical runtime instance and classifies the other URLs as endpoint routes:

- `canonical`: the address used by the instance worker
- `alias`: another name/address for the same runtime socket
- `relay`: a forwarding address that terminates at the same runtime

Aliases and relays are not runtime instances, agents, or subprocesses. They do
not own an independent sync schedule and must not duplicate imported agents.
Runtime subprocesses reported by the adapter belong under their owning agent or
execution; a process, hostname, container, or database is not sufficient
evidence of a separate runtime installation.

## Required Identity

Each instance has a unique stable `name`, mutable `display_name`, environment,
labels, endpoint, mode, and encrypted `auth_ref`. Operators must never rely on
color alone to distinguish instances.

Changing mutable settings uses `PATCH /v1/runtime-instances/{id}` and requires
an `actor` and `reason`. Operators can update the display name, environment,
labels, description, endpoint, control mode, encrypted credential reference,
automatic sync state, and sync interval. The instance UUID, stable key, and
adapter kind remain immutable.

The endpoint validates that `auth_ref` resolves before persisting the update.
Audit events contain the secret reference but never plaintext credential
material. Sync intervals must be between 15 seconds and 24 hours.

Unused instances are removed with `DELETE /v1/runtime-instances/{id}`. Removal
requires an actor, reason, and the exact stable key as confirmation. V1 uses a
soft removal: automatic sync is disabled and the instance disappears from
active API and console views, while imported data and audit/control history are
retained.

Failed and stale instances expose **Remove** beside their retry action in the
overview attention queue. The same confirmation requirements apply there as in
adapter settings.

Existing duplicate records are consolidated with
`POST /v1/runtime-instances/{canonicalId}/endpoints/consolidate`. This audited
operation requires the duplicate stable key as confirmation, archives the
duplicate record while retaining its imported history, and adds its URL to the
canonical instance as either an `alias` or `relay`. The console shows these
routes in the canonical instance's endpoint topology.

Example:

| Key | Display name | Environment | Endpoint |
|---|---|---|---|
| `gantry-development` | Gantry Development | `development` | `http://127.0.0.1:8787` |
| `gantry-staging` | Gantry Staging | `staging` | `http://127.0.0.1:8788` |
| `gantry-production` | Gantry Production | `production` | `http://127.0.0.1:8789` |

## Deployment Isolation

Run installations with different Compose project names, host ports, Postgres
databases or volumes, and Control API keys. No Gantry source change is needed.
When Capcom runs in Docker, use reachable container DNS names or
`host.docker.internal` instead of loopback host URLs.

Until runtimes expose a stable installation identity, Capcom can prevent exact
endpoint duplicates and operators can explicitly consolidate known aliases or
relays. It must not automatically merge two different sockets solely because
their inventories look alike: separately running Gantry installations may have
identical agents and skills.

## Failure Isolation

- Sync locks and sync schedules are per runtime instance UUID.
- A failed instance preserves its own last-known state and cannot degrade peers.
- Identical Gantry agent, skill, run, and task IDs remain distinct because all
  unique keys include `runtime_connection_id`.
- Control actions resolve the owning agent binding before selecting an endpoint
  and secret.

## Console Contract

Selectors and instance cards show display name, environment, endpoint, adapter,
status, and stable key. Agent and subagent views retain that context. Global
search results must include the owning instance before fleet search is added.

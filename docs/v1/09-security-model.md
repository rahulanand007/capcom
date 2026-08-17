# 09 - Security Model

## V1 Security Goals

- Avoid storing plaintext runtime keys in manifests.
- Separate read-only and control-enabled runtime modes.
- Require actor and reason for mutations.
- Preserve immutable audit logs.
- Keep the first version simple enough to build.

## Capcom Auth

Local automation can explicitly enable the platform admin compatibility path:

```text
CAPCOM_ADMIN_TOKEN
```

Automation requests may use:

```text
Authorization: Bearer <token>
```

The path is disabled when `CAPCOM_ADMIN_TOKEN` is empty. It bypasses tenant
membership checks, so production and hosted environments must leave it unset.
Setting `CAPCOM_DEPLOYMENT_MODE=hosted` makes startup fail if an admin token is
present or secure cookies are disabled, enforcing both boundaries in backend
configuration rather than relying only on deployment documentation.
Any future emergency production access must be a separately protected,
short-lived, audited break-glass flow rather than this bearer token.

The browser console now uses email/password authentication backed by Argon2id
credentials and opaque server-side sessions. `POST /auth/signup` atomically
creates a user, organization owner membership, and session. `POST /auth/login`
uses generic failures and throttling. Session cookies are HttpOnly and SameSite
Lax; state-changing requests also require the separate CSRF token. Set
`CAPCOM_SECURE_COOKIES=true` behind HTTPS. Runtime connections, runtime reads,
secrets, telemetry, and audit writes are scoped to the authenticated
organization in the application repositories.

The remaining hosted hardening work—forced PostgreSQL RLS, managed per-tenant
keys, private connector, SSRF-safe egress, full membership management, and OIDC—is
tracked in the post-V1 foundation document.

The implementation-ready post-V1 design is maintained in
[Hosted Product Foundation](../post-v1/01-hosted-product-foundation.md). The initial
hosted authentication scope is email/password plus secure server-side sessions;
OTP, outbound email, and OIDC are deferred. The plan also covers organization
ownership, PostgreSQL row-level security, managed per-tenant encryption, safe
egress, quotas, privacy lifecycle controls, and the outbound runtime connector.
The implemented session slice preserves the V1 admin-token path for local CLI
and worker compatibility while removing that token from the browser proxy.
The Go API no longer embeds the legacy console or publishes `/assets/*`; browser
credentials exist only as opaque HttpOnly server-managed sessions in the Next.js
application flow.

## Runtime Credentials

Runtime API keys should be stored as:

- encrypted local secret in Postgres, or
- external secret reference

V1 acceptable implementation:

- `secrets` table
- AES-GCM encrypted value
- encryption key from `CAPCOM_SECRET_KEY`

Implemented V1 details:

- `CAPCOM_SECRET_KEY` is standard base64 encoding of exactly 32 random bytes.
- Secret values are encrypted with AES-256-GCM before repository writes.
- The secret name is AES-GCM associated data.
- Ciphertext includes an internal format version and random nonce.
- Create and rotate require actor and reason and write append-only audit events.
- Gantry credentials are resolved immediately before an adapter request and sent as a Bearer token.
- Secret values, ciphertext, and Authorization headers are never returned by APIs or written to audit metadata.

Manifests reference secrets by name:

```yaml
auth:
  apiKeyRef: gantry-control-api-key
```

Inline secrets in YAML are rejected.

## Runtime Modes

| Mode | Behavior |
|---|---|
| read_only | sync/import/drift only; no runtime mutation |
| control_enabled | allows approved control actions |

Control service must check mode before every mutation.

## Gantry Scopes

Read-like V1 connection:

```text
sessions:read
agents:admin
jobs:read
skills:read
mcp:read
```

Control-enabled:

```text
sessions:read
agents:admin
jobs:read
jobs:write
skills:read
skills:admin
mcp:read
mcp:admin
```

## Audit Retention

V1:

- keep audit logs indefinitely
- keep runtime events at least 30 days
- do not hard-delete audit rows

## Sensitive Data

Avoid logging:

- runtime API keys
- secret values
- full credential payloads
- raw Authorization headers

Raw runtime payload storage should redact obvious secret fields.

## Threats And Mitigations

| Threat | Mitigation |
|---|---|
| Stolen Capcom admin token | Disabled by default; local automation only; hosted deployments require a separate audited break-glass design |
| Over-scoped Gantry key | Support read-only mode and document scopes |
| Accidental destructive action | No delete-agent action in V1; require reason and confirmation |
| Silent mutation | Audit before and after every control action |
| Runtime drift hidden by outage | Preserve last known state and mark runtime degraded |

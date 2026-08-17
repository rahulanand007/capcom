CREATE TABLE IF NOT EXISTS users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    email_normalized text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS password_credentials (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    password_hash text NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organizations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    slug text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS organization_memberships (
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('owner', 'admin', 'operator', 'viewer')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id)
);

CREATE TABLE IF NOT EXISTS user_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE,
    csrf_hash bytea NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    idle_expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_user_sessions_active
    ON user_sessions (token_hash, idle_expires_at)
    WHERE revoked_at IS NULL;

-- Existing local installations are assigned to a bootstrap organization. The
-- first account claims membership of it, preserving the operator's fleet.
INSERT INTO organizations (id, name, slug)
VALUES ('00000000-0000-4000-8000-000000000001', 'Capcom Local', 'capcom-local')
ON CONFLICT (slug) DO NOTHING;

ALTER TABLE runtime_connections ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations (id);
UPDATE runtime_connections SET organization_id = '00000000-0000-4000-8000-000000000001' WHERE organization_id IS NULL;
ALTER TABLE runtime_connections ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE runtime_connections DROP CONSTRAINT IF EXISTS runtime_connections_name_key;
DROP INDEX IF EXISTS runtime_connections_active_name_unique_idx;
DROP INDEX IF EXISTS runtime_connections_active_endpoint_unique_idx;
DROP INDEX IF EXISTS idx_runtime_connections_unique_endpoint;
CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_connections_org_name
    ON runtime_connections (organization_id, name) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_runtime_connections_org_endpoint
    ON runtime_connections (organization_id, runtime_type, lower(rtrim(endpoint, '/'))) WHERE archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_runtime_connections_org_status
    ON runtime_connections (organization_id, status) WHERE archived_at IS NULL;

ALTER TABLE secrets ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations (id);
UPDATE secrets SET organization_id = '00000000-0000-4000-8000-000000000001' WHERE organization_id IS NULL;
ALTER TABLE secrets ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE secrets DROP CONSTRAINT IF EXISTS secrets_name_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_secrets_org_name ON secrets (organization_id, name);

ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations (id);
UPDATE audit_events a SET organization_id = r.organization_id
FROM runtime_connections r WHERE a.runtime_connection_id = r.id AND a.organization_id IS NULL;
UPDATE audit_events SET organization_id = '00000000-0000-4000-8000-000000000001' WHERE organization_id IS NULL;
ALTER TABLE audit_events ALTER COLUMN organization_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_audit_events_org_created ON audit_events (organization_id, created_at DESC);

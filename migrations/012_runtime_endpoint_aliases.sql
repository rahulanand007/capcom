ALTER TABLE runtime_connections
    ADD COLUMN IF NOT EXISTS merged_into_runtime_connection_id uuid REFERENCES runtime_connections(id),
    ADD COLUMN IF NOT EXISTS archive_kind text;

ALTER TABLE runtime_connections
    DROP CONSTRAINT IF EXISTS runtime_connections_archive_kind_check;

ALTER TABLE runtime_connections
    ADD CONSTRAINT runtime_connections_archive_kind_check
    CHECK (archive_kind IS NULL OR archive_kind IN ('removed', 'alias', 'relay'));

CREATE TABLE IF NOT EXISTS runtime_connection_endpoints (
    id uuid PRIMARY KEY,
    runtime_connection_id uuid NOT NULL REFERENCES runtime_connections(id) ON DELETE CASCADE,
    endpoint text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('canonical', 'alias', 'relay')),
    auth_ref text,
    source_runtime_connection_id uuid REFERENCES runtime_connections(id) ON DELETE SET NULL,
    created_by text NOT NULL DEFAULT 'migration',
    reason text NOT NULL DEFAULT 'Backfilled canonical endpoint',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS runtime_connection_endpoints_owner_url_unique_idx
    ON runtime_connection_endpoints (runtime_connection_id, lower(rtrim(endpoint, '/')));

CREATE INDEX IF NOT EXISTS runtime_connection_endpoints_runtime_idx
    ON runtime_connection_endpoints (runtime_connection_id, kind, created_at);

INSERT INTO runtime_connection_endpoints (
    id, runtime_connection_id, endpoint, kind, auth_ref, created_by, reason
)
SELECT gen_random_uuid(), id, endpoint, 'canonical', auth_ref, 'migration',
       'Backfilled from the runtime instance canonical endpoint'
FROM runtime_connections
WHERE archived_at IS NULL
ON CONFLICT DO NOTHING;

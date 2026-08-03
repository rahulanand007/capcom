ALTER TABLE runtime_connections
    ADD COLUMN IF NOT EXISTS archived_at timestamptz,
    ADD COLUMN IF NOT EXISTS archived_by text,
    ADD COLUMN IF NOT EXISTS archive_reason text;

CREATE INDEX IF NOT EXISTS runtime_connections_active_idx
    ON runtime_connections (runtime_type, name)
    WHERE archived_at IS NULL;

ALTER TABLE runtime_connections
    DROP CONSTRAINT IF EXISTS runtime_connections_name_key;

DROP INDEX IF EXISTS idx_runtime_connections_unique_endpoint;

CREATE UNIQUE INDEX IF NOT EXISTS runtime_connections_active_name_unique_idx
    ON runtime_connections (name)
    WHERE archived_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS runtime_connections_active_endpoint_unique_idx
    ON runtime_connections (runtime_type, lower(rtrim(endpoint, '/')))
    WHERE archived_at IS NULL;

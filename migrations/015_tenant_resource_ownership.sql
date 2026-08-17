ALTER TABLE agents ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations (id);
UPDATE agents a SET organization_id = ownership.organization_id
FROM (
    SELECT b.agent_id, min(r.organization_id::text)::uuid AS organization_id
    FROM agent_runtime_bindings b JOIN runtime_connections r ON r.id=b.runtime_connection_id
    GROUP BY b.agent_id
) ownership WHERE ownership.agent_id=a.id AND a.organization_id IS NULL;
UPDATE agents SET organization_id='00000000-0000-4000-8000-000000000001' WHERE organization_id IS NULL;
ALTER TABLE agents ALTER COLUMN organization_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_agents_org_status ON agents (organization_id,status);

ALTER TABLE control_actions ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations (id);
UPDATE control_actions a SET organization_id=r.organization_id FROM runtime_connections r
WHERE r.id=a.runtime_connection_id AND a.organization_id IS NULL;
ALTER TABLE control_actions ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE control_actions DROP CONSTRAINT IF EXISTS control_actions_idempotency_key_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_control_actions_org_idempotency
    ON control_actions (organization_id,idempotency_key) WHERE idempotency_key IS NOT NULL;

ALTER TABLE runtime_sync_runs ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations (id);
UPDATE runtime_sync_runs s SET organization_id=r.organization_id FROM runtime_connections r
WHERE r.id=s.runtime_connection_id AND s.organization_id IS NULL;
ALTER TABLE runtime_sync_runs ALTER COLUMN organization_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_runtime_sync_runs_org_started ON runtime_sync_runs (organization_id,started_at DESC);

ALTER TABLE usage_observations ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations (id);
UPDATE usage_observations u SET organization_id=r.organization_id FROM runtime_connections r
WHERE r.id=u.runtime_connection_id AND u.organization_id IS NULL;
ALTER TABLE usage_observations ALTER COLUMN organization_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_usage_observations_org_observed ON usage_observations (organization_id,observed_at DESC);

ALTER TABLE telemetry_ingestion_runs ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations (id);
UPDATE telemetry_ingestion_runs t SET organization_id=r.organization_id FROM runtime_connections r
WHERE r.id=t.runtime_connection_id AND t.organization_id IS NULL;
ALTER TABLE telemetry_ingestion_runs ALTER COLUMN organization_id SET NOT NULL;
CREATE INDEX IF NOT EXISTS idx_telemetry_runs_org_started ON telemetry_ingestion_runs (organization_id,started_at DESC);

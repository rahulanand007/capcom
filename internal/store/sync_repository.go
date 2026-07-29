package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"capcom/internal/domain"
)

type SyncRepository struct {
	db *sql.DB
}

func NewSyncRepository(db *sql.DB) SyncRepository {
	return SyncRepository{db: db}
}

func (r SyncRepository) TryLock(ctx context.Context, runtimeID string) (func(), bool, error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("open sync lock connection: %w", err)
	}
	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, runtimeID).Scan(&locked); err != nil {
		conn.Close()
		return nil, false, fmt.Errorf("acquire runtime sync lock: %w", err)
	}
	if !locked {
		conn.Close()
		return nil, false, nil
	}
	return func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, runtimeID)
		_ = conn.Close()
	}, true, nil
}

func (r SyncRepository) CreateRun(ctx context.Context, run domain.RuntimeSyncRun) (domain.RuntimeSyncRun, error) {
	if run.ID == "" {
		id, err := newID()
		if err != nil {
			return domain.RuntimeSyncRun{}, err
		}
		run.ID = id
	}
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	run.Status = domain.SyncStatusRunning
	if _, err := r.db.ExecContext(ctx, `
INSERT INTO runtime_sync_runs (id, runtime_connection_id, trigger, status, started_at)
VALUES ($1, $2, $3, $4, $5)`, run.ID, run.RuntimeConnectionID, run.Trigger, run.Status, run.StartedAt); err != nil {
		return domain.RuntimeSyncRun{}, fmt.Errorf("create sync run: %w", err)
	}
	_, err := r.db.ExecContext(ctx, `
UPDATE runtime_connections SET last_sync_status = 'running', last_sync_started_at = $2,
updated_at = now() WHERE id = $1`, run.RuntimeConnectionID, run.StartedAt)
	if err != nil {
		return domain.RuntimeSyncRun{}, fmt.Errorf("mark runtime syncing: %w", err)
	}
	return run, nil
}

func (r SyncRepository) FailRun(ctx context.Context, run domain.RuntimeSyncRun, code, message string) (domain.RuntimeSyncRun, error) {
	finished := time.Now().UTC()
	run.Status = domain.SyncStatusFailed
	run.FinishedAt = &finished
	run.DurationMS = finished.Sub(run.StartedAt).Milliseconds()
	run.ErrorCode = code
	run.ErrorMessage = message
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return run, fmt.Errorf("begin failed sync update: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
UPDATE runtime_sync_runs SET status = 'failed', finished_at = $2, duration_ms = $3,
error_code = $4, error_message = $5 WHERE id = $1`, run.ID, finished, run.DurationMS, code, message); err != nil {
		return run, fmt.Errorf("fail sync run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE runtime_connections SET status = 'degraded', last_sync_status = 'failed',
last_sync_finished_at = $2, last_sync_duration_ms = $3, last_error = $4, updated_at = now()
WHERE id = $1`, run.RuntimeConnectionID, finished, run.DurationMS, message); err != nil {
		return run, fmt.Errorf("degrade runtime: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return run, fmt.Errorf("commit failed sync update: %w", err)
	}
	return run, nil
}

func (r SyncRepository) PersistSnapshot(ctx context.Context, run domain.RuntimeSyncRun, snapshot domain.RuntimeSnapshot, missingThreshold int) (domain.RuntimeSyncRun, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return run, fmt.Errorf("begin persist runtime snapshot: %w", err)
	}
	defer tx.Rollback()

	skillCount := 0
	bindingCount := 0
	for _, item := range snapshot.Agents {
		agentID, err := upsertSnapshotAgent(ctx, tx, run, item.Agent)
		if err != nil {
			return run, err
		}
		for _, skill := range item.Skills {
			if err := upsertSnapshotSkill(ctx, tx, run, agentID, skill); err != nil {
				return run, err
			}
			skillCount++
			bindingCount++
		}
		if item.Access.Source != "" {
			if err := upsertActualAccess(ctx, tx, run, agentID, item.Access); err != nil {
				return run, err
			}
		}
	}
	for _, execution := range snapshot.Executions {
		if err := upsertRuntimeExecution(ctx, tx, run.RuntimeConnectionID, execution); err != nil {
			return run, err
		}
	}
	for _, diagnostic := range snapshot.Diagnostics {
		if err := upsertRuntimeDiagnostic(ctx, tx, run.RuntimeConnectionID, diagnostic); err != nil {
			return run, err
		}
	}
	for _, item := range snapshot.Inventory {
		if err := upsertRuntimeInventory(ctx, tx, run.RuntimeConnectionID, item); err != nil {
			return run, err
		}
	}
	for _, capability := range snapshot.CapabilityCatalog {
		if err := upsertRuntimeCapability(ctx, tx, run.RuntimeConnectionID, capability); err != nil {
			return run, err
		}
	}
	for _, delegation := range snapshot.AgentDelegations {
		if err := upsertAgentDelegation(ctx, tx, run, delegation); err != nil {
			return run, err
		}
	}
	for _, execution := range snapshot.SubagentExecutions {
		if err := upsertSubagentExecution(ctx, tx, run.RuntimeConnectionID, execution); err != nil {
			return run, err
		}
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE agent_runtime_bindings b SET missing_successful_syncs = b.missing_successful_syncs + 1
WHERE b.runtime_connection_id = $1 AND (b.last_seen_sync_run_id IS NULL OR b.last_seen_sync_run_id <> $2)`, run.RuntimeConnectionID, run.ID); err != nil {
		return run, fmt.Errorf("advance missing agents: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE agents a SET status = 'stale', updated_at = now()
FROM agent_runtime_bindings b WHERE b.agent_id = a.id AND b.runtime_connection_id = $1
AND b.missing_successful_syncs >= $2`, run.RuntimeConnectionID, missingThreshold); err != nil {
		return run, fmt.Errorf("mark missing agents stale: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE agent_skill_bindings ab SET missing_successful_syncs = ab.missing_successful_syncs + 1,
status = CASE WHEN ab.missing_successful_syncs + 1 >= $3 THEN 'stale' ELSE ab.status END
FROM runtime_skills s WHERE ab.runtime_skill_id = s.id AND s.runtime_connection_id = $1
AND (ab.last_seen_sync_run_id IS NULL OR ab.last_seen_sync_run_id <> $2)`, run.RuntimeConnectionID, run.ID, missingThreshold); err != nil {
		return run, fmt.Errorf("advance missing skill bindings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE runtime_skills SET missing_successful_syncs = missing_successful_syncs + 1,
status = CASE WHEN missing_successful_syncs + 1 >= $3 THEN 'stale' ELSE status END
WHERE runtime_connection_id = $1 AND (last_seen_sync_run_id IS NULL OR last_seen_sync_run_id <> $2)`, run.RuntimeConnectionID, run.ID, missingThreshold); err != nil {
		return run, fmt.Errorf("advance missing runtime skills: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE agent_delegations SET missing_successful_syncs = missing_successful_syncs + 1,
status = CASE WHEN missing_successful_syncs + 1 >= $3 THEN 'stale' ELSE status END,
updated_at = now()
WHERE runtime_connection_id = $1 AND (last_seen_sync_run_id IS NULL OR last_seen_sync_run_id <> $2)`,
		run.RuntimeConnectionID, run.ID, missingThreshold); err != nil {
		return run, fmt.Errorf("advance missing agent delegations: %w", err)
	}

	finished := time.Now().UTC()
	run.Status = domain.SyncStatusSucceeded
	run.FinishedAt = &finished
	run.DurationMS = finished.Sub(run.StartedAt).Milliseconds()
	run.AgentsSeen = len(snapshot.Agents)
	run.SkillsSeen = skillCount
	run.BindingsSeen = bindingCount
	for _, item := range snapshot.Agents {
		if item.Access.Source != "" {
			run.AccessDocumentsSeen++
		}
	}
	run.ExecutionsSeen = len(snapshot.Executions)
	run.DiagnosticsSeen = len(snapshot.Diagnostics)
	run.InventorySeen = len(snapshot.Inventory)
	run.CapabilitiesSeen = len(snapshot.CapabilityCatalog)
	run.DelegationsSeen = len(snapshot.AgentDelegations)
	if _, err := tx.ExecContext(ctx, `
UPDATE runtime_sync_runs SET status = 'succeeded', finished_at = $2, duration_ms = $3,
agents_seen = $4, skills_seen = $5, bindings_seen = $6, access_documents_seen = $7, executions_seen = $8,
diagnostics_seen = $9, inventory_seen = $10, capabilities_seen = $11, delegations_seen = $12
WHERE id = $1`, run.ID, finished, run.DurationMS, run.AgentsSeen, run.SkillsSeen, run.BindingsSeen,
		run.AccessDocumentsSeen, run.ExecutionsSeen, run.DiagnosticsSeen, run.InventorySeen, run.CapabilitiesSeen,
		run.DelegationsSeen); err != nil {
		return run, fmt.Errorf("complete sync run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE runtime_connections SET status = 'active', last_sync_status = 'succeeded',
last_sync_at = $2, last_sync_finished_at = $2, last_sync_duration_ms = $3,
last_error = NULL, updated_at = now() WHERE id = $1`, run.RuntimeConnectionID, finished, run.DurationMS); err != nil {
		return run, fmt.Errorf("activate synced runtime: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return run, fmt.Errorf("commit runtime snapshot: %w", err)
	}
	return run, nil
}

func upsertAgentDelegation(ctx context.Context, tx *sql.Tx, run domain.RuntimeSyncRun, item domain.AgentDelegationSnapshot) error {
	metadata, err := json.Marshal(item.Metadata)
	if err != nil {
		return fmt.Errorf("marshal agent delegation metadata: %w", err)
	}
	raw, err := json.Marshal(item.Raw)
	if err != nil {
		return fmt.Errorf("marshal raw agent delegation: %w", err)
	}
	delegateKey := "ref:" + item.DelegateRef
	_, err = tx.ExecContext(ctx, `
INSERT INTO agent_delegations (runtime_connection_id,orchestrator_runtime_agent_id,delegate_key,
delegate_runtime_agent_id,delegate_ref,tool_name,display_name,persona,configured,resolved,revision,status,
observed_at,last_seen_sync_run_id,missing_successful_syncs,metadata_json,raw_runtime_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'active',$12,$13,0,$14::jsonb,$15::jsonb)
ON CONFLICT (runtime_connection_id,orchestrator_runtime_agent_id,delegate_key) DO UPDATE SET
delegate_runtime_agent_id=EXCLUDED.delegate_runtime_agent_id,delegate_ref=EXCLUDED.delegate_ref,
tool_name=EXCLUDED.tool_name,display_name=EXCLUDED.display_name,persona=EXCLUDED.persona,
configured=EXCLUDED.configured,resolved=EXCLUDED.resolved,revision=EXCLUDED.revision,status='active',
observed_at=EXCLUDED.observed_at,last_seen_sync_run_id=EXCLUDED.last_seen_sync_run_id,
missing_successful_syncs=0,metadata_json=EXCLUDED.metadata_json,raw_runtime_json=EXCLUDED.raw_runtime_json,
updated_at=now()`, run.RuntimeConnectionID, item.OrchestratorRuntimeAgentID, delegateKey,
		item.DelegateRuntimeAgentID, item.DelegateRef, item.ToolName, item.DisplayName, item.Persona,
		item.Configured, item.Resolved, item.Revision, item.ObservedAt, run.ID, string(metadata), string(raw))
	if err != nil {
		return fmt.Errorf("upsert agent delegation: %w", err)
	}
	return nil
}

func upsertRuntimeExecution(ctx context.Context, tx *sql.Tx, runtimeID string, execution domain.RuntimeExecutionSnapshot) error {
	metadata, err := json.Marshal(execution.Metadata)
	if err != nil {
		return fmt.Errorf("marshal runtime execution metadata: %w", err)
	}
	raw, err := json.Marshal(execution.Raw)
	if err != nil {
		return fmt.Errorf("marshal raw runtime execution: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO runtime_executions (runtime_connection_id,runtime_execution_id,parent_runtime_execution_id,
runtime_agent_id,kind,status,started_at,ended_at,observed_at,metadata_json,raw_runtime_json)
VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10::jsonb,$11::jsonb)
ON CONFLICT (runtime_connection_id,kind,runtime_execution_id) DO UPDATE SET
parent_runtime_execution_id=EXCLUDED.parent_runtime_execution_id,runtime_agent_id=EXCLUDED.runtime_agent_id,
status=EXCLUDED.status,started_at=COALESCE(runtime_executions.started_at,EXCLUDED.started_at),
ended_at=EXCLUDED.ended_at,observed_at=EXCLUDED.observed_at,metadata_json=EXCLUDED.metadata_json,
raw_runtime_json=EXCLUDED.raw_runtime_json,updated_at=now()`, runtimeID, execution.RuntimeExecutionID,
		execution.ParentRuntimeExecutionID, execution.RuntimeAgentID, execution.Kind, execution.Status,
		execution.StartedAt, execution.EndedAt, execution.ObservedAt, string(metadata), string(raw))
	if err != nil {
		return fmt.Errorf("upsert runtime execution: %w", err)
	}
	return nil
}

func upsertSubagentExecution(ctx context.Context, tx *sql.Tx, runtimeID string, execution domain.SubagentExecutionSnapshot) error {
	metadata, err := json.Marshal(execution.Metadata)
	if err != nil {
		return fmt.Errorf("marshal subagent execution metadata: %w", err)
	}
	raw, err := json.Marshal(execution.Raw)
	if err != nil {
		return fmt.Errorf("marshal raw subagent execution: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO subagent_executions (runtime_connection_id,runtime_execution_id,parent_run_id,runtime_agent_id,
subagent_type,status,description,summary,started_at,ended_at,observed_at,metadata_json,raw_runtime_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13::jsonb)
ON CONFLICT (runtime_connection_id,runtime_execution_id) DO UPDATE SET
parent_run_id=EXCLUDED.parent_run_id,runtime_agent_id=EXCLUDED.runtime_agent_id,
subagent_type=EXCLUDED.subagent_type,status=EXCLUDED.status,description=EXCLUDED.description,
summary=EXCLUDED.summary,started_at=COALESCE(subagent_executions.started_at,EXCLUDED.started_at),
ended_at=EXCLUDED.ended_at,observed_at=EXCLUDED.observed_at,metadata_json=EXCLUDED.metadata_json,
raw_runtime_json=EXCLUDED.raw_runtime_json,updated_at=now()`, runtimeID, execution.RuntimeExecutionID,
		execution.ParentRunID, execution.RuntimeAgentID, execution.SubagentType, execution.Status,
		execution.Description, execution.Summary, execution.StartedAt, execution.EndedAt,
		execution.ObservedAt, string(metadata), string(raw))
	if err != nil {
		return fmt.Errorf("upsert subagent execution: %w", err)
	}
	return nil
}

func upsertSnapshotAgent(ctx context.Context, tx *sql.Tx, run domain.RuntimeSyncRun, snapshot domain.AgentSnapshot) (string, error) {
	var agentID string
	findErr := tx.QueryRowContext(ctx, `SELECT agent_id FROM agent_runtime_bindings
WHERE runtime_connection_id = $1 AND runtime_agent_id = $2`, run.RuntimeConnectionID, snapshot.RuntimeAgentID).Scan(&agentID)
	if findErr != nil && findErr != sql.ErrNoRows {
		return "", fmt.Errorf("find snapshot agent: %w", findErr)
	}
	metadata, err := json.Marshal(snapshot.Metadata)
	if err != nil {
		return "", fmt.Errorf("marshal agent metadata: %w", err)
	}
	if findErr == sql.ErrNoRows {
		agentID, err = newID()
		if err != nil {
			return "", err
		}
		bindingID, err := newID()
		if err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agents (id, name, status, metadata_json)
VALUES ($1, $2, $3, $4::jsonb)`, agentID, snapshot.Name, snapshot.Status, string(metadata)); err != nil {
			return "", fmt.Errorf("insert snapshot agent: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_runtime_bindings
(id, agent_id, runtime_connection_id, runtime_agent_id, kind, parent_runtime_agent_id,
last_seen_at, last_seen_sync_run_id, missing_successful_syncs, raw_runtime_json)
VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,0,$9::jsonb)`, bindingID, agentID,
			run.RuntimeConnectionID, snapshot.RuntimeAgentID, snapshot.Kind, snapshot.ParentRuntimeAgentID,
			snapshot.ObservedAt, run.ID, string(metadata)); err != nil {
			return "", fmt.Errorf("insert snapshot binding: %w", err)
		}
		return agentID, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET name = $2, status = $3,
metadata_json = $4::jsonb, updated_at = now() WHERE id = $1`, agentID, snapshot.Name, snapshot.Status, string(metadata)); err != nil {
		return "", fmt.Errorf("update snapshot agent: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agent_runtime_bindings SET kind = $2,
parent_runtime_agent_id = NULLIF($3,''), last_seen_at = $4, last_seen_sync_run_id = $5,
missing_successful_syncs = 0, raw_runtime_json = $6::jsonb, updated_at = now() WHERE agent_id = $1`,
		agentID, snapshot.Kind, snapshot.ParentRuntimeAgentID, snapshot.ObservedAt, run.ID, string(metadata)); err != nil {
		return "", fmt.Errorf("update snapshot binding: %w", err)
	}
	return agentID, nil
}

func upsertSnapshotSkill(ctx context.Context, tx *sql.Tx, run domain.RuntimeSyncRun, agentID string, skill domain.AgentSkillSnapshot) error {
	toolIDs, _ := json.Marshal(skill.ToolIDs)
	workflows, _ := json.Marshal(skill.WorkflowRefs)
	metadata, err := json.Marshal(skill.Metadata)
	if err != nil {
		return fmt.Errorf("marshal skill metadata: %w", err)
	}
	var skillID string
	if err := tx.QueryRowContext(ctx, `
INSERT INTO runtime_skills (runtime_connection_id, runtime_skill_id, name, description, source,
status, version, tool_ids_json, workflow_refs_json, metadata_json, raw_runtime_json, observed_at, last_seen_sync_run_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10::jsonb,$10::jsonb,$11,$12)
ON CONFLICT (runtime_connection_id, runtime_skill_id) DO UPDATE SET name=EXCLUDED.name,
description=EXCLUDED.description, source=EXCLUDED.source, status=EXCLUDED.status, version=EXCLUDED.version,
tool_ids_json=EXCLUDED.tool_ids_json, workflow_refs_json=EXCLUDED.workflow_refs_json,
metadata_json=EXCLUDED.metadata_json, raw_runtime_json=EXCLUDED.raw_runtime_json,
observed_at=EXCLUDED.observed_at, last_seen_sync_run_id=EXCLUDED.last_seen_sync_run_id,
missing_successful_syncs=0 RETURNING id`, run.RuntimeConnectionID, skill.RuntimeSkillID, skill.Name,
		skill.Description, skill.Source, skill.Status, skill.Version, string(toolIDs), string(workflows),
		string(metadata), skill.ObservedAt, run.ID).Scan(&skillID); err != nil {
		return fmt.Errorf("upsert runtime skill: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_skill_bindings (agent_id, runtime_skill_id, status, observed_at, last_seen_sync_run_id)
VALUES ($1,$2,'active',$3,$4) ON CONFLICT (agent_id, runtime_skill_id) DO UPDATE SET
status='active', observed_at=EXCLUDED.observed_at, last_seen_sync_run_id=EXCLUDED.last_seen_sync_run_id,
missing_successful_syncs=0`, agentID, skillID, skill.ObservedAt, run.ID); err != nil {
		return fmt.Errorf("upsert agent skill binding: %w", err)
	}
	return nil
}

func upsertActualAccess(ctx context.Context, tx *sql.Tx, run domain.RuntimeSyncRun, agentID string, access domain.AccessDocument) error {
	accessJSON, err := json.Marshal(access)
	if err != nil {
		return fmt.Errorf("marshal actual access: %w", err)
	}
	rawJSON, err := json.Marshal(access.Raw)
	if err != nil {
		return fmt.Errorf("marshal raw actual access: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO access_actual_state (agent_id, runtime_connection_id, runtime_status, access_json,
raw_runtime_json, observed_at, last_seen_sync_run_id, freshness_status)
VALUES ($1,$2,'active',$3::jsonb,$4::jsonb,$5,$6,'live')
ON CONFLICT (agent_id, runtime_connection_id) DO UPDATE SET runtime_status='active',
access_json=EXCLUDED.access_json, raw_runtime_json=EXCLUDED.raw_runtime_json,
observed_at=EXCLUDED.observed_at, last_seen_sync_run_id=EXCLUDED.last_seen_sync_run_id,
freshness_status='live'`, agentID, run.RuntimeConnectionID, string(accessJSON), string(rawJSON), access.ObservedAt, run.ID)
	if err != nil {
		return fmt.Errorf("upsert actual access: %w", err)
	}
	return nil
}

func (r SyncRepository) ListRuns(ctx context.Context, runtimeID string, limit int) ([]domain.RuntimeSyncRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, runtime_connection_id, trigger, status, started_at,
finished_at, COALESCE(duration_ms,0), agents_seen, skills_seen, bindings_seen, access_documents_seen, executions_seen,
diagnostics_seen, inventory_seen, capabilities_seen, delegations_seen,
COALESCE(error_code,''), COALESCE(error_message,'') FROM runtime_sync_runs
WHERE runtime_connection_id=$1 ORDER BY started_at DESC LIMIT $2`, runtimeID, limit)
	if err != nil {
		return nil, fmt.Errorf("list sync runs: %w", err)
	}
	defer rows.Close()
	var result []domain.RuntimeSyncRun
	for rows.Next() {
		var run domain.RuntimeSyncRun
		var finished sql.NullTime
		if err := rows.Scan(&run.ID, &run.RuntimeConnectionID, &run.Trigger, &run.Status, &run.StartedAt, &finished,
			&run.DurationMS, &run.AgentsSeen, &run.SkillsSeen, &run.BindingsSeen, &run.AccessDocumentsSeen, &run.ExecutionsSeen,
			&run.DiagnosticsSeen, &run.InventorySeen, &run.CapabilitiesSeen, &run.DelegationsSeen,
			&run.ErrorCode, &run.ErrorMessage); err != nil {
			return nil, fmt.Errorf("scan sync run: %w", err)
		}
		if finished.Valid {
			run.FinishedAt = &finished.Time
		}
		result = append(result, run)
	}
	return result, rows.Err()
}

func (r SyncRepository) GetRun(ctx context.Context, runtimeID, runID string) (domain.RuntimeSyncRun, error) {
	var run domain.RuntimeSyncRun
	var finished sql.NullTime
	err := r.db.QueryRowContext(ctx, `SELECT id,runtime_connection_id,trigger,status,started_at,finished_at,
COALESCE(duration_ms,0),agents_seen,skills_seen,bindings_seen,access_documents_seen,executions_seen,
diagnostics_seen,inventory_seen,capabilities_seen,delegations_seen,
COALESCE(error_code,''),COALESCE(error_message,'') FROM runtime_sync_runs WHERE runtime_connection_id=$1 AND id=$2`, runtimeID, runID).Scan(
		&run.ID, &run.RuntimeConnectionID, &run.Trigger, &run.Status, &run.StartedAt, &finished, &run.DurationMS, &run.AgentsSeen,
		&run.SkillsSeen, &run.BindingsSeen, &run.AccessDocumentsSeen, &run.ExecutionsSeen,
		&run.DiagnosticsSeen, &run.InventorySeen, &run.CapabilitiesSeen, &run.DelegationsSeen, &run.ErrorCode, &run.ErrorMessage)
	if err != nil {
		return run, err
	}
	if finished.Valid {
		run.FinishedAt = &finished.Time
	}
	return run, nil
}

func (r SyncRepository) ListPersistedAgents(ctx context.Context, runtimeID string) ([]domain.PersistedAgent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT a.id,a.name,a.status,a.metadata_json,a.created_at,a.updated_at,
b.runtime_connection_id,b.runtime_agent_id,COALESCE(b.parent_runtime_agent_id,''),b.kind,
COALESCE(b.last_seen_at,a.updated_at),rc.last_sync_at,rc.status,
CASE WHEN rc.status='degraded' OR a.status='stale' THEN 'stale'
WHEN rc.last_sync_at < now() - make_interval(secs => rc.sync_interval_seconds) THEN 'cached' ELSE 'live' END
FROM agents a JOIN agent_runtime_bindings b ON b.agent_id=a.id
JOIN runtime_connections rc ON rc.id=b.runtime_connection_id
WHERE ($1='' OR b.runtime_connection_id=$1::uuid) ORDER BY a.name`, runtimeID)
	if err != nil {
		return nil, fmt.Errorf("list persisted agents: %w", err)
	}
	defer rows.Close()
	var result []domain.PersistedAgent
	for rows.Next() {
		var item domain.PersistedAgent
		var metadata []byte
		var last sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.Status, &metadata, &item.CreatedAt, &item.UpdatedAt,
			&item.RuntimeConnectionID, &item.RuntimeAgentID, &item.ParentRuntimeAgentID, &item.Kind,
			&item.ObservedAt, &last, &item.RuntimeStatus, &item.Freshness); err != nil {
			return nil, fmt.Errorf("scan persisted agent: %w", err)
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		if last.Valid {
			item.LastSuccessfulSyncAt = &last.Time
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r SyncRepository) GetPersistedAgent(ctx context.Context, agentID string) (domain.PersistedAgentDetail, error) {
	agents, err := r.ListPersistedAgents(ctx, "")
	if err != nil {
		return domain.PersistedAgentDetail{}, err
	}
	var found *domain.PersistedAgent
	for i := range agents {
		if agents[i].ID == agentID {
			found = &agents[i]
			break
		}
	}
	if found == nil {
		return domain.PersistedAgentDetail{}, sql.ErrNoRows
	}
	detail := domain.PersistedAgentDetail{Agent: *found}
	rows, err := r.db.QueryContext(ctx, `SELECT s.runtime_skill_id,s.name,s.description,s.source,ab.status,s.version,
s.tool_ids_json,s.workflow_refs_json,s.metadata_json,ab.observed_at FROM agent_skill_bindings ab
JOIN runtime_skills s ON s.id=ab.runtime_skill_id WHERE ab.agent_id=$1 ORDER BY s.name`, agentID)
	if err != nil {
		return detail, fmt.Errorf("list persisted agent skills: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var skill domain.AgentSkillSnapshot
		var tools, flows, metadata []byte
		if err := rows.Scan(&skill.RuntimeSkillID, &skill.Name, &skill.Description, &skill.Source, &skill.Status, &skill.Version,
			&tools, &flows, &metadata, &skill.ObservedAt); err != nil {
			return detail, err
		}
		_ = json.Unmarshal(tools, &skill.ToolIDs)
		_ = json.Unmarshal(flows, &skill.WorkflowRefs)
		_ = json.Unmarshal(metadata, &skill.Metadata)
		detail.Skills = append(detail.Skills, skill)
	}
	var accessJSON []byte
	err = r.db.QueryRowContext(ctx, `SELECT access_json FROM access_actual_state WHERE agent_id=$1 AND runtime_connection_id=$2`, agentID, found.RuntimeConnectionID).Scan(&accessJSON)
	if err != nil && err != sql.ErrNoRows {
		return detail, fmt.Errorf("get persisted access: %w", err)
	}
	if err == nil {
		_ = json.Unmarshal(accessJSON, &detail.Access)
	}
	return detail, nil
}

func (r SyncRepository) MarkAgentDeleted(ctx context.Context, agentID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE agents SET status='disabled',
metadata_json=COALESCE(metadata_json,'{}'::jsonb) || jsonb_build_object(
  'runtime_deleted', true,
  'runtime_deleted_at', CURRENT_TIMESTAMP
), updated_at=CURRENT_TIMESTAMP WHERE id=$1::uuid`, agentID)
	if err != nil {
		return fmt.Errorf("mark persisted agent deleted: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read marked agent count: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r SyncRepository) ListAgentDelegations(ctx context.Context, runtimeID, runtimeAgentID string) ([]domain.PersistedAgentDelegation, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,runtime_connection_id,orchestrator_runtime_agent_id,
delegate_runtime_agent_id,delegate_ref,tool_name,display_name,persona,configured,resolved,revision,status,
observed_at,metadata_json,raw_runtime_json FROM agent_delegations
WHERE ($1='' OR runtime_connection_id=$1::uuid)
AND ($2='' OR orchestrator_runtime_agent_id=$2 OR delegate_runtime_agent_id=$2)
ORDER BY orchestrator_runtime_agent_id,display_name,delegate_ref`, runtimeID, runtimeAgentID)
	if err != nil {
		return nil, fmt.Errorf("list agent delegations: %w", err)
	}
	defer rows.Close()
	var result []domain.PersistedAgentDelegation
	for rows.Next() {
		var item domain.PersistedAgentDelegation
		var metadata, raw []byte
		if err := rows.Scan(&item.ID, &item.RuntimeConnectionID, &item.OrchestratorRuntimeAgentID,
			&item.DelegateRuntimeAgentID, &item.DelegateRef, &item.ToolName, &item.DisplayName,
			&item.Persona, &item.Configured, &item.Resolved, &item.Revision, &item.Status,
			&item.ObservedAt, &metadata, &raw); err != nil {
			return nil, fmt.Errorf("scan agent delegation: %w", err)
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		_ = json.Unmarshal(raw, &item.Raw)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r SyncRepository) ListSubagentExecutions(ctx context.Context, runtimeID, agentID string) ([]domain.PersistedSubagentExecution, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,runtime_connection_id,runtime_execution_id,parent_run_id,
runtime_agent_id,subagent_type,status,description,summary,started_at,ended_at,observed_at,metadata_json,raw_runtime_json
FROM subagent_executions WHERE ($1='' OR runtime_connection_id=$1::uuid)
AND ($2='' OR runtime_agent_id=(SELECT runtime_agent_id FROM agent_runtime_bindings WHERE agent_id=$2::uuid))
ORDER BY observed_at DESC LIMIT 200`, runtimeID, agentID)
	if err != nil {
		return nil, fmt.Errorf("list subagent executions: %w", err)
	}
	defer rows.Close()
	var result []domain.PersistedSubagentExecution
	for rows.Next() {
		var item domain.PersistedSubagentExecution
		var started, ended sql.NullTime
		var metadata, raw []byte
		if err := rows.Scan(&item.ID, &item.RuntimeConnectionID, &item.RuntimeExecutionID, &item.ParentRunID,
			&item.RuntimeAgentID, &item.SubagentType, &item.Status, &item.Description, &item.Summary, &started, &ended,
			&item.ObservedAt, &metadata, &raw); err != nil {
			return nil, fmt.Errorf("scan subagent execution: %w", err)
		}
		if started.Valid {
			item.StartedAt = &started.Time
		}
		if ended.Valid {
			item.EndedAt = &ended.Time
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		_ = json.Unmarshal(raw, &item.Raw)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r SyncRepository) ListRuntimeExecutions(ctx context.Context, runtimeID, agentID, kind string, limit int) ([]domain.PersistedRuntimeExecution, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id,runtime_connection_id,runtime_execution_id,
COALESCE(parent_runtime_execution_id,''),runtime_agent_id,kind,status,started_at,ended_at,observed_at,
metadata_json,raw_runtime_json FROM runtime_executions
WHERE ($1='' OR runtime_connection_id=$1::uuid) AND ($2='' OR runtime_agent_id=$2)
AND ($3='' OR kind=$3) ORDER BY observed_at DESC LIMIT $4`, runtimeID, agentID, kind, limit)
	if err != nil {
		return nil, fmt.Errorf("list runtime executions: %w", err)
	}
	defer rows.Close()
	var result []domain.PersistedRuntimeExecution
	for rows.Next() {
		var item domain.PersistedRuntimeExecution
		var started, ended sql.NullTime
		var metadata, raw []byte
		if err := rows.Scan(&item.ID, &item.RuntimeConnectionID, &item.RuntimeExecutionID,
			&item.ParentRuntimeExecutionID, &item.RuntimeAgentID, &item.Kind, &item.Status, &started,
			&ended, &item.ObservedAt, &metadata, &raw); err != nil {
			return nil, fmt.Errorf("scan runtime execution: %w", err)
		}
		if started.Valid {
			item.StartedAt = &started.Time
		}
		if ended.Valid {
			item.EndedAt = &ended.Time
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		_ = json.Unmarshal(raw, &item.Raw)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r SyncRepository) GetRuntimeExecution(ctx context.Context, executionID string) (domain.PersistedRuntimeExecution, error) {
	var item domain.PersistedRuntimeExecution
	var started, ended sql.NullTime
	var metadata, raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT id,runtime_connection_id,runtime_execution_id,
COALESCE(parent_runtime_execution_id,''),runtime_agent_id,kind,status,started_at,ended_at,observed_at,
metadata_json,raw_runtime_json FROM runtime_executions WHERE id=$1::uuid`, executionID).Scan(
		&item.ID, &item.RuntimeConnectionID, &item.RuntimeExecutionID,
		&item.ParentRuntimeExecutionID, &item.RuntimeAgentID, &item.Kind, &item.Status, &started,
		&ended, &item.ObservedAt, &metadata, &raw,
	)
	if err != nil {
		return domain.PersistedRuntimeExecution{}, fmt.Errorf("get runtime execution: %w", err)
	}
	if started.Valid {
		item.StartedAt = &started.Time
	}
	if ended.Valid {
		item.EndedAt = &ended.Time
	}
	_ = json.Unmarshal(metadata, &item.Metadata)
	_ = json.Unmarshal(raw, &item.Raw)
	return item, nil
}

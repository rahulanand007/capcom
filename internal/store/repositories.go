package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"capcom/internal/domain"
	"capcom/internal/tenant"
)

type RuntimeConnectionRepository struct {
	db *sql.DB
}

type AgentRepository struct {
	db *sql.DB
}

type AuditRepository struct {
	db *sql.DB
}

type SecretRepository struct {
	db *sql.DB
}

func NewRuntimeConnectionRepository(db *sql.DB) RuntimeConnectionRepository {
	return RuntimeConnectionRepository{db: db}
}

func NewAgentRepository(db *sql.DB) AgentRepository {
	return AgentRepository{db: db}
}

func NewAuditRepository(db *sql.DB) AuditRepository {
	return AuditRepository{db: db}
}

func NewSecretRepository(db *sql.DB) SecretRepository {
	return SecretRepository{db: db}
}

func (r SecretRepository) Create(ctx context.Context, secret domain.Secret, ciphertext []byte) (domain.Secret, error) {
	if secret.ID == "" {
		id, err := newID()
		if err != nil {
			return domain.Secret{}, err
		}
		secret.ID = id
	}
	now := time.Now().UTC()
	secret.CreatedAt = now
	secret.UpdatedAt = now
	if _, err := r.db.ExecContext(ctx, `
INSERT INTO secrets (id, name, ciphertext, organization_id, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6)`, secret.ID, secret.Name, ciphertext, organizationIDForWrite(ctx), secret.CreatedAt, secret.UpdatedAt); err != nil {
		return domain.Secret{}, fmt.Errorf("create secret: %w", err)
	}
	return secret, nil
}

func (r SecretRepository) Rotate(ctx context.Context, name string, ciphertext []byte) (domain.Secret, error) {
	var secret domain.Secret
	if err := r.db.QueryRowContext(ctx, `
UPDATE secrets
SET ciphertext = $2, updated_at = now()
WHERE name = $1 AND organization_id=$3
RETURNING id, name, created_at, updated_at`, name, ciphertext, organizationIDForWrite(ctx)).Scan(
		&secret.ID, &secret.Name, &secret.CreatedAt, &secret.UpdatedAt,
	); err != nil {
		return domain.Secret{}, fmt.Errorf("rotate secret: %w", err)
	}
	return secret, nil
}

func (r SecretRepository) GetCiphertext(ctx context.Context, name string) ([]byte, error) {
	var ciphertext []byte
	if err := r.db.QueryRowContext(ctx, `SELECT ciphertext FROM secrets WHERE name = $1 AND organization_id=$2`, name, organizationIDForWrite(ctx)).Scan(&ciphertext); err != nil {
		return nil, fmt.Errorf("get secret ciphertext: %w", err)
	}
	return ciphertext, nil
}

func (r AuditRepository) Create(ctx context.Context, event domain.AuditEvent) (domain.AuditEvent, error) {
	if event.ID == "" {
		id, err := newID()
		if err != nil {
			return domain.AuditEvent{}, err
		}
		event.ID = id
	}
	event.CreatedAt = time.Now().UTC()

	beforeJSON, err := marshalNullableJSON(event.Before)
	if err != nil {
		return domain.AuditEvent{}, fmt.Errorf("marshal audit before: %w", err)
	}
	afterJSON, err := marshalNullableJSON(event.After)
	if err != nil {
		return domain.AuditEvent{}, fmt.Errorf("marshal audit after: %w", err)
	}
	metadataJSON, err := marshalNullableJSON(event.Metadata)
	if err != nil {
		return domain.AuditEvent{}, fmt.Errorf("marshal audit metadata: %w", err)
	}

	if _, err := r.db.ExecContext(ctx, `
INSERT INTO audit_events (
	id, organization_id, runtime_connection_id, agent_id, control_action_id, actor, event_type,
	target_type, target_id, reason, before_json, after_json, result, metadata_json, created_at
) VALUES ($1, $2, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid, $6, $7, $8, $9, $10, $11::jsonb, $12::jsonb, $13, COALESCE($14::jsonb, '{}'::jsonb), $15)`,
		event.ID,
		organizationIDForWrite(ctx),
		event.RuntimeConnectionID,
		event.AgentID,
		event.ControlActionID,
		event.Actor,
		event.EventType,
		event.TargetType,
		event.TargetID,
		event.Reason,
		beforeJSON,
		afterJSON,
		event.Result,
		metadataJSON,
		event.CreatedAt,
	); err != nil {
		return domain.AuditEvent{}, fmt.Errorf("create audit event: %w", err)
	}

	return event, nil
}

func (r RuntimeConnectionRepository) Create(ctx context.Context, conn domain.RuntimeConnection) (domain.RuntimeConnection, error) {
	if conn.ID == "" {
		id, err := newID()
		if err != nil {
			return domain.RuntimeConnection{}, err
		}
		conn.ID = id
	}
	now := time.Now().UTC()
	conn.CreatedAt = now
	conn.UpdatedAt = now
	if conn.DisplayName == "" {
		conn.DisplayName = conn.Name
	}
	if conn.Environment == "" {
		conn.Environment = "unspecified"
	}
	if conn.Labels == nil {
		conn.Labels = map[string]string{}
	}
	if conn.Metadata == nil {
		conn.Metadata = map[string]any{}
	}
	conn.OrganizationID = organizationIDForWrite(ctx)
	metadata, err := json.Marshal(conn.Metadata)
	if err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("marshal runtime metadata: %w", err)
	}
	labels, err := json.Marshal(conn.Labels)
	if err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("marshal runtime labels: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("begin runtime connection creation: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO runtime_connections (
	id, name, display_name, environment, runtime_type, mode, status, endpoint, auth_ref,
	metadata_json, labels_json, organization_id, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10::jsonb, $11::jsonb, $12, $13, $14)`,
		conn.ID, conn.Name, conn.DisplayName, conn.Environment, conn.Kind, conn.Mode, conn.Status,
		conn.BaseURL, conn.AuthRef, string(metadata), string(labels), conn.OrganizationID, conn.CreatedAt, conn.UpdatedAt); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("create runtime connection: %w", err)
	}
	endpointID, err := newID()
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO runtime_connection_endpoints (
	id, runtime_connection_id, endpoint, kind, auth_ref, created_by, reason, created_at, updated_at
) VALUES ($1, $2, $3, 'canonical', NULLIF($4, ''), 'system', 'Canonical endpoint created with runtime instance', $5, $5)`,
		endpointID, conn.ID, conn.BaseURL, conn.AuthRef, now); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("create canonical runtime endpoint: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("commit runtime connection creation: %w", err)
	}
	conn.Endpoints = []domain.RuntimeEndpoint{{
		ID: endpointID, RuntimeConnectionID: conn.ID, URL: conn.BaseURL,
		Kind: domain.RuntimeEndpointCanonical, AuthRef: conn.AuthRef,
		CreatedAt: now, UpdatedAt: now,
	}}
	return conn, nil
}

func (r RuntimeConnectionRepository) UpdateSettings(ctx context.Context, conn domain.RuntimeConnection) (domain.RuntimeConnection, error) {
	labels, err := json.Marshal(conn.Labels)
	if err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("marshal runtime labels: %w", err)
	}
	metadata, err := json.Marshal(conn.Metadata)
	if err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("marshal runtime metadata: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("begin runtime settings update: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE runtime_connections SET
display_name=$2, environment=$3, labels_json=$4::jsonb, mode=$5, endpoint=$6,
auth_ref=NULLIF($7, ''), metadata_json=$8::jsonb, sync_enabled=$9,
sync_interval_seconds=$10, updated_at=now()
WHERE id=$1 AND archived_at IS NULL AND ($11='' OR organization_id=$11::uuid)`, conn.ID, conn.DisplayName, conn.Environment, string(labels), conn.Mode,
		conn.BaseURL, conn.AuthRef, string(metadata), conn.SyncEnabled, conn.SyncIntervalSeconds, tenant.OrganizationID(ctx)); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("update runtime instance settings: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE runtime_connection_endpoints
SET endpoint=$2, auth_ref=NULLIF($3, ''), updated_at=now()
WHERE runtime_connection_id=$1 AND kind='canonical'`,
		conn.ID, conn.BaseURL, conn.AuthRef); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("update canonical runtime endpoint: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("commit runtime settings update: %w", err)
	}
	return r.Get(ctx, conn.ID)
}

func (r RuntimeConnectionRepository) Get(ctx context.Context, id string) (domain.RuntimeConnection, error) {
	var conn domain.RuntimeConnection
	var lastSyncedAt, lastSyncStartedAt, lastSyncFinishedAt sql.NullTime
	var lastSyncStatus, lastError sql.NullString
	var metadata, labels []byte
	if err := r.db.QueryRowContext(ctx, `
SELECT id, organization_id, name, display_name, environment, runtime_type, mode, status, endpoint, COALESCE(auth_ref, ''),
metadata_json, labels_json, created_at, updated_at, last_sync_at,
sync_enabled, sync_interval_seconds, last_sync_status, last_sync_started_at, last_sync_finished_at,
COALESCE(last_sync_duration_ms,0), last_error
FROM runtime_connections
WHERE id = $1 AND archived_at IS NULL AND ($2='' OR organization_id=$2::uuid)`, id, tenant.OrganizationID(ctx)).Scan(
		&conn.ID,
		&conn.OrganizationID,
		&conn.Name,
		&conn.DisplayName,
		&conn.Environment,
		&conn.Kind,
		&conn.Mode,
		&conn.Status,
		&conn.BaseURL,
		&conn.AuthRef,
		&metadata,
		&labels,
		&conn.CreatedAt,
		&conn.UpdatedAt,
		&lastSyncedAt,
		&conn.SyncEnabled,
		&conn.SyncIntervalSeconds,
		&lastSyncStatus,
		&lastSyncStartedAt,
		&lastSyncFinishedAt,
		&conn.LastSyncDurationMS,
		&lastError,
	); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("get runtime connection: %w", err)
	}
	if lastSyncedAt.Valid {
		conn.LastSyncedAt = &lastSyncedAt.Time
	}
	if lastSyncStartedAt.Valid {
		conn.LastSyncStartedAt = &lastSyncStartedAt.Time
	}
	if lastSyncFinishedAt.Valid {
		conn.LastSyncFinishedAt = &lastSyncFinishedAt.Time
	}
	if lastSyncStatus.Valid {
		conn.LastSyncStatus = domain.SyncStatus(lastSyncStatus.String)
	}
	if lastError.Valid {
		conn.LastError = lastError.String
	}
	if err := json.Unmarshal(metadata, &conn.Metadata); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("decode runtime metadata: %w", err)
	}
	if err := json.Unmarshal(labels, &conn.Labels); err != nil {
		return domain.RuntimeConnection{}, fmt.Errorf("decode runtime labels: %w", err)
	}
	endpoints, err := r.listEndpoints(ctx, conn.ID)
	if err != nil {
		return domain.RuntimeConnection{}, err
	}
	conn.Endpoints = endpoints
	return conn, nil
}

func (r RuntimeConnectionRepository) List(ctx context.Context) ([]domain.RuntimeConnection, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, organization_id, name, display_name, environment, runtime_type, mode, status, endpoint, COALESCE(auth_ref, ''),
metadata_json, labels_json, created_at, updated_at, last_sync_at,
sync_enabled, sync_interval_seconds, last_sync_status, last_sync_started_at, last_sync_finished_at,
COALESCE(last_sync_duration_ms,0), last_error
FROM runtime_connections
WHERE archived_at IS NULL AND ($1='' OR organization_id=$1::uuid)
ORDER BY name`, tenant.OrganizationID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list runtime connections: %w", err)
	}
	defer rows.Close()

	var conns []domain.RuntimeConnection
	for rows.Next() {
		var conn domain.RuntimeConnection
		var lastSyncedAt, lastSyncStartedAt, lastSyncFinishedAt sql.NullTime
		var lastSyncStatus, lastError sql.NullString
		var metadata, labels []byte
		if err := rows.Scan(
			&conn.ID,
			&conn.OrganizationID,
			&conn.Name,
			&conn.DisplayName,
			&conn.Environment,
			&conn.Kind,
			&conn.Mode,
			&conn.Status,
			&conn.BaseURL,
			&conn.AuthRef,
			&metadata,
			&labels,
			&conn.CreatedAt,
			&conn.UpdatedAt,
			&lastSyncedAt,
			&conn.SyncEnabled,
			&conn.SyncIntervalSeconds,
			&lastSyncStatus,
			&lastSyncStartedAt,
			&lastSyncFinishedAt,
			&conn.LastSyncDurationMS,
			&lastError,
		); err != nil {
			return nil, fmt.Errorf("scan runtime connection: %w", err)
		}
		if lastSyncedAt.Valid {
			conn.LastSyncedAt = &lastSyncedAt.Time
		}
		if lastSyncStartedAt.Valid {
			conn.LastSyncStartedAt = &lastSyncStartedAt.Time
		}
		if lastSyncFinishedAt.Valid {
			conn.LastSyncFinishedAt = &lastSyncFinishedAt.Time
		}
		if lastSyncStatus.Valid {
			conn.LastSyncStatus = domain.SyncStatus(lastSyncStatus.String)
		}
		if lastError.Valid {
			conn.LastError = lastError.String
		}
		if err := json.Unmarshal(metadata, &conn.Metadata); err != nil {
			return nil, fmt.Errorf("decode runtime metadata: %w", err)
		}
		if err := json.Unmarshal(labels, &conn.Labels); err != nil {
			return nil, fmt.Errorf("decode runtime labels: %w", err)
		}
		endpoints, err := r.listEndpoints(ctx, conn.ID)
		if err != nil {
			return nil, err
		}
		conn.Endpoints = endpoints
		conns = append(conns, conn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime connections: %w", err)
	}
	return conns, nil
}

// Archive removes a runtime instance from active operation without deleting its
// imported state, control history, or audit trail.
func (r RuntimeConnectionRepository) Archive(ctx context.Context, id, actor, reason string) error {
	result, err := r.db.ExecContext(ctx, `
UPDATE runtime_connections
SET archived_at = now(), archived_by = $2, archive_reason = $3,
    archive_kind = 'removed', sync_enabled = false, status = 'disabled', updated_at = now()
WHERE id = $1 AND archived_at IS NULL AND ($4='' OR organization_id=$4::uuid)`, id, actor, reason, tenant.OrganizationID(ctx))
	if err != nil {
		return fmt.Errorf("archive runtime connection: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("archive runtime connection rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("archive runtime connection: %w", sql.ErrNoRows)
	}
	return nil
}

func (r RuntimeConnectionRepository) listEndpoints(ctx context.Context, runtimeID string) ([]domain.RuntimeEndpoint, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, runtime_connection_id, endpoint, kind, COALESCE(auth_ref, ''),
       COALESCE(source_runtime_connection_id::text, ''), created_at, updated_at
FROM runtime_connection_endpoints
WHERE runtime_connection_id=$1
ORDER BY CASE kind WHEN 'canonical' THEN 0 WHEN 'alias' THEN 1 ELSE 2 END, created_at`, runtimeID)
	if err != nil {
		return nil, fmt.Errorf("list runtime endpoints: %w", err)
	}
	defer rows.Close()
	endpoints := []domain.RuntimeEndpoint{}
	for rows.Next() {
		var endpoint domain.RuntimeEndpoint
		if err := rows.Scan(
			&endpoint.ID, &endpoint.RuntimeConnectionID, &endpoint.URL, &endpoint.Kind,
			&endpoint.AuthRef, &endpoint.SourceRuntimeConnectionID, &endpoint.CreatedAt, &endpoint.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan runtime endpoint: %w", err)
		}
		endpoints = append(endpoints, endpoint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime endpoints: %w", err)
	}
	return endpoints, nil
}

// ConsolidateEndpoint converts an active duplicate runtime record into an
// endpoint route owned by the canonical runtime. Imported state is retained on
// the archived source record for audit and recovery.
func (r RuntimeConnectionRepository) ConsolidateEndpoint(
	ctx context.Context,
	canonicalID, duplicateID string,
	kind domain.RuntimeEndpointKind,
	actor, reason string,
) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin endpoint consolidation: %w", err)
	}
	defer tx.Rollback()

	var canonicalKind, duplicateKind domain.RuntimeKind
	var endpoint, authRef string
	if err := tx.QueryRowContext(ctx, `
SELECT runtime_type FROM runtime_connections
WHERE id=$1 AND archived_at IS NULL AND ($2='' OR organization_id=$2::uuid) FOR UPDATE`, canonicalID, tenant.OrganizationID(ctx)).Scan(&canonicalKind); err != nil {
		return fmt.Errorf("get canonical runtime connection: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `
SELECT runtime_type, endpoint, COALESCE(auth_ref, '') FROM runtime_connections
WHERE id=$1 AND archived_at IS NULL AND ($2='' OR organization_id=$2::uuid) FOR UPDATE`, duplicateID, tenant.OrganizationID(ctx)).Scan(&duplicateKind, &endpoint, &authRef); err != nil {
		return fmt.Errorf("get duplicate runtime connection: %w", err)
	}
	if canonicalKind != duplicateKind {
		return fmt.Errorf("runtime types must match")
	}
	endpointID, err := newID()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO runtime_connection_endpoints (
	id, runtime_connection_id, endpoint, kind, auth_ref,
	source_runtime_connection_id, created_by, reason
) VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7, $8)`,
		endpointID, canonicalID, endpoint, kind, authRef, duplicateID, actor, reason); err != nil {
		return fmt.Errorf("create runtime endpoint alias: %w", err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE runtime_connections
SET archived_at=now(), archived_by=$3, archive_reason=$4, archive_kind=$5,
    merged_into_runtime_connection_id=$2, sync_enabled=false,
    status='disabled', updated_at=now()
WHERE id=$1 AND archived_at IS NULL`,
		duplicateID, canonicalID, actor, reason, kind)
	if err != nil {
		return fmt.Errorf("archive duplicate runtime connection: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		if err != nil {
			return fmt.Errorf("archive duplicate runtime connection rows affected: %w", err)
		}
		return fmt.Errorf("archive duplicate runtime connection: %w", sql.ErrNoRows)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit endpoint consolidation: %w", err)
	}
	return nil
}

func (r AgentRepository) Create(ctx context.Context, agent domain.Agent, binding domain.AgentBinding) (domain.Agent, domain.AgentBinding, error) {
	if agent.ID == "" {
		id, err := newID()
		if err != nil {
			return domain.Agent{}, domain.AgentBinding{}, err
		}
		agent.ID = id
	}
	if binding.ID == "" {
		id, err := newID()
		if err != nil {
			return domain.Agent{}, domain.AgentBinding{}, err
		}
		binding.ID = id
	}
	binding.AgentID = agent.ID
	now := time.Now().UTC()
	agent.CreatedAt = now
	agent.UpdatedAt = now
	binding.CreatedAt = now
	binding.UpdatedAt = now

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Agent{}, domain.AgentBinding{}, fmt.Errorf("begin create agent: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO agents (id, organization_id, name, status, metadata_json, created_at, updated_at)
SELECT $1, organization_id, $2, $3, '{}'::jsonb, $4, $5 FROM runtime_connections WHERE id=$6 AND ($7='' OR organization_id=$7::uuid)`,
		agent.ID, agent.Name, agent.Status, agent.CreatedAt, agent.UpdatedAt, binding.RuntimeConnectionID, tenant.OrganizationID(ctx)); err != nil {
		return domain.Agent{}, domain.AgentBinding{}, fmt.Errorf("create agent: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_runtime_bindings (
	id, agent_id, runtime_connection_id, runtime_agent_id, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6)`,
		binding.ID, binding.AgentID, binding.RuntimeConnectionID, binding.RuntimeAgentID, binding.CreatedAt, binding.UpdatedAt); err != nil {
		return domain.Agent{}, domain.AgentBinding{}, fmt.Errorf("create agent binding: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return domain.Agent{}, domain.AgentBinding{}, fmt.Errorf("commit create agent: %w", err)
	}

	return agent, binding, nil
}

func (r AgentRepository) Get(ctx context.Context, id string) (domain.Agent, error) {
	var agent domain.Agent
	if err := r.db.QueryRowContext(ctx, `
SELECT id, name, status, created_at, updated_at
FROM agents
WHERE id = $1 AND ($2='' OR organization_id=$2::uuid)`, id, tenant.OrganizationID(ctx)).Scan(&agent.ID, &agent.Name, &agent.Status, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
		return domain.Agent{}, fmt.Errorf("get agent: %w", err)
	}
	return agent, nil
}

func (r AgentRepository) List(ctx context.Context) ([]domain.Agent, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, status, created_at, updated_at
FROM agents
WHERE ($1='' OR organization_id=$1::uuid)
ORDER BY name`, tenant.OrganizationID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []domain.Agent
	for rows.Next() {
		var agent domain.Agent
		if err := rows.Scan(&agent.ID, &agent.Name, &agent.Status, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, agent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agents: %w", err)
	}
	return agents, nil
}

func newID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	encoded := hex.EncodeToString(bytes[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]), nil
}

func marshalNullableJSON(value map[string]any) (any, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

func organizationIDForWrite(ctx context.Context) string {
	if organizationID := tenant.OrganizationID(ctx); organizationID != "" {
		return organizationID
	}
	return bootstrapOrganizationID
}

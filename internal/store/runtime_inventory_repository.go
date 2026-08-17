package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"capcom/internal/domain"
	"capcom/internal/tenant"
)

func upsertRuntimeDiagnostic(ctx context.Context, tx *sql.Tx, runtimeID string, item domain.RuntimeDiagnosticSnapshot) error {
	metadata, _ := json.Marshal(item.Metadata)
	raw, _ := json.Marshal(item.Raw)
	_, err := tx.ExecContext(ctx, `INSERT INTO runtime_diagnostics
(runtime_connection_id,check_id,status,message,observed_at,metadata_json,raw_runtime_json)
VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb)
ON CONFLICT (runtime_connection_id,check_id) DO UPDATE SET status=EXCLUDED.status,message=EXCLUDED.message,
observed_at=EXCLUDED.observed_at,metadata_json=EXCLUDED.metadata_json,raw_runtime_json=EXCLUDED.raw_runtime_json,updated_at=now()`,
		runtimeID, item.CheckID, item.Status, item.Message, item.ObservedAt, string(metadata), string(raw))
	if err != nil {
		return fmt.Errorf("upsert runtime diagnostic: %w", err)
	}
	return nil
}

func upsertRuntimeInventory(ctx context.Context, tx *sql.Tx, runtimeID string, item domain.RuntimeInventorySnapshot) error {
	metadata, _ := json.Marshal(item.Metadata)
	raw, _ := json.Marshal(item.Raw)
	_, err := tx.ExecContext(ctx, `INSERT INTO runtime_inventory_items
(runtime_connection_id,runtime_item_id,kind,name,status,provider,source,observed_at,metadata_json,raw_runtime_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb)
ON CONFLICT (runtime_connection_id,kind,runtime_item_id) DO UPDATE SET name=EXCLUDED.name,status=EXCLUDED.status,
provider=EXCLUDED.provider,source=EXCLUDED.source,observed_at=EXCLUDED.observed_at,metadata_json=EXCLUDED.metadata_json,
raw_runtime_json=EXCLUDED.raw_runtime_json,updated_at=now()`, runtimeID, item.RuntimeItemID, item.Kind, item.Name,
		item.Status, item.Provider, item.Source, item.ObservedAt, string(metadata), string(raw))
	if err != nil {
		return fmt.Errorf("upsert runtime inventory: %w", err)
	}
	return nil
}

func upsertRuntimeCapability(ctx context.Context, tx *sql.Tx, runtimeID string, item domain.RuntimeCapabilitySnapshot) error {
	metadata, _ := json.Marshal(item.Metadata)
	raw, _ := json.Marshal(item.Raw)
	_, err := tx.ExecContext(ctx, `INSERT INTO runtime_capabilities
(runtime_connection_id,runtime_capability_id,version,name,category,risk,can_description,cannot_description,source,observed_at,metadata_json,raw_runtime_json)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12::jsonb)
ON CONFLICT (runtime_connection_id,runtime_capability_id,version) DO UPDATE SET name=EXCLUDED.name,category=EXCLUDED.category,
risk=EXCLUDED.risk,can_description=EXCLUDED.can_description,cannot_description=EXCLUDED.cannot_description,
source=EXCLUDED.source,observed_at=EXCLUDED.observed_at,metadata_json=EXCLUDED.metadata_json,
raw_runtime_json=EXCLUDED.raw_runtime_json,updated_at=now()`, runtimeID, item.RuntimeCapabilityID, item.Version,
		item.Name, item.Category, item.Risk, item.Can, item.Cannot, item.Source, item.ObservedAt, string(metadata), string(raw))
	if err != nil {
		return fmt.Errorf("upsert runtime capability: %w", err)
	}
	return nil
}

func (r SyncRepository) ListRuntimeDiagnostics(ctx context.Context, runtimeID string) ([]domain.PersistedRuntimeDiagnostic, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,runtime_connection_id,check_id,status,message,observed_at,metadata_json,raw_runtime_json
FROM runtime_diagnostics WHERE runtime_connection_id=$1 AND ($2='' OR EXISTS(SELECT 1 FROM runtime_connections rc WHERE rc.id=runtime_diagnostics.runtime_connection_id AND rc.organization_id=$2::uuid)) ORDER BY check_id`, runtimeID, tenant.OrganizationID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list runtime diagnostics: %w", err)
	}
	defer rows.Close()
	var result []domain.PersistedRuntimeDiagnostic
	for rows.Next() {
		var item domain.PersistedRuntimeDiagnostic
		var metadata, raw []byte
		if err := rows.Scan(&item.ID, &item.RuntimeConnectionID, &item.CheckID, &item.Status, &item.Message,
			&item.ObservedAt, &metadata, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		_ = json.Unmarshal(raw, &item.Raw)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r SyncRepository) ListRuntimeInventory(ctx context.Context, runtimeID, kind string) ([]domain.PersistedRuntimeInventory, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,runtime_connection_id,runtime_item_id,kind,name,status,provider,source,
observed_at,metadata_json,raw_runtime_json FROM runtime_inventory_items
WHERE runtime_connection_id=$1 AND ($2='' OR kind=$2) AND ($3='' OR EXISTS(SELECT 1 FROM runtime_connections rc WHERE rc.id=runtime_inventory_items.runtime_connection_id AND rc.organization_id=$3::uuid)) ORDER BY kind,name`, runtimeID, kind, tenant.OrganizationID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list runtime inventory: %w", err)
	}
	defer rows.Close()
	var result []domain.PersistedRuntimeInventory
	for rows.Next() {
		var item domain.PersistedRuntimeInventory
		var metadata, raw []byte
		if err := rows.Scan(&item.ID, &item.RuntimeConnectionID, &item.RuntimeItemID, &item.Kind, &item.Name,
			&item.Status, &item.Provider, &item.Source, &item.ObservedAt, &metadata, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		_ = json.Unmarshal(raw, &item.Raw)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r SyncRepository) ListRuntimeCapabilities(ctx context.Context, runtimeID string) ([]domain.PersistedRuntimeCapability, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,runtime_connection_id,runtime_capability_id,version,name,category,risk,
can_description,cannot_description,source,observed_at,metadata_json,raw_runtime_json FROM runtime_capabilities
WHERE runtime_connection_id=$1 AND ($2='' OR EXISTS(SELECT 1 FROM runtime_connections rc WHERE rc.id=runtime_capabilities.runtime_connection_id AND rc.organization_id=$2::uuid)) ORDER BY risk,name`, runtimeID, tenant.OrganizationID(ctx))
	if err != nil {
		return nil, fmt.Errorf("list runtime capabilities: %w", err)
	}
	defer rows.Close()
	var result []domain.PersistedRuntimeCapability
	for rows.Next() {
		var item domain.PersistedRuntimeCapability
		var metadata, raw []byte
		if err := rows.Scan(&item.ID, &item.RuntimeConnectionID, &item.RuntimeCapabilityID, &item.Version, &item.Name,
			&item.Category, &item.Risk, &item.Can, &item.Cannot, &item.Source, &item.ObservedAt, &metadata, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		_ = json.Unmarshal(raw, &item.Raw)
		result = append(result, item)
	}
	return result, rows.Err()
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"capcom/internal/domain"
	"capcom/internal/tenant"
)

type ControlActionRepository struct{ db *sql.DB }

func NewControlActionRepository(db *sql.DB) ControlActionRepository {
	return ControlActionRepository{db: db}
}

func (r ControlActionRepository) Create(ctx context.Context, action domain.ControlAction) (domain.ControlAction, error) {
	if action.ID == "" {
		id, err := newID()
		if err != nil {
			return action, err
		}
		action.ID = id
	}
	now := time.Now().UTC()
	action.CreatedAt = now
	action.UpdatedAt = now
	parameters, err := json.Marshal(action.After)
	if err != nil {
		return action, fmt.Errorf("marshal control action parameters: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO control_actions
(id,organization_id,runtime_connection_id,agent_id,action_type,requested_by,reason,idempotency_key,parameters_json,status,created_at,updated_at)
SELECT $1,organization_id,$2,$3,$4,$5,$6,NULLIF($7,''),$8::jsonb,$9,$10,$10 FROM runtime_connections WHERE id=$2 AND ($11='' OR organization_id=$11::uuid)`, action.ID, action.RuntimeConnectionID,
		action.AgentID, action.Type, action.Actor, action.Reason, action.IdempotencyKey, string(parameters), action.Status, now, tenant.OrganizationID(ctx))
	if err != nil {
		return action, fmt.Errorf("create control action: %w", err)
	}
	return action, nil
}

func (r ControlActionRepository) FindByIdempotencyKey(ctx context.Context, key string) (domain.ControlAction, error) {
	var action domain.ControlAction
	var parameters, response []byte
	var errorText sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id,runtime_connection_id,COALESCE(agent_id::text,''),action_type,
requested_by,reason,COALESCE(idempotency_key,''),parameters_json,COALESCE(runtime_response_json,'{}'::jsonb),
status,error,created_at,updated_at FROM control_actions WHERE idempotency_key=$1 AND ($2='' OR EXISTS(SELECT 1 FROM runtime_connections rc WHERE rc.id=control_actions.runtime_connection_id AND rc.organization_id=$2::uuid))`, key, tenant.OrganizationID(ctx)).Scan(&action.ID, &action.RuntimeConnectionID,
		&action.AgentID, &action.Type, &action.Actor, &action.Reason, &action.IdempotencyKey, &parameters, &response, &action.Status, &errorText, &action.CreatedAt, &action.UpdatedAt)
	if err != nil {
		return action, err
	}
	_ = json.Unmarshal(parameters, &action.After)
	_ = json.Unmarshal(response, &action.Result)
	if action.Result == nil {
		action.Result = map[string]any{}
	}
	if errorText.Valid {
		action.Result["error"] = errorText.String
	}
	return action, nil
}

func (r ControlActionRepository) Update(ctx context.Context, action domain.ControlAction, runtimeRequest, runtimeResponse map[string]any, errorText string) (domain.ControlAction, error) {
	requestJSON, _ := json.Marshal(runtimeRequest)
	responseJSON, _ := json.Marshal(runtimeResponse)
	finished := any(nil)
	if action.Status == domain.ControlActionSucceeded || action.Status == domain.ControlActionFailed || action.Status == domain.ControlActionRejected {
		finished = time.Now().UTC()
	}
	err := r.db.QueryRowContext(ctx, `UPDATE control_actions SET status=$2,runtime_request_json=$3::jsonb,
runtime_response_json=$4::jsonb,error=NULLIF($5,''),updated_at=now(),finished_at=$6 WHERE id=$1
RETURNING updated_at`, action.ID, action.Status, string(requestJSON), string(responseJSON), errorText, finished).Scan(&action.UpdatedAt)
	if err != nil {
		return action, fmt.Errorf("update control action: %w", err)
	}
	action.Result = runtimeResponse
	return action, nil
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"capcom/internal/domain"
)

const bootstrapOrganizationID = "00000000-0000-4000-8000-000000000001"

var ErrIdentityNotFound = errors.New("identity not found")
var ErrEmailExists = errors.New("email already exists")

type AuthRepository struct{ db *sql.DB }

type NewAccount struct {
	Email, EmailNormalized, PasswordHash string
	TokenHash, CSRFHash                  []byte
	AbsoluteExpiry, IdleExpiry           time.Time
}

type LoginIdentity struct {
	User         domain.User
	Organization domain.Organization
	Role         string
	PasswordHash string
}

type NewSession struct {
	UserID, OrganizationID     string
	TokenHash, CSRFHash        []byte
	AbsoluteExpiry, IdleExpiry time.Time
}

func NewAuthRepository(db *sql.DB) AuthRepository { return AuthRepository{db: db} }

func (r AuthRepository) CreateAccount(ctx context.Context, input NewAccount) (domain.AuthSession, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AuthSession{}, fmt.Errorf("begin signup: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1128351823)`); err != nil {
		return domain.AuthSession{}, fmt.Errorf("lock signup: %w", err)
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email_normalized=$1)`, input.EmailNormalized).Scan(&exists); err != nil {
		return domain.AuthSession{}, fmt.Errorf("check signup email: %w", err)
	}
	if exists {
		return domain.AuthSession{}, ErrEmailExists
	}

	userID, err := newID()
	if err != nil {
		return domain.AuthSession{}, err
	}
	organizationID := bootstrapOrganizationID
	var memberCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM organization_memberships WHERE organization_id=$1`, bootstrapOrganizationID).Scan(&memberCount); err != nil {
		return domain.AuthSession{}, fmt.Errorf("check bootstrap organization: %w", err)
	}
	organizationName := "Capcom Local"
	organizationSlug := "capcom-local"
	if memberCount > 0 {
		organizationID, err = newID()
		if err != nil {
			return domain.AuthSession{}, err
		}
		prefix := strings.Split(input.EmailNormalized, "@")[0]
		if len(prefix) > 32 {
			prefix = prefix[:32]
		}
		organizationName = input.Email + " workspace"
		organizationSlug = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			return '-'
		}, strings.ToLower(prefix)) + "-" + organizationID[:8]
		if _, err := tx.ExecContext(ctx, `INSERT INTO organizations(id,name,slug) VALUES($1,$2,$3)`, organizationID, organizationName, organizationSlug); err != nil {
			return domain.AuthSession{}, fmt.Errorf("create organization: %w", err)
		}
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `INSERT INTO users(id,email,email_normalized,created_at,updated_at) VALUES($1,$2,$3,$4,$4)`, userID, input.Email, input.EmailNormalized, now); err != nil {
		return domain.AuthSession{}, fmt.Errorf("create user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO password_credentials(user_id,password_hash,changed_at) VALUES($1,$2,$3)`, userID, input.PasswordHash, now); err != nil {
		return domain.AuthSession{}, fmt.Errorf("create credentials: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO organization_memberships(organization_id,user_id,role) VALUES($1,$2,'owner')`, organizationID, userID); err != nil {
		return domain.AuthSession{}, fmt.Errorf("create membership: %w", err)
	}
	sessionID, err := newID()
	if err != nil {
		return domain.AuthSession{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_sessions(id,user_id,organization_id,token_hash,csrf_hash,absolute_expires_at,idle_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, sessionID, userID, organizationID, input.TokenHash, input.CSRFHash, input.AbsoluteExpiry, input.IdleExpiry); err != nil {
		return domain.AuthSession{}, fmt.Errorf("create signup session: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(organization_id,actor,event_type,target_type,target_id,reason,result) VALUES($1,$2,'user.signup','user',$3,'Self-service account creation','succeeded')`, organizationID, input.EmailNormalized, userID); err != nil {
		return domain.AuthSession{}, fmt.Errorf("audit signup: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.AuthSession{}, fmt.Errorf("commit signup: %w", err)
	}
	return domain.AuthSession{User: domain.User{ID: userID, Email: input.Email, CreatedAt: now}, Organization: domain.Organization{ID: organizationID, Name: organizationName, Slug: organizationSlug}, Role: "owner", AbsoluteExpiry: input.AbsoluteExpiry, IdleExpiry: input.IdleExpiry}, nil
}

func (r AuthRepository) FindLoginIdentity(ctx context.Context, email string) (LoginIdentity, error) {
	var out LoginIdentity
	err := r.db.QueryRowContext(ctx, `SELECT u.id,u.email,u.created_at,p.password_hash,o.id,o.name,o.slug,m.role
FROM users u JOIN password_credentials p ON p.user_id=u.id
JOIN organization_memberships m ON m.user_id=u.id AND m.status='active'
JOIN organizations o ON o.id=m.organization_id AND o.status='active'
WHERE u.email_normalized=$1 AND u.status='active'
ORDER BY m.created_at LIMIT 1`, email).Scan(&out.User.ID, &out.User.Email, &out.User.CreatedAt, &out.PasswordHash, &out.Organization.ID, &out.Organization.Name, &out.Organization.Slug, &out.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return LoginIdentity{}, ErrIdentityNotFound
	}
	if err != nil {
		return LoginIdentity{}, fmt.Errorf("find login identity: %w", err)
	}
	return out, nil
}

func (r AuthRepository) CreateSession(ctx context.Context, input NewSession) error {
	id, err := newID()
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session creation: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO user_sessions(id,user_id,organization_id,token_hash,csrf_hash,absolute_expires_at,idle_expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, input.UserID, input.OrganizationID, input.TokenHash, input.CSRFHash, input.AbsoluteExpiry, input.IdleExpiry)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(organization_id,actor,event_type,target_type,target_id,reason,result) SELECT $1,email,'user.login','session',$2,'Password authentication','succeeded' FROM users WHERE id=$3`, input.OrganizationID, id, input.UserID); err != nil {
		return fmt.Errorf("audit login: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session creation: %w", err)
	}
	return nil
}

func (r AuthRepository) FindSession(ctx context.Context, tokenHash []byte) (domain.Principal, error) {
	var p domain.Principal
	err := r.db.QueryRowContext(ctx, `SELECT s.id,u.id,u.email,o.id,o.name,o.slug,m.role,s.csrf_hash
FROM user_sessions s JOIN users u ON u.id=s.user_id AND u.status='active'
JOIN organizations o ON o.id=s.organization_id AND o.status='active'
JOIN organization_memberships m ON m.user_id=u.id AND m.organization_id=o.id AND m.status='active'
WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.absolute_expires_at>now() AND s.idle_expires_at>now()`, tokenHash).Scan(&p.SessionID, &p.UserID, &p.Email, &p.OrganizationID, &p.Organization.Name, &p.Organization.Slug, &p.Role, &p.CSRFHash)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Principal{}, ErrIdentityNotFound
	}
	if err != nil {
		return domain.Principal{}, fmt.Errorf("find session: %w", err)
	}
	p.Organization.ID = p.OrganizationID
	_, _ = r.db.ExecContext(ctx, `UPDATE user_sessions SET last_seen_at=now(),idle_expires_at=LEAST(absolute_expires_at,now()+interval '12 hours') WHERE id=$1 AND last_seen_at<now()-interval '5 minutes'`, p.SessionID)
	return p, nil
}

func (r AuthRepository) RevokeSession(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, `WITH revoked AS (UPDATE user_sessions SET revoked_at=now() WHERE id=$1 AND revoked_at IS NULL RETURNING organization_id,user_id,id)
INSERT INTO audit_events(organization_id,actor,event_type,target_type,target_id,reason,result)
SELECT revoked.organization_id,users.email,'user.logout','session',revoked.id::text,'User initiated logout','succeeded' FROM revoked JOIN users ON users.id=revoked.user_id`, sessionID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

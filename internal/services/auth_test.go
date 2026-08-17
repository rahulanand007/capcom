package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"capcom/internal/domain"
	"capcom/internal/store"
)

type authStoreStub struct {
	identity store.LoginIdentity
	findErr  error
	created  store.NewSession
}

func (s *authStoreStub) CreateAccount(context.Context, store.NewAccount) (domain.AuthSession, error) {
	return domain.AuthSession{}, errors.New("unused")
}
func (s *authStoreStub) FindLoginIdentity(context.Context, string) (store.LoginIdentity, error) {
	return s.identity, s.findErr
}
func (s *authStoreStub) CreateSession(_ context.Context, input store.NewSession) error {
	s.created = input
	return nil
}
func (s *authStoreStub) FindSession(context.Context, []byte) (domain.Principal, error) {
	return domain.Principal{}, errors.New("unused")
}
func (s *authStoreStub) RevokeSession(context.Context, string) error { return nil }

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword("correct horse battery staple", hash) {
		t.Fatal("expected password to verify")
	}
	if verifyPassword("wrong password entirely", hash) {
		t.Fatal("wrong password verified")
	}
}

func TestValidatePasswordPolicy(t *testing.T) {
	for _, test := range []struct {
		name, password string
		wantErr        bool
	}{
		{"long passphrase", "a suitably long passphrase", false},
		{"short", "short password", true},
		{"blocked", "password123", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validatePassword(test.password)
			if (err != nil) != test.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, test.wantErr)
			}
		})
	}
}

func TestLoginCreatesOpaqueSessionForMembership(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	repo := &authStoreStub{identity: store.LoginIdentity{User: domain.User{ID: "user-1", Email: "operator@example.com"}, Organization: domain.Organization{ID: "org-1", Name: "Ops", Slug: "ops"}, Role: "owner", PasswordHash: hash}}
	service := NewAuthService(repo)
	service.now = func() time.Time { return time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC) }
	session, err := service.Login(context.Background(), "operator@example.com", "correct horse battery staple", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if session.SessionToken == "" || session.CSRFToken == "" {
		t.Fatal("expected opaque session and csrf tokens")
	}
	if repo.created.OrganizationID != "org-1" {
		t.Fatalf("organization=%q", repo.created.OrganizationID)
	}
}

func TestLoginNormalizesUnicodePassword(t *testing.T) {
	normalized := "a suitably long caf\u00e9 passphrase"
	hash, err := hashPassword(normalized)
	if err != nil {
		t.Fatal(err)
	}
	repo := &authStoreStub{identity: store.LoginIdentity{User: domain.User{ID: "user-1", Email: "operator@example.com"}, Organization: domain.Organization{ID: "org-1"}, Role: "owner", PasswordHash: hash}}
	service := NewAuthService(repo)
	if _, err := service.Login(context.Background(), "operator@example.com", "a suitably long cafe\u0301 passphrase", "127.0.0.1"); err != nil {
		t.Fatalf("decomposed password should normalize before verification: %v", err)
	}
}

func TestLoginLimiterBoundsAndExpiresAttempts(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		limiter.Fail("account:test", 5, now)
	}
	if limiter.Allow("account:test", now) {
		t.Fatal("expected account to be temporarily blocked")
	}
	if !limiter.Allow("account:test", now.Add(16*time.Minute)) {
		t.Fatal("expected stale attempt state to expire")
	}
	for i := 0; i < 10_050; i++ {
		limiter.Fail(fmt.Sprintf("ip:%d", i), 25, now)
	}
	if len(limiter.attempts) > 10_000 {
		t.Fatalf("attempt map grew to %d", len(limiter.attempts))
	}
}

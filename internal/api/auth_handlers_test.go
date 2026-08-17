package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"capcom/internal/domain"
)

type authServiceStub struct{}

func (authServiceStub) Signup(context.Context, string, string) (domain.AuthSession, error) {
	return domain.AuthSession{User: domain.User{ID: "user-1", Email: "operator@example.com"}, Organization: domain.Organization{ID: "org-1", Name: "Ops", Slug: "ops"}, Role: "owner", SessionToken: "session-token", CSRFToken: "csrf-token", AbsoluteExpiry: time.Now().Add(time.Hour)}, nil
}
func (authServiceStub) Login(context.Context, string, string, string) (domain.AuthSession, error) {
	return domain.AuthSession{}, errors.New("unused")
}
func (authServiceStub) Authenticate(context.Context, string) (domain.Principal, error) {
	return domain.Principal{UserID: "user-1", Email: "operator@example.com", OrganizationID: "org-1", Organization: domain.Organization{ID: "org-1", Name: "Ops", Slug: "ops"}, Role: "owner", SessionID: "session-1"}, nil
}
func (authServiceStub) VerifyCSRF(_ domain.Principal, token string) error {
	if token != "csrf-token" {
		return errors.New("invalid csrf")
	}
	return nil
}
func (authServiceStub) Logout(context.Context, string) error { return nil }

func TestSignupSetsServerSessionCookies(t *testing.T) {
	router := NewRouter(RouterConfig{Version: "test", Auth: authServiceStub{}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/auth/signup", strings.NewReader(`{"email":"operator@example.com","password":"correct horse battery staple"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	if cookies[0].Name != sessionCookieName || !cookies[0].HttpOnly {
		t.Fatalf("session cookie=%+v", cookies[0])
	}
	if cookies[1].Name != csrfCookieName || cookies[1].HttpOnly {
		t.Fatalf("csrf cookie=%+v", cookies[1])
	}
}

func TestSessionMutationRequiresCSRF(t *testing.T) {
	router := NewRouter(RouterConfig{Version: "test", Auth: authServiceStub{}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status without csrf=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-token"})
	req.Header.Set("X-CSRF-Token", "csrf-token")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status with csrf=%d body=%s", rec.Code, rec.Body.String())
	}
}

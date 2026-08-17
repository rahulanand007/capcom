package api

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"capcom/internal/domain"
	"capcom/internal/services"
	"capcom/internal/tenant"
)

const (
	sessionCookieName = "capcom_session"
	csrfCookieName    = "capcom_csrf"
)

type authRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type authResponse struct {
	User         domain.User         `json:"user"`
	Organization domain.Organization `json:"organization"`
	Role         string              `json:"role"`
}

func handleSignup(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth == nil {
			writeAPIError(w, http.StatusServiceUnavailable, errors.New("authentication not configured"))
			return
		}
		var req authRequest
		if err := decodeAuthRequest(r, &req); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		session, err := cfg.Auth.Signup(r.Context(), req.Email, req.Password)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		setAuthCookies(w, cfg, session)
		writeJSON(w, http.StatusCreated, authResponse{session.User, session.Organization, session.Role})
	}
}

func handleLogin(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.Auth == nil {
			writeAPIError(w, http.StatusServiceUnavailable, errors.New("authentication not configured"))
			return
		}
		var req authRequest
		if err := decodeAuthRequest(r, &req); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		session, err := cfg.Auth.Login(r.Context(), req.Email, req.Password, clientAddress(r))
		if err != nil {
			writeAuthError(w, err)
			return
		}
		setAuthCookies(w, cfg, session)
		writeJSON(w, http.StatusOK, authResponse{session.User, session.Organization, session.Role})
	}
}

func handleLogout(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := tenant.PrincipalFrom(r.Context())
		if !ok || principal.PlatformAdmin {
			writeAPIError(w, http.StatusUnauthorized, errors.New("session required"))
			return
		}
		if err := cfg.Auth.Logout(r.Context(), principal.SessionID); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		clearAuthCookies(w, cfg)
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleMe(cfg RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := tenant.PrincipalFrom(r.Context())
		if !ok || p.PlatformAdmin {
			writeAPIError(w, http.StatusUnauthorized, errors.New("user session required"))
			return
		}
		writeJSON(w, http.StatusOK, authResponse{User: domain.User{ID: p.UserID, Email: p.Email}, Organization: p.Organization, Role: p.Role})
	}
}

func decodeAuthRequest(r *http.Request, out *authRequest) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	if strings.TrimSpace(out.Email) == "" || out.Password == "" {
		return errors.New("email and password are required")
	}
	return nil
}

func setAuthCookies(w http.ResponseWriter, cfg RouterConfig, s domain.AuthSession) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: s.SessionToken, Path: "/", HttpOnly: true, Secure: cfg.SecureCookies, SameSite: http.SameSiteLaxMode, Expires: s.AbsoluteExpiry, MaxAge: int(time.Until(s.AbsoluteExpiry).Seconds())})
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: s.CSRFToken, Path: "/", HttpOnly: false, Secure: cfg.SecureCookies, SameSite: http.SameSiteLaxMode, Expires: s.AbsoluteExpiry, MaxAge: int(time.Until(s.AbsoluteExpiry).Seconds())})
}
func clearAuthCookies(w http.ResponseWriter, cfg RouterConfig) {
	for _, cookie := range []http.Cookie{{Name: sessionCookieName, HttpOnly: true}, {Name: csrfCookieName}} {
		cookie.Value = ""
		cookie.Path = "/"
		cookie.Secure = cfg.SecureCookies
		cookie.SameSite = http.SameSiteLaxMode
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
		http.SetCookie(w, &cookie)
	}
}
func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, services.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, publicErrorResponse{Error: publicErrorDetail{Code: "INVALID_CREDENTIALS", Message: "The email or password is incorrect."}})
	case errors.Is(err, services.ErrEmailUnavailable):
		writeJSON(w, http.StatusConflict, publicErrorResponse{Error: publicErrorDetail{Code: "ACCOUNT_UNAVAILABLE", Message: "An account could not be created with those details."}})
	case errors.Is(err, services.ErrWeakPassword):
		writeJSON(w, http.StatusUnprocessableEntity, publicErrorResponse{Error: publicErrorDetail{Code: "WEAK_PASSWORD", Message: "Use a password between 15 and 128 characters that is not commonly used."}})
	case errors.Is(err, services.ErrRateLimited):
		writeAPIError(w, http.StatusTooManyRequests, err)
	default:
		writeAPIError(w, http.StatusInternalServerError, err)
	}
}
func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func requestActor(r *http.Request, supplied string) string {
	if principal, ok := tenant.PrincipalFrom(r.Context()); ok && !principal.PlatformAdmin {
		return principal.Email
	}
	return strings.TrimSpace(supplied)
}

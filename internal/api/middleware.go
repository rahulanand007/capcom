package api

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"capcom/internal/domain"
	"capcom/internal/tenant"
)

func sessionAuth(next http.Handler, token string, auth AuthService) http.Handler {
	expected := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		if token != "" && strings.HasPrefix(header, "Bearer ") {
			providedToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			provided := sha256.Sum256([]byte(providedToken))
			if providedToken != "" && subtle.ConstantTimeCompare(expected[:], provided[:]) == 1 {
				ctx := tenant.WithPrincipal(r.Context(), domain.Principal{PlatformAdmin: true, Role: "platform_admin"})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		if auth == nil {
			writeAPIError(w, http.StatusUnauthorized, errors.New("session authentication is not configured"))
			return
		}
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeAPIError(w, http.StatusUnauthorized, errors.New("session required"))
			return
		}
		principal, err := auth.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", HttpOnly: true, MaxAge: -1, Expires: time.Unix(1, 0), SameSite: http.SameSiteLaxMode})
			http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), SameSite: http.SameSiteLaxMode})
			writeAPIError(w, http.StatusUnauthorized, err)
			return
		}
		if !authorizedForRequest(principal, r) {
			writeAPIError(w, http.StatusForbidden, errors.New("role does not allow this action"))
			return
		}
		if requiresCSRF(r.Method) {
			if err := auth.VerifyCSRF(principal, r.Header.Get("X-CSRF-Token")); err != nil {
				writeAPIError(w, http.StatusForbidden, err)
				return
			}
		}
		w.Header().Set("Cache-Control", "private, no-store")
		next.ServeHTTP(w, r.WithContext(tenant.WithPrincipal(r.Context(), principal)))
	})
}

func isPublicPath(r *http.Request) bool {
	return r.URL.Path == "/healthz" || r.URL.Path == "/" ||
		(r.Method == http.MethodPost && (r.URL.Path == "/auth/login" || r.URL.Path == "/auth/signup"))
}

func requiresCSRF(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func authorizedForRequest(principal domain.Principal, r *http.Request) bool {
	if r.URL.Path == "/auth/logout" {
		return true
	}
	if principal.PlatformAdmin || principal.Role == "owner" || principal.Role == "admin" {
		return true
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	if principal.Role != "operator" {
		return false
	}
	// Operators may execute runtime actions, but they cannot manage credentials,
	// connection ownership, or organization configuration.
	if strings.HasPrefix(r.URL.Path, "/v1/secrets") {
		return false
	}
	if strings.HasPrefix(r.URL.Path, "/v1/runtime-connections") || strings.HasPrefix(r.URL.Path, "/v1/runtime-instances") {
		return strings.HasSuffix(r.URL.Path, "/sync") || strings.HasSuffix(r.URL.Path, "/test")
	}
	return true
}

// corsMiddleware answers CORS preflight requests and adds the allow-origin
// headers for browser clients served from a different origin (the Next.js
// console). It runs outside adminAuth so that credential-less OPTIONS
// preflights are never rejected as unauthorized.
func corsMiddleware(next http.Handler, allowedOrigins []string) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowed[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-CSRF-Token")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Max-Age", "600")
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
				)
				writeAPIError(w, http.StatusInternalServerError, errors.New("panic recovered"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func requestLogger(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		next.ServeHTTP(recorder, r)

		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

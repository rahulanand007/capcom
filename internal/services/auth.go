package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"capcom/internal/domain"
	"capcom/internal/store"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailUnavailable   = errors.New("email unavailable")
	ErrWeakPassword       = errors.New("password does not meet requirements")
	ErrRateLimited        = errors.New("authentication rate limited")
	ErrInvalidSession     = errors.New("invalid session")
	ErrInvalidCSRF        = errors.New("invalid csrf token")
)

type AuthStore interface {
	CreateAccount(context.Context, store.NewAccount) (domain.AuthSession, error)
	FindLoginIdentity(context.Context, string) (store.LoginIdentity, error)
	CreateSession(context.Context, store.NewSession) error
	FindSession(context.Context, []byte) (domain.Principal, error)
	RevokeSession(context.Context, string) error
}

type AuthService struct {
	repo      AuthStore
	now       func() time.Time
	dummyHash string
	limiter   *loginLimiter
}

func NewAuthService(repo AuthStore) *AuthService {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	return &AuthService{repo: repo, now: time.Now, dummyHash: encodePassword("not-a-real-password", salt), limiter: newLoginLimiter()}
}

func (s *AuthService) Signup(ctx context.Context, email, password string) (domain.AuthSession, error) {
	display, normalized, err := normalizeEmail(email)
	if err != nil {
		return domain.AuthSession{}, ErrEmailUnavailable
	}
	password = norm.NFC.String(password)
	if err := validatePassword(password); err != nil {
		return domain.AuthSession{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return domain.AuthSession{}, err
	}
	token, tokenHash, err := randomToken()
	if err != nil {
		return domain.AuthSession{}, err
	}
	csrf, csrfHash, err := randomToken()
	if err != nil {
		return domain.AuthSession{}, err
	}
	now := s.now().UTC()
	session, err := s.repo.CreateAccount(ctx, store.NewAccount{Email: display, EmailNormalized: normalized, PasswordHash: hash, TokenHash: tokenHash, CSRFHash: csrfHash, AbsoluteExpiry: now.Add(7 * 24 * time.Hour), IdleExpiry: now.Add(12 * time.Hour)})
	if errors.Is(err, store.ErrEmailExists) {
		return domain.AuthSession{}, ErrEmailUnavailable
	}
	if err != nil {
		return domain.AuthSession{}, err
	}
	session.SessionToken, session.CSRFToken = token, csrf
	return session, nil
}

func (s *AuthService) Login(ctx context.Context, email, password, remoteAddress string) (domain.AuthSession, error) {
	_, normalized, emailErr := normalizeEmail(email)
	password = norm.NFC.String(password)
	accountKey, ipKey := "account:"+normalized, "ip:"+remoteAddress
	if !s.limiter.Allow(accountKey, s.now()) || !s.limiter.Allow(ipKey, s.now()) {
		return domain.AuthSession{}, ErrRateLimited
	}
	identity, err := s.repo.FindLoginIdentity(ctx, normalized)
	hash := s.dummyHash
	if err == nil && emailErr == nil {
		hash = identity.PasswordHash
	}
	valid := verifyPassword(password, hash)
	if err != nil || emailErr != nil || !valid {
		s.limiter.Fail(accountKey, 5, s.now())
		s.limiter.Fail(ipKey, 25, s.now())
		return domain.AuthSession{}, ErrInvalidCredentials
	}
	s.limiter.Success(accountKey)
	token, tokenHash, err := randomToken()
	if err != nil {
		return domain.AuthSession{}, err
	}
	csrf, csrfHash, err := randomToken()
	if err != nil {
		return domain.AuthSession{}, err
	}
	now := s.now().UTC()
	if err := s.repo.CreateSession(ctx, store.NewSession{UserID: identity.User.ID, OrganizationID: identity.Organization.ID, TokenHash: tokenHash, CSRFHash: csrfHash, AbsoluteExpiry: now.Add(7 * 24 * time.Hour), IdleExpiry: now.Add(12 * time.Hour)}); err != nil {
		return domain.AuthSession{}, err
	}
	return domain.AuthSession{User: identity.User, Organization: identity.Organization, Role: identity.Role, SessionToken: token, CSRFToken: csrf, AbsoluteExpiry: now.Add(7 * 24 * time.Hour), IdleExpiry: now.Add(12 * time.Hour)}, nil
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (domain.Principal, error) {
	if token == "" {
		return domain.Principal{}, ErrInvalidSession
	}
	h := sha256.Sum256([]byte(token))
	p, err := s.repo.FindSession(ctx, h[:])
	if errors.Is(err, store.ErrIdentityNotFound) {
		return domain.Principal{}, ErrInvalidSession
	}
	return p, err
}

func (s *AuthService) VerifyCSRF(principal domain.Principal, token string) error {
	h := sha256.Sum256([]byte(token))
	if token == "" || subtle.ConstantTimeCompare(h[:], principal.CSRFHash) != 1 {
		return ErrInvalidCSRF
	}
	return nil
}

func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	return s.repo.RevokeSession(ctx, sessionID)
}

func normalizeEmail(raw string) (string, string, error) {
	display := strings.TrimSpace(norm.NFC.String(raw))
	if len(display) > 254 {
		return "", "", fmt.Errorf("email too long")
	}
	address, err := mail.ParseAddress(display)
	if err != nil || address.Address != display || !strings.Contains(display, "@") {
		return "", "", fmt.Errorf("invalid email")
	}
	return display, strings.ToLower(display), nil
}

var commonPasswords = map[string]struct{}{"password": {}, "password123": {}, "123456789012345": {}, "qwertyuiopasdfg": {}, "letmeinletmeinlet": {}}

func validatePassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrWeakPassword
	}
	length := utf8.RuneCountInString(password)
	if length < 15 || length > 128 {
		return ErrWeakPassword
	}
	if _, blocked := commonPasswords[strings.ToLower(password)]; blocked {
		return ErrWeakPassword
	}
	return nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	return encodePassword(password, salt), nil
}

func encodePassword(password string, salt []byte) string {
	hash := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
}

func verifyPassword(password, encoded string) bool {
	var memory uint32
	var iterations uint32
	var parallelism uint8
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false
	}
	if memory < 8*1024 || memory > 1024*1024 || iterations < 1 || iterations > 10 || parallelism < 1 || parallelism > 16 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func randomToken() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	h := sha256.Sum256([]byte(token))
	return token, h[:], nil
}

type loginAttempt struct {
	failures     int
	blockedUntil time.Time
	updatedAt    time.Time
}
type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{attempts: map[string]loginAttempt{}} }
func (l *loginLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.attempts[key]
	if !a.updatedAt.IsZero() && now.Sub(a.updatedAt) >= 15*time.Minute {
		delete(l.attempts, key)
		return true
	}
	return !now.Before(a.blockedUntil)
}
func (l *loginLimiter) Fail(key string, threshold int, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.attempts) >= 10_000 {
		var oldestKey string
		var oldest time.Time
		for candidate, attempt := range l.attempts {
			if oldestKey == "" || attempt.updatedAt.Before(oldest) {
				oldestKey, oldest = candidate, attempt.updatedAt
			}
		}
		delete(l.attempts, oldestKey)
	}
	a := l.attempts[key]
	a.failures++
	a.updatedAt = now
	if a.failures >= threshold {
		a.blockedUntil = now.Add(5 * time.Minute)
	}
	l.attempts[key] = a
}
func (l *loginLimiter) Success(key string) { l.mu.Lock(); defer l.mu.Unlock(); delete(l.attempts, key) }

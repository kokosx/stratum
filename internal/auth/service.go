package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kokosx/stratum/internal/authz"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"golang.org/x/crypto/bcrypt"
)

const (
	CookieName      = "stratum_session"
	sessionLifetime = 30 * 24 * time.Hour
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrSetupUnavailable   = errors.New("setup is no longer available")
	ErrInvalidSetupCode   = errors.New("invalid setup code")
)

type User struct {
	ID     string
	Email  string
	Role   string
	Status string
}

type Service struct {
	database      *sql.DB
	queries       *db.Queries
	setupCode     string
	secureCookies bool
	dummyHash     []byte
	setupMu       sync.Mutex
}

func NewService(database *sql.DB, queries *db.Queries, secureCookies bool) (*Service, error) {
	hasAdmin, err := queries.HasAdmin(context.Background())
	if err != nil {
		return nil, fmt.Errorf("check initial setup: %w", err)
	}

	dummyHash, err := bcrypt.GenerateFromPassword([]byte("invalid-password-placeholder"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("prepare password verification: %w", err)
	}
	service := &Service{database: database, queries: queries, secureCookies: secureCookies, dummyHash: dummyHash}
	if !hasAdmin {
		code, err := newSetupCode()
		if err != nil {
			return nil, fmt.Errorf("generate setup code: %w", err)
		}
		service.setupCode = code
	}
	return service, nil
}

func (s *Service) SetupCode() string { return s.setupCode }

func (s *Service) HasAdmin(ctx context.Context) (bool, error) {
	return s.queries.HasAdmin(ctx)
}

func (s *Service) Setup(ctx context.Context, code, siteTitle, email, password string) (string, error) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()

	if s.setupCode == "" {
		return "", ErrSetupUnavailable
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(code)), []byte(s.setupCode)) != 1 {
		return "", ErrInvalidSetupCode
	}
	if strings.TrimSpace(siteTitle) == "" || password == "" {
		return "", errors.New("site title, email and password are required")
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return "", err
	}
	if err := validatePassword(password); err != nil {
		return "", err
	}

	hasAdmin, err := s.queries.HasAdmin(ctx)
	if err != nil {
		return "", fmt.Errorf("check setup status: %w", err)
	}
	if hasAdmin {
		return "", ErrSetupUnavailable
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	userID, err := randomToken(16)
	if err != nil {
		return "", err
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin setup: %w", err)
	}
	qtx := s.queries.WithTx(tx)
	if err := qtx.CreateUser(ctx, db.CreateUserParams{ID: userID, Email: email, PasswordHash: string(passwordHash), Role: "admin", CreatedAt: now, UpdatedAt: now}); err != nil {
		tx.Rollback()
		return "", fmt.Errorf("create administrator: %w", err)
	}
	if err := qtx.UpdateSiteTitle(ctx, db.UpdateSiteTitleParams{SiteTitle: strings.TrimSpace(siteTitle), UpdatedAt: now}); err != nil {
		tx.Rollback()
		return "", fmt.Errorf("set site title: %w", err)
	}
	if err := qtx.CreateSession(ctx, db.CreateSessionParams{TokenHash: tokenHash, UserID: userID, CreatedAt: now, ExpiresAt: time.Now().Add(sessionLifetime).Unix()}); err != nil {
		tx.Rollback()
		return "", fmt.Errorf("create session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit setup: %w", err)
	}
	s.setupCode = ""
	return token, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (string, error) {
	user, err := s.queries.GetUserByEmail(ctx, strings.TrimSpace(strings.ToLower(email)))
	passwordHash := s.dummyHash
	if err == nil {
		passwordHash = []byte(user.PasswordHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword(passwordHash, []byte(password))
	if err != nil || passwordErr != nil || user.Status != "active" || !authz.ValidRole(user.Role) {
		return "", ErrInvalidCredentials
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return "", err
	}
	now := time.Now()
	if err := s.queries.CreateSession(ctx, db.CreateSessionParams{TokenHash: tokenHash, UserID: user.ID, CreatedAt: now.Unix(), ExpiresAt: now.Add(sessionLifetime).Unix()}); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

func (s *Service) UserForToken(ctx context.Context, token string) (User, error) {
	if token == "" {
		return User{}, sql.ErrNoRows
	}
	user, err := s.queries.GetSessionUser(ctx, tokenDigest(token))
	if err != nil {
		return User{}, err
	}
	if user.ExpiresAt <= time.Now().Unix() {
		_ = s.queries.DeleteSession(ctx, tokenDigest(token))
		return User{}, sql.ErrNoRows
	}
	if user.Status != "active" || !authz.ValidRole(user.Role) {
		_ = s.queries.DeleteSession(ctx, tokenDigest(token))
		return User{}, sql.ErrNoRows
	}
	return User{ID: user.ID, Email: user.Email, Role: user.Role, Status: user.Status}, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.queries.DeleteSession(ctx, tokenDigest(token))
}

func (s *Service) SecureCookies() bool { return s.secureCookies }

func (s *Service) CreateUser(ctx context.Context, email, password, role string) error {
	var err error
	email, err = normalizeEmail(email)
	if err != nil {
		return err
	}
	role = strings.TrimSpace(role)
	if password == "" || !authz.ValidRole(role) {
		return errors.New("email, password, and valid role are required")
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	id, err := randomToken(16)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	return s.queries.CreateUser(ctx, db.CreateUserParams{ID: id, Email: email, PasswordHash: string(hash), Role: role, CreatedAt: now, UpdatedAt: now})
}

func (s *Service) UpdateUser(ctx context.Context, id, role, status string) error {
	if !authz.ValidRole(role) || (status != "active" && status != "disabled") {
		return errors.New("invalid role or account status")
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin user update: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	target, err := qtx.GetUserByID(ctx, id)
	if err != nil {
		return err
	}
	if target.Role == string(authz.RoleAdmin) && target.Status == "active" && (role != string(authz.RoleAdmin) || status != "active") {
		count, err := qtx.CountActiveAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return errors.New("cannot disable or demote the last active administrator")
		}
	}
	now := time.Now().Unix()
	if err := qtx.UpdateUserRole(ctx, db.UpdateUserRoleParams{ID: id, Role: role, UpdatedAt: now}); err != nil {
		return err
	}
	if err := qtx.UpdateUserStatus(ctx, db.UpdateUserStatusParams{ID: id, Status: status, UpdatedAt: now}); err != nil {
		return err
	}
	if err := qtx.DeleteSessionsForUser(ctx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ResetPassword changes a user's password and invalidates every active session.
func (s *Service) ResetPassword(ctx context.Context, id, password string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin password reset: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)
	if _, err := qtx.GetUserByID(ctx, id); err != nil {
		return err
	}
	if err := qtx.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{ID: id, PasswordHash: string(hash), UpdatedAt: time.Now().Unix()}); err != nil {
		return err
	}
	if err := qtx.DeleteSessionsForUser(ctx, id); err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || parsed.Name != "" {
		return "", errors.New("email must be a valid address")
	}
	return email, nil
}

func validatePassword(password string) error {
	if utf8.RuneCountInString(password) < 12 {
		return errors.New("password must contain at least 12 characters")
	}
	if len(password) > 72 {
		return errors.New("password must not exceed 72 bytes")
	}
	return nil
}

func newSessionToken() (token, hash string, err error) {
	token, err = randomToken(32)
	if err != nil {
		return "", "", err
	}
	return token, tokenDigest(token), nil
}

func randomToken(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func tokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func newSetupCode() (string, error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i := range bytes {
		bytes[i] = alphabet[int(bytes[i])%len(alphabet)]
	}
	return string(bytes[:4]) + "-" + string(bytes[4:8]) + "-" + string(bytes[8:]), nil
}

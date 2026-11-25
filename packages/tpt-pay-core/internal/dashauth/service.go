package dashauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/secretbox"
)

// Lockout policy. Five wrong passwords, or five wrong codes, lock that factor for 15 minutes. A
// failure more than 15 minutes after the previous one starts a new count. The counts are stored in
// the database, so every server instance shares them.
const (
	maxFailures   = 5
	lockDuration  = 15 * time.Minute
	failureWindow = 15 * time.Minute
	issuerName    = "TPT Pay"
)

// Errors the caller can tell apart. Messages are generic on purpose: a sign-in never reveals whether
// the email exists, and a wrong code never reveals whether the password was right.
var (
	ErrInvalidCredentials = errors.New("dashauth: invalid credentials")
	ErrLocked             = errors.New("dashauth: the account is temporarily locked")
	ErrInvalidCode        = errors.New("dashauth: invalid two-factor code")
	ErrNotEnrolled        = errors.New("dashauth: two-factor authentication is not enrolled")
)

// User is a dashboard user, without any secret.
type User struct {
	ID          string `json:"id"`
	MerchantID  string `json:"merchant_id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	TOTPEnabled bool   `json:"totp_enabled"`
}

// Service authenticates dashboard users.
type Service struct {
	pool       *pgxpool.Pool
	box        *secretbox.Box
	clk        clock.Clock
	iterations int
	// dummyHash is checked for an unknown email, so a missing account costs the same time as a
	// wrong password.
	dummyHash string
}

// NewService returns a service. iterations is the PBKDF2 work factor for new passwords; pass
// DefaultIterations outside tests.
func NewService(pool *pgxpool.Pool, box *secretbox.Box, clk clock.Clock, iterations int) (*Service, error) {
	dummy, err := HashPassword("not a real password", iterations)
	if err != nil {
		return nil, err
	}
	return &Service{pool: pool, box: box, clk: clk, iterations: iterations, dummyHash: dummy}, nil
}

// CreateUser adds a dashboard user in a merchant. Two-factor is enrolled separately.
func (s *Service) CreateUser(ctx context.Context, merchantID, email, name, password string) (*User, error) {
	email = normalizeEmail(email)
	if email == "" || !strings.Contains(email, "@") {
		return nil, errors.New("dashauth: a valid email is required")
	}
	if len(password) < 12 {
		return nil, errors.New("dashauth: the password must be at least 12 characters")
	}
	hash, err := HashPassword(password, s.iterations)
	if err != nil {
		return nil, err
	}
	id := "usr_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO dashboard_users (id, merchant_id, email, name, password_hash)
		VALUES ($1, $2, $3, $4, $5)
	`, id, merchantID, email, name, hash); err != nil {
		return nil, fmt.Errorf("create dashboard user: %w", err)
	}
	return &User{ID: id, MerchantID: merchantID, Email: email, Name: name}, nil
}

// EnrollTOTP creates a new second-factor secret for a user and returns its otpauth:// URI. The
// secret is stored encrypted and is shown only through this return value. Enrolling again replaces
// the old secret, so the old authenticator stops working.
func (s *Service) EnrollTOTP(ctx context.Context, userID string) (string, error) {
	secret, err := NewTOTPSecret()
	if err != nil {
		return "", err
	}
	sealed, err := s.box.Seal(secret)
	if err != nil {
		return "", fmt.Errorf("seal totp secret: %w", err)
	}
	var email string
	err = s.pool.QueryRow(ctx, `
		UPDATE dashboard_users SET totp_ciphertext = $2, totp_last_step = -1
		WHERE id = $1 RETURNING email
	`, userID, sealed).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", fmt.Errorf("enroll totp: %w", err)
	}
	return OTPAuthURI(issuerName, email, secret), nil
}

// Authenticate checks an email and password. It returns the user when both are right. A locked
// account is refused before the password is checked, so a lock cannot be probed.
func (s *Service) Authenticate(ctx context.Context, email, password string) (*User, error) {
	now := s.clk.Now()
	var (
		id, merchantID, name, hash string
		enrolled                   bool
		c                          counter
	)
	err := s.pool.QueryRow(ctx, `
		SELECT id, merchant_id, name, password_hash, totp_ciphertext IS NOT NULL,
		       password_failures, password_last_failure_at, password_locked_until
		FROM dashboard_users WHERE email = $1
	`, normalizeEmail(email)).Scan(&id, &merchantID, &name, &hash, &enrolled,
		&c.failures, &c.lastFailure, &c.lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = VerifyPassword(password, s.dummyHash) // same cost as a real check
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("load dashboard user: %w", err)
	}
	if c.locked(now) {
		return nil, ErrLocked
	}

	ok, err := VerifyPassword(password, hash)
	if err != nil {
		return nil, err
	}
	if !ok {
		if err := s.recordFailure(ctx, id, "password", now); err != nil {
			return nil, err
		}
		return nil, ErrInvalidCredentials
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE dashboard_users
		SET password_failures = 0, password_last_failure_at = NULL, password_locked_until = NULL
		WHERE id = $1
	`, id); err != nil {
		return nil, fmt.Errorf("clear password failures: %w", err)
	}
	return &User{ID: id, MerchantID: merchantID, Email: normalizeEmail(email), Name: name, TOTPEnabled: enrolled}, nil
}

// VerifyTOTP checks a six-digit code for a user who has enrolled. A code can be used once: its time
// step is recorded, and an earlier or repeated step is refused.
func (s *Service) VerifyTOTP(ctx context.Context, userID, code string) error {
	now := s.clk.Now()
	var (
		sealed   *string
		lastStep int64
		c        counter
	)
	err := s.pool.QueryRow(ctx, `
		SELECT totp_ciphertext, totp_last_step, totp_failures, totp_last_failure_at, totp_locked_until
		FROM dashboard_users WHERE id = $1
	`, userID).Scan(&sealed, &lastStep, &c.failures, &c.lastFailure, &c.lockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCode
	}
	if err != nil {
		return fmt.Errorf("load totp state: %w", err)
	}
	if sealed == nil {
		return ErrNotEnrolled
	}
	if c.locked(now) {
		return ErrLocked
	}

	secret, err := s.box.Open(*sealed)
	if err != nil {
		return fmt.Errorf("open totp secret: %w", err)
	}
	step, ok := matchTOTP(secret, code, now)
	if !ok || step <= lastStep {
		if err := s.recordFailure(ctx, userID, "totp", now); err != nil {
			return err
		}
		return ErrInvalidCode
	}

	// Recording the step is a compare-and-set, so two requests with the same code cannot both win.
	tag, err := s.pool.Exec(ctx, `
		UPDATE dashboard_users
		SET totp_last_step = $2, totp_failures = 0, totp_last_failure_at = NULL, totp_locked_until = NULL
		WHERE id = $1 AND totp_last_step < $2
	`, userID, step)
	if err != nil {
		return fmt.Errorf("record totp step: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidCode
	}
	return nil
}

// recordFailure counts a wrong password or code under a row lock, so concurrent failures are all
// counted.
func (s *Service) recordFailure(ctx context.Context, userID, factor string, now time.Time) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var c counter
		err := tx.QueryRow(ctx, fmt.Sprintf(`
			SELECT %[1]s_failures, %[1]s_last_failure_at, %[1]s_locked_until
			FROM dashboard_users WHERE id = $1 FOR UPDATE
		`, factor), userID).Scan(&c.failures, &c.lastFailure, &c.lockedUntil)
		if err != nil {
			return fmt.Errorf("read %s failures: %w", factor, err)
		}
		next := c.afterFailure(now)
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			UPDATE dashboard_users
			SET %[1]s_failures = $2, %[1]s_last_failure_at = $3, %[1]s_locked_until = $4
			WHERE id = $1
		`, factor), userID, next.failures, next.lastFailure, next.lockedUntil); err != nil {
			return fmt.Errorf("record %s failure: %w", factor, err)
		}
		return nil
	})
}

// counter is the lockout state of one factor, with the policy applied in memory.
type counter struct {
	failures    int
	lastFailure *time.Time
	lockedUntil *time.Time
}

func (c counter) locked(now time.Time) bool {
	return c.lockedUntil != nil && now.Before(*c.lockedUntil)
}

// afterFailure applies one more failure. The fifth failure in a window locks the factor and starts
// the count again.
func (c counter) afterFailure(now time.Time) counter {
	if c.lastFailure == nil || now.Sub(*c.lastFailure) > failureWindow {
		c.failures = 0
	}
	c.failures++
	at := now
	c.lastFailure = &at
	if c.failures >= maxFailures {
		until := now.Add(lockDuration)
		c.lockedUntil = &until
		c.failures = 0
	}
	return c
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

package dashauth_test

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/dashauth"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/secretbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

const password = "a sufficiently long passphrase"

type env struct {
	pool  *pgxpool.Pool
	clk   *clock.Test
	svc   *dashauth.Service
	merch string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()
	var merchant string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Dashboard Merchant', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid").Scan(&merchant))

	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	box, err := secretbox.New(base64.StdEncoding.EncodeToString(raw))
	require.NoError(t, err)

	clk := clock.NewTest(time.Date(2030, 2, 1, 9, 0, 0, 0, time.UTC))
	// 100,000 is the minimum the service accepts; the production default is larger and slower.
	svc, err := dashauth.NewService(pool, box, clk, 100_000)
	require.NoError(t, err)
	return &env{pool: pool, clk: clk, svc: svc, merch: merchant}
}

func (e *env) user(t *testing.T, email string) *dashauth.User {
	t.Helper()
	u, err := e.svc.CreateUser(context.Background(), e.merch, email, "Owner", password)
	require.NoError(t, err)
	return u
}

// secretFrom reads the base32 secret out of an otpauth URI, the way an authenticator app would.
func secretFrom(t *testing.T, uri string) []byte {
	t.Helper()
	u, err := url.Parse(uri)
	require.NoError(t, err)
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(u.Query().Get("secret"))
	require.NoError(t, err)
	return secret
}

// codeNow computes the six-digit code an authenticator app shows at a time. It is written out here,
// independently of the package, so the test checks the service against the standard rather than
// against itself.
func codeNow(secret []byte, at time.Time) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix()/30))
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000)
}

func TestService_AuthenticateAndLock(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	u := e.user(t, "Owner@Example.com ")

	got, err := e.svc.Authenticate(ctx, "owner@example.com", password)
	require.NoError(t, err)
	assert.Equal(t, u.ID, got.ID)
	assert.Equal(t, u.MerchantID, got.MerchantID)
	assert.False(t, got.TOTPEnabled)

	_, err = e.svc.Authenticate(ctx, "owner@example.com", "wrong password entirely")
	assert.ErrorIs(t, err, dashauth.ErrInvalidCredentials)
	_, err = e.svc.Authenticate(ctx, "nobody@example.com", password)
	assert.ErrorIs(t, err, dashauth.ErrInvalidCredentials, "an unknown email looks the same as a wrong password")

	// Five wrong passwords lock the account. A correct password is refused while it is locked.
	for i := 0; i < 4; i++ {
		_, _ = e.svc.Authenticate(ctx, "owner@example.com", "nope")
	}
	_, _ = e.svc.Authenticate(ctx, "owner@example.com", "nope")
	_, err = e.svc.Authenticate(ctx, "owner@example.com", password)
	assert.ErrorIs(t, err, dashauth.ErrLocked, "a locked account is refused before the password is checked")

	// After the lock ends, the correct password works again.
	_, _ = e.clk.Set(e.clk.Now().Add(16 * time.Minute))
	_, err = e.svc.Authenticate(ctx, "owner@example.com", password)
	require.NoError(t, err)
}

func TestService_TOTPEnrollVerifyAndReplay(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	u := e.user(t, "totp@example.com")

	_, err := e.svc.Authenticate(ctx, "totp@example.com", password)
	require.NoError(t, err)

	err = e.svc.VerifyTOTP(ctx, u.ID, "123456")
	assert.ErrorIs(t, err, dashauth.ErrNotEnrolled, "a user who has not enrolled has no second factor")

	uri, err := e.svc.EnrollTOTP(ctx, u.ID)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(uri, "otpauth://totp/"))
	secret := secretFrom(t, uri)

	got, err := e.svc.Authenticate(ctx, "totp@example.com", password)
	require.NoError(t, err)
	assert.True(t, got.TOTPEnabled)

	code := codeNow(secret, e.clk.Now())
	require.NoError(t, e.svc.VerifyTOTP(ctx, u.ID, code), "the current code is accepted")
	assert.ErrorIs(t, e.svc.VerifyTOTP(ctx, u.ID, code), dashauth.ErrInvalidCode, "a code can be used once")

	// The secret is stored encrypted, never in the clear.
	var stored string
	require.NoError(t, e.pool.QueryRow(ctx, `SELECT totp_ciphertext FROM dashboard_users WHERE id = $1`, u.ID).Scan(&stored))
	assert.NotContains(t, stored, base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret))
}

func TestService_WrongCodesLockTheSecondFactor(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	u := e.user(t, "lock@example.com")
	uri, err := e.svc.EnrollTOTP(ctx, u.ID)
	require.NoError(t, err)
	secret := secretFrom(t, uri)

	for i := 0; i < 5; i++ {
		err = e.svc.VerifyTOTP(ctx, u.ID, "000000")
	}
	// The fifth wrong code is the one that locks, and it is reported like any other wrong code.
	assert.ErrorIs(t, err, dashauth.ErrInvalidCode)
	assert.ErrorIs(t, e.svc.VerifyTOTP(ctx, u.ID, "000000"), dashauth.ErrLocked, "the next attempt is refused")

	// Even the right code is refused while the factor is locked.
	assert.ErrorIs(t, e.svc.VerifyTOTP(ctx, u.ID, codeNow(secret, e.clk.Now())), dashauth.ErrLocked)

	_, _ = e.clk.Set(e.clk.Now().Add(16 * time.Minute))
	require.NoError(t, e.svc.VerifyTOTP(ctx, u.ID, codeNow(secret, e.clk.Now())))
}

func TestHTTP_ServiceTokenIsRequired(t *testing.T) {
	e := newEnv(t)
	e.user(t, "http@example.com")
	const token = "a-service-token-of-at-least-32-characters"
	h := e.svc.Handler(token)

	body := `{"email":"http@example.com","password":"` + password + `"}`
	for name, header := range map[string]string{"missing": "", "wrong": "nope"} {
		req := httptest.NewRequest(http.MethodPost, "/internal/dashboard/authenticate", strings.NewReader(body))
		if header != "" {
			req.Header.Set("X-Dashboard-Service-Token", header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, name)
	}

	req := httptest.NewRequest(http.MethodPost, "/internal/dashboard/authenticate", strings.NewReader(body))
	req.Header.Set("X-Dashboard-Service-Token", token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		User struct {
			Email       string `json:"email"`
			TOTPEnabled bool   `json:"totp_enabled"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "http@example.com", out.User.Email)
	assert.False(t, out.User.TOTPEnabled)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

func TestHTTP_BadBodiesAndErrorCodes(t *testing.T) {
	e := newEnv(t)
	const token = "a-service-token-of-at-least-32-characters"
	h := e.svc.Handler(token)
	post := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("X-Dashboard-Service-Token", token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	assert.Equal(t, http.StatusBadRequest, post("/internal/dashboard/authenticate", `{"email":"x","surprise":1}`).Code)
	assert.Equal(t, http.StatusBadRequest, post("/internal/dashboard/authenticate", `{"email":"x"} {}`).Code)
	assert.Equal(t, http.StatusUnauthorized, post("/internal/dashboard/authenticate", `{"email":"x","password":"y"}`).Code)
	assert.Equal(t, http.StatusUnauthorized, post("/internal/dashboard/verify-totp", `{"user_id":"usr_none","code":"123456"}`).Code)
}

func TestValidServiceToken(t *testing.T) {
	assert.False(t, dashauth.ValidServiceToken("short"))
	assert.True(t, dashauth.ValidServiceToken(strings.Repeat("x", 32)))
}

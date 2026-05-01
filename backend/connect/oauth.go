// tpt-payments/backend/connect/oauth.go
package connect

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
)

// OAuthAuthorizationCode represents a temporary authorization code
type OAuthAuthorizationCode struct {
	Code         string    `json:"code"`
	AccountID    string    `json:"account_id"`
	PlatformID   string    `json:"platform_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	RedirectURI  string    `json:"redirect_uri"`
	Scope        []string  `json:"scope"`
	CreatedAt    time.Time `json:"created_at"`
}

// OAuthToken represents an access token for connected accounts
type OAuthToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	AccountID    string    `json:"account_id"`
	PlatformID   string    `json:"platform_id"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        []string  `json:"scope"`
	CreatedAt    time.Time `json:"created_at"`
}

// OAuthService handles OAuth 2.0 flow for connected accounts
type OAuthService struct {
	db *sql.DB
}

// NewOAuthService creates OAuth service
func NewOAuthService(db *sql.DB) *OAuthService {
	return &OAuthService{db: db}
}

// GenerateAuthorizationCode creates a temporary authorization code
func (s *OAuthService) GenerateAuthorizationCode(ctx context.Context, accountID string, platformID string, redirectURI string, scope []string) (string, error) {
	codeBytes := make([]byte, 32)
	_, err := rand.Read(codeBytes)
	if err != nil {
		return "", err
	}
	
	code := base64.URLEncoding.EncodeToString(codeBytes)
	expiresAt := time.Now().Add(10 * time.Minute)
	
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO oauth_authorization_codes (code, account_id, platform_id, expires_at, redirect_uri, scope, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		code, accountID, platformID, expiresAt, redirectURI, scope, time.Now(),
	)
	
	return code, err
}

// ExchangeCodeForToken exchanges authorization code for access token
func (s *OAuthService) ExchangeCodeForToken(ctx context.Context, code string, redirectURI string) (*OAuthToken, error) {
	var authCode OAuthAuthorizationCode
	
	err := s.db.QueryRowContext(ctx, `
		SELECT code, account_id, platform_id, expires_at, redirect_uri, scope, created_at
		FROM oauth_authorization_codes WHERE code = $1`,
		code,
	).Scan(
		&authCode.Code, &authCode.AccountID, &authCode.PlatformID, &authCode.ExpiresAt,
		&authCode.RedirectURI, &authCode.Scope, &authCode.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	if time.Now().After(authCode.ExpiresAt) {
		return nil, nil // authorization code expired
	}
	
	if authCode.RedirectURI != redirectURI {
		return nil, nil // redirect URI mismatch
	}
	
	// Delete used authorization code
	_, err = s.db.ExecContext(ctx, `DELETE FROM oauth_authorization_codes WHERE code = $1`, code)
	if err != nil {
		return nil, err
	}
	
	// Generate tokens
	accessToken := "acc_" + uuid.NewString()
	refreshToken := "ref_" + uuid.NewString()
	
	token := &OAuthToken{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		AccountID:    authCode.AccountID,
		PlatformID:   authCode.PlatformID,
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scope:        authCode.Scope,
		CreatedAt:    time.Now(),
	}
	
	// Store token
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO oauth_tokens (access_token, refresh_token, account_id, platform_id, expires_at, scope, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		token.AccessToken, token.RefreshToken, token.AccountID, token.PlatformID,
		token.ExpiresAt, token.Scope, token.CreatedAt,
	)
	
	return token, err
}

// RefreshAccessToken issues new access token using refresh token
func (s *OAuthService) RefreshAccessToken(ctx context.Context, refreshToken string) (*OAuthToken, error) {
	var token OAuthToken
	
	err := s.db.QueryRowContext(ctx, `
		SELECT access_token, refresh_token, account_id, platform_id, expires_at, scope, created_at
		FROM oauth_tokens WHERE refresh_token = $1`,
		refreshToken,
	).Scan(
		&token.AccessToken, &token.RefreshToken, &token.AccountID, &token.PlatformID,
		&token.ExpiresAt, &token.Scope, &token.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	// Generate new access token
	newAccessToken := "acc_" + uuid.NewString()
	newExpiresAt := time.Now().Add(24 * time.Hour)
	
	_, err = s.db.ExecContext(ctx, `
		UPDATE oauth_tokens SET access_token = $1, expires_at = $2 WHERE refresh_token = $3`,
		newAccessToken, newExpiresAt, refreshToken,
	)
	if err != nil {
		return nil, err
	}
	
	token.AccessToken = newAccessToken
	token.ExpiresAt = newExpiresAt
	
	return &token, nil
}
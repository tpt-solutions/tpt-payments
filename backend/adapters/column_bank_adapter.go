// tpt-payments/backend/adapters/column_bank_adapter.go
package adapters

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ColumnBankAdapter implements CardNetworkAdapter for Column.dev bank sponsor API
// Column is an FDIC insured bank providing modern programmatic banking APIs
type ColumnBankAdapter struct {
	apiKey       string
	baseURL      string
	client       *http.Client
	accountID    string
	webhookSecret string
}

// NewColumnBankAdapter creates a new Column bank adapter instance
func NewColumnBankAdapter(apiKey string, isLive bool) *ColumnBankAdapter {
	baseURL := "https://sandbox.column.dev/v1"
	if isLive {
		baseURL = "https://api.column.dev/v1"
	}

	return &ColumnBankAdapter{
		apiKey: apiKey,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
				MaxIdleConns:    10,
				IdleConnTimeout: 90 * time.Second,
			},
		},
	}
}

// Authorize implements CardNetworkAdapter
func (c *ColumnBankAdapter) Authorize(ctx context.Context, req AuthRequest) (*AuthResponse, error) {
	columnReq := map[string]interface{}{
		"amount":              req.Amount,
		"currency":            req.Currency,
		"card_token":          req.CardToken,
		"merchant_id":         req.MerchantID,
		"external_id":         req.IdempotencyKey,
		"payment_intent_id":   req.PaymentIntentID,
		"client_ip":           req.ClientIP,
		"user_agent":          req.UserAgent,
		"metadata":            req.Metadata,
		"three_d_secure":      req.ThreeDSecureEnabled,
	}

	var columnResp struct {
		ID              string `json:"id"`
		Status          string `json:"status"`
		ApprovalCode    string `json:"approval_code"`
		NetworkID       string `json:"network_transaction_id"`
		ErrorCode       string `json:"error_code"`
		ErrorMessage    string `json:"error_message"`
		CreatedAt       int64  `json:"created_at"`
		ExpiresAt       int64  `json:"expires_at"`
	}

	if err := c.post(ctx, "/authorizations", columnReq, &columnResp); err != nil {
		return nil, err
	}

	return &AuthResponse{
		AuthorizationID: columnResp.ID,
		Status:          mapColumnStatus(columnResp.Status),
		ApprovalCode:    columnResp.ApprovalCode,
		NetworkID:       columnResp.NetworkID,
		ErrorCode:       mapColumnError(columnResp.ErrorCode),
		ErrorMessage:    columnResp.ErrorMessage,
		CreatedAt:       columnResp.CreatedAt,
		ExpiresAt:       columnResp.ExpiresAt,
	}, nil
}

// Capture implements CardNetworkAdapter
func (c *ColumnBankAdapter) Capture(ctx context.Context, req CaptureRequest) (*CaptureResponse, error) {
	columnReq := map[string]interface{}{
		"amount":       req.Amount,
		"external_id":  req.IdempotencyKey,
		"metadata":     req.Metadata,
	}

	path := fmt.Sprintf("/authorizations/%s/capture", req.AuthorizationID)

	var columnResp struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		NetworkID  string `json:"network_transaction_id"`
		ErrorCode  string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
		CreatedAt  int64  `json:"created_at"`
		SettledAt  int64  `json:"settled_at"`
	}

	if err := c.post(ctx, path, columnReq, &columnResp); err != nil {
		return nil, err
	}

	return &CaptureResponse{
		CaptureID:   columnResp.ID,
		Status:      mapColumnStatus(columnResp.Status),
		NetworkID:   columnResp.NetworkID,
		ErrorCode:   mapColumnError(columnResp.ErrorCode),
		ErrorMessage: columnResp.ErrorMessage,
		CreatedAt:   columnResp.CreatedAt,
		SettledAt:   columnResp.SettledAt,
	}, nil
}

// Void implements CardNetworkAdapter
func (c *ColumnBankAdapter) Void(ctx context.Context, req VoidRequest) (*VoidResponse, error) {
	columnReq := map[string]interface{}{
		"reason":      req.Reason,
		"external_id": req.IdempotencyKey,
	}

	path := fmt.Sprintf("/authorizations/%s/void", req.AuthorizationID)

	var columnResp struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		ErrorCode  string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
		CreatedAt  int64  `json:"created_at"`
	}

	if err := c.post(ctx, path, columnReq, &columnResp); err != nil {
		return nil, err
	}

	return &VoidResponse{
		VoidID:     columnResp.ID,
		Status:     mapColumnStatus(columnResp.Status),
		ErrorCode:  mapColumnError(columnResp.ErrorCode),
		ErrorMessage: columnResp.ErrorMessage,
		CreatedAt:  columnResp.CreatedAt,
	}, nil
}

// Refund implements CardNetworkAdapter
func (c *ColumnBankAdapter) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	columnReq := map[string]interface{}{
		"amount":       req.Amount,
		"reason":       req.Reason,
		"external_id":  req.IdempotencyKey,
		"metadata":     req.Metadata,
	}

	path := fmt.Sprintf("/captures/%s/refund", req.CaptureID)

	var columnResp struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		NetworkID  string `json:"network_transaction_id"`
		ErrorCode  string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
		CreatedAt  int64  `json:"created_at"`
	}

	if err := c.post(ctx, path, columnReq, &columnResp); err != nil {
		return nil, err
	}

	return &RefundResponse{
		RefundID:    columnResp.ID,
		Status:      mapColumnStatus(columnResp.Status),
		NetworkID:   columnResp.NetworkID,
		ErrorCode:   mapColumnError(columnResp.ErrorCode),
		ErrorMessage: columnResp.ErrorMessage,
		CreatedAt:   columnResp.CreatedAt,
	}, nil
}

// Tokenize implements CardNetworkAdapter
func (c *ColumnBankAdapter) Tokenize(ctx context.Context, req TokenizeRequest) (*TokenResponse, error) {
	columnReq := map[string]interface{}{
		"card_number":  req.CardNumber,
		"exp_month":    req.ExpMonth,
		"exp_year":     req.ExpYear,
		"card_holder":  req.CardHolder,
		"cvv":          req.CVV,
		"merchant_id":  req.MerchantID,
	}

	var columnResp struct {
		Token           string `json:"token"`
		CardFingerprint string `json:"fingerprint"`
		Last4           string `json:"last4"`
		ExpMonth        int    `json:"exp_month"`
		ExpYear         int    `json:"exp_year"`
		CardBrand       string `json:"brand"`
		CreatedAt       int64  `json:"created_at"`
	}

	if err := c.post(ctx, "/tokens", columnReq, &columnResp); err != nil {
		return nil, err
	}

	return &TokenResponse{
		Token:           columnResp.Token,
		CardFingerprint: columnResp.CardFingerprint,
		Last4:           columnResp.Last4,
		ExpMonth:        columnResp.ExpMonth,
		ExpYear:         columnResp.ExpYear,
		CardBrand:       columnResp.CardBrand,
		CreatedAt:       columnResp.CreatedAt,
	}, nil
}

// post sends a POST request to Column API
func (c *ColumnBankAdapter) post(ctx context.Context, path string, body interface{}, result interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetworkError, err)
	}

	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetworkError, err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetworkError, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetworkError, err)
	}

	if resp.StatusCode >= 400 {
		return mapColumnHTTPError(resp.StatusCode, respBody)
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("%w: response parse error: %v", ErrNetworkError, err)
		}
	}

	return nil
}

// mapColumnStatus maps Column status codes to standard TPT status codes
func mapColumnStatus(columnStatus string) string {
	switch columnStatus {
	case "pending":
		return "pending"
	case "authorized", "approved":
		return "authorized"
	case "completed", "succeeded":
		return "succeeded"
	case "declined":
		return "declined"
	case "failed":
		return "failed"
	case "expired":
		return "expired"
	case "canceled":
		return "canceled"
	default:
		return "pending"
	}
}

// mapColumnError maps Column error codes to standard TPT error codes
func mapColumnError(columnErrorCode string) string {
	switch columnErrorCode {
	case "card_declined":
		return ErrCardDeclined
	case "expired_card":
		return ErrExpiredCard
	case "insufficient_funds":
		return ErrInsufficientFunds
	case "invalid_card_number":
		return ErrInvalidCardNumber
	case "invalid_expiry":
		return ErrInvalidExpiry
	case "invalid_cvv":
		return ErrInvalidCVV
	case "duplicate_transaction":
		return ErrDuplicateTransaction
	case "authorization_expired":
		return ErrAuthorizationExpired
	case "already_captured":
		return ErrCaptureAlreadyDone
	case "refund_not_allowed":
		return ErrRefundNotAllowed
	case "rate_limit_exceeded":
		return ErrRateLimitExceeded
	case "fraud_detected", "risk_blocked":
		return ErrFraudDetected
	default:
		return ErrNetworkError
	}
}

// mapColumnHTTPError maps HTTP status codes to standard errors
func mapColumnHTTPError(statusCode int, body []byte) error {
	var errResp struct {
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
	}

	if err := json.Unmarshal(body, &errResp); err == nil && errResp.ErrorCode != "" {
		return fmt.Errorf("%w: %s", mapColumnError(errResp.ErrorCode), errResp.Message)
	}

	switch statusCode {
	case http.StatusTooManyRequests:
		return fmt.Errorf("%w: rate limit exceeded", ErrRateLimitExceeded)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: authentication failed", ErrNetworkError)
	case http.StatusNotFound:
		return fmt.Errorf("%w: resource not found", ErrNetworkError)
	default:
		return fmt.Errorf("%w: HTTP %d: %s", ErrNetworkError, statusCode, string(body))
	}
}

// Verify interface implementation
var _ CardNetworkAdapter = (*ColumnBankAdapter)(nil)
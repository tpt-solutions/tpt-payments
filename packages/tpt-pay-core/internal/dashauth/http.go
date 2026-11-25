package dashauth

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// MaxServiceTokenLength is the minimum length of the service token. Shorter tokens are refused at
// startup, so a weak shared secret cannot be configured by accident.
const MinServiceTokenLength = 32

// Handler serves the internal routes the frontend calls. Every request must carry the service
// token in X-Dashboard-Service-Token, compared in constant time; without it the routes are not
// reachable. They are not part of the public API and are not in the OpenAPI document.
func (s *Service) Handler(serviceToken string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/dashboard/authenticate", s.handleAuthenticate)
	mux.HandleFunc("POST /internal/dashboard/verify-totp", s.handleVerifyTOTP)
	return requireServiceToken(serviceToken, mux)
}

func requireServiceToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("X-Dashboard-Service-Token")
		if len(got) == 0 || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

type authenticateRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Service) handleAuthenticate(w http.ResponseWriter, r *http.Request) {
	var body authenticateRequest
	if !decodeBody(w, r, &body) {
		return
	}
	user, err := s.Authenticate(r.Context(), body.Email, body.Password)
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
	case errors.Is(err, ErrLocked):
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "locked"})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"user": user})
	}
}

type verifyTOTPRequest struct {
	UserID string `json:"user_id"`
	Code   string `json:"code"`
}

func (s *Service) handleVerifyTOTP(w http.ResponseWriter, r *http.Request) {
	var body verifyTOTPRequest
	if !decodeBody(w, r, &body) {
		return
	}
	err := s.VerifyTOTP(r.Context(), body.UserID, body.Code)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, ErrInvalidCode):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_code"})
	case errors.Is(err, ErrLocked):
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "locked"})
	case errors.Is(err, ErrNotEnrolled):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "not_enrolled"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

// decodeBody reads a small JSON object. Unknown fields and trailing data are refused.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil || dec.More() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ValidServiceToken reports whether a configured token is long enough to use.
func ValidServiceToken(token string) bool {
	return len(strings.TrimSpace(token)) >= MinServiceTokenLength
}

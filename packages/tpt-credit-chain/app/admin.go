package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cosmossdk.io/math"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// The admin API lets a command run in another shell submit transactions to a running node. It
// listens on loopback only, and every request must carry the admin token, which the node writes to
// its home directory with mode 0600. Transfers also carry the sender's signature, which the chain
// checks itself; the token is what protects key binding, which no signature covers.

// DefaultAdminAddr is where the admin API listens unless told otherwise.
const DefaultAdminAddr = "127.0.0.1:26670"

const adminTokenFile = "admin.token"

// AdminTokenPath is where the admin token lives in a node home.
func AdminTokenPath(home string) (string, error) {
	dir, err := expandHome(home)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, adminTokenFile), nil
}

// LoadOrCreateAdminToken returns the admin token in a node home, creating it on first use.
func LoadOrCreateAdminToken(home string) (string, error) {
	path, err := AdminTokenPath(home)
	if err != nil {
		return "", err
	}
	if raw, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(raw)), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read admin token: %w", err)
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("no randomness for the admin token: %w", err)
	}
	token := hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write admin token: %w", err)
	}
	return token, nil
}

// ServeAdmin starts the admin API on addr. It refuses any address that is not loopback.
func (app *App) ServeAdmin(addr, token string) (*http.Server, net.Listener, error) {
	if err := requireLoopback(addr); err != nil {
		return nil, nil, err
	}
	if len(token) < 32 {
		return nil, nil, errors.New("the admin token must be at least 32 characters")
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("admin listen on %s: %w", addr, err)
	}
	srv := &http.Server{Handler: app.adminHandler(token), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return srv, ln, nil
}

func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("admin address %q: %w", addr, err)
	}
	ip := net.ParseIP(host)
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return nil
	}
	return fmt.Errorf("the admin API listens on loopback only; %q is not a loopback address", addr)
}

func (app *App) adminHandler(token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /admin/tx/send", app.adminSend)
	mux.HandleFunc("POST /admin/tx/batch-send", app.adminBatchSend)
	mux.HandleFunc("POST /admin/tx/bind-key", app.adminBindKey)
	return requireAdminToken(token, mux)
}

func requireAdminToken(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeAdminJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// SendRequest is the body of a signed transfer, as the CLI sends it.
type SendRequest struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Amount    string `json:"amount"`
	Nonce     uint64 `json:"nonce"`
	Signature string `json:"signature"` // hex
}

// BatchSendRequest is the body of a signed batch transfer.
type BatchSendRequest struct {
	From      string                    `json:"from"`
	Entries   []issuance.BatchSendEntry `json:"entries"`
	Nonce     uint64                    `json:"nonce"`
	Signature string                    `json:"signature"` // hex
}

// BindKeyRequest binds an account's public key.
type BindKeyRequest struct {
	Address   string `json:"address"`
	PublicKey string `json:"public_key"` // hex
}

func (app *App) adminSend(w http.ResponseWriter, r *http.Request) {
	var req SendRequest
	if !decodeAdmin(w, r, &req) {
		return
	}
	amount, ok := math.NewIntFromString(req.Amount)
	if !ok {
		writeAdminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid amount"})
		return
	}
	sig, err := hex.DecodeString(req.Signature)
	if err != nil {
		writeAdminJSON(w, http.StatusBadRequest, map[string]string{"error": "signature must be hex"})
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	event, err := app.IssuanceKeeper.SendCreditsSigned(issuance.SignedSend{
		From: req.From, To: req.To, Amount: amount, Nonce: req.Nonce, Signature: sig,
	})
	if err != nil {
		writeTxError(w, err)
		return
	}
	app.LogTx("tx-send", fmt.Sprintf("Send: %s -> %s, %s ucredit", req.From, req.To, req.Amount))
	writeAdminJSON(w, http.StatusOK, event)
}

func (app *App) adminBatchSend(w http.ResponseWriter, r *http.Request) {
	var req BatchSendRequest
	if !decodeAdmin(w, r, &req) {
		return
	}
	sig, err := hex.DecodeString(req.Signature)
	if err != nil {
		writeAdminJSON(w, http.StatusBadRequest, map[string]string{"error": "signature must be hex"})
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	event, err := app.IssuanceKeeper.BatchSendSigned(issuance.SignedBatch{
		From: req.From, Entries: req.Entries, Nonce: req.Nonce, Signature: sig,
	})
	if err != nil {
		writeTxError(w, err)
		return
	}
	app.LogTx("tx-batch-send", fmt.Sprintf("BatchSend: %s, %d recipients", req.From, len(req.Entries)))
	writeAdminJSON(w, http.StatusOK, event)
}

func (app *App) adminBindKey(w http.ResponseWriter, r *http.Request) {
	var req BindKeyRequest
	if !decodeAdmin(w, r, &req) {
		return
	}
	pub, err := hex.DecodeString(req.PublicKey)
	if err != nil {
		writeAdminJSON(w, http.StatusBadRequest, map[string]string{"error": "public_key must be hex"})
		return
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	if err := app.IssuanceKeeper.BindKey(req.Address, pub); err != nil {
		writeTxError(w, err)
		return
	}
	app.LogTx("tx-bind-key", fmt.Sprintf("BindKey: %s", req.Address))
	writeAdminJSON(w, http.StatusOK, map[string]string{"address": req.Address, "public_key": req.PublicKey})
}

// writeTxError maps a rejected transaction to a 4xx with the reason. Authorization failures are
// refused without saying which check failed beyond the reason the keeper gives.
func writeTxError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, issuance.ErrBadSignature) || errors.Is(err, issuance.ErrNoKey) || errors.Is(err, issuance.ErrBadNonce) {
		status = http.StatusForbidden
	}
	writeAdminJSON(w, status, map[string]string{"error": err.Error()})
}

func decodeAdmin(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil || dec.More() {
		writeAdminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return false
	}
	return true
}

func writeAdminJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// Package vault is a minimal client for HashiCorp Vault's Transit secrets engine. It is used to
// unwrap the webhook secret-encryption key at startup, so the plaintext key does not have to be
// stored in the environment or in a file.
//
// Only encrypt and decrypt are implemented. Keys never leave Vault: the service holds a
// ciphertext and asks Vault to decrypt it.
package vault

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotConfigured is returned when the client is built without the settings it needs.
var ErrNotConfigured = errors.New("vault: address, token, and transit key are required")

// Transit calls one transit key.
type Transit struct {
	addr   string
	token  string
	mount  string
	key    string
	client *http.Client
}

// NewTransit returns a client for transit key `key` mounted at `mount` (default "transit").
// The address must be https, except for loopback addresses, so the token is not sent in clear.
func NewTransit(addr, token, mount, key string) (*Transit, error) {
	if addr == "" || token == "" || key == "" {
		return nil, ErrNotConfigured
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("vault: address %q is not a URL", addr)
	}
	if u.Scheme != "https" && !isLoopback(u.Hostname()) {
		return nil, fmt.Errorf("vault: address must use https unless it is a loopback address")
	}
	if mount == "" {
		mount = "transit"
	}
	return &Transit{
		addr:   strings.TrimRight(addr, "/"),
		token:  token,
		mount:  strings.Trim(mount, "/"),
		key:    key,
		client: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// Encrypt returns a Vault ciphertext ("vault:v1:...") for plaintext.
func (t *Transit) Encrypt(ctx context.Context, plaintext []byte) (string, error) {
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	body := map[string]string{"plaintext": base64.StdEncoding.EncodeToString(plaintext)}
	if err := t.post(ctx, "encrypt", body, &out); err != nil {
		return "", err
	}
	if out.Data.Ciphertext == "" {
		return "", errors.New("vault: encrypt returned no ciphertext")
	}
	return out.Data.Ciphertext, nil
}

// Decrypt returns the plaintext of a Vault ciphertext.
func (t *Transit) Decrypt(ctx context.Context, ciphertext string) ([]byte, error) {
	var out struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := t.post(ctx, "decrypt", map[string]string{"ciphertext": ciphertext}, &out); err != nil {
		return nil, err
	}
	plaintext, err := base64.StdEncoding.DecodeString(out.Data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("vault: decrypt returned malformed plaintext: %w", err)
	}
	return plaintext, nil
}

func (t *Transit) post(ctx context.Context, op string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/v1/%s/%s/%s", t.addr, t.mount, op, url.PathEscape(t.key))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("X-Vault-Token", t.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		// The URL has no secret in it; the transport error does not include headers.
		return fmt.Errorf("vault: %s request failed: %w", op, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("vault: read %s response: %w", op, err)
	}
	if resp.StatusCode != http.StatusOK {
		// Report the status and Vault's error list, never the request.
		var e struct {
			Errors []string `json:"errors"`
		}
		_ = json.Unmarshal(raw, &e)
		return fmt.Errorf("vault: %s returned HTTP %d %v", op, resp.StatusCode, e.Errors)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("vault: decode %s response: %w", op, err)
	}
	return nil
}

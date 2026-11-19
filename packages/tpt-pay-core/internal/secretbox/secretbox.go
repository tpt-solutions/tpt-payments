// Package secretbox encrypts secrets at rest with AES-256-GCM.
//
// Ciphertext format: "v1:" + base64(nonce || ciphertext). The version prefix allows
// the scheme to change without a flag day. The key is supplied by configuration
// (WEBHOOK_SECRET_KEY, 32 random bytes, base64). Moving the key into Vault or a KMS
// is tracked separately; this package only needs a key, not a particular store.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const version = "v1:"

// Box seals and opens secrets with a single key.
type Box struct {
	aead cipher.AEAD
}

// New builds a Box from a base64-encoded 32-byte key.
func New(keyB64 string) (*Box, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keyB64))
	if err != nil {
		return nil, fmt.Errorf("secret key is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secret key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext with a fresh random nonce.
func (b *Box) Seal(plaintext []byte) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	out := b.aead.Seal(nonce, nonce, plaintext, nil)
	return version + base64.StdEncoding.EncodeToString(out), nil
}

// ErrOpen is returned when ciphertext is malformed, from an unknown version, or was
// produced under a different key.
var ErrOpen = errors.New("secretbox: cannot open ciphertext")

// Open decrypts a value produced by Seal.
func (b *Box) Open(sealed string) ([]byte, error) {
	if !strings.HasPrefix(sealed, version) {
		return nil, ErrOpen
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, version))
	if err != nil {
		return nil, ErrOpen
	}
	ns := b.aead.NonceSize()
	if len(raw) < ns {
		return nil, ErrOpen
	}
	pt, err := b.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return nil, ErrOpen
	}
	return pt, nil
}

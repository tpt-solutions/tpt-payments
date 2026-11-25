package app

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// KeysCommand returns the key management commands. A key file holds one Ed25519 seed, hex encoded,
// and is written with mode 0600. The chain never sees the file; it sees only the public key, and the
// signatures made with the private one.
func KeysCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Key management for signed transfers",
	}
	newCmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new key file and print its public key",
		Long: `Creates an Ed25519 key in a new file and prints the public key. The file is never overwritten.
Bind the public key to an account with: tptcreditd tx issuance bind-key.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out, _ := cmd.Flags().GetString("out")
			if out == "" {
				return errors.New("--out is required")
			}
			pub, err := newKeyFile(out)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), hex.EncodeToString(pub)); err != nil {
				return err
			}
			return nil
		},
	}
	newCmd.Flags().String("out", "", "the new key file (must not exist)")
	cmd.AddCommand(newCmd)
	return cmd
}

// newKeyFile writes a fresh seed to path, refusing to replace an existing file.
func newKeyFile(path string) (ed25519.PublicKey, error) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		return nil, fmt.Errorf("no randomness for a key: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("%s already exists; choose another file", path)
		}
		return nil, fmt.Errorf("create key file: %w", err)
	}
	if _, err := f.WriteString(hex.EncodeToString(seed) + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write key file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return ed25519.NewKeyFromSeed(seed).Public().(ed25519.PublicKey), nil
}

// loadSigningKey reads a key file written by newKeyFile.
func loadSigningKey(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("the key file must hold one 32-byte hex seed")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

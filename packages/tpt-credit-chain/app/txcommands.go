package app

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"cosmossdk.io/math"
	"github.com/spf13/cobra"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// The transaction commands sign on this machine and then submit. If the node runs in this process,
// the transaction is applied directly. Otherwise it is posted to the running node's admin API on
// loopback, with the admin token from the node home. Either way the chain verifies the signature.

// homeDir is the node home the commands operate on.
func homeDir() string {
	if h := os.Getenv("TPTCREDIT_HOME"); h != "" {
		return h
	}
	return DefaultNodeHome
}

// SendCreditsCmd returns the signed send command.
func SendCreditsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send credits between accounts (signed by the sender's key)",
		RunE: func(cmd *cobra.Command, args []string) error {
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			amount, _ := cmd.Flags().GetString("amount")
			keyPath, _ := cmd.Flags().GetString("key-file")
			nonce, _ := cmd.Flags().GetUint64("nonce")
			adminAddr, _ := cmd.Flags().GetString("admin-addr")
			if from == "" || to == "" || amount == "" || keyPath == "" || nonce == 0 {
				return errors.New("--from, --to, --amount, --key-file and --nonce are required")
			}
			amountInt, ok := math.NewIntFromString(amount)
			if !ok {
				return fmt.Errorf("invalid amount: %s", amount)
			}
			priv, err := loadSigningKey(keyPath)
			if err != nil {
				return err
			}
			sig := ed25519.Sign(priv, issuance.SendSigningBytes(from, to, amountInt, nonce))

			if GlobalApp != nil {
				GlobalApp.mu.Lock()
				defer GlobalApp.mu.Unlock()
				event, err := GlobalApp.IssuanceKeeper.SendCreditsSigned(issuance.SignedSend{
					From: from, To: to, Amount: amountInt, Nonce: nonce, Signature: sig,
				})
				if err != nil {
					return fmt.Errorf("send failed: %w", err)
				}
				GlobalApp.LogTx("tx-send", fmt.Sprintf("Send: %s -> %s, %s ucredit", from, to, amount))
				return printJSON(event)
			}
			body, err := submitRemote(adminAddr, "/admin/tx/send", SendRequest{
				From: from, To: to, Amount: amount, Nonce: nonce, Signature: hex.EncodeToString(sig),
			})
			if err != nil {
				return err
			}
			return printRaw(body)
		},
	}
	cmd.Flags().String("from", "", "Sender address")
	cmd.Flags().String("to", "", "Recipient address")
	cmd.Flags().String("amount", "", "Amount in ucredit")
	cmd.Flags().String("key-file", "", "the sender's key file (see tptcreditd keys new)")
	cmd.Flags().Uint64("nonce", 0, "one more than the sender's last accepted nonce")
	cmd.Flags().String("admin-addr", DefaultAdminAddr, "the running node's admin address (used when the node is not in this process)")
	return cmd
}

// BatchSendCmd returns the signed batch-send command.
func BatchSendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch-send",
		Short: "Send credits to several recipients (signed by the sender's key)",
		RunE: func(cmd *cobra.Command, args []string) error {
			from, _ := cmd.Flags().GetString("from")
			entriesJSON, _ := cmd.Flags().GetString("entries")
			keyPath, _ := cmd.Flags().GetString("key-file")
			nonce, _ := cmd.Flags().GetUint64("nonce")
			adminAddr, _ := cmd.Flags().GetString("admin-addr")
			if from == "" || entriesJSON == "" || keyPath == "" || nonce == 0 {
				return errors.New("--from, --entries, --key-file and --nonce are required")
			}
			var entries []issuance.BatchSendEntry
			if err := json.Unmarshal([]byte(entriesJSON), &entries); err != nil {
				return fmt.Errorf("invalid entries JSON: %w", err)
			}
			priv, err := loadSigningKey(keyPath)
			if err != nil {
				return err
			}
			sig := ed25519.Sign(priv, issuance.BatchSigningBytes(from, entries, nonce))

			if GlobalApp != nil {
				GlobalApp.mu.Lock()
				defer GlobalApp.mu.Unlock()
				event, err := GlobalApp.IssuanceKeeper.BatchSendSigned(issuance.SignedBatch{
					From: from, Entries: entries, Nonce: nonce, Signature: sig,
				})
				if err != nil {
					return fmt.Errorf("batch send failed: %w", err)
				}
				GlobalApp.LogTx("tx-batch-send", fmt.Sprintf("BatchSend: %s, %d recipients", from, len(entries)))
				return printJSON(event)
			}
			body, err := submitRemote(adminAddr, "/admin/tx/batch-send", BatchSendRequest{
				From: from, Entries: entries, Nonce: nonce, Signature: hex.EncodeToString(sig),
			})
			if err != nil {
				return err
			}
			return printRaw(body)
		},
	}
	cmd.Flags().String("from", "", "Sender address")
	cmd.Flags().String("entries", "", "JSON array of {to,amount} objects")
	cmd.Flags().String("key-file", "", "the sender's key file (see tptcreditd keys new)")
	cmd.Flags().Uint64("nonce", 0, "one more than the sender's last accepted nonce")
	cmd.Flags().String("admin-addr", DefaultAdminAddr, "the running node's admin address (used when the node is not in this process)")
	return cmd
}

// BindKeyCmd returns the command that binds an account's public key. It can be run once per account.
func BindKeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bind-key",
		Short: "Bind an account's public key (once per account; needs the admin token)",
		RunE: func(cmd *cobra.Command, args []string) error {
			address, _ := cmd.Flags().GetString("address")
			pubHex, _ := cmd.Flags().GetString("public-key")
			adminAddr, _ := cmd.Flags().GetString("admin-addr")
			if address == "" || pubHex == "" {
				return errors.New("--address and --public-key are required")
			}
			pub, err := hex.DecodeString(pubHex)
			if err != nil || len(pub) != ed25519.PublicKeySize {
				return errors.New("--public-key must be a 32-byte hex Ed25519 key")
			}

			if GlobalApp != nil {
				GlobalApp.mu.Lock()
				defer GlobalApp.mu.Unlock()
				if err := GlobalApp.IssuanceKeeper.BindKey(address, pub); err != nil {
					return err
				}
				GlobalApp.LogTx("tx-bind-key", fmt.Sprintf("BindKey: %s", address))
				return printJSON(map[string]string{"address": address, "public_key": pubHex})
			}
			body, err := submitRemote(adminAddr, "/admin/tx/bind-key", BindKeyRequest{Address: address, PublicKey: pubHex})
			if err != nil {
				return err
			}
			return printRaw(body)
		},
	}
	cmd.Flags().String("address", "", "account address")
	cmd.Flags().String("public-key", "", "the account's Ed25519 public key, hex")
	cmd.Flags().String("admin-addr", DefaultAdminAddr, "the running node's admin address (used when the node is not in this process)")
	return cmd
}

// submitRemote posts a request to the running node's admin API and returns the response body.
func submitRemote(addr, path string, body any) ([]byte, error) {
	tokenPath, err := AdminTokenPath(homeDir())
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(tokenPath)
	if err != nil {
		return nil, fmt.Errorf("the chain is not running here and no admin token was found at %s: start the node first", tokenPath)
	}
	token := strings.TrimSpace(string(raw))

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no running node answered at %s: %w", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(out, &e)
		if e.Error == "" {
			e.Error = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("the node refused the transaction (HTTP %d): %s", resp.StatusCode, e.Error)
	}
	return out, nil
}

func printJSON(v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func printRaw(body []byte) error {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, body, "", "  "); err != nil {
		fmt.Println(string(body))
		return nil
	}
	fmt.Println(pretty.String())
	return nil
}

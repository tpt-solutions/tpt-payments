package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"cosmossdk.io/math"
	"github.com/spf13/cobra"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// GlobalApp holds the current running app instance for CLI commands.
var GlobalApp *App

// TxCommand returns the transaction subcommands.
func TxCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tx",
		Short: "Transaction subcommands",
	}

	issuanceCmd := &cobra.Command{
		Use:   "issuance",
		Short: "Issuance module transactions",
	}

	issuanceCmd.AddCommand(
		RegisterHumanCmd(),
		UpdateFrequencyCmd(),
		SendCreditsCmd(),
		BatchSendCmd(),
	)

	cmd.AddCommand(issuanceCmd)

	// Add data directory reset command.
	cmd.AddCommand(&cobra.Command{
		Use:   "reset",
		Short: "Reset chain state (reinitialize)",
		RunE: func(cmd *cobra.Command, args []string) error {
			home := os.Getenv("TPTCREDIT_HOME")
			if home == "" {
				home = DefaultNodeHome
			}

			// Expand ~.
			if strings.HasPrefix(home, "~/") {
				homeDir, _ := os.UserHomeDir()
				home = homeDir + home[1:]
			}

			chainID, _ := cmd.Flags().GetString("chain-id")
			if chainID == "" {
				chainID = "tpt-credit-devnet-1"
			}

			genesis := NewDefaultGenesis(chainID, "devnode")
			return genesis.Write(home)
		},
	})
	cmd.Commands()[1].Flags().String("chain-id", "tpt-credit-devnet-1", "Chain ID")

	return cmd
}

// QueryCommand returns the query subcommands.
func QueryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query subcommands",
	}

	issuanceCmd := &cobra.Command{
		Use:   "issuance",
		Short: "Query issuance module state",
	}
	issuanceCmd.AddCommand(
		QueryAccountCmd(),
		QuerySupplyCmd(),
		QueryAccountsCmd(),
	)

	burnCmd := &cobra.Command{
		Use:   "burn",
		Short: "Query burn module state",
	}
	burnCmd.AddCommand(
		QueryBurnRecordCmd(),
		QueryBurnSummaryCmd(),
		QueryBurnSummariesCmd(),
	)

	cmd.AddCommand(issuanceCmd)
	cmd.AddCommand(burnCmd)
	cmd.AddCommand(StatusCmd())

	return cmd
}

// VersionCommand returns the version subcommand.
func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the application version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("tptcreditd v0.1.0")
			fmt.Println("TPT Credit Chain - Sovereign Credit Blockchain")
			fmt.Println("Core modules: issuance (immutable), burn (immutable)")
			return nil
		},
	}
}

// StatusCmd returns the status subcommand.
func StatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Query the chain status",
		RunE: func(cmd *cobra.Command, args []string) error {
			view, release, err := queryView(os.Getenv("TPTCREDIT_HOME"), false)
			if err != nil {
				return err
			}
			defer release()
			snapshot := view.GetStateSnapshot()
			if GlobalApp == nil {
				snapshot["source"] = "state_file"
				snapshot["note"] = "read from the last state the node saved; event counts are not persisted and read as zero"
			}
			snapshotJSON, _ := json.MarshalIndent(snapshot, "", "  ")
			fmt.Println(string(snapshotJSON))
			return nil
		},
	}
}

// RegisterHumanCmd returns the register-human CLI command.
func RegisterHumanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register-human",
		Short: "Register a new verified human",
		Long:  `Register a new verified human identity with KYC attestation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, _ := cmd.Flags().GetString("address")
			uuid, _ := cmd.Flags().GetString("uuid")
			tier, _ := cmd.Flags().GetString("tier")
			freq, _ := cmd.Flags().GetString("freq")

			if addr == "" || uuid == "" {
				return fmt.Errorf("--address and --uuid are required")
			}
			if tier == "" {
				tier = "kyc"
			}
			if freq == "" {
				freq = "monthly"
			}

			if GlobalApp == nil {
				return fmt.Errorf("chain is not running; use 'tptcreditd start' first")
			}
			GlobalApp.mu.Lock()
			defer GlobalApp.mu.Unlock()

			event, err := GlobalApp.IssuanceKeeper.RegisterHuman(
				context.Background(),
				addr, uuid, tier, issuance.PayoutFrequency(freq),
			)
			if err != nil {
				return fmt.Errorf("registration failed: %w", err)
			}

			GlobalApp.LogTx("tx-register-human", fmt.Sprintf("RegisterHuman: %s", addr))

			eventJSON, _ := json.MarshalIndent(event, "", "  ")
			fmt.Println(string(eventJSON))
			return nil
		},
	}

	cmd.Flags().String("address", "", "Cosmos bech32 address")
	cmd.Flags().String("uuid", "", "SHA-256 hash of KYC provider unique person ID")
	cmd.Flags().String("tier", "kyc", "Identity tier (kyc, biometric, dna)")
	cmd.Flags().String("freq", "monthly", "Payout frequency (weekly, biweekly, monthly, quarterly, annual)")
	return cmd
}

// UpdateFrequencyCmd returns the update-frequency CLI command.
func UpdateFrequencyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-frequency",
		Short: "Update payout frequency for an account",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, _ := cmd.Flags().GetString("address")
			freq, _ := cmd.Flags().GetString("freq")

			if addr == "" || freq == "" {
				return fmt.Errorf("--address and --freq are required")
			}

			if GlobalApp == nil {
				return fmt.Errorf("chain is not running; use 'tptcreditd start' first")
			}
			GlobalApp.mu.Lock()
			defer GlobalApp.mu.Unlock()

			event, err := GlobalApp.IssuanceKeeper.UpdatePayoutFrequency(
				addr, issuance.PayoutFrequency(freq),
			)
			if err != nil {
				return fmt.Errorf("frequency update failed: %w", err)
			}

			GlobalApp.LogTx("tx-update-frequency", fmt.Sprintf("UpdateFrequency: %s -> %s", addr, freq))

			eventJSON, _ := json.MarshalIndent(event, "", "  ")
			fmt.Println(string(eventJSON))
			return nil
		},
	}

	cmd.Flags().String("address", "", "Cosmos bech32 address")
	cmd.Flags().String("freq", "", "New payout frequency (weekly, biweekly, monthly, quarterly, annual)")
	return cmd
}

// SendCreditsCmd returns the send-credits CLI command.
func SendCreditsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Send credits between accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			amount, _ := cmd.Flags().GetString("amount")

			if from == "" || to == "" || amount == "" {
				return fmt.Errorf("--from, --to, and --amount are required")
			}

			if GlobalApp == nil {
				return fmt.Errorf("chain is not running; use 'tptcreditd start' first")
			}
			GlobalApp.mu.Lock()
			defer GlobalApp.mu.Unlock()

			amountInt, ok := math.NewIntFromString(amount)
			if !ok {
				return fmt.Errorf("invalid amount: %s", amount)
			}

			event, err := GlobalApp.IssuanceKeeper.SendCredits(from, to, amountInt)
			if err != nil {
				return fmt.Errorf("send failed: %w", err)
			}

			GlobalApp.LogTx("tx-send", fmt.Sprintf("Send: %s -> %s, %s ucredit", from, to, amount))

			eventJSON, _ := json.MarshalIndent(event, "", "  ")
			fmt.Println(string(eventJSON))
			return nil
		},
	}

	cmd.Flags().String("from", "", "Sender address")
	cmd.Flags().String("to", "", "Recipient address")
	cmd.Flags().String("amount", "", "Amount in ucredit")
	return cmd
}

// BatchSendCmd returns the batch-send CLI command.
func BatchSendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch-send",
		Short: "Send credits to multiple recipients",
		RunE: func(cmd *cobra.Command, args []string) error {
			from, _ := cmd.Flags().GetString("from")
			entriesJSON, _ := cmd.Flags().GetString("entries")

			if from == "" || entriesJSON == "" {
				return fmt.Errorf("--from and --entries are required")
			}

			if GlobalApp == nil {
				return fmt.Errorf("chain is not running; use 'tptcreditd start' first")
			}
			GlobalApp.mu.Lock()
			defer GlobalApp.mu.Unlock()

			var entries []issuance.BatchSendEntry
			if err := json.Unmarshal([]byte(entriesJSON), &entries); err != nil {
				return fmt.Errorf("invalid entries JSON: %w", err)
			}

			event, err := GlobalApp.IssuanceKeeper.BatchSend(from, entries)
			if err != nil {
				return fmt.Errorf("batch send failed: %w", err)
			}

			GlobalApp.LogTx("tx-batch-send", fmt.Sprintf("BatchSend: %s, %d recipients", from, len(entries)))

			eventJSON, _ := json.MarshalIndent(event, "", "  ")
			fmt.Println(string(eventJSON))
			return nil
		},
	}

	cmd.Flags().String("from", "", "Sender address")
	cmd.Flags().String("entries", "", "JSON array of {to,amount} objects")
	return cmd
}

// QueryAccountCmd returns the account query command.
func QueryAccountCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Query an account by address",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, _ := cmd.Flags().GetString("address")
			if addr == "" {
				if len(args) > 0 {
					addr = args[0]
				} else {
					return fmt.Errorf("address is required")
				}
			}

			view, release, err := queryView(os.Getenv("TPTCREDIT_HOME"), true)
			if err != nil {
				return err
			}
			defer release()

			acct := view.IssuanceKeeper.GetAccount(addr)
			if acct == nil {
				return fmt.Errorf("account %q not found", addr)
			}

			acctJSON, _ := json.MarshalIndent(acct, "", "  ")
			fmt.Println(string(acctJSON))
			return nil
		},
	}

	cmd.Flags().String("address", "", "Account address to query")
	return cmd
}

// QuerySupplyCmd returns the supply query command.
func QuerySupplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "supply",
		Short: "Query the total supply",
		RunE: func(cmd *cobra.Command, args []string) error {
			view, release, err := queryView(os.Getenv("TPTCREDIT_HOME"), true)
			if err != nil {
				return err
			}
			defer release()

			supply := view.IssuanceKeeper.GetSupply()
			totalBalance := view.IssuanceKeeper.GetTotalBalance()
			humanCount := view.IssuanceKeeper.GetHumanCount()

			fmt.Printf("Total supply (year credits): %s ucredit\n", supply.String())
			fmt.Printf("Total spendable balance: %s ucredit\n", totalBalance.String())
			fmt.Printf("Verified humans: %d\n", humanCount)
			return nil
		},
	}

	return cmd
}

// QueryAccountsCmd returns the list accounts command.
func QueryAccountsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "List all registered accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			view, release, err := queryView(os.Getenv("TPTCREDIT_HOME"), true)
			if err != nil {
				return err
			}
			defer release()

			accounts := view.IssuanceKeeper.GetAccounts()
			if len(accounts) == 0 {
				fmt.Println("No registered accounts.")
				return nil
			}

			for _, acct := range accounts {
				fmt.Printf("%s | tier=%s freq=%s balance=%s next_due=%d\n",
					acct.Address, acct.IdentityTier, acct.PayoutFrequency,
					acct.Balance.String(), acct.NextDueAt)
			}
			fmt.Printf("Total: %d accounts\n", len(accounts))
			return nil
		},
	}

	return cmd
}

// QueryBurnRecordCmd returns the burn record query command.
func QueryBurnRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Query a burn record for an address and year",
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, _ := cmd.Flags().GetString("address")
			year, _ := cmd.Flags().GetInt64("year")

			if addr == "" {
				if len(args) > 0 {
					addr = args[0]
				} else {
					return fmt.Errorf("--address is required")
				}
			}
			if year == 0 {
				year = 1
			}

			view, release, err := queryView(os.Getenv("TPTCREDIT_HOME"), true)
			if err != nil {
				return err
			}
			defer release()

			record := view.BurnKeeper.GetBurnRecord(addr, year)
			if record == nil {
				fmt.Printf("No burn record for %s year %d\n", addr, year)
				history := view.BurnKeeper.GetBurnRecordsByAddress(addr)
				if len(history) > 0 {
					fmt.Println("Available records:")
					for _, r := range history {
						fmt.Printf("  year=%d burned=%d redistributed=%d\n",
							r.YearHeight, r.BurnedAmount, r.RedistributedAmount)
					}
				}
				return nil
			}

			recordJSON, _ := json.MarshalIndent(record, "", "  ")
			fmt.Println(string(recordJSON))
			return nil
		},
	}

	cmd.Flags().String("address", "", "Account address")
	cmd.Flags().Int64("year", 1, "Year (1-based)")
	return cmd
}

// QueryBurnSummaryCmd returns the burn summary query command.
func QueryBurnSummaryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Query the burn summary for a year",
		RunE: func(cmd *cobra.Command, args []string) error {
			year, _ := cmd.Flags().GetInt64("year")
			if year == 0 {
				year = 1
			}

			view, release, err := queryView(os.Getenv("TPTCREDIT_HOME"), true)
			if err != nil {
				return err
			}
			defer release()

			summary := view.BurnKeeper.GetBurnSummary(year)
			if summary == nil {
				return fmt.Errorf("no burn summary for year %d", year)
			}

			summaryJSON, _ := json.MarshalIndent(summary, "", "  ")
			fmt.Println(string(summaryJSON))
			return nil
		},
	}

	cmd.Flags().Int64("year", 1, "Year (1-based)")
	return cmd
}

// QueryBurnSummariesCmd returns the list burn summaries command.
func QueryBurnSummariesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "summaries",
		Short: "List all burn summaries",
		RunE: func(cmd *cobra.Command, args []string) error {
			view, release, err := queryView(os.Getenv("TPTCREDIT_HOME"), true)
			if err != nil {
				return err
			}
			defer release()

			summaries := view.BurnKeeper.GetAllBurnSummaries()
			if len(summaries) == 0 {
				fmt.Println("No burn summaries yet.")
				return nil
			}

			for i, s := range summaries {
				fmt.Printf("Year %d (block %d): burned=%d ucredit, %d humans, per_human=%d, remainder=%d\n",
					i+1, s.YearHeight, s.TotalBurned, s.ActiveHumans, s.PerHumanAmount, s.Remainder)
			}
			return nil
		},
	}

	return cmd
}

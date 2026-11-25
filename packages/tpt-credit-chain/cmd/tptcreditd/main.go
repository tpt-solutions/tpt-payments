// tptcreditd is the main binary for the TPT Credit sovereign credit chain.
//
// The chain runs the issuance and burn modules with an in-memory block
// simulator. Production deployment will integrate with Cosmos SDK/CometBFT.
//
// Usage:
//
//	tptcreditd init <moniker> --chain-id <chain-id>
//	tptcreditd start
//	tptcreditd tx issuance register-human --address <addr> --uuid <sha256> --tier kyc --freq monthly
//	tptcreditd tx issuance update-frequency --address <addr> --freq weekly
//	tptcreditd tx issuance send --from <addr> --to <addr> --amount 1000000
//	tptcreditd query issuance account <address>
//	tptcreditd query issuance supply
//	tptcreditd query burn record --address <addr> --year 1
//	tptcreditd query burn summary --year 1
//	tptcreditd query status
//	tptcreditd version
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/app"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "tptcreditd",
		Short: "TPT Credit chain daemon (sovereign credit blockchain)",
		Long: `TPT Credit is a sovereign credit blockchain with immutable issuance
and burn modules.

Core parameters (hardcoded at genesis, cannot be changed by governance):
  - Annual allocation: 12,000 credits per verified human per year
  - On-ramps: disabled (no fiat-to-credit conversions)
  - Off-ramps: disabled (no credit-to-fiat conversions)
  - Transaction fees: zero
  - Annual full-burn: 100% of balances burn and redistribute to active accounts

Block time target: 2 seconds
Denomination: ucredit (1 credit = 1,000,000 ucredit)
`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			home := os.Getenv("TPTCREDIT_HOME")
			if home == "" {
				home = app.DefaultNodeHome
			}
			os.Setenv("TPTCREDIT_HOME", home)
			return nil
		},
	}

	// Register all subcommands.
	rootCmd.AddCommand(
		InitCmd(),
		StartCmd(),
		app.TxCommand(),
		app.QueryCommand(),
		app.KeysCommand(),
		app.VersionCommand(),
	)

	// Execute.
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// InitCmd initializes the chain node directory.
//
// The home directory is taken from --home, then TPTCREDIT_HOME, then the default. An existing
// genesis is never replaced unless --overwrite is passed.
func InitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init <moniker>",
		Short: "Initialize the chain node directory",
		Long:  `Initialize a home directory for the chain with genesis file, config, and key directories.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := resolveHome(cmd)
			if err != nil {
				return err
			}
			overwrite, _ := cmd.Flags().GetBool("overwrite")
			if !overwrite {
				if _, err := os.Stat(filepath.Join(home, "config", "genesis.json")); err == nil {
					return fmt.Errorf("%s is already initialized; pass --overwrite to replace its genesis", home)
				}
			}

			if overwrite {
				// A replaced genesis must not resume the state of the chain it replaces.
				if err := app.RemoveState(home); err != nil {
					return err
				}
			}

			chainID, _ := cmd.Flags().GetString("chain-id")
			if chainID == "" {
				chainID = "tpt-credit-devnet-1"
			}

			moniker := args[0]
			genesis := app.NewDefaultGenesis(chainID, moniker)
			return genesis.Write(home)
		},
	}

	cmd.Flags().String("chain-id", "tpt-credit-devnet-1", "Chain ID for the genesis file")
	cmd.Flags().String("home", "", "Node home directory (default: $TPTCREDIT_HOME, then "+app.DefaultNodeHome+")")
	cmd.Flags().Bool("overwrite", false, "Replace an existing genesis file")
	return cmd
}

// resolveHome returns the node home directory: --home, then TPTCREDIT_HOME, then the default.
// A leading ~/ is expanded.
func resolveHome(cmd *cobra.Command) (string, error) {
	home, _ := cmd.Flags().GetString("home")
	if home == "" {
		home = os.Getenv("TPTCREDIT_HOME")
	}
	if home == "" {
		home = app.DefaultNodeHome
	}
	if strings.HasPrefix(home, "~/") {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		home = filepath.Join(userHome, home[2:])
	}
	return home, nil
}

// StartCmd starts the chain node.
func StartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the chain node",
		Long:  `Start the TPT Credit chain node with the configured genesis state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := resolveHome(cmd)
			if err != nil {
				return err
			}

			genesis, err := app.LoadGenesis(home)
			if err != nil {
				return fmt.Errorf("failed to load genesis: %w", err)
			}

			chainApp, err := app.NewAppFromGenesis(home, genesis)
			if err != nil {
				return fmt.Errorf("failed to create chain app: %w", err)
			}

			app.GlobalApp = chainApp

			fmt.Printf("Starting TPT Credit chain...\n")
			fmt.Printf("Home directory: %s\n", home)
			fmt.Printf("Chain ID: %s\n", genesis.ChainID)
			fmt.Printf("Block interval: %s\n", app.BlockInterval)
			fmt.Println()

			return chainApp.Start()
		},
	}

	cmd.Flags().String("home", "", "Node home directory (default: $TPTCREDIT_HOME, then "+app.DefaultNodeHome+")")
	return cmd
}

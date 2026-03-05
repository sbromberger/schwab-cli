package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sbromberger/schwab-cli/cmd"
	"github.com/sbromberger/schwab-cli/config"
)

const usage = `schwab-cli — Charles Schwab account balance tool

Usage:
  schwab-cli login                        Authenticate via OAuth (manual flow)
  schwab-cli balances                     Show account balances and positions
  schwab-cli balances --json              Show raw JSON response
  schwab-cli balances --interval 30       Refresh every 30 seconds (Ctrl-C to quit)
  schwab-cli balances --color             Force ANSI color (useful with external watch tools)

Config:
  $XDG_CONFIG_HOME/schwab-cli/config.toml  (default: ~/.config/schwab-cli/config.toml)

  Example config.toml:
    APP_CONFIG = ".app"          # path to credentials file (relative = same dir as config.toml)

    [accounts.176]               # last 3 digits of account number
    name  = "Roth IRA"
    order = 1

    [accounts.234]
    name  = "Brokerage"
    order = 2

  The credentials file must be owned by you and chmod 600:
    APP_KEY = "your-app-key"
    SECRET  = "your-secret"
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	command := os.Args[1]

	// Load config for all commands up front so we fail early with a clear message.
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	cfgDir := config.ConfigDir()

	switch command {
	case "login":
		loginFlags := flag.NewFlagSet("login", flag.ExitOnError)
		loginFlags.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: schwab-cli login\n\n  Authenticate via OAuth (manual flow).\n")
		}
		loginFlags.Parse(os.Args[2:])
		cmd.Login(cfg, cfgDir)

	case "balances":
		balancesFlags := flag.NewFlagSet("balances", flag.ExitOnError)
		jsonOut := balancesFlags.Bool("json", false, "Output raw JSON instead of a human-readable table")
		forceColor := balancesFlags.Bool("color", false, "Force ANSI color output even when stdout is not a TTY")
		interval := balancesFlags.Int("interval", 0, "Refresh every N seconds until Ctrl-C (0 = run once)")
		balancesFlags.Usage = func() {
			fmt.Fprintf(os.Stderr, "Usage: schwab-cli balances [flags]\n\nFlags:\n")
			balancesFlags.PrintDefaults()
		}
		balancesFlags.Parse(os.Args[2:])
		cmd.Balances(cfg, cfgDir, *jsonOut, *forceColor, *interval)

	case "help", "--help", "-h":
		fmt.Print(usage)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %q\n\n", command)
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
}

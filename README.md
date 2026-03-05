# schwab-cli

A command-line tool for viewing Charles Schwab account balances and positions, with live market index quotes.

## Prerequisites

- A [Charles Schwab developer account](https://developer.schwab.com) with an app registered for the Trader API
- Go 1.21 or later (for building from source)
- The redirect URI for your app must be set to `https://127.0.0.1`

## Installation

```sh
git clone https://github.com/sbromberger/schwab-cli.git
cd schwab-cli
GOOS=linux go build -o schwab-cli .   # or omit GOOS for native build
```

## Configuration

schwab-cli uses two config files.

### Main config

Located at `$XDG_CONFIG_HOME/schwab-cli/config.toml` (defaults to `~/.config/schwab-cli/config.toml`).

```toml
APP_CONFIG = ".credentials"   # path to credentials file; relative = same dir as config.toml

[accounts.123]                # last 3 digits of your account number
name  = "IRA"
order = 1

[accounts.456]
name  = "Brokerage"
order = 2
```

`order` controls the display order. Accounts without an entry appear after configured ones.

### Credentials file

The path specified by `APP_CONFIG` in the main config. Must be owned by the running user and have exactly `0600` permissions — the tool refuses to start otherwise.

```toml
APP_KEY = "your-app-key"
SECRET  = "your-secret"
```

```sh
chmod 600 ~/.config/schwab-cli/.credentials
```

## Usage

### Authenticate

```sh
schwab-cli login
```

Prints an OAuth authorization URL. Open it in a browser, approve access, then paste the redirect URL back into the terminal. The token is saved to `~/.config/schwab-cli/token.json` and refreshed automatically (access token expires after 30 minutes; refresh token after 7 days).

### View balances

```sh
schwab-cli balances
```

Displays a table of account balances with day-change dollar and percentage columns, followed by an aggregated positions section. DJIA, NASDAQ, and S&P 500 quotes are shown at the top.

```sh
schwab-cli balances --interval 30    # auto-refresh every 30 seconds (Ctrl-C to quit)
schwab-cli balances --color          # force ANSI color when stdout is not a TTY
schwab-cli balances --json           # print raw JSON response from the API
```

### Help

```sh
schwab-cli help
```

## Output

The balance table shows:

| Column | Description |
|---|---|
| ACCOUNT/LABEL | Masked account number (`***NNN`) and optional label from config |
| VALUE | Current liquidation value |
| ∆day ($) | Day change in dollars (green = positive, red = negative) |
| ∆day (%) | Day change as a percentage |

A **TOTAL** row appears at the bottom of the account section.

Below the accounts, an aggregated positions section lists all holdings across all accounts, one row per symbol, sorted alphabetically. Quantities, market values, and day P&L are summed across accounts.

## Security

- The credentials file is checked for `0600` permissions and correct ownership before any secrets are read.
- OAuth tokens are stored in `token.json` (also written as `0600`) in the config directory.
- Token writes are atomic (written to a temp file, then renamed).

## Dependencies

- [`github.com/BurntSushi/toml`](https://github.com/BurntSushi/toml) — TOML config parsing
- [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term) — TTY detection
- [`golang.org/x/text`](https://pkg.go.dev/golang.org/x/text) — locale-aware number formatting

## License

MIT. See [LICENSE](LICENSE).

---

*This package was developed with the assistance of AI (Claude by Anthropic).*

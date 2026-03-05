# schwab-cli

A command-line tool for viewing Charles Schwab account balances and positions, with live market index quotes.

## Prerequisites

- A [Charles Schwab developer account](https://developer.schwab.com) with an app registered for the Trader API
- Go 1.21 or later (for building from source)
- The redirect URI for your app must be set to `https://127.0.0.1` (this is what the tool expects)

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

### Authentication model

schwab-cli uses the [OAuth 2.0 Authorization Code flow](https://developer.schwab.com/products/trader-api--individual/details/documentation/Retail%20Trader%20API%20Production). No password is ever handled by this tool. The flow works as follows:

1. `schwab-cli login` constructs an authorization URL containing your app key and the redirect URI, and prints it to the terminal.
2. You open that URL in a browser, log in to Schwab, and approve access. Schwab redirects your browser to the redirect URI with an authorization code appended as a query parameter. Copy the full URL from the browser's address bar.
3. Paste that redirect URL into the terminal. schwab-cli extracts the authorization code and exchanges it for an access token and a refresh token via a direct HTTPS call to Schwab's token endpoint.
4. The tokens are saved locally. On subsequent runs, the access token is used directly; when it expires (after 30 minutes), the refresh token is used to obtain a new one automatically. If the refresh token expires (after 7 days), you run `schwab-cli login` again.

At no point does schwab-cli act as a server, open a listening port, or transmit your credentials anywhere other than directly to `api.schwabapi.com`.

### Credential and token storage

- **Credentials file** (`APP_KEY`, `SECRET`): Before reading, the tool checks that the file is owned by the current user and has exactly `0600` permissions. It exits with an error if either check fails — this prevents secrets from being readable by other users or processes.
- **Token file** (`token.json`): Written with `0600` permissions. Writes are atomic — the token is first written to a temporary file in the same directory, then renamed into place, so a crash mid-write can never produce a corrupted or partially written token file.
- **Config directory**: `~/.config/schwab-cli/` — entirely local, never synced or transmitted.

### What the API token can do

The Schwab Trader API token grants read access to account balances and positions, and the ability to place trades if your app is configured for it. schwab-cli only uses read endpoints. The token cannot be used to transfer funds, change account settings, or access personal information beyond what is shown in the tool's output.

## Dependencies

- [`github.com/BurntSushi/toml`](https://github.com/BurntSushi/toml) — TOML config parsing
- [`golang.org/x/term`](https://pkg.go.dev/golang.org/x/term) — TTY detection
- [`golang.org/x/text`](https://pkg.go.dev/golang.org/x/text) — locale-aware number formatting

## License

MIT. See [LICENSE](LICENSE).

---

*This package was developed with the assistance of AI (Claude by Anthropic).*

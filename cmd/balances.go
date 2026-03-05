package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/sbromberger/schwab-cli/api"
	"github.com/sbromberger/schwab-cli/auth"
	"github.com/sbromberger/schwab-cli/config"
)

const (
	ansiReset      = "\033[0m"
	ansiGreen      = "\033[32m"
	ansiRed        = "\033[31m"
	ansiBold       = "\033[1m"
	ansiDim        = "\033[2m"
	ansiHome       = "\033[H"    // move cursor to top-left (no erase)
	ansiEraseToEnd = "\033[J"    // erase from cursor to end of screen
	ansiHideCursor = "\033[?25l" // hide cursor
	ansiShowCursor = "\033[?25h" // show cursor
)

// printer is a locale-aware printer for en-US formatting (commas in numbers, etc.)
var printer = message.NewPrinter(language.AmericanEnglish)

// Balances fetches and displays account balances.
// If jsonOut is true, raw JSON is printed instead of the human-readable table.
// If forceColor is true, ANSI color codes are emitted even when stdout is not a TTY.
// If interval > 0, the display refreshes every interval seconds until interrupted.
func Balances(cfg *config.Config, configDir string, jsonOut bool, forceColor bool, interval int) {
	if interval > 0 {
		// Hide cursor for the duration of the loop; always restore on exit.
		fmt.Print(ansiHideCursor)
		defer fmt.Print(ansiShowCursor)

		// Set up clean exit on Ctrl-C.
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		first := true
		for {
			select {
			case <-sig:
				fmt.Println()
				return
			default:
			}
			if first {
				fmt.Print("\033[2J") // clear screen on first draw only
				first = false
			}
			fmt.Print(ansiHome)
			fetchAndPrint(cfg, configDir, jsonOut, forceColor)
			fmt.Print(ansiEraseToEnd)
			fmt.Printf("\n%sUpdated: %s — refreshing every %ds  (Ctrl-C to quit)%s\n",
				ansiDim, time.Now().Format("15:04:05"), interval, ansiReset)
			select {
			case <-sig:
				fmt.Println()
				return
			case <-time.After(time.Duration(interval) * time.Second):
			}
		}
	}
	fetchAndPrint(cfg, configDir, jsonOut, forceColor)
}

// fetchAndPrint performs one fetch+display cycle.
func fetchAndPrint(cfg *config.Config, configDir string, jsonOut bool, forceColor bool) {
	token, err := auth.LoadToken(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "No token found. Run `schwab-cli login` first.")
		} else {
			fmt.Fprintf(os.Stderr, "error loading token: %v\n", err)
		}
		os.Exit(1)
	}

	token, err = auth.EnsureFresh(cfg, token, configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error refreshing token: %v\n", err)
		os.Exit(1)
	}

	accounts, raw, err := api.GetAccounts(token.AccessToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching accounts: %v\n", err)
		os.Exit(1)
	}

	if jsonOut {
		printJSON(raw)
		return
	}

	indices, err := api.GetIndexQuotes(token.AccessToken)
	if err != nil {
		// Non-fatal: display table without the index line.
		fmt.Fprintf(os.Stderr, "warning: could not fetch index quotes: %v\n", err)
	}

	printTable(accounts, cfg.Accounts, indices, forceColor)
}

// printJSON pretty-prints the raw API response.
func printJSON(raw []byte) {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		os.Stdout.Write(raw)
		return
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		os.Stdout.Write(raw)
		return
	}
	fmt.Println(string(out))
}

// aggPosition accumulates position data across accounts for a single symbol.
type aggPosition struct {
	symbol      string
	description string
	quantity    float64
	marketValue float64
	dayPL       float64 // sum of currentDayProfitLoss across accounts
}

// printTable renders the account balance table followed by an aggregated positions section.
func printTable(accounts []api.Account, accountEntries map[string]config.AccountEntry, indices []api.IndexQuote, forceColor bool) {
	isTTY := forceColor || term.IsTerminal(int(os.Stdout.Fd()))

	hasConfig := len(accountEntries) > 0

	// accountRow holds a formatted account summary row.
	type accountRow struct {
		account   string
		liqVal    string
		dayChange string
		dayPct    string
		sortKey   int
		positive  bool
		zero      bool
	}

	accountHeader := "ACCOUNT"
	if hasConfig {
		accountHeader = "ACCOUNT/LABEL"
	}

	// Column headers for the account summary section.
	hAccount := accountHeader
	hLiqVal := "VALUE"
	hDayChange := "∆day ($)"
	hDayPct := "∆day (%)"

	// Column headers for the positions section.
	hSymbol := "SYMBOL"
	hDesc := "DESCRIPTION"
	hQty := "QTY"
	hMktVal := "MKT VALUE"
	hPosDayChg := "∆day ($)"
	hPosDayPct := "∆day (%)"

	rows := make([]accountRow, 0, len(accounts))
	var totalCurLiq, totalIniLiq float64

	// aggMap accumulates positions keyed by symbol.
	aggMap := make(map[string]*aggPosition)
	// aggOrder tracks insertion order for stable symbol ordering (overridden by alpha sort later).
	var aggSymbols []string

	for _, a := range accounts {
		cur := a.SecuritiesAccount.CurrentBalances
		ini := a.SecuritiesAccount.InitialBalances

		acctNum := a.SecuritiesAccount.AccountNumber
		masked := maskAccount(acctNum)
		display := displayAccount(masked)
		sortKey := math.MaxInt
		if e, ok := accountEntries[masked]; ok {
			if e.Name != "" {
				display = displayAccount(masked) + " " + e.Name
			}
			if e.Order > 0 {
				sortKey = e.Order
			}
		}

		dayChangeDollar := cur.LiquidationValue - ini.LiquidationValue
		var dayChangePct float64
		if ini.LiquidationValue != 0 {
			dayChangePct = (dayChangeDollar / ini.LiquidationValue) * 100
		}

		totalCurLiq += cur.LiquidationValue
		totalIniLiq += ini.LiquidationValue

		rows = append(rows, accountRow{
			account:   display,
			liqVal:    formatDollar(cur.LiquidationValue),
			dayChange: formatChangeDollar(dayChangeDollar),
			dayPct:    formatChangePct(dayChangePct),
			sortKey:   sortKey,
			positive:  dayChangeDollar > 0,
			zero:      dayChangeDollar == 0,
		})

		// Accumulate positions into aggMap.
		for _, p := range a.SecuritiesAccount.Positions {
			sym := p.Instrument.Symbol
			if _, exists := aggMap[sym]; !exists {
				aggMap[sym] = &aggPosition{
					symbol:      sym,
					description: p.Instrument.Description,
				}
				aggSymbols = append(aggSymbols, sym)
			}
			aggMap[sym].quantity += p.Quantity
			aggMap[sym].marketValue += p.MarketValue
			aggMap[sym].dayPL += p.CurrentDayPL
		}
	}

	// Sort account rows by configured order.
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].sortKey < rows[j].sortKey
	})

	// Sort aggregated symbols alphabetically.
	sort.Strings(aggSymbols)

	// Build total row.
	totalChangeDollar := totalCurLiq - totalIniLiq
	var totalChangePct float64
	if totalIniLiq != 0 {
		totalChangePct = (totalChangeDollar / totalIniLiq) * 100
	}

	// ── Compute account section column widths ──────────────────────────────
	w0 := len(hAccount)
	w1 := len(hLiqVal)
	w2 := len(hDayChange)
	w3 := len(hDayPct)

	totalLiqStr := formatDollar(totalCurLiq)
	totalChgStr := formatChangeDollar(totalChangeDollar)
	totalPctStr := formatChangePct(totalChangePct)

	allAccountRows := append(rows, accountRow{
		account:   "TOTAL",
		liqVal:    totalLiqStr,
		dayChange: totalChgStr,
		dayPct:    totalPctStr,
	})
	for _, r := range allAccountRows {
		if l := len(r.account); l > w0 {
			w0 = l
		}
		if l := len(r.liqVal); l > w1 {
			w1 = l
		}
		if l := len(r.dayChange); l > w2 {
			w2 = l
		}
		if l := len(r.dayPct); l > w3 {
			w3 = l
		}
	}

	sepLen := w0 + 4 + w1 + 4 + w2 + 4 + w3

	// ── Compute aggregated position section column widths ──────────────────
	pw0 := len(hSymbol)
	pw1 := len(hDesc)
	pw2 := len(hQty)
	pw3 := len(hMktVal)
	pw4 := len(hPosDayChg)
	pw5 := len(hPosDayPct)

	for _, sym := range aggSymbols {
		ap := aggMap[sym]
		// dayPct for aggregate: dayPL / (marketValue - dayPL) * 100
		// (marketValue - dayPL) approximates the opening market value.
		var apDayPct float64
		openVal := ap.marketValue - ap.dayPL
		if openVal != 0 {
			apDayPct = (ap.dayPL / openVal) * 100
		}
		if l := len(ap.symbol); l > pw0 {
			pw0 = l
		}
		if l := len(ap.description); l > pw1 {
			pw1 = l
		}
		if l := len(formatQty(ap.quantity)); l > pw2 {
			pw2 = l
		}
		if l := len(formatDollar(ap.marketValue)); l > pw3 {
			pw3 = l
		}
		if l := len(formatChangeDollar(ap.dayPL)); l > pw4 {
			pw4 = l
		}
		if l := len(formatChangePct(apDayPct)); l > pw5 {
			pw5 = l
		}
	}

	// ── Helpers ────────────────────────────────────────────────────────────

	sep := func() {
		fmt.Println(strings.Repeat("-", sepLen))
	}

	printAccountRow := func(account, liqVal, dayChange, dayPct string, positive, zero, bold bool) {
		colorStart, colorEnd := "", ""
		if isTTY && !zero {
			if positive {
				colorStart = ansiGreen
			} else {
				colorStart = ansiRed
			}
			colorEnd = ansiReset
		}
		boldStart, boldEnd := "", ""
		if isTTY && bold {
			boldStart = ansiBold
			boldEnd = ansiReset
		}
		fmt.Printf("%s%-*s    %*s    %s%*s    %*s%s%s\n",
			boldStart,
			w0, account,
			w1, liqVal,
			colorStart,
			w2, dayChange,
			w3, dayPct,
			colorEnd,
			boldEnd,
		)
	}

	printAggPositionRow := func(ap *aggPosition) {
		var dayPct float64
		openVal := ap.marketValue - ap.dayPL
		if openVal != 0 {
			dayPct = (ap.dayPL / openVal) * 100
		}
		colorStart, colorEnd := "", ""
		if isTTY && ap.dayPL != 0 {
			if ap.dayPL > 0 {
				colorStart = ansiGreen
			} else {
				colorStart = ansiRed
			}
			colorEnd = ansiReset
		}
		dimStart, dimEnd := "", ""
		if isTTY {
			dimStart = ansiDim
			dimEnd = ansiReset
		}
		fmt.Printf("%s%-*s  %-*s  %*s  %*s  %s%*s  %*s%s%s\n",
			dimStart,
			pw0, ap.symbol,
			pw1, ap.description,
			pw2, formatQty(ap.quantity),
			pw3, formatDollar(ap.marketValue),
			colorStart,
			pw4, formatChangeDollar(ap.dayPL),
			pw5, formatChangePct(dayPct),
			colorEnd,
			dimEnd,
		)
	}

	printPositionHeader := func() {
		fmt.Printf("%s%-*s  %-*s  %*s  %*s  %*s  %*s%s\n",
			ansiDim,
			pw0, hSymbol,
			pw1, hDesc,
			pw2, hQty,
			pw3, hMktVal,
			pw4, hPosDayChg,
			pw5, hPosDayPct,
			ansiReset,
		)
	}

	// ── Render ─────────────────────────────────────────────────────────────

	// Index quotes line.
	if len(indices) > 0 {
		parts := make([]string, 0, len(indices))
		for _, q := range indices {
			chgColor, chgReset := "", ""
			if isTTY && q.NetChange != 0 {
				if q.NetChange > 0 {
					chgColor = ansiGreen
				} else {
					chgColor = ansiRed
				}
				chgReset = ansiReset
			}
			parts = append(parts, fmt.Sprintf("%s: %s  %s%s  %s%s",
				q.Label,
				formatDollar(q.Last),
				chgColor,
				formatChangeDollar(q.NetChange),
				formatChangePct(q.NetChangePct),
				chgReset,
			))
		}
		fmt.Println(strings.Join(parts, "    ") + "\033[K")
		fmt.Println()
	}

	// Account section header.
	fmt.Printf("%-*s    %*s    %*s    %*s\n",
		w0, hAccount,
		w1, hLiqVal,
		w2, hDayChange,
		w3, hDayPct,
	)
	sep()

	for _, r := range rows {
		printAccountRow(r.account, r.liqVal, r.dayChange, r.dayPct, r.positive, r.zero, false)
	}

	sep()
	printAccountRow("TOTAL", totalLiqStr, totalChgStr, totalPctStr,
		totalChangeDollar > 0, totalChangeDollar == 0, true)

	// ── Aggregated positions section ────────────────────────────────────────
	if len(aggSymbols) > 0 {
		fmt.Println()
		printPositionHeader()
		for _, sym := range aggSymbols {
			printAggPositionRow(aggMap[sym])
		}
	}
}

// maskAccount returns the last 3 digits of an account number (no prefix),
// used as the config key. e.g. "12345678" → "678"
func maskAccount(acct string) string {
	if len(acct) <= 3 {
		return acct
	}
	return acct[len(acct)-3:]
}

// displayAccount returns the masked account number with *** prefix for display.
// e.g. "678" → "***678"
func displayAccount(masked string) string {
	return "***" + masked
}

// formatDollar formats a float as $1,234.56 using the locale-aware printer.
func formatDollar(v float64) string {
	return printer.Sprintf("$%.2f", v)
}

// formatChangeDollar formats a signed dollar change as +$1,234.56 or -$1,234.56.
func formatChangeDollar(v float64) string {
	if v > 0 {
		return printer.Sprintf("+$%.2f", v)
	}
	if v < 0 {
		return printer.Sprintf("-$%.2f", -v)
	}
	return "$0.00"
}

// formatChangePct formats a signed percentage change as +1.23% or -1.23%.
func formatChangePct(v float64) string {
	if v > 0 {
		return printer.Sprintf("+%.2f%%", v)
	}
	if v < 0 {
		return printer.Sprintf("-%.2f%%", -v)
	}
	return "0.00%"
}

// formatQty formats a position quantity to 3 decimal places.
func formatQty(v float64) string {
	return printer.Sprintf("%.3f", v)
}

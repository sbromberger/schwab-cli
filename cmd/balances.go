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
	// descCache persists across interval ticks; each symbol is looked up at most once.
	descCache := make(map[string]string)
	// mktHours caches today's equity market hours; re-fetched when the date changes.
	var mktHours api.MarketHours

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
			mktHours = refreshMarketHours(cfg, configDir, mktHours)
			fetchAndPrint(cfg, configDir, jsonOut, forceColor, descCache, mktHours)
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
	mktHours = refreshMarketHours(cfg, configDir, mktHours)
	fetchAndPrint(cfg, configDir, jsonOut, forceColor, descCache, mktHours)
}

// refreshMarketHours returns a fresh MarketHours if the cached value is for a
// different calendar date (or was never fetched). On error it returns the
// previous cached value so the display degrades gracefully.
func refreshMarketHours(cfg *config.Config, configDir string, cached api.MarketHours) api.MarketHours {
	today := time.Now().Format("2006-01-02")
	if cached.Date == today {
		return cached
	}
	token, err := auth.LoadToken(configDir)
	if err != nil {
		return cached
	}
	token, err = auth.EnsureFresh(cfg, token, configDir)
	if err != nil {
		return cached
	}
	h, err := api.GetEquityMarketHours(token.AccessToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch market hours: %v\n", err)
		return cached
	}
	return h
}

// fetchAndPrint performs one fetch+display cycle.
func fetchAndPrint(cfg *config.Config, configDir string, jsonOut bool, forceColor bool, descCache map[string]string, mktHours api.MarketHours) {
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
		fmt.Fprintf(os.Stderr, "warning: could not fetch index quotes: %v\n", err)
	}

	// Collect unique position symbols for the quotes call.
	symbolSet := make(map[string]struct{})
	for _, a := range accounts {
		for _, p := range a.SecuritiesAccount.Positions {
			symbolSet[p.Instrument.Symbol] = struct{}{}
		}
	}
	posSymbols := make([]string, 0, len(symbolSet))
	for sym := range symbolSet {
		posSymbols = append(posSymbols, sym)
	}
	symbolQuotes, err := api.GetSymbolQuotes(token.AccessToken, posSymbols)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not fetch position quotes: %v\n", err)
		symbolQuotes = nil
	}

	// For symbols with no description from the quotes call, look them up via
	// the instruments endpoint. Results are cached so each symbol is queried once.
	var needDesc []string
	for sym, q := range symbolQuotes {
		if q.Description == "" {
			if _, cached := descCache[sym]; !cached {
				needDesc = append(needDesc, sym)
			}
		}
	}
	if len(needDesc) > 0 {
		descs, err := api.GetInstrumentDescriptions(token.AccessToken, needDesc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not fetch instrument descriptions: %v\n", err)
		}
		for sym, desc := range descs {
			descCache[sym] = desc
		}
		// Mark looked-up symbols even when no description was found, so we
		// don't re-query them on the next tick.
		for _, sym := range needDesc {
			if _, ok := descCache[sym]; !ok {
				descCache[sym] = ""
			}
		}
	}
	// Merge cached descriptions into symbolQuotes.
	for sym, desc := range descCache {
		if q, ok := symbolQuotes[sym]; ok && q.Description == "" && desc != "" {
			q.Description = desc
			symbolQuotes[sym] = q
		}
	}

	printTable(accounts, cfg.Accounts, indices, symbolQuotes, forceColor, mktHours)
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
	symbol         string
	description    string
	marketValue    float64
	currentPrice   float64 // from quotes API
	priceChange    float64 // per-share daily $ change from quotes API
	priceChangePct float64 // per-share daily % change from quotes API
	stale          bool    // true if the quote is from a previous calendar day
}

// isToday returns true if the Unix millisecond timestamp falls on today's local date.
func isToday(unixMs int64) bool {
	if unixMs == 0 {
		return false
	}
	t := time.UnixMilli(unixMs)
	now := time.Now()
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// printTable renders the account balance table followed by an aggregated positions section.
func printTable(accounts []api.Account, accountEntries map[string]config.AccountEntry, indices []api.IndexQuote, symbolQuotes map[string]api.SymbolQuote, forceColor bool, mktHours api.MarketHours) {
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
	hDayChange := " ∆day$"
	hDayPct := " ∆day%"

	// Column headers for the positions section.
	hSymbol  := "SYMBOL"
	hName    := "NAME"
	hPrice   := "PRICE"
	hChgD    := hDayChange
	hChgP    := hDayPct
	hMktVal  := "MKT VALUE"

	rows := make([]accountRow, 0, len(accounts))
	var totalCurLiq, totalIniLiq, totalChangeDollar float64

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

		var dayChangeDollar float64
		if ini.LiquidationValue != 0 {
			dayChangeDollar = cur.LiquidationValue - ini.LiquidationValue
		}
		var dayChangePct float64
		if ini.LiquidationValue != 0 {
			dayChangePct = (dayChangeDollar / ini.LiquidationValue) * 100
		}

		totalCurLiq += cur.LiquidationValue
		if ini.LiquidationValue != 0 {
			totalIniLiq += ini.LiquidationValue
			totalChangeDollar += dayChangeDollar
		}

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
			aggMap[sym].marketValue += p.MarketValue
		}
	}

	// Populate quote data for each aggregated position.
	for sym, ap := range aggMap {
		if q, ok := symbolQuotes[sym]; ok {
			ap.currentPrice   = q.LastPrice
			ap.priceChange    = q.NetChange
			ap.priceChangePct = q.NetChangePct
			ap.stale          = !isToday(q.QuoteTime) && mktHours.HasOpenedToday()
			if ap.description == "" {
				ap.description = q.Description
			}
		}
	}

	// Sort account rows by configured order.
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].sortKey < rows[j].sortKey
	})

	// Sort aggregated symbols alphabetically.
	sort.Strings(aggSymbols)

	// Build total row.
	var totalChangePct float64
	if totalIniLiq != 0 {
		totalChangePct = (totalChangeDollar / totalIniLiq) * 100
	}

	// ── Compute account section column widths ──────────────────────────────
	w0 := len(hAccount)
	w1 := len(hLiqVal)
	w2 := visLen(hDayChange)
	w3 := visLen(hDayPct)

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

	sepLen    := w0 + 4 + w1 + 4 + w2 + 4 + w3

	// ── Compute aggregated position section column widths ──────────────────
	pw0 := len(hSymbol)
	pw1 := len(hName)
	pw2 := len(hPrice)
	pw3 := len(hChgD)
	pw4 := len(hChgP)
	pw5 := len(hMktVal)

	for _, sym := range aggSymbols {
		ap := aggMap[sym]
		if l := len(ap.symbol); l > pw0 {
			pw0 = l
		}
		if l := len(ap.description); l > pw1 {
			pw1 = l
		}
		if l := len(formatDollar(ap.currentPrice)); l > pw2 {
			pw2 = l
		}
		if l := len(formatChangeDollar(ap.priceChange)); l > pw3 {
			pw3 = l
		}
		if l := len(formatChangePct(ap.priceChangePct)); l > pw4 {
			pw4 = l
		}
		if l := len(formatDollar(ap.marketValue)); l > pw5 {
			pw5 = l
		}
	}
	for _, q := range indices {
		if l := len(q.Label); l > pw0 {
			pw0 = l
		}
		if l := len(q.Description); l > pw1 {
			pw1 = l
		}
		if l := len(formatDollar(q.Last)); l > pw2 {
			pw2 = l
		}
		if l := len(formatChangeDollar(q.NetChange)); l > pw3 {
			pw3 = l
		}
		if l := len(formatChangePct(q.NetChangePct)); l > pw4 {
			pw4 = l
		}
	}

	posSepLen := pw0 + 2 + pw1 + 2 + pw2 + 2 + pw3 + 2 + pw4 + 2 + pw5

	// ── Helpers ────────────────────────────────────────────────────────────

	sep := func() {
		fmt.Println(strings.Repeat("─", sepLen))
	}

	posSep := func() {
		fmt.Println(strings.Repeat("─", posSepLen))
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
		dimStart, dimEnd := "", ""
		if isTTY && ap.stale {
			dimStart = ansiDim
			dimEnd = ansiReset
		}
		colorStart, colorEnd := "", ""
		if isTTY && ap.priceChange != 0 {
			if ap.priceChange > 0 {
				colorStart = ansiGreen
			} else {
				colorStart = ansiRed
			}
			if ap.stale {
				colorEnd = ansiReset + ansiDim
			} else {
				colorEnd = ansiReset
			}
		}
		fmt.Printf("%s%-*s  %-*s  %*s  %s%*s  %*s%s  %*s%s\n",
			dimStart,
			pw0, ap.symbol,
			pw1, ap.description,
			pw2, formatDollar(ap.currentPrice),
			colorStart,
			pw3, formatChangeDollar(ap.priceChange),
			pw4, formatChangePct(ap.priceChangePct),
			colorEnd,
			pw5, formatDollar(ap.marketValue),
			dimEnd,
		)
	}

	regularOpen := mktHours.IsRegularMarketOpen()
	printIndexRow := func(q api.IndexQuote) {
		dimStart, dimEnd := "", ""
		if isTTY && !regularOpen {
			dimStart = ansiDim
			dimEnd = ansiReset
		}
		colorStart, colorEnd := "", ""
		if isTTY && q.NetChange != 0 {
			if q.NetChange > 0 {
				colorStart = ansiGreen
			} else {
				colorStart = ansiRed
			}
			// If dimming, reset+re-apply dim after the colored section so
			// the remaining fields (mkt value) stay dim rather than plain.
			if !regularOpen {
				colorEnd = ansiReset + ansiDim
			} else {
				colorEnd = ansiReset
			}
		}
		fmt.Printf("%s%-*s  %-*s  %*s  %s%*s  %*s%s  %*s%s\n",
			dimStart,
			pw0, q.Label,
			pw1, q.Description,
			pw2, formatDollar(q.Last),
			colorStart,
			pw3, formatChangeDollar(q.NetChange),
			pw4, formatChangePct(q.NetChangePct),
			colorEnd,
			pw5, "",
			dimEnd,
		)
	}

	printPositionHeader := func() {
		fmt.Printf("%-*s  %-*s  %*s  %*s  %*s  %*s\n",
			pw0, hSymbol,
			pw1, hName,
			pw2, hPrice,
			pw3, hChgD,
			pw4, hChgP,
			pw5, hMktVal,
		)
	}

	// ── Render ─────────────────────────────────────────────────────────────

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

	// ── Aggregated positions section (indices at top, then holdings) ────────
	if len(indices) > 0 || len(aggSymbols) > 0 {
		fmt.Println()
		printPositionHeader()
		posSep()
		for _, q := range indices {
			printIndexRow(q)
		}
		if len(indices) > 0 && len(aggSymbols) > 0 {
			posSep()
		}
		for _, sym := range aggSymbols {
			printAggPositionRow(aggMap[sym])
		}
		posSep()
	}
}

// visLen returns the number of terminal columns s occupies.
// ∆ (U+2206) is 3 UTF-8 bytes but renders as 1 column, so we subtract 2 per occurrence.
func visLen(s string) int {
	return len(s) - 2*strings.Count(s, "∆")
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

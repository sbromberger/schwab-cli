package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

const marketBase = "https://api.schwabapi.com/marketdata/v1"

// IndexQuote holds the display fields for a market index.
type IndexQuote struct {
	Symbol       string
	Label        string
	Description  string
	Last         float64
	NetChange    float64
	NetChangePct float64
}

// quoteResponse is the per-symbol object returned by the quotes endpoint.
type quoteResponse struct {
	Description string `json:"description"`
	Quote       struct {
		LastPrice        float64 `json:"lastPrice"`
		NetChange        float64 `json:"netChange"`
		NetPercentChange float64 `json:"netPercentChange"`
		QuoteTime        int64   `json:"quoteTime"`  // Unix milliseconds; equities
		TradeTime        int64   `json:"tradeTime"`  // Unix milliseconds; mutual funds
	} `json:"quote"`
	// Regular is populated for equities/ETFs and contains the official
	// regular-market close price and change, unaffected by after-hours trading.
	Regular struct {
		LastPrice  float64 `json:"regularMarketLastPrice"`
		NetChange  float64 `json:"regularMarketNetChange"`
		NetPctChange float64 `json:"regularMarketPercentChange"`
	} `json:"regular"`
}

// indexDefs defines the indices we display, in order.
var indexDefs = []struct{ symbol, label, description string }{
	{"$DJI", "DJIA", "Dow Jones Industrial Average"},
	{"$COMPX", "NASDAQ", "NASDAQ Composite Index"},
	{"$SPX", "S&P 500", "S&P 500 Index"},
}

// SymbolQuote holds the real-time quote fields for a single equity/fund symbol.
type SymbolQuote struct {
	LastPrice    float64
	NetChange    float64
	NetChangePct float64
	QuoteTime    int64  // Unix milliseconds; 0 if not provided
	Description  string
}

// GetSymbolQuotes fetches real-time quotes for the given symbols.
// Returns a map keyed by symbol; symbols absent from the response are not in the map.
func GetSymbolQuotes(accessToken string, symbols []string) (map[string]SymbolQuote, error) {
	if len(symbols) == 0 {
		return nil, nil
	}
	url := marketBase + "/quotes?symbols=" + strings.Join(symbols, ",") + ""

	body, err := doGet(accessToken, url)
	if err != nil {
		return nil, fmt.Errorf("fetching symbol quotes: %w", err)
	}

	var raw map[string]quoteResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing symbol quotes: %w", err)
	}

	result := make(map[string]SymbolQuote, len(raw))
	for sym, q := range raw {
		qt := q.Quote.QuoteTime
		if qt == 0 {
			qt = q.Quote.TradeTime
		}
		// Prefer regular-market fields (official close) over quote fields,
		// which may reflect after-hours activity for equities/ETFs.
		// Mutual funds and indices have no regular block so they fall back
		// to the quote fields.
		lastPrice := q.Regular.LastPrice
		netChange := q.Regular.NetChange
		netChangePct := q.Regular.NetPctChange
		if lastPrice == 0 {
			lastPrice = q.Quote.LastPrice
			netChange = q.Quote.NetChange
			netChangePct = q.Quote.NetPercentChange
		}
		result[sym] = SymbolQuote{
			LastPrice:    lastPrice,
			NetChange:    netChange,
			NetChangePct: netChangePct,
			QuoteTime:    qt,
			Description:  q.Description,
		}
	}
	return result, nil
}

// GetIndexQuotes fetches the DJIA, NASDAQ, and S&P 500 quotes.
func GetIndexQuotes(accessToken string) ([]IndexQuote, error) {
	symbols := make([]string, len(indexDefs))
	for i, d := range indexDefs {
		symbols[i] = d.symbol
	}
	url := marketBase + "/quotes?symbols=" + strings.Join(symbols, ",") + ""

	body, err := doGet(accessToken, url)
	if err != nil {
		return nil, fmt.Errorf("fetching index quotes: %w", err)
	}

	var raw map[string]quoteResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing index quotes: %w", err)
	}

	quotes := make([]IndexQuote, 0, len(indexDefs))
	for _, d := range indexDefs {
		q, ok := raw[d.symbol]
		if !ok {
			continue
		}
		desc := d.description
		if q.Description != "" {
			desc = q.Description
		}
		quotes = append(quotes, IndexQuote{
			Symbol:       d.symbol,
			Label:        d.label,
			Description:  desc,
			Last:         q.Quote.LastPrice,
			NetChange:    q.Quote.NetChange,
			NetChangePct: q.Quote.NetPercentChange,
		})
	}
	return quotes, nil
}

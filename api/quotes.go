package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

const marketBase = "https://api.schwabapi.com/marketdata/v1"

// IndexQuote holds the display fields for a market index.
type IndexQuote struct {
	Symbol     string
	Label      string
	Last       float64
	NetChange  float64
	NetChangePct float64
}

// quoteResponse is the per-symbol object returned by the quotes endpoint.
type quoteResponse struct {
	Quote struct {
		LastPrice        float64 `json:"lastPrice"`
		NetChange        float64 `json:"netChange"`
		NetPercentChange float64 `json:"netPercentChange"`
	} `json:"quote"`
}

// indexDefs defines the indices we display, in order.
var indexDefs = []struct{ symbol, label string }{
	{"$DJI", "DJIA"},
	{"$COMPX", "NASDAQ"},
	{"$SPX", "S&P 500"},
}

// GetIndexQuotes fetches the DJIA, NASDAQ, and S&P 500 quotes.
func GetIndexQuotes(accessToken string) ([]IndexQuote, error) {
	symbols := make([]string, len(indexDefs))
	for i, d := range indexDefs {
		symbols[i] = d.symbol
	}
	url := marketBase + "/quotes?symbols=" + strings.Join(symbols, ",") + "&fields=quote"

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
		quotes = append(quotes, IndexQuote{
			Symbol:       d.symbol,
			Label:        d.label,
			Last:         q.Quote.LastPrice,
			NetChange:    q.Quote.NetChange,
			NetChangePct: q.Quote.NetPercentChange,
		})
	}
	return quotes, nil
}

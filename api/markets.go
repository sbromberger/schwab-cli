package api

import (
	"encoding/json"
	"fmt"
	"time"
)

// MarketSession is a single trading session window.
type MarketSession struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// MarketHours holds the hours and open status for a single market product.
type MarketHours struct {
	Date         string `json:"date"`
	IsOpen       bool   `json:"isOpen"`
	SessionHours struct {
		RegularMarket []MarketSession `json:"regularMarket"`
	} `json:"sessionHours"`
}

// IsRegularMarketOpen returns true if time.Now() falls within any regular
// market session window, regardless of the IsOpen flag.
func (h MarketHours) IsRegularMarketOpen() bool {
	now := time.Now()
	for _, s := range h.SessionHours.RegularMarket {
		if now.After(s.Start) && now.Before(s.End) {
			return true
		}
	}
	return false
}

// HasOpenedToday returns true if the regular market session has already
// started today (i.e. time.Now() is past the session open time).
func (h MarketHours) HasOpenedToday() bool {
	if len(h.SessionHours.RegularMarket) == 0 {
		return false
	}
	return time.Now().After(h.SessionHours.RegularMarket[0].Start)
}

// marketsResponse is the shape of the /markets/equity endpoint.
type marketsResponse struct {
	Equity map[string]MarketHours `json:"equity"`
}

// GetEquityMarketHours fetches today's equity market hours from the
// /marketdata/v1/markets/equity endpoint and returns the EQ product entry.
func GetEquityMarketHours(accessToken string) (MarketHours, error) {
	url := marketBase + "/markets/equity"

	body, err := doGet(accessToken, url)
	if err != nil {
		return MarketHours{}, fmt.Errorf("fetching market hours: %w", err)
	}

	var resp marketsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return MarketHours{}, fmt.Errorf("parsing market hours: %w", err)
	}

	h, ok := resp.Equity["EQ"]
	if !ok {
		return MarketHours{}, fmt.Errorf("EQ market hours not found in response")
	}
	return h, nil
}

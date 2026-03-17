package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// instrumentsResponse is the shape of the instruments endpoint response.
type instrumentsResponse struct {
	Instruments []struct {
		Symbol      string `json:"symbol"`
		Description string `json:"description"`
	} `json:"instruments"`
}

// GetInstrumentDescriptions fetches the description for each symbol using the
// instruments search endpoint. Returns a map keyed by symbol; symbols not
// found or without a description are absent from the map.
func GetInstrumentDescriptions(accessToken string, symbols []string) (map[string]string, error) {
	if len(symbols) == 0 {
		return nil, nil
	}
	url := marketBase + "/instruments?symbol=" + strings.Join(symbols, ",") + "&projection=symbol-search"

	body, err := doGet(accessToken, url)
	if err != nil {
		return nil, fmt.Errorf("fetching instrument descriptions: %w", err)
	}

	var resp instrumentsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing instrument descriptions: %w", err)
	}

	// Build a set of requested symbols for exact-match filtering.
	requested := make(map[string]struct{}, len(symbols))
	for _, s := range symbols {
		requested[s] = struct{}{}
	}

	result := make(map[string]string)
	for _, inst := range resp.Instruments {
		if _, ok := requested[inst.Symbol]; ok && inst.Description != "" {
			result[inst.Symbol] = inst.Description
		}
	}
	return result, nil
}

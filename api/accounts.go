package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const traderBase = "https://api.schwabapi.com/trader/v1"

// AccountNumber maps a masked account number to its API hash value.
type AccountNumber struct {
	AccountNumber string `json:"accountNumber"`
	HashValue     string `json:"hashValue"`
}

// Balances holds the balance figures we care about for display.
type Balances struct {
	LiquidationValue float64 `json:"liquidationValue"`
	CashBalance      float64 `json:"cashBalance"`
	LongMarketValue  float64 `json:"longMarketValue"`
	ShortMarketValue float64 `json:"shortMarketValue"`
	// Margin accounts use BuyingPower; cash accounts use BuyingPowerNonMarginableTrade.
	BuyingPower                    float64 `json:"buyingPower"`
	BuyingPowerNonMarginableTrade  float64 `json:"buyingPowerNonMarginableTrade"`
}

// Instrument describes the security held in a position.
type Instrument struct {
	Symbol      string `json:"symbol"`
	Description string `json:"description"`
	AssetType   string `json:"assetType"`
}

// Position is a single holding within an account.
type Position struct {
	Instrument        Instrument `json:"instrument"`
	Quantity          float64    `json:"longQuantity"`
	MarketValue       float64    `json:"marketValue"`
	CurrentDayPLOpen  float64    `json:"currentDayProfitLossPercentage"`
	CurrentDayPL      float64    `json:"currentDayProfitLoss"`
}

// SecuritiesAccount is the inner account object returned by the accounts endpoint.
type SecuritiesAccount struct {
	Type            string     `json:"type"`
	AccountNumber   string     `json:"accountNumber"`
	CurrentBalances Balances   `json:"currentBalances"`
	InitialBalances Balances   `json:"initialBalances"`
	Positions       []Position `json:"positions"`
}

// Account is a single element of the GET /accounts response array.
type Account struct {
	SecuritiesAccount SecuritiesAccount `json:"securitiesAccount"`
}

// GetAccountNumbers fetches the list of account numbers and their hash values.
func GetAccountNumbers(accessToken string) ([]AccountNumber, error) {
	body, err := doGet(accessToken, traderBase+"/accounts/accountNumbers")
	if err != nil {
		return nil, err
	}
	var nums []AccountNumber
	if err := json.Unmarshal(body, &nums); err != nil {
		return nil, fmt.Errorf("parsing accountNumbers response: %w", err)
	}
	return nums, nil
}

// GetAccounts fetches all accounts with balance information.
// The raw JSON body is also returned so the --json flag can print it verbatim.
func GetAccounts(accessToken string) ([]Account, []byte, error) {
	body, err := doGet(accessToken, traderBase+"/accounts?fields=positions")
	if err != nil {
		return nil, nil, err
	}
	var accounts []Account
	if err := json.Unmarshal(body, &accounts); err != nil {
		return nil, nil, fmt.Errorf("parsing accounts response: %w", err)
	}
	return accounts, body, nil
}

// doGet performs an authenticated GET request and returns the response body.
func doGet(accessToken, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d: %s", url, resp.StatusCode, string(body))
	}

	return body, nil
}

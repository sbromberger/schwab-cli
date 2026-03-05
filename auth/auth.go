package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sbromberger/schwab-cli/config"
)

const (
	authBaseURL  = "https://api.schwabapi.com/v1/oauth/authorize"
	tokenURL     = "https://api.schwabapi.com/v1/oauth/token"
	redirectURI  = "https://127.0.0.1"
)

// AuthURL returns the URL the user should open in their browser to begin OAuth.
func AuthURL(cfg *config.Config) string {
	return fmt.Sprintf("%s?client_id=%s&redirect_uri=%s",
		authBaseURL,
		url.QueryEscape(cfg.AppKey),
		url.QueryEscape(redirectURI),
	)
}

// ExchangeCode exchanges an authorization code for tokens.
// code is the value of the `code` query parameter from the redirect URL.
func ExchangeCode(cfg *config.Config, code string) (*Token, error) {
	vals := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {code},
		"redirect_uri": {redirectURI},
	}
	return postToken(cfg, vals)
}

// Refresh uses the refresh token to obtain a new access token.
// Returns a descriptive error if the refresh token has expired.
func Refresh(cfg *config.Config, t *Token) (*Token, error) {
	vals := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {t.RefreshToken},
	}
	newToken, err := postToken(cfg, vals)
	if err != nil {
		// Provide a helpful message for the common 7-day expiry case.
		return nil, fmt.Errorf("%w\n\nRefresh token may have expired (Schwab enforces a 7-day limit).\nRun `schwab-cli login` to re-authenticate.", err)
	}
	return newToken, nil
}

// EnsureFresh returns a valid token, refreshing it if necessary.
// If the token was refreshed, it is saved to disk automatically.
func EnsureFresh(cfg *config.Config, t *Token, configDir string) (*Token, error) {
	if !t.IsExpired() {
		return t, nil
	}
	refreshed, err := Refresh(cfg, t)
	if err != nil {
		return nil, err
	}
	if err := SaveToken(configDir, refreshed); err != nil {
		return nil, fmt.Errorf("token refreshed but could not be saved: %w", err)
	}
	return refreshed, nil
}

// ParseCodeFromRedirect extracts the `code` query parameter from a redirect URL
// pasted by the user.
func ParseCodeFromRedirect(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	code := u.Query().Get("code")
	if code == "" {
		return "", errors.New("no `code` parameter found in the redirect URL")
	}
	return code, nil
}

// tokenResponse is the shape of Schwab's token endpoint response.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	// Error fields returned on failure.
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// postToken sends a POST to the token endpoint and returns a Token.
func postToken(cfg *config.Config, vals url.Values) (*Token, error) {
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", basicAuth(cfg.AppKey, cfg.Secret))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("parsing token response (status %d): %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := tr.ErrorDescription
		if msg == "" {
			msg = tr.Error
		}
		if msg == "" {
			msg = string(body)
		}
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, msg)
	}

	expiresIn := tr.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 1800 // Schwab default: 30 minutes
	}

	return &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}

// basicAuth returns the HTTP Basic Authorization header value for the given
// client ID and secret.
func basicAuth(clientID, secret string) string {
	creds := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + secret))
	return "Basic " + creds
}

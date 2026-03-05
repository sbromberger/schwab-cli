package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/sbromberger/schwab-cli/auth"
	"github.com/sbromberger/schwab-cli/config"
)

// Login runs the manual OAuth flow:
//  1. Print the authorization URL.
//  2. Read the redirect URL pasted by the user.
//  3. Exchange the code for tokens and save them.
func Login(cfg *config.Config, configDir string) {
	authURL := auth.AuthURL(cfg)

	fmt.Println("Open this URL in your browser and log in:")
	fmt.Println()
	fmt.Println(" ", authURL)
	fmt.Println()
	fmt.Println("After logging in, Schwab will redirect your browser to a URL starting with")
	fmt.Println("https://127.0.0.1 (it may show a connection error — that's expected).")
	fmt.Println()
	fmt.Print("Paste the full redirect URL here: ")

	reader := bufio.NewReader(os.Stdin)
	redirectURL, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading input: %v\n", err)
		os.Exit(1)
	}
	redirectURL = strings.TrimSpace(redirectURL)

	code, err := auth.ParseCodeFromRedirect(redirectURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Exchanging authorization code for tokens...")
	token, err := auth.ExchangeCode(cfg, code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := auth.SaveToken(configDir, token); err != nil {
		fmt.Fprintf(os.Stderr, "error saving token: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Logged in successfully. Token saved to %s/token.json\n", configDir)
	fmt.Printf("Access token expires at: %s\n", token.ExpiresAt.Format("2006-01-02 15:04:05 MST"))
	fmt.Println("Refresh token is valid for 7 days — run any command at least once a week to keep it active.")
}

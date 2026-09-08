// Command session-key performs the one-time Last.fm desktop-auth handshake and prints
// a session key for a single user. Run it once per Navidrome user, then paste the
// resulting key into the plugin's "User Session Keys" config.
//
// Usage:
//
//	go run . -apikey <API_KEY> -apisecret <SHARED_SECRET>
//
// This is an ordinary host program (not WASM) and uses only the standard library.
package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
)

const apiURL = "https://ws.audioscrobbler.com/2.0/"

func main() {
	apiKey := flag.String("apikey", "", "Last.fm API key")
	apiSecret := flag.String("apisecret", "", "Last.fm shared secret")
	flag.Parse()

	if *apiKey == "" || *apiSecret == "" {
		fmt.Fprintln(os.Stderr, "both -apikey and -apisecret are required")
		fmt.Fprintln(os.Stderr, "  go run . -apikey <API_KEY> -apisecret <SHARED_SECRET>")
		os.Exit(2)
	}

	if err := run(*apiKey, *apiSecret); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(apiKey, apiSecret string) error {
	// Step 1: request an unauthorized token.
	token, err := getToken(apiKey, apiSecret)
	if err != nil {
		return fmt.Errorf("auth.getToken: %w", err)
	}

	// Step 2: have the user authorize the token in their browser.
	authURL := fmt.Sprintf("https://www.last.fm/api/auth/?api_key=%s&token=%s",
		url.QueryEscape(apiKey), url.QueryEscape(token))
	fmt.Println("\n1) Open this URL in your browser and click \"Yes, allow access\":\n")
	fmt.Println("   " + authURL)
	fmt.Print("\n2) After authorizing, press Enter here to continue... ")
	bufio.NewReader(os.Stdin).ReadString('\n')

	// Step 3: exchange the authorized token for a session key.
	name, key, err := getSession(apiKey, apiSecret, token)
	if err != nil {
		return fmt.Errorf("auth.getSession: %w (did you authorize the token?)", err)
	}

	fmt.Printf("\nSuccess! Last.fm account: %s\n", name)
	fmt.Printf("Session key: %s\n", key)
	fmt.Println("\nPaste this session key into the plugin config for this user.")
	return nil
}

func getToken(apiKey, apiSecret string) (string, error) {
	params := map[string]string{"method": "auth.getToken", "api_key": apiKey}
	params["api_sig"] = sign(params, apiSecret)
	params["format"] = "json"

	var out struct {
		Token string `json:"token"`
		Error int    `json:"error"`
		Msg   string `json:"message"`
	}
	if err := apiGet(params, &out); err != nil {
		return "", err
	}
	if out.Error != 0 {
		return "", fmt.Errorf("last.fm error %d: %s", out.Error, out.Msg)
	}
	return out.Token, nil
}

func getSession(apiKey, apiSecret, token string) (name, key string, err error) {
	params := map[string]string{"method": "auth.getSession", "api_key": apiKey, "token": token}
	params["api_sig"] = sign(params, apiSecret)
	params["format"] = "json"

	var out struct {
		Session struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"session"`
		Error int    `json:"error"`
		Msg   string `json:"message"`
	}
	if err := apiGet(params, &out); err != nil {
		return "", "", err
	}
	if out.Error != 0 {
		return "", "", fmt.Errorf("last.fm error %d: %s", out.Error, out.Msg)
	}
	return out.Session.Name, out.Session.Key, nil
}

func apiGet(params map[string]string, out any) error {
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	resp, err := http.Get(apiURL + "?" + q.Encode())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

// sign matches the Last.fm api_sig algorithm used by the plugin.
func sign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k == "format" || k == "callback" || k == "api_sig" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params[k])
	}
	sb.WriteString(secret)

	sum := md5.Sum([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

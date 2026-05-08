package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type OAuthTokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
	Email        string    `json:"email"`
}

type claimEntry struct {
	TokensJSON []byte
	CreatedAt  time.Time
}

type stateData struct {
	Instance string `json:"instance"`
	Nonce    string `json:"nonce"`
}

const claimTTL = 5 * time.Minute

var (
	clientID           string
	clientSecret       string
	proxyBaseURL       string
	listenPort         string
	allowHTTPInstances bool

	claimStore   = map[string]claimEntry{}
	claimStoreMu sync.Mutex
)

func main() {
	clientID = os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret = os.Getenv("GOOGLE_CLIENT_SECRET")
	proxyBaseURL = strings.TrimRight(os.Getenv("PROXY_BASE_URL"), "/")
	listenPort = os.Getenv("PORT")
	if listenPort == "" {
		listenPort = "8090"
	}
	allowHTTPInstances = strings.EqualFold(os.Getenv("ALLOW_HTTP_INSTANCES"), "true")

	if clientID == "" || clientSecret == "" {
		log.Fatal("GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are required")
	}
	if proxyBaseURL == "" {
		log.Fatal("PROXY_BASE_URL is required")
	}

	go cleanupLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/auth-url", handleAuthURL)
	mux.HandleFunc("/callback", handleCallback)
	mux.HandleFunc("/claim", handleClaim)

	addr := ":" + listenPort
	log.Printf("oauth-proxy listening on %s, base=%s", addr, proxyBaseURL)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func getOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  proxyBaseURL + "/callback",
		Scopes: []string{
			"https://mail.google.com/",
			"https://www.googleapis.com/auth/userinfo.email",
		},
		Endpoint: google.Endpoint,
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func handleAuthURL(w http.ResponseWriter, r *http.Request) {
	encodedState := r.URL.Query().Get("state")
	if encodedState == "" {
		http.Error(w, "missing state", http.StatusBadRequest)
		return
	}
	if _, err := decodeState(encodedState); err != nil {
		http.Error(w, "invalid state: "+err.Error(), http.StatusBadRequest)
		return
	}
	cfg := getOAuthConfig()
	authURL := cfg.AuthCodeURL(encodedState, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	mode := r.URL.Query().Get("mode")
	if mode == "redirect" {
		http.Redirect(w, r, authURL, http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"url": authURL})
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	encodedState := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	errParam := r.URL.Query().Get("error")

	st, err := decodeState(encodedState)
	if err != nil {
		http.Error(w, "invalid state: "+err.Error(), http.StatusBadRequest)
		return
	}

	instance := strings.TrimRight(st.Instance, "/")
	if !isValidInstance(instance) {
		http.Error(w, "invalid instance URL", http.StatusBadRequest)
		return
	}

	finalize := instance + "/api/oauth/google/finalize"

	if errParam != "" {
		redirectFinalize(w, r, finalize, url.Values{
			"error": {errParam},
			"state": {st.Nonce},
		})
		return
	}

	if code == "" {
		redirectFinalize(w, r, finalize, url.Values{
			"error": {"missing code"},
			"state": {st.Nonce},
		})
		return
	}

	cfg := getOAuthConfig()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	token, err := cfg.Exchange(ctx, code)
	if err != nil {
		redirectFinalize(w, r, finalize, url.Values{
			"error": {"exchange failed: " + err.Error()},
			"state": {st.Nonce},
		})
		return
	}

	email, err := fetchUserEmail(ctx, token.AccessToken)
	if err != nil {
		redirectFinalize(w, r, finalize, url.Values{
			"error": {"get email failed: " + err.Error()},
			"state": {st.Nonce},
		})
		return
	}

	tokenData := OAuthTokenData{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
		Email:        email,
	}
	tokensJSON, err := json.Marshal(tokenData)
	if err != nil {
		redirectFinalize(w, r, finalize, url.Values{
			"error": {"marshal tokens failed"},
			"state": {st.Nonce},
		})
		return
	}

	claimCode, err := generateClaimCode()
	if err != nil {
		redirectFinalize(w, r, finalize, url.Values{
			"error": {"generate claim failed"},
			"state": {st.Nonce},
		})
		return
	}

	claimStoreMu.Lock()
	claimStore[claimCode] = claimEntry{TokensJSON: tokensJSON, CreatedAt: time.Now()}
	claimStoreMu.Unlock()

	redirectFinalize(w, r, finalize, url.Values{
		"claim": {claimCode},
		"state": {st.Nonce},
	})
}

func handleClaim(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	claimStoreMu.Lock()
	entry, ok := claimStore[code]
	if ok {
		delete(claimStore, code)
	}
	claimStoreMu.Unlock()

	if !ok {
		http.Error(w, "claim not found", http.StatusNotFound)
		return
	}
	if time.Since(entry.CreatedAt) > claimTTL {
		http.Error(w, "claim expired", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(entry.TokensJSON)
}

func decodeState(encoded string) (*stateData, error) {
	if encoded == "" {
		return nil, fmt.Errorf("empty state")
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		raw, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("base64 decode: %w", err)
		}
	}
	var st stateData
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("json decode: %w", err)
	}
	if st.Instance == "" || st.Nonce == "" {
		return nil, fmt.Errorf("missing instance or nonce")
	}
	return &st, nil
}

func isValidInstance(instance string) bool {
	if instance == "" {
		return false
	}
	u, err := url.Parse(instance)
	if err != nil {
		return false
	}
	if u.Host == "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme == "http" && allowHTTPInstances {
		return true
	}
	return false
}

func redirectFinalize(w http.ResponseWriter, r *http.Request, base string, params url.Values) {
	target := base + "?" + params.Encode()
	http.Redirect(w, r, target, http.StatusFound)
}

func generateClaimCode() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func fetchUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("userinfo status %d: %s", resp.StatusCode, string(body))
	}
	var info struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &info); err != nil {
		return "", fmt.Errorf("decode userinfo: %w", err)
	}
	if info.Email == "" {
		return "", fmt.Errorf("empty email")
	}
	return info.Email, nil
}

func cleanupLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-claimTTL)
		claimStoreMu.Lock()
		for k, v := range claimStore {
			if v.CreatedAt.Before(cutoff) {
				delete(claimStore, k)
			}
		}
		claimStoreMu.Unlock()
	}
}

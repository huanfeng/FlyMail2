package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"mail2im/internal/core"
	"mail2im/internal/models"
)

const oauthCloseScript = `<html><body><script>
if(window.opener){window.opener.postMessage({type:'oauth_done',state:'%s',status:'%s'},window.location.origin)}
setTimeout(function(){window.close()},500)
</script><p>%s</p></body></html>`

// GET /api/oauth/google/url?state=<nanoid>
func GoogleOAuthURL(c *gin.Context) {
	state := c.Query("state")
	if state == "" || len(state) < 8 {
		c.JSON(400, gin.H{"error": "invalid state"})
		return
	}

	core.SetOAuthPending(state)

	if core.IsUsingBuiltinProxy() {
		instanceURL := getInstanceBaseURL(c)
		statePayload := map[string]string{"instance": instanceURL, "nonce": state}
		stateJSON, _ := json.Marshal(statePayload)
		encodedState := base64.RawURLEncoding.EncodeToString(stateJSON)

		proxyURL := core.GetProxyBaseURL()
		authURL := fmt.Sprintf("%s/auth-url?state=%s&mode=redirect", strings.TrimRight(proxyURL, "/"), url.QueryEscape(encodedState))
		c.JSON(200, gin.H{"url": authURL, "state": state, "mode": "proxy"})
		return
	}

	cfg, err := core.GetOAuthConfig()
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	c.JSON(200, gin.H{"url": authURL, "state": state, "mode": "custom"})
}

// GET /api/oauth/google/callback?code=...&state=...
func GoogleOAuthCallback(c *gin.Context) {
	state := c.Query("state")
	code := c.Query("code")
	errParam := c.Query("error")

	if errParam != "" {
		core.SetOAuthError(state, errParam)
		c.Data(200, "text/html; charset=utf-8", []byte(fmt.Sprintf(oauthCloseScript, state, "error", "Authorization failed: "+errParam)))
		return
	}

	if _, ok := core.GetOAuthState(state); !ok {
		c.Data(400, "text/html; charset=utf-8", []byte("Invalid or expired state"))
		return
	}

	cfg, err := core.GetOAuthConfig()
	if err != nil {
		core.SetOAuthError(state, err.Error())
		c.Data(200, "text/html; charset=utf-8", []byte(fmt.Sprintf(oauthCloseScript, state, "error", "OAuth not configured")))
		return
	}

	token, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		core.SetOAuthError(state, err.Error())
		c.Data(200, "text/html; charset=utf-8", []byte(fmt.Sprintf(oauthCloseScript, state, "error", "Token exchange failed")))
		return
	}

	email, err := getGoogleUserEmail(token.AccessToken)
	if err != nil {
		core.SetOAuthError(state, "get email: "+err.Error())
		c.Data(200, "text/html; charset=utf-8", []byte(fmt.Sprintf(oauthCloseScript, state, "error", "Failed to get email")))
		return
	}

	tokenData := &core.OAuthTokenData{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
		Email:        email,
	}

	accountID, err := createOrUpdateOAuthAccount(tokenData)
	if err != nil {
		core.SetOAuthError(state, err.Error())
		c.Data(200, "text/html; charset=utf-8", []byte(fmt.Sprintf(oauthCloseScript, state, "error", "Failed to save account: "+err.Error())))
		return
	}

	core.SetOAuthDone(state, accountID)
	c.Data(200, "text/html; charset=utf-8", []byte(fmt.Sprintf(oauthCloseScript, state, "done", "Authorization successful! You can close this window.")))
}

// GET /api/oauth/google/finalize?claim=...&state=...  (proxy mode)
func GoogleOAuthFinalize(c *gin.Context) {
	state := c.Query("state")
	claimCode := c.Query("claim")
	errParam := c.Query("error")

	if errParam != "" {
		core.SetOAuthError(state, errParam)
		c.Data(200, "text/html; charset=utf-8", []byte(fmt.Sprintf(oauthCloseScript, state, "error", "Authorization failed: "+errParam)))
		return
	}

	if _, ok := core.GetOAuthState(state); !ok {
		c.Data(400, "text/html; charset=utf-8", []byte("Invalid or expired state"))
		return
	}

	tokenData, err := core.FetchTokensFromProxy(claimCode)
	if err != nil {
		core.SetOAuthError(state, err.Error())
		c.Data(200, "text/html; charset=utf-8", []byte(fmt.Sprintf(oauthCloseScript, state, "error", "Failed to retrieve tokens: "+err.Error())))
		return
	}

	accountID, err := createOrUpdateOAuthAccount(tokenData)
	if err != nil {
		core.SetOAuthError(state, err.Error())
		c.Data(200, "text/html; charset=utf-8", []byte(fmt.Sprintf(oauthCloseScript, state, "error", "Failed to save account: "+err.Error())))
		return
	}

	core.SetOAuthDone(state, accountID)
	c.Data(200, "text/html; charset=utf-8", []byte(fmt.Sprintf(oauthCloseScript, state, "done", "Authorization successful! You can close this window.")))
}

// GET /api/oauth/google/status?state=...
func GoogleOAuthStatus(c *gin.Context) {
	state := c.Query("state")
	s, ok := core.GetOAuthState(state)
	if !ok {
		c.JSON(404, gin.H{"status": "not_found"})
		return
	}
	c.JSON(200, gin.H{
		"status":     s.Status,
		"account_id": s.AccountID,
		"error":      s.Error,
	})
}

// POST /api/oauth/google/revoke  body: {account_id}
func GoogleOAuthRevoke(c *gin.Context) {
	var input struct {
		AccountID uint `json:"account_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.AccountID == 0 {
		c.JSON(400, gin.H{"error": "account_id required"})
		return
	}
	var account models.Account
	if err := core.DB.First(&account, input.AccountID).Error; err != nil {
		c.JSON(404, gin.H{"error": "account not found"})
		return
	}
	core.DB.Model(&account).Updates(map[string]interface{}{
		"oauth_token": "",
		"auth_type":   "password",
	})
	c.JSON(200, gin.H{"ok": true})
}

func createOrUpdateOAuthAccount(tokenData *core.OAuthTokenData) (uint, error) {
	email := tokenData.Email
	if email == "" {
		return 0, fmt.Errorf("missing email in token data")
	}
	var account models.Account
	result := core.DB.Where("email = ?", email).First(&account)
	if result.Error != nil {
		account = models.Account{
			Email:      email,
			Login:      email,
			IMAPServer: "imap.gmail.com",
			IMAPPort:   993,
			SSLMode:    "ssl",
			UseSSL:     true,
			Provider:   "gmail",
			AuthType:   "oauth2",
			Enabled:    true,
			UseIDLE:    true,
			Status:     "Active",
		}
		account.DisplayName = strings.Split(email, "@")[0]
		if err := core.DB.Create(&account).Error; err != nil {
			return 0, fmt.Errorf("create account: %w", err)
		}
	} else {
		core.DB.Model(&account).Update("auth_type", "oauth2")
	}

	if err := core.SaveOAuthToken(account.ID, tokenData); err != nil {
		return 0, fmt.Errorf("save token: %w", err)
	}

	if core.Watcher != nil {
		go core.Watcher.RestartWorker(account.ID)
	}
	return account.ID, nil
}

func getInstanceBaseURL(c *gin.Context) string {
	if v, _ := core.GetSystemSetting("instance_base_url"); v != "" {
		return strings.TrimRight(v, "/")
	}
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s", scheme, c.Request.Host)
}

func getGoogleUserEmail(accessToken string) (string, error) {
	req, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var info struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &info); err != nil || info.Email == "" {
		return "", fmt.Errorf("no email in response")
	}
	return info.Email, nil
}

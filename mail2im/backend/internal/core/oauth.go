package core

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"mail2im/internal/models"
)

type OAuthTokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
	Email        string    `json:"email"`
}

func GetOAuthConfig() (*oauth2.Config, error) {
	clientID, _ := GetSystemSetting("oauth_google_client_id")
	clientSecretEnc, _ := GetSystemSetting("oauth_google_client_secret_enc")
	redirectURI, _ := GetSystemSetting("oauth_google_redirect_uri")
	if clientID == "" {
		return nil, fmt.Errorf("Google OAuth not configured")
	}
	secret := clientSecretEnc
	if clientSecretEnc != "" {
		if dec, err := Decrypt(clientSecretEnc); err == nil {
			secret = dec
		}
	}
	if redirectURI == "" {
		redirectURI = "http://localhost:8080/api/oauth/google/callback"
	}
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: secret,
		RedirectURL:  redirectURI,
		Scopes: []string{
			"https://mail.google.com/",
			"https://www.googleapis.com/auth/userinfo.email",
		},
		Endpoint: google.Endpoint,
	}, nil
}

func ParseOAuthToken(account models.Account) (*OAuthTokenData, error) {
	if account.OAuthToken == "" {
		return nil, fmt.Errorf("no oauth token stored")
	}
	raw, err := Decrypt(account.OAuthToken)
	if err != nil {
		return nil, fmt.Errorf("decrypt oauth token: %w", err)
	}
	var data OAuthTokenData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("parse oauth token JSON: %w", err)
	}
	return &data, nil
}

func SaveOAuthToken(accountID uint, data *OAuthTokenData) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	enc, err := Encrypt(string(raw))
	if err != nil {
		return err
	}
	expiry := data.Expiry
	return DB.Model(&models.Account{}).Where("id = ?", accountID).Updates(map[string]interface{}{
		"oauth_token":         enc,
		"password_expires_at": &expiry,
	}).Error
}

func IsTokenExpired(data *OAuthTokenData) bool {
	return time.Now().Add(5 * time.Minute).After(data.Expiry)
}

func RefreshAccessToken(ctx context.Context, data *OAuthTokenData) (*OAuthTokenData, error) {
	cfg, err := GetOAuthConfig()
	if err != nil {
		return nil, err
	}
	token := &oauth2.Token{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		Expiry:       data.Expiry,
		TokenType:    data.TokenType,
	}
	tokenSource := cfg.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	refreshTok := newToken.RefreshToken
	if refreshTok == "" {
		refreshTok = data.RefreshToken
	}
	return &OAuthTokenData{
		AccessToken:  newToken.AccessToken,
		RefreshToken: refreshTok,
		TokenType:    newToken.TokenType,
		Expiry:       newToken.Expiry,
		Email:        data.Email,
	}, nil
}

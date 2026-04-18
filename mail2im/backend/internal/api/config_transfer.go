package api

import (
	"fmt"
	"flymail-core/httputil"
	"mail2im/internal/core"
	"mail2im/internal/models"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ExportAccount struct {
	Email             string `json:"email"`
	DisplayName       string `json:"display_name"`
	Login             string `json:"login"`
	Password          string `json:"password"`
	AuthType          string `json:"auth_type"`
	Provider          string `json:"provider"`
	IMAPServer        string `json:"imap_server"`
	IMAPPort          int    `json:"imap_port"`
	SSLMode           string `json:"ssl_mode"`
	UseIDLE           bool   `json:"use_idle"`
	PollIntervalDay   int    `json:"poll_interval_day"`
	PollIntervalNight int    `json:"poll_interval_night"`
	Timezone          string `json:"timezone"`
	ProxyName         string `json:"proxy_name,omitempty"`
	OAuthToken        string `json:"oauth_token,omitempty"`
	Status            string `json:"status,omitempty"`
	Enabled           bool   `json:"enabled"`
}

type ExportProxy struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type ConfigExportResponse struct {
	Version        string            `json:"version"`
	GeneratedAt    time.Time         `json:"generated_at"`
	Accounts       []ExportAccount   `json:"accounts,omitempty"`
	Proxies        []ExportProxy     `json:"proxies,omitempty"`
	Channels       []ExportChannel   `json:"channels,omitempty"`
	SystemSettings map[string]string `json:"system_settings,omitempty"`
}

type ConfigImportRequest struct {
	Accounts       []ExportAccount   `json:"accounts"`
	Proxies        []ExportProxy     `json:"proxies"`
	Channels       []ExportChannel   `json:"channels"`
	SystemSettings map[string]string `json:"system_settings"`
	Overwrite      bool              `json:"overwrite"`
}

type ExportChannel struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Config      string `json:"config"`
	Status      string `json:"status"`
	MinPriority int    `json:"min_priority"`
	QuietMode   string `json:"quiet_mode"`
	QuietEnable bool   `json:"quiet_enable"`
	QuietStart  string `json:"quiet_start,omitempty"`
	QuietEnd    string `json:"quiet_end,omitempty"`
}

type exportRequest struct {
	Sections []string `json:"sections"`
	Password string   `json:"password"`
}

func ExportConfig(c *gin.Context) {
	var req exportRequest
	_ = c.ShouldBindJSON(&req)

	sections := parseSectionSelection(req.Sections)
	if sections["accounts"] {
		user, ok := CurrentUser(c)
		if !ok {
			httputil.Unauthorized(c, "unauthorized", nil)
			return
		}
		if req.Password == "" {
			httputil.BadRequest(c, "password_required", nil)
			return
		}
		if err := core.VerifyUserPassword(user, req.Password); err != nil {
			httputil.Unauthorized(c, "invalid_password", nil)
			return
		}
	}

	resp := ConfigExportResponse{
		Version:     "1.0",
		GeneratedAt: time.Now().UTC(),
	}

	var proxies []models.Proxy
	if sections["proxies"] || sections["accounts"] {
		if err := core.DB.Find(&proxies).Error; err != nil {
			httputil.InternalError(c, err.Error(), err)
			return
		}
	}

	proxyNameByID := make(map[uint]string)
	if len(proxies) > 0 {
		for _, p := range proxies {
			proxyNameByID[p.ID] = p.Name
		}
	}

	if sections["proxies"] {
		for _, p := range proxies {
			resp.Proxies = append(resp.Proxies, ExportProxy{
				Name:     p.Name,
				Type:     p.Type,
				Host:     p.Host,
				Port:     p.Port,
				Username: p.Username,
				Password: decryptOrKeep(p.Password),
			})
		}
	}

	if sections["accounts"] {
		var accounts []models.Account
		if err := core.DB.Find(&accounts).Error; err != nil {
			httputil.InternalError(c, err.Error(), err)
			return
		}
		for _, acc := range accounts {
			proxyName := ""
			if acc.ProxyID != nil {
				proxyName = proxyNameByID[*acc.ProxyID]
			}

			sslMode := acc.SSLMode
			if sslMode == "" {
				if acc.UseSSL {
					sslMode = "ssl"
				} else {
					sslMode = "none"
				}
			}

			resp.Accounts = append(resp.Accounts, ExportAccount{
				Email:             acc.Email,
				DisplayName:       acc.DisplayName,
				Login:             acc.Login,
				Password:          decryptOrKeep(acc.Password),
				AuthType:          acc.AuthType,
				Provider:          acc.Provider,
				IMAPServer:        acc.IMAPServer,
				IMAPPort:          acc.IMAPPort,
				SSLMode:           sslMode,
				UseIDLE:           acc.UseIDLE,
				PollIntervalDay:   acc.PollIntervalDay,
				PollIntervalNight: acc.PollIntervalNight,
				Timezone:          acc.Timezone,
				ProxyName:         proxyName,
				OAuthToken:        decryptOrKeep(acc.OAuthToken),
				Status:            acc.Status,
				Enabled:           acc.Enabled,
			})
		}
	}

	if sections["channels"] {
		var channels []models.Channel
		if err := core.DB.Find(&channels).Error; err != nil {
			httputil.InternalError(c, err.Error(), err)
			return
		}
		for _, ch := range channels {
			resp.Channels = append(resp.Channels, ExportChannel{
				Name:        ch.Name,
				Type:        ch.Type,
				Config:      ch.Config,
				Status:      ch.Status,
				MinPriority: ch.MinPriority,
				QuietMode:   ch.QuietMode,
				QuietEnable: ch.QuietEnable,
				QuietStart:  ch.QuietStart,
				QuietEnd:    ch.QuietEnd,
			})
		}
	}

	if sections["settings"] {
		var systemSettings []models.SystemSetting
		if err := core.DB.Find(&systemSettings).Error; err != nil {
			httputil.InternalError(c, err.Error(), err)
			return
		}
		if len(systemSettings) > 0 {
			resp.SystemSettings = make(map[string]string, len(systemSettings))
			for _, s := range systemSettings {
				resp.SystemSettings[s.Key] = s.Value
			}
		}
	}

	c.JSON(200, resp)
}

func ImportConfig(c *gin.Context) {
	var payload ConfigImportRequest
	if err := c.ShouldBindJSON(&payload); err != nil {
		httputil.BadRequest(c, err.Error(), err)
		return
	}

	if len(payload.Accounts) == 0 && len(payload.Proxies) == 0 && payload.SystemSettings == nil {
		httputil.BadRequest(c, "no data to import", nil)
		return
	}

	// Preload existing data for conflict detection
	existingProxyMap := make(map[string]models.Proxy)
	if len(payload.Proxies) > 0 || len(payload.Accounts) > 0 {
		var existingProxies []models.Proxy
		if err := core.DB.Find(&existingProxies).Error; err != nil {
			httputil.InternalError(c, err.Error(), err)
			return
		}
		for _, p := range existingProxies {
			existingProxyMap[p.Name] = p
		}
	}

	existingAccountMap := make(map[string]models.Account)
	if len(payload.Accounts) > 0 {
		var existingAccounts []models.Account
		if err := core.DB.Find(&existingAccounts).Error; err != nil {
			httputil.InternalError(c, err.Error(), err)
			return
		}
		for _, a := range existingAccounts {
			existingAccountMap[a.Email] = a
		}
	}

	existingChannelMap := make(map[string]models.Channel)

	conflicts := struct {
		Accounts []string `json:"accounts,omitempty"`
		Proxies  []string `json:"proxies,omitempty"`
		Channels []string `json:"channels,omitempty"`
	}{}

	for _, p := range payload.Proxies {
		if _, ok := existingProxyMap[p.Name]; ok {
			conflicts.Proxies = append(conflicts.Proxies, p.Name)
		}
	}
	for _, acc := range payload.Accounts {
		if _, ok := existingAccountMap[acc.Email]; ok {
			conflicts.Accounts = append(conflicts.Accounts, acc.Email)
		}
	}
	if len(payload.Channels) > 0 {
		var existingChannels []models.Channel
		if err := core.DB.Find(&existingChannels).Error; err != nil {
			httputil.InternalError(c, err.Error(), err)
			return
		}
		for _, ch := range existingChannels {
			existingChannelMap[ch.Name] = ch
		}
		for _, ch := range payload.Channels {
			if _, ok := existingChannelMap[ch.Name]; ok {
				conflicts.Channels = append(conflicts.Channels, ch.Name)
			}
		}
	}

	if (len(conflicts.Accounts) > 0 || len(conflicts.Proxies) > 0) && !payload.Overwrite {
		httputil.ErrorWithInfo(c, httputil.CodeConflict, "conflicts detected, retry with overwrite=true to replace existing records", &httputil.ErrorInfo{
			Metadata: map[string]any{
				"conflicts": conflicts,
			},
		})
		return
	}

	imported := map[string]int{
		"accounts":        0,
		"proxies":         0,
		"channels":        0,
		"system_settings": 0,
	}

	proxyNameToID := make(map[string]uint)
	for _, p := range payload.Proxies {
		encryptedPwd, err := core.Encrypt(p.Password)
		if err != nil {
			httputil.InternalError(c, fmt.Sprintf("Failed to encrypt proxy password: %v", err), nil)
			return
		}

		existing, ok := existingProxyMap[p.Name]
		if !ok {
			newProxy := models.Proxy{
				Name:     p.Name,
				Type:     p.Type,
				Host:     p.Host,
				Port:     p.Port,
				Username: p.Username,
				Password: encryptedPwd,
			}
			if err := core.DB.Create(&newProxy).Error; err != nil {
				httputil.InternalError(c, err.Error(), err)
				return
			}
		} else {
			existing.Type = p.Type
			existing.Host = p.Host
			existing.Port = p.Port
			existing.Username = p.Username
			existing.Password = encryptedPwd
			if err := core.DB.Save(&existing).Error; err != nil {
				httputil.InternalError(c, err.Error(), err)
				return
			}
		}

		existingProxyMap[p.Name] = existing
		proxyNameToID[p.Name] = existing.ID
		imported["proxies"]++
	}

	if len(payload.Accounts) > 0 {
		for name, p := range existingProxyMap {
			if _, ok := proxyNameToID[name]; !ok {
				proxyNameToID[name] = p.ID
			}
		}
	}

	for _, acc := range payload.Accounts {
		existing, hasExisting := existingAccountMap[acc.Email]

		var proxyID *uint
		if acc.ProxyName != "" {
			if id, ok := proxyNameToID[acc.ProxyName]; ok {
				proxyID = &id
			}
		} else if hasExisting && existing.ProxyID != nil {
			proxyID = existing.ProxyID
		}

		req := CreateAccountRequest{
			Email:             acc.Email,
			DisplayName:       acc.DisplayName,
			Login:             acc.Login,
			Password:          acc.Password,
			AuthType:          acc.AuthType,
			Provider:          acc.Provider,
			IMAPServer:        acc.IMAPServer,
			IMAPPort:          acc.IMAPPort,
			SSLMode:           acc.SSLMode,
			ProxyID:           proxyID,
			UseIDLE:           acc.UseIDLE,
			PollIntervalDay:   acc.PollIntervalDay,
			PollIntervalNight: acc.PollIntervalNight,
			Timezone:          acc.Timezone,
			Enabled:           &acc.Enabled,
		}

		target := &existing
		if !hasExisting {
			target = nil
		}

		account, err := toAccount(req, target, true)
		if err != nil {
			httputil.InternalError(c, fmt.Sprintf("Failed to encrypt account password: %v", err), nil)
			return
		}
		if acc.Status != "" {
			account.Status = acc.Status
		}
		if acc.OAuthToken != "" {
			encryptedToken, err := core.Encrypt(acc.OAuthToken)
			if err != nil {
				account.OAuthToken = acc.OAuthToken
			} else {
				account.OAuthToken = encryptedToken
			}
		}

		if hasExisting {
			if err := core.DB.Save(account).Error; err != nil {
				httputil.InternalError(c, err.Error(), err)
				return
			}
			existingAccountMap[acc.Email] = *account
			go core.Watcher.RestartWorker(account.ID)
		} else {
			if err := core.DB.Create(account).Error; err != nil {
				httputil.InternalError(c, err.Error(), err)
				return
			}
			existingAccountMap[acc.Email] = *account
			go core.Watcher.StartWorker(*account)
		}

		imported["accounts"]++
	}

	for _, ch := range payload.Channels {
		existing, hasExisting := existingChannelMap[ch.Name]
		if hasExisting {
			existing.Type = ch.Type
			existing.Config = ch.Config
			existing.Status = ch.Status
			existing.MinPriority = ch.MinPriority
			existing.QuietMode = ch.QuietMode
			existing.QuietEnable = ch.QuietEnable
			existing.QuietStart = ch.QuietStart
			existing.QuietEnd = ch.QuietEnd
			if err := core.DB.Save(&existing).Error; err != nil {
				httputil.InternalError(c, err.Error(), err)
				return
			}
			existingChannelMap[ch.Name] = existing
		} else {
			newCh := models.Channel{
				Name:        ch.Name,
				Type:        ch.Type,
				Config:      ch.Config,
				Status:      ch.Status,
				MinPriority: ch.MinPriority,
				QuietMode:   ch.QuietMode,
				QuietEnable: ch.QuietEnable,
				QuietStart:  ch.QuietStart,
				QuietEnd:    ch.QuietEnd,
			}
			if err := core.DB.Create(&newCh).Error; err != nil {
				httputil.InternalError(c, err.Error(), err)
				return
			}
			existingChannelMap[ch.Name] = newCh
		}
		imported["channels"]++
	}

	if payload.SystemSettings != nil {
		for k, v := range payload.SystemSettings {
			if err := core.SetSystemSetting(k, v); err != nil {
				httputil.InternalError(c, err.Error(), err)
				return
			}
			imported["system_settings"]++
		}
	}

	core.RecordSystemLog("config_import", "success", "Imported configuration", fmt.Sprintf("accounts=%d proxies=%d channels=%d settings=%d", imported["accounts"], imported["proxies"], imported["channels"], imported["system_settings"]))
	httputil.Success(c, gin.H{"imported": imported})
}

func parseSectionSelection(raw interface{}) map[string]bool {
	sections := map[string]bool{
		"accounts": true,
		"proxies":  true,
		"settings": true,
		"channels": true,
	}

	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return sections
		}
		for k := range sections {
			sections[k] = false
		}
		for _, part := range strings.Split(v, ",") {
			p := strings.TrimSpace(strings.ToLower(part))
			if _, ok := sections[p]; ok {
				sections[p] = true
			}
		}
		return sections
	case []string:
		if len(v) == 0 {
			return sections
		}
		for k := range sections {
			sections[k] = false
		}
		for _, part := range v {
			p := strings.TrimSpace(strings.ToLower(part))
			if _, ok := sections[p]; ok {
				sections[p] = true
			}
		}
		return sections
	}

	return sections
}

func decryptOrKeep(val string) string {
	if val == "" {
		return ""
	}
	if plain, err := core.Decrypt(val); err == nil && plain != "" {
		return plain
	}
	return val
}

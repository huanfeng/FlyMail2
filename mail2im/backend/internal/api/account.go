package api

import (
	"mail2im/internal/core"
	"mail2im/internal/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

type CreateAccountRequest struct {
	Email             string `json:"email" binding:"required,email"`
	DisplayName       string `json:"display_name"`
	Login             string `json:"login"`
	Password          string `json:"password"` // Plain text
	AuthType          string `json:"auth_type"`
	Provider          string `json:"provider"`
	IMAPServer        string `json:"imap_server"`
	IMAPPort          int    `json:"imap_port"`
	SSLMode           string `json:"ssl_mode"`
	UseSSL            bool   `json:"use_ssl"` // legacy fallback
	ProxyID           *uint  `json:"proxy_id"`
	UseIDLE           bool   `json:"use_idle"`
	PollIntervalDay   int    `json:"poll_interval_day"`
	PollIntervalNight int    `json:"poll_interval_night"`
	Timezone          string `json:"timezone"`
	Enabled           *bool  `json:"enabled"`
}

func toAccount(input CreateAccountRequest, existing *models.Account, encryptPassword bool) (*models.Account, error) {
	target := &models.Account{}
	if existing != nil {
		target = existing
	}

	sslMode := input.SSLMode
	if sslMode == "" {
		if input.UseSSL {
			sslMode = "ssl"
		} else {
			sslMode = "none"
		}
	}

	target.Email = input.Email
	target.DisplayName = input.DisplayName
	if input.Login != "" {
		target.Login = input.Login
	} else {
		target.Login = input.Email
	}
	target.AuthType = input.AuthType
	target.Provider = input.Provider
	target.IMAPServer = input.IMAPServer
	target.IMAPPort = input.IMAPPort
	target.SSLMode = sslMode
	target.UseSSL = sslMode == "ssl"
	target.ProxyID = input.ProxyID
	target.UseIDLE = input.UseIDLE
	target.PollIntervalDay = input.PollIntervalDay
	target.PollIntervalNight = input.PollIntervalNight
	target.Timezone = input.Timezone
	if target.Status == "" {
		target.Status = "Active"
	}
	if input.Enabled != nil {
		target.Enabled = *input.Enabled
	} else if existing == nil && !target.Enabled {
		target.Enabled = true
	}

	if input.Password != "" {
		encryptedPass, err := core.Encrypt(input.Password)
		if err != nil {
			return nil, err
		}
		if encryptPassword {
			target.Password = encryptedPass
		} else {
			target.Password = input.Password
		}
	}

	return target, nil
}

func CreateAccount(c *gin.Context) {
	var input CreateAccountRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account, err := toAccount(input, nil, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt password"})
		return
	}

	if err := core.DB.Create(account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var created models.Account
	if err := core.DB.Preload("Proxy").First(&created, account.ID).Error; err != nil {
		created = *account
	}

	if created.Enabled {
		go core.Watcher.StartWorker(created)
	}
	c.JSON(http.StatusOK, created)
}

func CreateAccounts(c *gin.Context) {
	var inputs []CreateAccountRequest
	if err := c.ShouldBindJSON(&inputs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var created []models.Account
	for _, input := range inputs {
		account, err := toAccount(input, nil, true)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt password"})
			return
		}

		if err := core.DB.Create(account).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var createdAcc models.Account
		if err := core.DB.Preload("Proxy").First(&createdAcc, account.ID).Error; err != nil {
			createdAcc = *account
		}

		// Start worker immediately
		if createdAcc.Enabled {
			go core.Watcher.StartWorker(createdAcc)
		}

		created = append(created, createdAcc)
	}

	c.JSON(http.StatusOK, gin.H{"count": len(created), "message": "Accounts created successfully"})
}

func GetAccounts(c *gin.Context) {
	var accounts []models.Account
	if err := core.DB.Preload("Proxy").Find(&accounts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, accounts)
}

func GetAccount(c *gin.Context) {
	id := c.Param("id")
	var account models.Account
	if err := core.DB.Preload("Proxy").First(&account, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}
	c.JSON(http.StatusOK, account)
}

func UpdateAccount(c *gin.Context) {
	id := c.Param("id")
	var existing models.Account
	if err := core.DB.Preload("Proxy").First(&existing, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	var input CreateAccountRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account, err := toAccount(input, &existing, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt password"})
		return
	}

	if err := core.DB.Save(account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if account.Enabled {
		go core.Watcher.RestartWorker(account.ID)
	} else {
		go core.Watcher.StopWorker(account.ID)
	}
	c.JSON(http.StatusOK, account)
}

func DeleteAccount(c *gin.Context) {
	id := c.Param("id")

	// Stop worker first
	// Convert id string to uint... skipping for brevity, GORM handles string ID usually but Watcher needs uint
	// Let's fetch first
	var account models.Account
	if err := core.DB.First(&account, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Account not found"})
		return
	}

	core.Watcher.StopWorker(account.ID)

	if err := core.DB.Delete(&account).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Deleted"})
}

func TestAccountConnection(c *gin.Context) {
	var input CreateAccountRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account, err := toAccount(input, nil, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if account.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is required for testing"})
		return
	}

	// Load proxy if provided
	if account.ProxyID != nil {
		var proxy models.Proxy
		if err := core.DB.First(&proxy, account.ProxyID).Error; err == nil {
			account.Proxy = &proxy
		}
	}

	info, latency, err := core.TestIMAPConnection(*account)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "Connection successful",
		"supports_idle": info != nil && info.SupportsIDLE,
		"capabilities":  info.Capabilities,
		"security":      info.SecurityMode,
		"latency_ms":    latency.Milliseconds(),
	})
}

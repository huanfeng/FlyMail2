package sync

import (
	"time"

	"flymail/modules/email/account"
	"flymail/modules/email/message"
)

// MonitorStatus represents the monitoring status of an account
type MonitorStatus struct {
	IsActive        bool      `json:"is_active"`
	IsIDLESupported bool      `json:"is_idle_supported"`
	Mode            string    `json:"mode"` // "idle" or "polling"
	LastCheck       time.Time `json:"last_check"`
	LastError       string    `json:"last_error,omitempty"`
	ErrorCount      int       `json:"error_count"`
	EmailsReceived  int64     `json:"emails_received"`
}

// AccountStatus represents the sync status for a specific account
type AccountStatus struct {
	AccountID uint          `json:"account_id"`
	Status    MonitorStatus `json:"status"`
}

// SyncResult represents the result of a sync operation
type SyncResult struct {
	AccountID    uint   `json:"account_id"`
	NewEmails    int    `json:"new_emails"`
	TotalEmails  int    `json:"total_emails"`
	UpdatedCount int    `json:"updated_count"`
	DeletedCount int    `json:"deleted_count"`
	Error        string `json:"error,omitempty"`
}

// Callbacks for dependency injection
type Callbacks struct {
	OnNewEmail func(account *account.EmailAccount, emails []*message.Email)
}

// EmailProtocol interface for IMAP operations
type EmailProtocol interface {
	Connect() error
	Disconnect() error
	IsConnected() bool
	SupportsIDLE() (bool, error)
	SelectFolder(folderName string) error
	GetUIDs() ([]uint32, error)
	FetchEmails(uids []uint32) ([]*message.Email, error)
	FetchEmailByUID(uid uint32) (*message.Email, error)
	DeleteEmail(uid uint32) error
	StartIDLE() error
	StopIDLE() error
	WaitForNewEmail(timeout time.Duration) (bool, error)
}

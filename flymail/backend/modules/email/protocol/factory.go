package protocol

import (
	"fmt"

	"flymail/modules/email/account"
	"flymail/modules/email/sync"
)

// Factory creates email protocol instances
type Factory struct{}

// NewFactory creates a new protocol factory
func NewFactory() *Factory {
	return &Factory{}
}

// CreateProtocol creates an EmailProtocol instance for the given account
func (f *Factory) CreateProtocol(acc *account.EmailAccount) (sync.EmailProtocol, error) {
	switch acc.Type {
	case "imap":
		return NewIMAPClient(
			acc.ImapServer,
			acc.ImapPort,
			acc.Username,
			acc.Password,
			acc.ImapSSL,
			acc.AccountID,
		), nil

	case "smtp":
		// SMTP is only for sending, not for sync
		return nil, fmt.Errorf("SMTP accounts cannot be used for email sync")

	case "oauth":
		// OAuth support can be added later
		return nil, fmt.Errorf("OAuth accounts are not yet supported")

	default:
		return nil, fmt.Errorf("unsupported account type: %s", acc.Type)
	}
}

// CreateSMTPClient creates an SMTP client for sending emails
func (f *Factory) CreateSMTPClient(acc *account.EmailAccount) (*SMTPClient, error) {
	if acc.SmtpServer == "" || acc.SmtpPort == 0 {
		return nil, fmt.Errorf("SMTP server configuration is missing")
	}

	return NewSMTPClient(
		acc.SmtpServer,
		acc.SmtpPort,
		acc.Username,
		acc.Password,
		acc.SmtpSSL,
	), nil
}

// TestConnection tests both IMAP and SMTP connections for an account
func (f *Factory) TestConnection(acc *account.EmailAccount) (*ConnectionTestResult, error) {
	result := &ConnectionTestResult{
		AccountID: acc.AccountID,
	}

	// Test IMAP if configured
	if acc.ImapServer != "" && acc.ImapPort > 0 {
		imapClient := NewIMAPClient(
			acc.ImapServer,
			acc.ImapPort,
			acc.Username,
			acc.Password,
			acc.ImapSSL,
			acc.AccountID,
		)

		if err := imapClient.Connect(); err != nil {
			result.IMAPError = err.Error()
		} else {
			result.IMAP = true
			result.SupportsIDLE, _ = imapClient.SupportsIDLE()

			// Get capabilities
			// TODO: Find a way to get capabilities using public API
			// The caps field is not exported in the client library

			imapClient.Disconnect()
		}
	}

	// Test SMTP if configured
	if acc.SmtpServer != "" && acc.SmtpPort > 0 {
		smtpClient := NewSMTPClient(
			acc.SmtpServer,
			acc.SmtpPort,
			acc.Username,
			acc.Password,
			acc.SmtpSSL,
		)

		if err := smtpClient.TestConnection(); err != nil {
			result.SMTPError = err.Error()
		} else {
			result.SMTP = true
		}
	}

	return result, nil
}

// ConnectionTestResult represents the result of connection tests
type ConnectionTestResult struct {
	AccountID    uint     `json:"account_id"`
	IMAP         bool     `json:"imap"`
	SMTP         bool     `json:"smtp"`
	SupportsIDLE bool     `json:"supports_idle"`
	Capabilities []string `json:"capabilities"`
	IMAPError    string   `json:"imap_error,omitempty"`
	SMTPError    string   `json:"smtp_error,omitempty"`
}

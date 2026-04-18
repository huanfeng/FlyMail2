package protocol

import (
	"fmt"

	"flymail/modules/email/account"
	"flymail/modules/email/sync"

	coresmtp "flymail-core/smtp"
	"flymail-core/types"
)

// Factory creates email protocol instances
type Factory struct{}

// NewFactory creates a new protocol factory
func NewFactory() *Factory {
	return &Factory{}
}

// CreateProtocol creates an EmailProtocol instance for the given account.
// Uses core/imap (go-imap/v2) via CoreAdapter.
func (f *Factory) CreateProtocol(acc *account.EmailAccount) (sync.EmailProtocol, error) {
	switch acc.Type {
	case "imap":
		cfg := buildIMAPConfig(acc)
		return NewCoreAdapter(cfg, acc.AccountID), nil

	case "smtp":
		return nil, fmt.Errorf("SMTP accounts cannot be used for email sync")

	case "oauth":
		return nil, fmt.Errorf("OAuth accounts are not yet supported")

	default:
		return nil, fmt.Errorf("unsupported account type: %s", acc.Type)
	}
}

// CreateSMTPClient creates an SMTP client for sending emails.
// Uses core/smtp with unified SSL/STARTTLS/proxy support.
func (f *Factory) CreateSMTPClient(acc *account.EmailAccount) (*coresmtp.Client, error) {
	if acc.SmtpServer == "" || acc.SmtpPort == 0 {
		return nil, fmt.Errorf("SMTP server configuration is missing")
	}

	security := types.SecurityNone
	if acc.SmtpSSL {
		security = types.SecuritySSL
	}

	cfg := types.SMTPConfig{
		Host:     acc.SmtpServer,
		Port:     acc.SmtpPort,
		Username: acc.Username,
		Password: acc.Password,
		Security: security,
	}

	return coresmtp.NewClient(cfg), nil
}

// TestConnection tests both IMAP and SMTP connections for an account.
func (f *Factory) TestConnection(acc *account.EmailAccount) (*types.ConnectionTestResult, error) {
	result := &types.ConnectionTestResult{}

	// Test IMAP
	if acc.ImapServer != "" && acc.ImapPort > 0 {
		cfg := buildIMAPConfig(acc)
		adapter := NewCoreAdapter(cfg, acc.AccountID)
		imapResult, err := adapter.TestConnection()
		if err != nil {
			result.IMAPError = err.Error()
		} else {
			result.IMAP = imapResult.IMAP
			result.SupportsIDLE = imapResult.SupportsIDLE
			result.Capabilities = imapResult.Capabilities
			result.SecurityMode = imapResult.SecurityMode
			result.IMAPError = imapResult.IMAPError
		}
	}

	// Test SMTP
	if acc.SmtpServer != "" && acc.SmtpPort > 0 {
		smtpClient, err := f.CreateSMTPClient(acc)
		if err != nil {
			result.SMTPError = err.Error()
		} else if err := smtpClient.TestConnection(); err != nil {
			result.SMTPError = err.Error()
		} else {
			result.SMTP = true
		}
	}

	return result, nil
}

func buildIMAPConfig(acc *account.EmailAccount) types.IMAPConfig {
	security := types.SecurityNone
	if acc.ImapSSL {
		security = types.SecuritySSL
	}

	return types.IMAPConfig{
		Host:         acc.ImapServer,
		Port:         acc.ImapPort,
		Username:     acc.Username,
		Password:     acc.Password,
		Security:     security,
		ClientName:   "FlyMail",
		ClientVendor: "FlyMail",
	}
}

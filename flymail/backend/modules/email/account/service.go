package account

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	corevimap "flymail-core/imap"
	"flymail-core/logger"
	"flymail-core/types"
)

var (
	// ErrAccountExists indicates that an account already exists
	ErrAccountExists = errors.New("account already exists")
	// ErrAccountNotFound indicates that an account was not found
	ErrAccountNotFound = errors.New("account not found")
)

// FolderSyncer interface for syncing folders
type FolderSyncer interface {
	SyncFolders(ctx context.Context, accountID uint, folders []*Folder) error
}

// Folder represents a mail folder (basic definition for account module)
type Folder struct {
	AccountID uint
	Name      string
	RawName   string
	Delimiter string
}

// EmailCounter interface for counting emails
type EmailCounter interface {
	CountByAccount(ctx context.Context, accountID uint) (int64, error)
	CountUnreadByAccount(ctx context.Context, accountID uint) (int64, error)
	GetTotalSizeByAccount(ctx context.Context, accountID uint) (int64, error)
}

// Service interface for email account operations
type Service interface {
	CreateAccount(ctx context.Context, userID uint, account *EmailAccount) error
	GetAccount(ctx context.Context, userID uint, accountID uint) (*EmailAccount, error)
	GetAccounts(ctx context.Context, userID uint) ([]EmailAccount, error)
	UpdateAccount(ctx context.Context, userID uint, accountID uint, updates map[string]interface{}) error
	DeleteAccount(ctx context.Context, userID uint, accountID uint) error
	GetAccountStats(ctx context.Context, userID uint, accountID uint) (*AccountStats, error)
	TestConnection(ctx context.Context, account *EmailAccount) (*TestConnectionResult, error)
	SetEmailMonitor(emailMonitor interface{})
	SetFolderSyncer(folderSyncer FolderSyncer)
	SetEmailCounter(emailCounter EmailCounter)
}

// service implements Service interface
type service struct {
	repo         Repository
	folderSyncer FolderSyncer
	emailCounter EmailCounter
	emailMonitor interface{} // Will be set via setter to avoid circular dependency
}

// NewService creates a new account service
func NewService(repo Repository) Service {
	return &service{
		repo: repo,
	}
}

// SetEmailMonitor sets the email monitor (used to avoid circular dependency)
func (s *service) SetEmailMonitor(emailMonitor interface{}) {
	s.emailMonitor = emailMonitor
}

// SetFolderSyncer sets the folder syncer
func (s *service) SetFolderSyncer(folderSyncer FolderSyncer) {
	s.folderSyncer = folderSyncer
}

// SetEmailCounter sets the email counter
func (s *service) SetEmailCounter(emailCounter EmailCounter) {
	s.emailCounter = emailCounter
}

// CreateAccount creates a new email account
func (s *service) CreateAccount(ctx context.Context, userID uint, account *EmailAccount) error {
	// Set user ID
	account.UserID = userID

	// Log account details before saving
	logger.Debug("Creating account in database",
		zap.String("name", account.Name),
		zap.String("email", account.Email),
		zap.String("username", account.Username),
		zap.Bool("has_password", account.Password != ""),
		zap.Int("password_length", len(account.Password)),
	)

	// Set default initial sync option if not specified
	if account.InitialSyncOption == "" {
		account.InitialSyncOption = "full"
		account.InitialSyncDays = 0
		account.InitialSyncCount = 0
	}

	// Create account
	if err := s.repo.Create(ctx, account); err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}

	// Initialize account after creation
	go s.initializeAccount(context.Background(), account)

	return nil
}

// GetAccount retrieves an email account by ID
func (s *service) GetAccount(ctx context.Context, userID uint, accountID uint) (*EmailAccount, error) {
	account, err := s.repo.GetByID(ctx, accountID, userID)
	if err != nil {
		return nil, err
	}

	return account, nil
}

// GetAccounts retrieves all email accounts for a user
func (s *service) GetAccounts(ctx context.Context, userID uint) ([]EmailAccount, error) {
	accounts, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Convert []*EmailAccount to []EmailAccount
	result := make([]EmailAccount, len(accounts))
	for i, account := range accounts {
		result[i] = *account
	}

	return result, nil
}

// UpdateAccount updates an email account
func (s *service) UpdateAccount(ctx context.Context, userID uint, accountID uint, updates map[string]interface{}) error {
	// Verify account belongs to user
	if _, err := s.repo.GetByID(ctx, accountID, userID); err != nil {
		return err
	}

	// Update account
	if err := s.repo.UpdateFields(ctx, accountID, updates); err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	return nil
}

// DeleteAccount deletes an email account
func (s *service) DeleteAccount(ctx context.Context, userID uint, accountID uint) error {
	// Delete account (this should cascade delete emails)
	if err := s.repo.Delete(ctx, accountID); err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	return nil
}

// GetAccountStats retrieves statistics for an email account
func (s *service) GetAccountStats(ctx context.Context, userID uint, accountID uint) (*AccountStats, error) {
	// Verify account belongs to user
	_, err := s.repo.GetByID(ctx, accountID, userID)
	if err != nil {
		return nil, err
	}

	stats := &AccountStats{}

	// Get email counts if email counter is available
	if s.emailCounter != nil {
		totalEmails, err := s.emailCounter.CountByAccount(ctx, accountID)
		if err != nil {
			return nil, fmt.Errorf("failed to get total emails: %w", err)
		}
		stats.TotalEmails = totalEmails

		unreadEmails, err := s.emailCounter.CountUnreadByAccount(ctx, accountID)
		if err != nil {
			return nil, fmt.Errorf("failed to get unread emails: %w", err)
		}
		stats.UnreadEmails = unreadEmails

		totalSize, err := s.emailCounter.GetTotalSizeByAccount(ctx, accountID)
		if err != nil {
			return nil, fmt.Errorf("failed to get total size: %w", err)
		}
		stats.StorageUsed = totalSize
	}

	return stats, nil
}

// TestConnection tests the account connection
func (s *service) TestConnection(ctx context.Context, account *EmailAccount) (*TestConnectionResult, error) {
	result := &TestConnectionResult{
		IMAP: false,
		SMTP: false,
	}

	// Test IMAP connection
	if account.ImapServer != "" {
		capabilities, supportsIDLE, err := s.testIMAPConnection(account)
		if err == nil {
			result.IMAP = true
			result.SupportsIDLE = supportsIDLE
			result.Capabilities = capabilities
		}
	}

	// TODO: Test SMTP connection when SMTP client is available

	return result, nil
}

// initializeAccount initializes a newly created account
func (s *service) initializeAccount(ctx context.Context, account *EmailAccount) {
	logger.Info("Starting account initialization",
		zap.Uint("account_id", account.AccountID),
		zap.String("email", account.Email),
		zap.String("initial_sync_option", account.InitialSyncOption))

	// Mark account as syncing
	updateData := map[string]interface{}{
		"is_syncing": true,
	}
	if err := s.repo.UpdateFields(ctx, account.AccountID, updateData); err != nil {
		logger.Error("Failed to update account sync status", zap.Error(err))
	}

	// Step 1: Test connection and get capabilities
	if err := s.testAndUpdateCapabilities(ctx, account); err != nil {
		logger.Error("Failed to test account connection",
			zap.Uint("account_id", account.AccountID),
			zap.Error(err))
		s.updateSyncError(ctx, account, err)
		return
	}

	// Step 2: Sync folder structure
	if s.folderSyncer != nil {
		if err := s.syncFolders(ctx, account); err != nil {
			logger.Error("Failed to sync folders",
				zap.Uint("account_id", account.AccountID),
				zap.Error(err))
			s.updateSyncError(ctx, account, err)
			return
		}
	}

	// Step 3: Add account to email monitor
	if s.emailMonitor != nil {
		if monitor, ok := s.emailMonitor.(interface {
			StartMonitoringAccount(accountID uint) error
		}); ok {
			if err := monitor.StartMonitoringAccount(account.AccountID); err != nil {
				logger.Error("Failed to add account to monitor",
					zap.Uint("account_id", account.AccountID),
					zap.Error(err))
			}
		}
	}

	// Step 4: Note - Initial sync is now handled by the new task system
	logger.Debug("Account created, initial sync should be handled by task system",
		zap.String("initial_sync_option", account.InitialSyncOption),
		zap.Uint("account_id", account.AccountID))

	logger.Info("Account initialization completed successfully",
		zap.Uint("account_id", account.AccountID))
}

// testAndUpdateCapabilities tests the account connection and updates capabilities
func (s *service) testAndUpdateCapabilities(ctx context.Context, account *EmailAccount) error {
	capabilities, supportsIDLE, err := s.testIMAPConnection(account)
	if err != nil {
		return err
	}

	capabilitiesStr := ""
	for i, cap := range capabilities {
		if i > 0 {
			capabilitiesStr += ","
		}
		capabilitiesStr += cap
	}

	// Update account with capabilities
	now := time.Now()
	updateData := map[string]interface{}{
		"capabilities":          capabilitiesStr,
		"supports_idle":         supportsIDLE,
		"last_capability_check": &now,
	}

	if err := s.repo.UpdateFields(ctx, account.AccountID, updateData); err != nil {
		return fmt.Errorf("failed to update capabilities: %w", err)
	}

	account.Capabilities = capabilitiesStr
	account.SupportsIDLE = &supportsIDLE
	account.LastCapabilityCheck = &now

	return nil
}

// testIMAPConnection tests IMAP connection and returns capabilities
func (s *service) testIMAPConnection(account *EmailAccount) ([]string, bool, error) {
	cfg := buildIMAPConfigFromAccount(account)
	session, err := corevimap.Dial(cfg)
	if err != nil {
		return nil, false, fmt.Errorf("failed to connect to IMAP server: %w", err)
	}
	defer session.Close()

	return session.Capabilities, session.SupportsIDLE, nil
}

// syncFolders syncs the folder structure for the account
func (s *service) syncFolders(ctx context.Context, account *EmailAccount) error {
	cfg := buildIMAPConfigFromAccount(account)
	session, err := corevimap.Dial(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to IMAP server: %w", err)
	}
	defer session.Close()

	// List all folders via core/imap
	folderInfos, err := session.ListFolders()
	if err != nil {
		return fmt.Errorf("failed to list folders: %w", err)
	}

	var folders []*Folder
	for _, fi := range folderInfos {
		folder := &Folder{
			AccountID: account.AccountID,
			Name:      fi.Name,
			RawName:   fi.Path,
			Delimiter: fi.Delimiter,
		}
		folders = append(folders, folder)
	}

	// Sync folders to database
	if s.folderSyncer != nil {
		if err := s.folderSyncer.SyncFolders(ctx, account.AccountID, folders); err != nil {
			return fmt.Errorf("failed to sync folders to database: %w", err)
		}
	}

	logger.Info("Synced folders for account",
		zap.Uint("account_id", account.AccountID),
		zap.Int("folder_count", len(folders)))

	return nil
}

// updateSyncError updates the account with sync error information
func (s *service) updateSyncError(ctx context.Context, account *EmailAccount, err error) {
	now := time.Now()
	updateData := map[string]interface{}{
		"is_syncing":      false,
		"sync_error":      err.Error(),
		"last_sync_error": &now,
	}

	if updateErr := s.repo.UpdateFields(ctx, account.AccountID, updateData); updateErr != nil {
		logger.Error("Failed to update account sync error",
			zap.Uint("account_id", account.AccountID),
			zap.Error(updateErr))
	}
}

func buildIMAPConfigFromAccount(account *EmailAccount) types.IMAPConfig {
	security := types.SecurityNone
	if account.ImapSSL {
		security = types.SecuritySSL
	}
	return types.IMAPConfig{
		Host:         account.ImapServer,
		Port:         account.ImapPort,
		Username:     account.Username,
		Password:     account.Password,
		Security:     security,
		ClientName:   "FlyMail",
		ClientVendor: "FlyMail",
	}
}

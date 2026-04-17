package sync

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"flymail/modules/email/account"
	"flymail/modules/email/message"
	"flymail/pkg/logger"
)

// Service interface for email sync operations
type Service interface {
	// Start/stop operations
	Start() error
	Stop()
	// Account monitoring
	StartMonitoringAccount(account *account.EmailAccount) error
	StopMonitoringAccount(accountID uint) error
	RestartMonitoringAccount(accountID uint) error
	// Status operations
	GetStatus() map[uint]MonitorStatus
	GetAccountStatus(accountID uint) (MonitorStatus, error)
	// Configuration
	SetConfig(config *Config)
	GetConfig() *Config
	// Callbacks
	SetCallbacks(callbacks Callbacks)
	// Manual sync
	SyncAccount(ctx context.Context, accountID uint) (*SyncResult, error)
	SyncAllAccounts(ctx context.Context) (map[uint]*SyncResult, error)
	// Dependency injection
	SetProtocolFactory(factory ProtocolFactory)
}

// ProtocolFactory creates EmailProtocol instances for accounts
type ProtocolFactory interface {
	CreateProtocol(account *account.EmailAccount) (EmailProtocol, error)
}

// service implements Service interface
type service struct {
	accountRepo     account.Repository
	messageRepo     message.Repository
	messageService  message.Service
	monitors        map[uint]*AccountMonitor
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	config          *Config
	callbacks       Callbacks
	protocolFactory ProtocolFactory
}

// NewService creates a new email sync service
func NewService(
	accountRepo account.Repository,
	messageRepo message.Repository,
	messageService message.Service,
	config *Config,
) Service {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &service{
		accountRepo:    accountRepo,
		messageRepo:    messageRepo,
		messageService: messageService,
		monitors:       make(map[uint]*AccountMonitor),
		ctx:            ctx,
		cancel:         cancel,
		config:         config,
	}
}

// SetProtocolFactory sets the protocol factory
func (s *service) SetProtocolFactory(factory ProtocolFactory) {
	s.protocolFactory = factory
}

// Start starts monitoring all email accounts
func (s *service) Start() error {
	logger.Info("Starting email sync service")

	// Load all active email accounts
	accounts, err := s.accountRepo.GetActiveAccounts(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load email accounts: %w", err)
	}

	// Start monitoring each account
	for _, acc := range accounts {
		if err := s.StartMonitoringAccount(acc); err != nil {
			logger.Error("Failed to start monitoring account",
				zap.Uint("account_id", acc.AccountID),
				zap.Error(err))
		}
	}

	logger.Info("Email sync service started",
		zap.Int("accounts", len(accounts)))

	return nil
}

// Stop stops all monitoring
func (s *service) Stop() {
	logger.Info("Stopping email sync service")

	s.cancel()

	s.mu.Lock()
	for accountID, monitor := range s.monitors {
		logger.Info("Stopping monitor for account", zap.Uint("account_id", accountID))
		monitor.Stop()
	}
	s.monitors = make(map[uint]*AccountMonitor)
	s.mu.Unlock()

	logger.Info("Email sync service stopped")
}

// StartMonitoringAccount starts monitoring a specific account
func (s *service) StartMonitoringAccount(emailAccount *account.EmailAccount) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already monitoring
	if _, exists := s.monitors[emailAccount.AccountID]; exists {
		return fmt.Errorf("account %d is already being monitored", emailAccount.AccountID)
	}

	// Create monitor
	monitor := NewAccountMonitor(
		emailAccount,
		s.accountRepo,
		s.messageRepo,
		s.messageService,
		s.config,
	)

	// Set callbacks
	if s.callbacks.OnNewEmail != nil {
		monitor.SetOnNewEmail(s.callbacks.OnNewEmail)
	}

	// Create protocol if factory is available
	if s.protocolFactory != nil {
		protocol, err := s.protocolFactory.CreateProtocol(emailAccount)
		if err != nil {
			return fmt.Errorf("failed to create protocol: %w", err)
		}
		monitor.SetProtocol(protocol)
	}

	// Start monitoring
	if err := monitor.Start(s.ctx); err != nil {
		return fmt.Errorf("failed to start monitor: %w", err)
	}

	s.monitors[emailAccount.AccountID] = monitor

	logger.Info("Started monitoring account",
		zap.Uint("account_id", emailAccount.AccountID),
		zap.String("email", emailAccount.Email))

	return nil
}

// StopMonitoringAccount stops monitoring a specific account
func (s *service) StopMonitoringAccount(accountID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	monitor, exists := s.monitors[accountID]
	if !exists {
		return fmt.Errorf("account %d is not being monitored", accountID)
	}

	monitor.Stop()
	delete(s.monitors, accountID)

	logger.Info("Stopped monitoring account", zap.Uint("account_id", accountID))

	return nil
}

// RestartMonitoringAccount restarts monitoring for a specific account
func (s *service) RestartMonitoringAccount(accountID uint) error {
	// Get account
	acc, err := s.accountRepo.GetByID(context.Background(), accountID, 0)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Stop if currently monitoring
	s.StopMonitoringAccount(accountID)

	// Start monitoring
	return s.StartMonitoringAccount(acc)
}

// GetStatus returns the status of all monitors
func (s *service) GetStatus() map[uint]MonitorStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := make(map[uint]MonitorStatus)
	for accountID, monitor := range s.monitors {
		status[accountID] = monitor.GetStatus()
	}

	return status
}

// GetAccountStatus returns the status of a specific account monitor
func (s *service) GetAccountStatus(accountID uint) (MonitorStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	monitor, exists := s.monitors[accountID]
	if !exists {
		return MonitorStatus{}, fmt.Errorf("account %d is not being monitored", accountID)
	}

	return monitor.GetStatus(), nil
}

// SetConfig updates the configuration
func (s *service) SetConfig(config *Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
}

// GetConfig returns the current configuration
func (s *service) GetConfig() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// SetCallbacks sets the callbacks
func (s *service) SetCallbacks(callbacks Callbacks) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callbacks = callbacks
}

// SyncAccount manually syncs a specific account
func (s *service) SyncAccount(ctx context.Context, accountID uint) (*SyncResult, error) {
	// Get account
	acc, err := s.accountRepo.GetByID(ctx, accountID, 0)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	result := &SyncResult{
		AccountID: accountID,
	}

	// Check if monitor exists
	s.mu.RLock()
	monitor, exists := s.monitors[accountID]
	s.mu.RUnlock()

	if exists {
		// Use existing monitor to sync
		if err := monitor.syncEmails(ctx); err != nil {
			result.Error = err.Error()
			return result, err
		}
	} else {
		// Create temporary monitor for sync
		monitor := NewAccountMonitor(acc, s.accountRepo, s.messageRepo, s.messageService, s.config)

		if s.protocolFactory != nil {
			protocol, err := s.protocolFactory.CreateProtocol(acc)
			if err != nil {
				result.Error = err.Error()
				return result, err
			}
			monitor.SetProtocol(protocol)
			defer protocol.Disconnect()
		}

		if err := monitor.syncEmails(ctx); err != nil {
			result.Error = err.Error()
			return result, err
		}
	}

	// Get stats
	total, _, err := s.messageService.GetAccountStats(ctx, accountID)
	if err == nil {
		result.TotalEmails = int(total)
	}

	return result, nil
}

// SyncAllAccounts manually syncs all accounts
func (s *service) SyncAllAccounts(ctx context.Context) (map[uint]*SyncResult, error) {
	// Get all active accounts
	accounts, err := s.accountRepo.GetActiveAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get active accounts: %w", err)
	}

	results := make(map[uint]*SyncResult)
	var wg sync.WaitGroup

	for _, acc := range accounts {
		wg.Add(1)
		go func(account *account.EmailAccount) {
			defer wg.Done()
			result, _ := s.SyncAccount(ctx, account.AccountID)
			s.mu.Lock()
			results[account.AccountID] = result
			s.mu.Unlock()
		}(acc)
	}

	wg.Wait()
	return results, nil
}

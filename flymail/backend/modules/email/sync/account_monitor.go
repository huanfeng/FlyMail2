package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"flymail/modules/email/account"
	"flymail/modules/email/message"
	"flymail-core/logger"
)

// AccountMonitor monitors a single email account
type AccountMonitor struct {
	account        *account.EmailAccount
	accountRepo    account.Repository
	messageRepo    message.Repository
	messageService message.Service
	config         *Config
	status         MonitorStatus
	statusMu       sync.RWMutex
	stopCh         chan struct{}
	wg             sync.WaitGroup
	onNewEmail     func(account *account.EmailAccount, emails []*message.Email)
	protocol       EmailProtocol
	protocolMu     sync.Mutex
}

// NewAccountMonitor creates a new account monitor
func NewAccountMonitor(
	emailAccount *account.EmailAccount,
	accountRepo account.Repository,
	messageRepo message.Repository,
	messageService message.Service,
	config *Config,
) *AccountMonitor {
	return &AccountMonitor{
		account:        emailAccount,
		accountRepo:    accountRepo,
		messageRepo:    messageRepo,
		messageService: messageService,
		config:         config,
		stopCh:         make(chan struct{}),
		status: MonitorStatus{
			Mode: "polling",
		},
	}
}

// SetProtocol sets the email protocol implementation
func (m *AccountMonitor) SetProtocol(protocol EmailProtocol) {
	m.protocolMu.Lock()
	defer m.protocolMu.Unlock()
	m.protocol = protocol
}

// SetOnNewEmail sets the callback function when new emails are received
func (m *AccountMonitor) SetOnNewEmail(fn func(account *account.EmailAccount, emails []*message.Email)) {
	m.onNewEmail = fn
}

// Start starts monitoring the account
func (m *AccountMonitor) Start(ctx context.Context) error {
	m.statusMu.Lock()
	if m.status.IsActive {
		m.statusMu.Unlock()
		return fmt.Errorf("monitor is already active")
	}
	m.status.IsActive = true
	m.status.ErrorCount = 0
	m.status.LastError = ""
	m.statusMu.Unlock()

	m.wg.Add(1)
	go m.monitorLoop(ctx)

	return nil
}

// Stop stops monitoring the account
func (m *AccountMonitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()

	m.protocolMu.Lock()
	if m.protocol != nil {
		m.protocol.Disconnect()
		m.protocol = nil
	}
	m.protocolMu.Unlock()

	m.statusMu.Lock()
	m.status.IsActive = false
	m.statusMu.Unlock()
}

// GetStatus returns the current monitoring status
func (m *AccountMonitor) GetStatus() MonitorStatus {
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	return m.status
}

// monitorLoop is the main monitoring loop
func (m *AccountMonitor) monitorLoop(ctx context.Context) {
	defer m.wg.Done()

	logger.Info("Starting monitor loop for account",
		zap.Uint("account_id", m.account.AccountID),
		zap.String("email", m.account.Email))

	// Initial sync
	if err := m.syncEmails(ctx); err != nil {
		logger.Error("Initial sync failed",
			zap.Uint("account_id", m.account.AccountID),
			zap.Error(err))
		m.updateError(err.Error())
	}

	// Determine monitoring mode
	if m.config.EnableIDLE && m.account.SupportsIDLE != nil && *m.account.SupportsIDLE {
		m.statusMu.Lock()
		m.status.Mode = "idle"
		m.status.IsIDLESupported = true
		m.statusMu.Unlock()
		m.idleLoop(ctx)
	} else {
		m.pollingLoop(ctx)
	}
}

// idleLoop uses IMAP IDLE for real-time monitoring
func (m *AccountMonitor) idleLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		default:
			if err := m.idleMonitor(ctx); err != nil {
				logger.Error("IDLE monitoring error",
					zap.Uint("account_id", m.account.AccountID),
					zap.Error(err))
				m.updateError(err.Error())

				// Fallback to polling if IDLE fails too many times
				if m.status.ErrorCount >= m.config.MaxRetries {
					logger.Warn("Switching to polling mode due to repeated IDLE errors",
						zap.Uint("account_id", m.account.AccountID))
					m.statusMu.Lock()
					m.status.Mode = "polling"
					m.statusMu.Unlock()
					m.pollingLoop(ctx)
					return
				}

				// Wait before retrying
				select {
				case <-time.After(m.config.RetryInterval):
				case <-ctx.Done():
					return
				case <-m.stopCh:
					return
				}
			}
		}
	}
}

// pollingLoop uses periodic polling for monitoring
func (m *AccountMonitor) pollingLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.GetCurrentPollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			// Adjust ticker interval based on time of day
			ticker.Reset(m.config.GetCurrentPollInterval())

			if err := m.syncEmails(ctx); err != nil {
				logger.Error("Sync error",
					zap.Uint("account_id", m.account.AccountID),
					zap.Error(err))
				m.updateError(err.Error())
			} else {
				m.clearError()
			}
		}
	}
}

// idleMonitor performs IDLE monitoring
func (m *AccountMonitor) idleMonitor(ctx context.Context) error {
	m.protocolMu.Lock()
	protocol := m.protocol
	m.protocolMu.Unlock()

	if protocol == nil {
		return fmt.Errorf("protocol not initialized")
	}

	// Connect if not connected
	if !protocol.IsConnected() {
		if err := protocol.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	// Select INBOX
	if err := protocol.SelectFolder("INBOX"); err != nil {
		return fmt.Errorf("failed to select INBOX: %w", err)
	}

	// Start IDLE
	if err := protocol.StartIDLE(); err != nil {
		return fmt.Errorf("failed to start IDLE: %w", err)
	}
	defer protocol.StopIDLE()

	// Wait for new emails or timeout
	hasNew, err := protocol.WaitForNewEmail(25 * time.Minute) // IDLE timeout is usually 30 minutes
	if err != nil {
		return fmt.Errorf("IDLE wait error: %w", err)
	}

	if hasNew {
		// Sync new emails
		if err := m.syncEmails(ctx); err != nil {
			return fmt.Errorf("sync error after IDLE notification: %w", err)
		}
	}

	return nil
}

// syncEmails synchronizes emails for the account
func (m *AccountMonitor) syncEmails(ctx context.Context) error {
	m.statusMu.Lock()
	m.status.LastCheck = time.Now()
	m.statusMu.Unlock()

	m.protocolMu.Lock()
	protocol := m.protocol
	m.protocolMu.Unlock()

	if protocol == nil {
		return fmt.Errorf("protocol not initialized")
	}

	// Connect if not connected
	if !protocol.IsConnected() {
		if err := protocol.Connect(); err != nil {
			return fmt.Errorf("failed to connect: %w", err)
		}
	}

	// Select INBOX for now (can be extended to sync multiple folders)
	if err := protocol.SelectFolder("INBOX"); err != nil {
		return fmt.Errorf("failed to select INBOX: %w", err)
	}

	// Get remote UIDs
	remoteUIDs, err := protocol.GetUIDs()
	if err != nil {
		return fmt.Errorf("failed to get UIDs: %w", err)
	}

	// Get local UIDs
	localUIDs, err := m.messageService.GetUIDList(ctx, m.account.AccountID, "INBOX")
	if err != nil {
		return fmt.Errorf("failed to get local UIDs: %w", err)
	}

	// Convert to map for faster lookup
	localUIDMap := make(map[uint32]bool)
	for _, uid := range localUIDs {
		localUIDMap[uid] = true
	}

	// Find new UIDs
	var newUIDs []uint32
	for _, uid := range remoteUIDs {
		if !localUIDMap[uid] {
			newUIDs = append(newUIDs, uid)
		}
	}

	// Fetch new emails
	if len(newUIDs) > 0 {
		newEmails, err := protocol.FetchEmails(newUIDs)
		if err != nil {
			return fmt.Errorf("failed to fetch emails: %w", err)
		}

		// Save emails
		if err := m.messageService.CreateEmailsBatch(ctx, newEmails); err != nil {
			return fmt.Errorf("failed to save emails: %w", err)
		}

		// Update stats
		m.statusMu.Lock()
		m.status.EmailsReceived += int64(len(newEmails))
		m.statusMu.Unlock()

		// Call callback
		if m.onNewEmail != nil && len(newEmails) > 0 {
			m.onNewEmail(m.account, newEmails)
		}

		logger.Info("Synced new emails",
			zap.Uint("account_id", m.account.AccountID),
			zap.Int("count", len(newEmails)))
	}

	// Update last sync time
	now := time.Now()
	if err := m.accountRepo.UpdateFields(ctx, m.account.AccountID, map[string]interface{}{
		"last_sync": now,
	}); err != nil {
		logger.Error("Failed to update last sync time",
			zap.Uint("account_id", m.account.AccountID),
			zap.Error(err))
	}

	return nil
}

// updateError updates the error status
func (m *AccountMonitor) updateError(errMsg string) {
	m.statusMu.Lock()
	m.status.LastError = errMsg
	m.status.ErrorCount++
	m.statusMu.Unlock()
}

// clearError clears the error status
func (m *AccountMonitor) clearError() {
	m.statusMu.Lock()
	m.status.LastError = ""
	m.status.ErrorCount = 0
	m.statusMu.Unlock()
}

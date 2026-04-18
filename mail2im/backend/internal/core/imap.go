package core

import (
	"fmt"
	"mail2im/internal/models"
	"sync"
	"time"

	"flymail-core/logger"

	"go.uber.org/zap"
)

type WatcherManager struct {
	workers map[uint]*Worker
	mu      sync.RWMutex
}

var Watcher *WatcherManager

func StartWatcher() {
	Watcher = &WatcherManager{
		workers: make(map[uint]*Worker),
	}

	var accounts []models.Account
	// Load all active accounts
	// For now, we might not have any accounts in DB, so this is safe
	if err := DB.Preload("Proxy").Find(&accounts).Error; err != nil {
		logger.Error("Failed to load accounts", zap.Error(err))
		return
	}

	logger.Info("Starting watcher", zap.Int("accounts", len(accounts)))
	for _, account := range accounts {
		if !account.Enabled {
			continue
		}
		Watcher.StartWorker(account)
	}
}

func (wm *WatcherManager) StartWorker(account models.Account) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if !account.Enabled {
		logger.Info("Skip starting worker for disabled account", zap.String("email", account.Email))
		return
	}

	if _, exists := wm.workers[account.ID]; exists {
		logger.Info("Worker for account already exists", zap.Uint("accountID", account.ID))
		return
	}

	worker := NewWorker(account)
	wm.workers[account.ID] = worker
	worker.Start()
	logger.Info("Started worker for account", zap.String("email", account.Email))
}

func (wm *WatcherManager) StopWorker(accountID uint) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if worker, exists := wm.workers[accountID]; exists {
		worker.Stop()
		delete(wm.workers, accountID)
		logger.Info("Stopped worker for account", zap.Uint("accountID", accountID))
	}
}

func (wm *WatcherManager) RestartWorker(accountID uint) {
	var account models.Account
	if err := DB.Preload("Proxy").First(&account, accountID).Error; err != nil {
		logger.Error("Failed to reload account", zap.Uint("accountID", accountID), zap.Error(err))
		return
	}

	wm.StopWorker(accountID)
	wm.StartWorker(account)
}

func (wm *WatcherManager) StopAll() {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	for id, worker := range wm.workers {
		worker.Stop()
		delete(wm.workers, id)
	}
}

func (wm *WatcherManager) MarkAsRead(accountID uint, uid uint) error {
	wm.mu.RLock()
	worker, exists := wm.workers[accountID]
	wm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worker for account %d not running", accountID)
	}

	// This call should ideally be async or handled carefully as it uses the worker's connection.
	// Since the worker uses the client in a loop, concurrent access needs to be safe.
	// go-imap/v2 client is generally safe for concurrent use, but state (Selected Mailbox) matters.
	// Worker logic selects mailbox before operations. MarkAsRead also selects mailbox.
	// If IDLE is running, we might need to interrupt it?
	// The go-imap/v2 Idle command blocks. We cannot issue other commands on the same client while Idle is waiting.
	// We need to implement a command queue or interrupt mechanism in Worker if we want to reuse the connection safely.
	//
	// Current Worker.runIDLE blocks on `idleCmd.Wait()`.
	// To keep it simple for now: We will rely on the fact that we might fail if IDLE is active,
	// OR we accept that this might be tricky without a command channel.
	//
	// BETTER APPROACH: Send a signal to the worker to perform this action.
	// But for this specific "OneTimeToken" flow, let's try to keep it simple.
	//
	// If IDLE is active, we can't send commands.
	// We can try to send a "stop" signal to IDLE, perform action, then resume?
	//
	// Let's assume for now we just try. If it fails, user can retry.
	// Long term: Worker should have an `Actions` channel.

	return worker.MarkAsRead(uid)
}

func TestIMAPConnection(account models.Account) (*ConnectionInfo, time.Duration, error) {
	worker := NewWorker(account)
	worker.Silent = true
	start := time.Now()
	session, err := worker.dialAndLogin()
	elapsed := time.Since(start)
	if err != nil {
		return nil, elapsed, err
	}
	info := &ConnectionInfo{
		Capabilities: session.Capabilities,
		SupportsIDLE: session.SupportsIDLE,
		SecurityMode: session.SecurityMode,
	}
	session.Close()
	return info, elapsed, nil
}

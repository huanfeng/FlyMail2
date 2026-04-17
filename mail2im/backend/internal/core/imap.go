package core

import (
	"fmt"
	"log"
	"mail2im/internal/models"
	"sync"
	"time"
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
		log.Printf("Failed to load accounts: %v", err)
		return
	}

	log.Printf("Starting watcher with %d accounts", len(accounts))
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
		log.Printf("Skip starting worker for disabled account: %s", account.Email)
		return
	}

	if _, exists := wm.workers[account.ID]; exists {
		log.Printf("Worker for account %d already exists", account.ID)
		return
	}

	worker := NewWorker(account)
	wm.workers[account.ID] = worker
	worker.Start()
	log.Printf("Started worker for account: %s", account.Email)
}

func (wm *WatcherManager) StopWorker(accountID uint) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if worker, exists := wm.workers[accountID]; exists {
		worker.Stop()
		delete(wm.workers, accountID)
		log.Printf("Stopped worker for account ID: %d", accountID)
	}
}

func (wm *WatcherManager) RestartWorker(accountID uint) {
	var account models.Account
	if err := DB.Preload("Proxy").First(&account, accountID).Error; err != nil {
		log.Printf("Failed to reload account %d: %v", accountID, err)
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
	conn, info, err := worker.dialAndLogin()
	if conn != nil {
		conn.Logout()
	}
	return info, time.Since(start), err
}

package core

import (
	"sync"
	"time"
)

type WorkerState string

const (
	StateDisconnected WorkerState = "disconnected"
	StateConnecting   WorkerState = "connecting"
	StateIDLE         WorkerState = "idle"
	StatePolling      WorkerState = "polling"
	StateError        WorkerState = "error"
)

type WorkerStatus struct {
	AccountID   uint        `json:"account_id"`
	Email       string      `json:"email"`
	State       WorkerState `json:"state"`
	LastMessage string      `json:"last_message"`
	LastUpdate  time.Time   `json:"last_update"`
	Logs        []LogEntry  `json:"logs"` // Keep last 50 logs
}

type LogEntry struct {
	Time    time.Time   `json:"time"`
	Level   string      `json:"level"`
	State   WorkerState `json:"state"`
	Message string      `json:"message"`
}

type DebugService struct {
	workers map[uint]*WorkerStatus
	mu      sync.RWMutex
}

var Debug *DebugService

func InitDebugService() {
	Debug = &DebugService{
		workers: make(map[uint]*WorkerStatus),
	}
}

func (d *DebugService) UpdateWorkerStatus(accountID uint, email string, state WorkerState, msg string) {
	d.Log(accountID, email, "info", state, msg)
}

func (d *DebugService) Log(accountID uint, email string, level string, state WorkerState, msg string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	w, ok := d.workers[accountID]
	if !ok {
		w = &WorkerStatus{
			AccountID: accountID,
			Email:     email,
			Logs:      make([]LogEntry, 0, 50),
		}
		d.workers[accountID] = w
	}

	w.State = state
	w.LastMessage = msg
	w.LastUpdate = time.Now()

	// Append log
	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		State:   state,
		Message: msg,
	}
	if len(w.Logs) >= 50 {
		w.Logs = w.Logs[1:]
	}
	w.Logs = append(w.Logs, entry)
}

func (d *DebugService) GetAllStatuses() []*WorkerStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	list := make([]*WorkerStatus, 0, len(d.workers))
	for _, w := range d.workers {
		list = append(list, w)
	}
	return list
}

func (d *DebugService) GetWorkerStatus(accountID uint) *WorkerStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.workers[accountID]
}

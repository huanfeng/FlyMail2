package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"flymail-core/logger"
)

// Manager interface for task management
type Manager interface {
	// Registration
	RegisterHandler(handler Handler)

	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Task operations
	CreateTask(ctx context.Context, config *Config) error
	UpdateTask(ctx context.Context, config *Config) error
	DeleteTask(ctx context.Context, taskID string) error
	GetTask(ctx context.Context, taskID string) (*Config, error)
	ListTasks(ctx context.Context) ([]*Config, error)

	// Execution
	ExecuteTask(ctx context.Context, taskID string) error
	ExecuteTaskWithConfig(ctx context.Context, config *Config) error

	// Monitoring
	GetTaskStatus(ctx context.Context, taskID string) (*Config, error)
	GetTaskLogs(ctx context.Context, taskID string, limit int) ([]*Log, error)
	Subscribe() <-chan Event
	Unsubscribe(ch <-chan Event)

	// Statistics
	GetStats() Stats
}

// Stats represents task manager statistics
type Stats struct {
	TotalTasks     int
	RunningTasks   int
	PendingTasks   int
	CompletedTasks int64
	FailedTasks    int64
	Workers        int
	QueueSize      int
}

// manager implements Manager interface
type manager struct {
	configRepo  ConfigRepository
	logRepo     LogRepository
	handlers    map[string]Handler
	queue       *Queue
	cron        *cron.Cron
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	mu          sync.RWMutex
	running     map[string]bool // Track running tasks
	workers     int
	eventChan   chan Event
	subscribers []chan Event
	subMu       sync.RWMutex

	// Stats
	completedTasks int64
	failedTasks    int64
}

// NewManager creates a new task manager
func NewManager(configRepo ConfigRepository, logRepo LogRepository, workers int) Manager {
	ctx, cancel := context.WithCancel(context.Background())

	return &manager{
		configRepo:  configRepo,
		logRepo:     logRepo,
		handlers:    make(map[string]Handler),
		queue:       NewQueue(),
		cron:        cron.New(cron.WithSeconds()),
		ctx:         ctx,
		cancel:      cancel,
		running:     make(map[string]bool),
		workers:     workers,
		eventChan:   make(chan Event, 100),
		subscribers: make([]chan Event, 0),
	}
}

// RegisterHandler registers a task handler
func (m *manager) RegisterHandler(handler Handler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[handler.TaskName()] = handler
}

// Start starts the task manager
func (m *manager) Start(ctx context.Context) error {
	logger.Info("Starting task manager", zap.Int("workers", m.workers))

	// Start event dispatcher
	m.wg.Add(1)
	go m.eventDispatcher()

	// Start workers
	for i := 0; i < m.workers; i++ {
		m.wg.Add(1)
		go m.worker(i)
	}

	// Start cron scheduler
	m.cron.Start()

	// Load and schedule existing tasks
	if err := m.loadAndScheduleTasks(); err != nil {
		return fmt.Errorf("failed to load tasks: %w", err)
	}

	// Start periodic scanner
	m.wg.Add(1)
	go m.periodicScanner()

	logger.Info("Task manager started successfully")
	return nil
}

// Stop stops the task manager
func (m *manager) Stop(ctx context.Context) error {
	logger.Info("Stopping task manager")

	// Stop cron scheduler
	cronCtx := m.cron.Stop()
	<-cronCtx.Done()

	// Cancel context
	m.cancel()

	// Close queue
	m.queue.Close()

	// Wait for workers to finish
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("Task manager stopped successfully")
		return nil
	case <-ctx.Done():
		logger.Warn("Task manager stop timeout")
		return ctx.Err()
	}
}

// CreateTask creates a new task
func (m *manager) CreateTask(ctx context.Context, config *Config) error {
	// Validate task type
	switch config.TaskType {
	case TypeLoop:
		if config.LoopInterval <= 0 {
			return fmt.Errorf("loop interval must be positive")
		}
	case TypeScheduled:
		if _, err := cron.ParseStandard(config.CronExpression); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	case TypeOnce:
		if config.ExecuteAt == nil || config.ExecuteAt.Before(time.Now()) {
			return fmt.Errorf("execute_at must be in the future")
		}
	default:
		return ErrInvalidTaskType
	}

	// Generate task ID if not provided
	if config.TaskID == "" {
		config.TaskID = fmt.Sprintf("%s_%d", config.TaskName, time.Now().Unix())
	}

	// Create in database
	if err := m.configRepo.Create(ctx, config); err != nil {
		return err
	}

	// Schedule if active
	if config.IsActive {
		if err := m.scheduleTask(config); err != nil {
			logger.Error("Failed to schedule task", zap.String("task_id", config.TaskID), zap.Error(err))
		}
	}

	// Send event
	m.sendEvent(Event{
		Type:      "task_created",
		TaskID:    config.TaskID,
		TaskName:  config.TaskName,
		Status:    StatusPending,
		Timestamp: time.Now(),
	})

	return nil
}

// UpdateTask updates an existing task
func (m *manager) UpdateTask(ctx context.Context, config *Config) error {
	// Get existing task
	existing, err := m.configRepo.GetByTaskID(ctx, config.TaskID)
	if err != nil {
		return err
	}

	// Stop existing scheduled task
	m.unscheduleTask(existing.TaskID)

	// Update in database
	config.ID = existing.ID
	if err := m.configRepo.Update(ctx, config); err != nil {
		return err
	}

	// Reschedule if active
	if config.IsActive {
		if err := m.scheduleTask(config); err != nil {
			logger.Error("Failed to schedule task", zap.String("task_id", config.TaskID), zap.Error(err))
		}
	}

	// Send event
	m.sendEvent(Event{
		Type:      "task_updated",
		TaskID:    config.TaskID,
		TaskName:  config.TaskName,
		Status:    StatusPending,
		Timestamp: time.Now(),
	})

	return nil
}

// DeleteTask deletes a task
func (m *manager) DeleteTask(ctx context.Context, taskID string) error {
	// Get task
	config, err := m.configRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}

	// Stop scheduled task
	m.unscheduleTask(taskID)

	// Delete from database
	if err := m.configRepo.Delete(ctx, config.ID); err != nil {
		return err
	}

	// Send event
	m.sendEvent(Event{
		Type:      "task_deleted",
		TaskID:    taskID,
		TaskName:  config.TaskName,
		Timestamp: time.Now(),
	})

	return nil
}

// GetTask gets a task by ID
func (m *manager) GetTask(ctx context.Context, taskID string) (*Config, error) {
	return m.configRepo.GetByTaskID(ctx, taskID)
}

// ListTasks lists all tasks
func (m *manager) ListTasks(ctx context.Context) ([]*Config, error) {
	return m.configRepo.GetAll(ctx)
}

// ExecuteTask executes a task by ID
func (m *manager) ExecuteTask(ctx context.Context, taskID string) error {
	config, err := m.configRepo.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}

	return m.ExecuteTaskWithConfig(ctx, config)
}

// ExecuteTaskWithConfig executes a task with given config
func (m *manager) ExecuteTaskWithConfig(ctx context.Context, config *Config) error {
	// Check if handler exists
	m.mu.RLock()
	_, exists := m.handlers[config.TaskName]
	m.mu.RUnlock()

	if !exists {
		return ErrHandlerNotFound
	}

	// Check if already running
	m.mu.Lock()
	if m.running[config.TaskID] {
		m.mu.Unlock()
		return ErrTaskAlreadyRunning
	}
	m.running[config.TaskID] = true
	m.mu.Unlock()

	// Create task
	task := &Task{
		TaskType:    config.TaskType,
		TaskName:    config.TaskName,
		TaskID:      config.TaskID,
		ExtraConfig: config.ExtraConfig,
		Priority:    config.Priority,
		UserID:      config.UserID,
		ConfigID:    config.ID,
	}

	// Add to queue
	return m.queue.Push(task)
}

// GetTaskStatus gets task status
func (m *manager) GetTaskStatus(ctx context.Context, taskID string) (*Config, error) {
	return m.configRepo.GetByTaskID(ctx, taskID)
}

// GetTaskLogs gets task execution logs
func (m *manager) GetTaskLogs(ctx context.Context, taskID string, limit int) ([]*Log, error) {
	return m.logRepo.GetByTaskID(ctx, taskID, limit)
}

// Subscribe subscribes to task events
func (m *manager) Subscribe() <-chan Event {
	m.subMu.Lock()
	defer m.subMu.Unlock()

	ch := make(chan Event, 10)
	m.subscribers = append(m.subscribers, ch)
	return ch
}

// Unsubscribe unsubscribes from task events
func (m *manager) Unsubscribe(ch <-chan Event) {
	m.subMu.Lock()
	defer m.subMu.Unlock()

	for i, sub := range m.subscribers {
		if sub == ch {
			close(sub)
			m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
			break
		}
	}
}

// GetStats returns task manager statistics
func (m *manager) GetStats() Stats {
	m.mu.RLock()
	runningCount := len(m.running)
	m.mu.RUnlock()

	return Stats{
		TotalTasks:     0, // Would need to query database
		RunningTasks:   runningCount,
		PendingTasks:   m.queue.Size(),
		CompletedTasks: m.completedTasks,
		FailedTasks:    m.failedTasks,
		Workers:        m.workers,
		QueueSize:      m.queue.Size(),
	}
}

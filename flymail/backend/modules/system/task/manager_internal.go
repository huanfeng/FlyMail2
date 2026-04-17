package task

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"flymail-core/logger"
)

// worker processes tasks from the queue
func (m *manager) worker(id int) {
	defer m.wg.Done()
	logger.Info("Task worker started", zap.Int("worker_id", id))

	for {
		select {
		case <-m.ctx.Done():
			logger.Info("Task worker stopping", zap.Int("worker_id", id))
			return
		default:
			// Pop task from queue with timeout
			task, err := m.queue.Pop(time.Second)
			if err != nil {
				if err == ErrQueueClosed {
					logger.Info("Task queue closed", zap.Int("worker_id", id))
					return
				}
				continue
			}

			// Execute task
			m.executeTask(task)
		}
	}
}

// executeTask executes a single task
func (m *manager) executeTask(task *Task) {
	logger.Info("Executing task",
		zap.String("task_id", task.TaskID),
		zap.String("task_name", task.TaskName))

	// Create log entry
	log := &Log{
		UserID:    task.UserID,
		TaskType:  task.TaskType,
		TaskName:  task.TaskName,
		TaskID:    task.TaskID,
		Status:    StatusRunning,
		StartTime: time.Now(),
	}

	// Update task status to running
	now := time.Now()
	if err := m.configRepo.UpdateStatus(m.ctx, task.TaskID, true, StatusRunning, &now); err != nil {
		logger.Error("Failed to update task status", zap.Error(err))
	}

	// Send running event
	m.sendEvent(Event{
		Type:      "task_started",
		TaskID:    task.TaskID,
		TaskName:  task.TaskName,
		Status:    StatusRunning,
		Timestamp: time.Now(),
	})

	// Get handler
	m.mu.RLock()
	handler, exists := m.handlers[task.TaskName]
	m.mu.RUnlock()

	if !exists {
		log.Status = StatusFailed
		log.ErrorMessage = "handler not found"
		m.finishTask(task, log, fmt.Errorf("handler not found"))
		return
	}

	// Create context with timeout
	timeout := 5 * time.Minute // Default timeout
	if config, err := m.configRepo.GetByTaskID(m.ctx, task.TaskID); err == nil && config.Timeout > 0 {
		timeout = time.Duration(config.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(m.ctx, timeout)
	defer cancel()

	// Execute handler
	err := handler.Execute(ctx, task)
	endTime := time.Now()
	log.EndTime = &endTime
	log.Duration = endTime.Sub(log.StartTime).Milliseconds()

	if err != nil {
		log.Status = StatusFailed
		log.ErrorMessage = err.Error()
		m.finishTask(task, log, err)
		return
	}

	log.Status = StatusCompleted
	m.finishTask(task, log, nil)
}

// finishTask completes task execution
func (m *manager) finishTask(task *Task, log *Log, err error) {
	// Clear running flag
	m.mu.Lock()
	delete(m.running, task.TaskID)
	m.mu.Unlock()

	// Save log
	if err := m.logRepo.Create(m.ctx, log); err != nil {
		logger.Error("Failed to save task log", zap.Error(err))
	}

	// Update task status
	now := time.Now()
	if err := m.configRepo.UpdateStatus(m.ctx, task.TaskID, false, log.Status, &now); err != nil {
		logger.Error("Failed to update task status", zap.Error(err))
	}

	// Update stats
	if log.Status == StatusCompleted {
		atomic.AddInt64(&m.completedTasks, 1)
	} else {
		atomic.AddInt64(&m.failedTasks, 1)
	}

	// Send completion event
	m.sendEvent(Event{
		Type:      "task_completed",
		TaskID:    task.TaskID,
		TaskName:  task.TaskName,
		Status:    log.Status,
		Message:   log.ErrorMessage,
		Timestamp: time.Now(),
	})

	// Reschedule if needed
	if task.TaskType == TypeLoop && log.Status == StatusCompleted {
		config, err := m.configRepo.GetByTaskID(m.ctx, task.TaskID)
		if err == nil && config.IsActive {
			// Schedule next run
			go func() {
				time.Sleep(time.Duration(config.LoopInterval) * time.Second)
				if err := m.ExecuteTaskWithConfig(m.ctx, config); err != nil {
					logger.Error("Failed to reschedule loop task", zap.Error(err))
				}
			}()
		}
	}
}

// loadAndScheduleTasks loads and schedules all active tasks
func (m *manager) loadAndScheduleTasks() error {
	configs, err := m.configRepo.GetAll(m.ctx)
	if err != nil {
		return err
	}

	for _, config := range configs {
		if !config.IsActive {
			continue
		}

		if err := m.scheduleTask(config); err != nil {
			logger.Error("Failed to schedule task",
				zap.String("task_id", config.TaskID),
				zap.Error(err))
		}
	}

	return nil
}

// scheduleTask schedules a task based on its type
func (m *manager) scheduleTask(config *Config) error {
	switch config.TaskType {
	case TypeScheduled:
		return m.scheduleCronTask(config)
	case TypeLoop:
		return m.scheduleLoopTask(config)
	case TypeOnce:
		return m.scheduleOnceTask(config)
	default:
		return ErrInvalidTaskType
	}
}

// scheduleCronTask schedules a cron task
func (m *manager) scheduleCronTask(config *Config) error {
	_, err := m.cron.AddFunc(config.CronExpression, func() {
		if err := m.ExecuteTaskWithConfig(m.ctx, config); err != nil {
			logger.Error("Failed to execute scheduled task", zap.Error(err))
		}
	})

	if err != nil {
		return err
	}

	// Store entry ID for later removal
	m.mu.Lock()
	// Note: In production, you'd want to store the entryID mapping
	m.mu.Unlock()

	return nil
}

// scheduleLoopTask schedules a loop task
func (m *manager) scheduleLoopTask(config *Config) error {
	// Execute immediately
	return m.ExecuteTaskWithConfig(m.ctx, config)
}

// scheduleOnceTask schedules a one-time task
func (m *manager) scheduleOnceTask(config *Config) error {
	if config.ExecuteAt == nil {
		return fmt.Errorf("execute_at is required for once tasks")
	}

	// Calculate delay
	delay := time.Until(*config.ExecuteAt)
	if delay < 0 {
		// Execute immediately if past due
		return m.ExecuteTaskWithConfig(m.ctx, config)
	}

	// Schedule execution
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
			if err := m.ExecuteTaskWithConfig(m.ctx, config); err != nil {
				logger.Error("Failed to execute once task", zap.Error(err))
			}
		case <-m.ctx.Done():
			return
		}
	}()

	return nil
}

// unscheduleTask removes a scheduled task
func (m *manager) unscheduleTask(taskID string) {
	// In production, you'd look up and remove the cron entry
	// For now, this is a placeholder
}

// periodicScanner scans for tasks that need to be executed
func (m *manager) periodicScanner() {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.scanPendingTasks()
		}
	}
}

// scanPendingTasks scans for pending tasks
func (m *manager) scanPendingTasks() {
	// Check for pending once tasks
	tasks, err := m.configRepo.GetPendingOnceTasks(m.ctx)
	if err != nil {
		logger.Error("Failed to get pending tasks", zap.Error(err))
		return
	}

	for _, config := range tasks {
		if err := m.ExecuteTaskWithConfig(m.ctx, config); err != nil {
			logger.Error("Failed to execute pending task",
				zap.String("task_id", config.TaskID),
				zap.Error(err))
		}
	}
}

// eventDispatcher dispatches events to subscribers
func (m *manager) eventDispatcher() {
	defer m.wg.Done()

	for {
		select {
		case <-m.ctx.Done():
			return
		case event := <-m.eventChan:
			m.subMu.RLock()
			for _, ch := range m.subscribers {
				select {
				case ch <- event:
				default:
					// Skip if subscriber is slow
				}
			}
			m.subMu.RUnlock()
		}
	}
}

// sendEvent sends an event
func (m *manager) sendEvent(event Event) {
	select {
	case m.eventChan <- event:
	default:
		// Skip if event channel is full
	}
}

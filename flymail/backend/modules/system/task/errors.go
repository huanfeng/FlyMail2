package task

import "errors"

// Common errors
var (
	ErrQueueClosed        = errors.New("task queue is closed")
	ErrTimeout            = errors.New("operation timed out")
	ErrTaskNotFound       = errors.New("task not found")
	ErrHandlerNotFound    = errors.New("task handler not found")
	ErrTaskAlreadyRunning = errors.New("task is already running")
	ErrInvalidTaskType    = errors.New("invalid task type")
	ErrInvalidCron        = errors.New("invalid cron expression")
)

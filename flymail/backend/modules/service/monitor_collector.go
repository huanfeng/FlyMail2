package service

import (
	"sync"
	"time"
)

// MonitorCollector collects real-time monitoring data
type MonitorCollector struct {
	mu sync.RWMutex

	// 任务状态
	taskStatus TaskStatus

	// 错误记录（环形缓冲区）
	recentErrors []ErrorRecord
	errorIndex   int

	// 请求统计
	requestCount     int64
	requestStartTime time.Time

	// 活跃操作
	activeOperations map[string]int
}

// TaskStatus represents task queue status
type TaskStatus struct {
	Running   int
	Pending   int
	Completed int64
	Failed    int
}

// ErrorRecord represents an error record
type ErrorRecord struct {
	Timestamp time.Time
	Operation string
	Error     string
	Count     int
}

// NewMonitorCollector creates a new monitor collector
func NewMonitorCollector() *MonitorCollector {
	return &MonitorCollector{
		recentErrors:     make([]ErrorRecord, 100), // 保存最近100个错误
		activeOperations: make(map[string]int),
		requestStartTime: time.Now(),
	}
}

// UpdateTaskStatus updates task status
func (c *MonitorCollector) UpdateTaskStatus(running, pending, failed int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.taskStatus.Running = running
	c.taskStatus.Pending = pending
	c.taskStatus.Failed = failed
}

// IncrementTaskCompleted increments completed task count
func (c *MonitorCollector) IncrementTaskCompleted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.taskStatus.Completed++
}

// GetTaskStatus returns current task status
func (c *MonitorCollector) GetTaskStatus() TaskStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.taskStatus
}

// RecordError records an error
func (c *MonitorCollector) RecordError(operation string, err error) {
	if err == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// 查找是否已有相同的错误记录
	errStr := err.Error()
	for i := range c.recentErrors {
		if c.recentErrors[i].Operation == operation && c.recentErrors[i].Error == errStr &&
			time.Since(c.recentErrors[i].Timestamp) < 5*time.Minute {
			// 如果是5分钟内的相同错误，增加计数
			c.recentErrors[i].Count++
			return
		}
	}

	// 添加新的错误记录
	c.recentErrors[c.errorIndex] = ErrorRecord{
		Timestamp: time.Now(),
		Operation: operation,
		Error:     errStr,
		Count:     1,
	}
	c.errorIndex = (c.errorIndex + 1) % len(c.recentErrors)
}

// GetRecentErrors returns recent errors
func (c *MonitorCollector) GetRecentErrors(limit int) []ErrorRecord {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := []ErrorRecord{}
	count := 0

	// 从最新的错误开始返回
	for i := 0; i < len(c.recentErrors) && count < limit; i++ {
		idx := (c.errorIndex - 1 - i + len(c.recentErrors)) % len(c.recentErrors)
		if c.recentErrors[idx].Timestamp.IsZero() {
			continue
		}
		result = append(result, c.recentErrors[idx])
		count++
	}

	return result
}

// IncrementRequestCount increments request count
func (c *MonitorCollector) IncrementRequestCount() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requestCount++
}

// GetRequestRate returns requests per second
func (c *MonitorCollector) GetRequestRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	duration := time.Since(c.requestStartTime).Seconds()
	if duration == 0 {
		return 0
	}
	return float64(c.requestCount) / duration
}

// StartOperation marks an operation as started
func (c *MonitorCollector) StartOperation(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activeOperations[name]++
}

// EndOperation marks an operation as ended
func (c *MonitorCollector) EndOperation(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if count, exists := c.activeOperations[name]; exists && count > 0 {
		c.activeOperations[name]--
		if c.activeOperations[name] == 0 {
			delete(c.activeOperations, name)
		}
	}
}

// GetActiveOperations returns active operations count
func (c *MonitorCollector) GetActiveOperations() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]int)
	for k, v := range c.activeOperations {
		result[k] = v
	}
	return result
}

package monitor

import (
	"sync"
	"sync/atomic"
	"time"
)

// Collector collects real-time monitoring data
type Collector struct {
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

	// 活跃连接数（SSE等）
	activeConnections int32
}

// NewCollector creates a new monitor collector
func NewCollector() *Collector {
	return &Collector{
		recentErrors:     make([]ErrorRecord, 100), // 保存最近100个错误
		activeOperations: make(map[string]int),
		requestStartTime: time.Now(),
	}
}

// UpdateTaskStatus updates task status
func (c *Collector) UpdateTaskStatus(running, pending, failed int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.taskStatus.Running = running
	c.taskStatus.Pending = pending
	c.taskStatus.Failed = failed
}

// IncrementTaskCompleted increments completed task count
func (c *Collector) IncrementTaskCompleted() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.taskStatus.Completed++
}

// GetTaskStatus returns current task status
func (c *Collector) GetTaskStatus() TaskStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.taskStatus
}

// RecordError records an error
func (c *Collector) RecordError(operation string, err error) {
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
func (c *Collector) GetRecentErrors(limit int) []ErrorInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var errors []ErrorInfo
	now := time.Now()

	// 从最新的错误开始返回
	for i := 0; i < len(c.recentErrors) && len(errors) < limit; i++ {
		idx := (c.errorIndex - 1 - i + len(c.recentErrors)) % len(c.recentErrors)
		err := c.recentErrors[idx]
		if err.Operation != "" && now.Sub(err.Timestamp) < 30*time.Minute {
			errors = append(errors, ErrorInfo{
				Timestamp: err.Timestamp.Unix(),
				Operation: err.Operation,
				Error:     err.Error,
				Count:     err.Count,
			})
		}
	}

	return errors
}

// IncrementRequest increments request counter
func (c *Collector) IncrementRequest() {
	atomic.AddInt64(&c.requestCount, 1)
}

// GetRequestRate returns the request rate (requests per second)
func (c *Collector) GetRequestRate() float64 {
	count := atomic.LoadInt64(&c.requestCount)
	duration := time.Since(c.requestStartTime).Seconds()
	if duration < 1 {
		duration = 1
	}
	return float64(count) / duration
}

// StartOperation records the start of an operation
func (c *Collector) StartOperation(operation string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.activeOperations[operation]++
}

// EndOperation records the end of an operation
func (c *Collector) EndOperation(operation string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if count, exists := c.activeOperations[operation]; exists && count > 0 {
		c.activeOperations[operation]--
		if c.activeOperations[operation] == 0 {
			delete(c.activeOperations, operation)
		}
	}
}

// GetActiveOperations returns a copy of active operations
func (c *Collector) GetActiveOperations() map[string]int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	operations := make(map[string]int)
	for k, v := range c.activeOperations {
		operations[k] = v
	}
	return operations
}

// IncrementActiveConnections increments active connection count
func (c *Collector) IncrementActiveConnections() {
	atomic.AddInt32(&c.activeConnections, 1)
}

// DecrementActiveConnections decrements active connection count
func (c *Collector) DecrementActiveConnections() {
	atomic.AddInt32(&c.activeConnections, -1)
}

// GetActiveConnections returns the number of active connections
func (c *Collector) GetActiveConnections() int {
	return int(atomic.LoadInt32(&c.activeConnections))
}

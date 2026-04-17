package monitor

import "time"

// SystemStatus represents system-level status
type SystemStatus struct {
	Uptime     int64          `json:"uptime"`     // 运行时长（秒）
	Memory     MemoryStatus   `json:"memory"`     // 内存状态
	GC         GCStatus       `json:"gc"`         // GC状态
	Goroutines int            `json:"goroutines"` // Goroutine数量
	Database   DatabaseStatus `json:"database"`   // 数据库状态
}

// MemoryStatus represents memory usage status
type MemoryStatus struct {
	Used         uint64  `json:"used"`          // 已使用内存（字节）
	Total        uint64  `json:"total"`         // 系统分配的总内存（字节）
	HeapAlloc    uint64  `json:"heap_alloc"`    // 堆内存分配（字节）
	HeapInuse    uint64  `json:"heap_inuse"`    // 堆内存使用（字节）
	UsagePercent float64 `json:"usage_percent"` // 内存使用百分比
}

// GCStatus represents garbage collection status
type GCStatus struct {
	NumGC      uint32 `json:"num_gc"`         // GC运行次数
	LastGC     int64  `json:"last_gc"`        // 上次GC时间（Unix时间戳）
	PauseTotal uint64 `json:"pause_total_ns"` // GC总暂停时间（纳秒）
	PauseAvg   uint64 `json:"pause_avg_ns"`   // 平均GC暂停时间（纳秒）
}

// DatabaseStatus represents database connection status
type DatabaseStatus struct {
	Connected       bool `json:"connected"`        // 是否连接
	OpenConnections int  `json:"open_connections"` // 打开的连接数
	InUse           int  `json:"in_use"`           // 正在使用的连接数
	Idle            int  `json:"idle"`             // 空闲连接数
	MaxOpen         int  `json:"max_open"`         // 最大连接数
}

// ServiceHealth represents health status of a service
type ServiceHealth struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message,omitempty"`
}

// HealthStatus represents overall health status
type HealthStatus struct {
	Healthy  bool                     `json:"healthy"`
	Services map[string]ServiceHealth `json:"services"`
	Uptime   int64                    `json:"uptime"` // 运行时长（秒）
}

// RealtimeStatus represents real-time monitoring status
type RealtimeStatus struct {
	ActiveConnections int         `json:"active_connections"` // 活跃连接数
	RunningTasks      int         `json:"running_tasks"`      // 正在运行的任务数
	PendingTasks      int         `json:"pending_tasks"`      // 等待中的任务数
	RecentErrors      []ErrorInfo `json:"recent_errors"`      // 最近的错误信息
	RequestRate       float64     `json:"request_rate"`       // 请求速率（每秒）
}

// ErrorInfo represents error information
type ErrorInfo struct {
	Timestamp int64  `json:"timestamp"`
	Operation string `json:"operation"`
	Error     string `json:"error"`
	Count     int    `json:"count"`
}

// MonitorSummary represents a summary of all monitoring data
type MonitorSummary struct {
	Timestamp time.Time      `json:"timestamp"`
	System    SystemStatus   `json:"system"`
	Health    HealthStatus   `json:"health"`
	Realtime  RealtimeStatus `json:"realtime"`
}

// TaskStatus represents task queue status
type TaskStatus struct {
	Running   int   `json:"running"`
	Pending   int   `json:"pending"`
	Completed int64 `json:"completed"`
	Failed    int   `json:"failed"`
}

// ErrorRecord represents an error record
type ErrorRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Operation string    `json:"operation"`
	Error     string    `json:"error"`
	Count     int       `json:"count"`
}

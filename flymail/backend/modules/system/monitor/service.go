package monitor

import (
	"context"
	"runtime"
	"runtime/debug"
	"time"

	"gorm.io/gorm"
)

// Service interface for system monitoring operations
type Service interface {
	GetSystemStatus(ctx context.Context) (*SystemStatus, error)
	GetHealthStatus(ctx context.Context) (*HealthStatus, error)
	GetRealtimeStatus(ctx context.Context) (*RealtimeStatus, error)
	GetMonitorSummary(ctx context.Context) (*MonitorSummary, error)
	// Collector access
	GetCollector() *Collector
}

// Dependencies for dependency injection
type Dependencies struct {
	SSEHub interface {
		GetConnectionCount() int
	}
}

// service implements Service interface
type service struct {
	collector    *Collector
	db           *gorm.DB
	startTime    time.Time
	dependencies Dependencies
}

// NewService creates a new monitor service
func NewService(collector *Collector, db *gorm.DB) Service {
	return &service{
		collector: collector,
		db:        db,
		startTime: time.Now(),
	}
}

// SetDependencies sets optional dependencies
func (s *service) SetDependencies(deps Dependencies) {
	s.dependencies = deps
}

// GetCollector returns the collector instance
func (s *service) GetCollector() *Collector {
	return s.collector
}

// GetSystemStatus returns system-level status
func (s *service) GetSystemStatus(ctx context.Context) (*SystemStatus, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 获取 GC 统计信息
	gcStats := debug.GCStats{}
	debug.ReadGCStats(&gcStats)

	// 计算平均 GC 暂停时间
	var pauseAvg uint64
	if gcStats.NumGC > 0 {
		pauseAvg = uint64(gcStats.PauseTotal.Nanoseconds()) / uint64(gcStats.NumGC)
	}

	// 获取数据库状态
	dbStatus := s.getDatabaseStatus()

	// 计算内存使用百分比
	var memUsagePercent float64
	if m.Sys > 0 {
		memUsagePercent = float64(m.HeapAlloc) / float64(m.Sys) * 100
	}

	return &SystemStatus{
		Uptime: int64(time.Since(s.startTime).Seconds()),
		Memory: MemoryStatus{
			Used:         m.Alloc,
			Total:        m.Sys,
			HeapAlloc:    m.HeapAlloc,
			HeapInuse:    m.HeapInuse,
			UsagePercent: memUsagePercent,
		},
		GC: GCStatus{
			NumGC:      uint32(gcStats.NumGC),
			LastGC:     gcStats.LastGC.Unix(),
			PauseTotal: uint64(gcStats.PauseTotal.Nanoseconds()),
			PauseAvg:   pauseAvg,
		},
		Goroutines: runtime.NumGoroutine(),
		Database:   dbStatus,
	}, nil
}

// GetHealthStatus returns health status of various services
func (s *service) GetHealthStatus(ctx context.Context) (*HealthStatus, error) {
	services := make(map[string]ServiceHealth)
	allHealthy := true

	// 检查数据库健康状态
	dbHealth := s.checkDatabaseHealth()
	services["database"] = dbHealth
	if !dbHealth.Healthy {
		allHealthy = false
	}

	// 检查 SSE 服务健康状态
	if s.dependencies.SSEHub != nil {
		sseHealth := s.checkSSEHealth()
		services["sse"] = sseHealth
		if !sseHealth.Healthy {
			allHealthy = false
		}
	}

	// 检查邮件服务健康状态（基于最近的错误）
	emailHealth := s.checkEmailServiceHealth()
	services["email"] = emailHealth
	if !emailHealth.Healthy {
		allHealthy = false
	}

	return &HealthStatus{
		Healthy:  allHealthy,
		Services: services,
		Uptime:   int64(time.Since(s.startTime).Seconds()),
	}, nil
}

// GetRealtimeStatus returns real-time monitoring status
func (s *service) GetRealtimeStatus(ctx context.Context) (*RealtimeStatus, error) {
	taskStatus := s.collector.GetTaskStatus()

	// 获取活跃连接数
	activeConnections := s.collector.GetActiveConnections()
	if s.dependencies.SSEHub != nil {
		// 如果有SSE hub，使用其连接数
		activeConnections = s.dependencies.SSEHub.GetConnectionCount()
	}

	return &RealtimeStatus{
		ActiveConnections: activeConnections,
		RunningTasks:      taskStatus.Running,
		PendingTasks:      taskStatus.Pending,
		RecentErrors:      s.collector.GetRecentErrors(10),
		RequestRate:       s.collector.GetRequestRate(),
	}, nil
}

// GetMonitorSummary returns a summary of all monitoring data
func (s *service) GetMonitorSummary(ctx context.Context) (*MonitorSummary, error) {
	system, err := s.GetSystemStatus(ctx)
	if err != nil {
		return nil, err
	}

	health, err := s.GetHealthStatus(ctx)
	if err != nil {
		return nil, err
	}

	realtime, err := s.GetRealtimeStatus(ctx)
	if err != nil {
		return nil, err
	}

	return &MonitorSummary{
		Timestamp: time.Now(),
		System:    *system,
		Health:    *health,
		Realtime:  *realtime,
	}, nil
}

// getDatabaseStatus returns database connection status
func (s *service) getDatabaseStatus() DatabaseStatus {
	sqlDB, err := s.db.DB()
	if err != nil {
		return DatabaseStatus{Connected: false}
	}

	stats := sqlDB.Stats()
	return DatabaseStatus{
		Connected:       true,
		OpenConnections: stats.OpenConnections,
		InUse:           stats.InUse,
		Idle:            stats.Idle,
		MaxOpen:         stats.MaxOpenConnections,
	}
}

// checkDatabaseHealth checks database health
func (s *service) checkDatabaseHealth() ServiceHealth {
	sqlDB, err := s.db.DB()
	if err != nil {
		return ServiceHealth{
			Healthy: false,
			Message: "无法获取数据库连接: " + err.Error(),
		}
	}

	// 尝试 ping 数据库
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		return ServiceHealth{
			Healthy: false,
			Message: "数据库连接失败: " + err.Error(),
		}
	}

	// 检查连接池状态
	stats := sqlDB.Stats()
	if stats.OpenConnections == 0 {
		return ServiceHealth{
			Healthy: false,
			Message: "没有可用的数据库连接",
		}
	}

	return ServiceHealth{Healthy: true}
}

// checkSSEHealth checks SSE service health
func (s *service) checkSSEHealth() ServiceHealth {
	if s.dependencies.SSEHub == nil {
		return ServiceHealth{
			Healthy: false,
			Message: "SSE服务未初始化",
		}
	}

	// 简单检查，如果需要可以添加更多逻辑
	return ServiceHealth{Healthy: true}
}

// checkEmailServiceHealth checks email service health based on recent errors
func (s *service) checkEmailServiceHealth() ServiceHealth {
	errors := s.collector.GetRecentErrors(50)

	// 统计邮件相关的错误
	emailErrors := 0
	for _, err := range errors {
		if err.Operation == "email_send" || err.Operation == "email_sync" ||
			err.Operation == "imap_connect" || err.Operation == "smtp_connect" {
			emailErrors += err.Count
		}
	}

	// 如果最近有超过10个邮件错误，认为服务不健康
	if emailErrors > 10 {
		return ServiceHealth{
			Healthy: false,
			Message: "邮件服务出现多个错误",
		}
	}

	return ServiceHealth{Healthy: true}
}

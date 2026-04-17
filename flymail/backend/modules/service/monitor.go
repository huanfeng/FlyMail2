package service

import (
	"context"
	"runtime"
	"runtime/debug"
	"time"

	"gorm.io/gorm"

	"flymail/modules/realtime"
)

// monitorService implements the MonitorService interface
type monitorService struct {
	collector *MonitorCollector
	db        *gorm.DB
	sseHub    *realtime.Hub
	startTime time.Time
}

// NewMonitorService creates a new monitor service
func NewMonitorService(collector *MonitorCollector, db *gorm.DB, sseHub *realtime.Hub) MonitorService {
	return &monitorService{
		collector: collector,
		db:        db,
		sseHub:    sseHub,
		startTime: time.Now(),
	}
}

// GetSystemStatus returns system-level status
func (s *monitorService) GetSystemStatus(ctx context.Context) (*SystemStatus, error) {
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
func (s *monitorService) GetHealthStatus(ctx context.Context) (*HealthStatus, error) {
	services := make(map[string]ServiceHealth)
	allHealthy := true

	// 检查数据库健康状态
	dbHealth := s.checkDatabaseHealth()
	services["database"] = dbHealth
	if !dbHealth.Healthy {
		allHealthy = false
	}

	// 检查 SSE 服务健康状态
	sseHealth := s.checkSSEHealth()
	services["sse"] = sseHealth
	if !sseHealth.Healthy {
		allHealthy = false
	}

	// 检查邮件服务健康状态（基于最近的错误）
	emailHealth := s.checkEmailServiceHealth()
	services["email"] = emailHealth
	if !emailHealth.Healthy {
		allHealthy = false
	}

	// 检查任务服务健康状态
	taskHealth := s.checkTaskServiceHealth()
	services["task"] = taskHealth
	if !taskHealth.Healthy {
		allHealthy = false
	}

	return &HealthStatus{
		Healthy:  allHealthy,
		Services: services,
		Uptime:   int64(time.Since(s.startTime).Seconds()),
	}, nil
}

// GetRealtimeStatus returns real-time monitoring status
func (s *monitorService) GetRealtimeStatus(ctx context.Context) (*RealtimeStatus, error) {
	// 获取活跃连接数
	activeConnections := 0
	// TODO: 如果需要，可以在 SSE Hub 中添加获取客户端数量的方法

	// 获取任务状态
	runningTasks := 0
	pendingTasks := 0
	if s.collector != nil {
		taskStatus := s.collector.GetTaskStatus()
		runningTasks = taskStatus.Running
		pendingTasks = taskStatus.Pending
	}

	// 获取最近的错误
	recentErrors := []ErrorInfo{}
	if s.collector != nil {
		errors := s.collector.GetRecentErrors(10) // 获取最近10个错误
		for _, err := range errors {
			recentErrors = append(recentErrors, ErrorInfo{
				Timestamp: err.Timestamp.Unix(),
				Operation: err.Operation,
				Error:     err.Error,
				Count:     err.Count,
			})
		}
	}

	// 计算请求速率
	requestRate := 0.0
	if s.collector != nil {
		requestRate = s.collector.GetRequestRate()
	}

	return &RealtimeStatus{
		ActiveConnections: activeConnections,
		RunningTasks:      runningTasks,
		PendingTasks:      pendingTasks,
		RecentErrors:      recentErrors,
		RequestRate:       requestRate,
	}, nil
}

// GetMonitorSummary returns a summary of all monitoring data
func (s *monitorService) GetMonitorSummary(ctx context.Context) (*MonitorSummary, error) {
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

// getDatabaseStatus 获取数据库连接状态
func (s *monitorService) getDatabaseStatus() DatabaseStatus {
	status := DatabaseStatus{
		Connected: false,
	}

	if s.db == nil {
		return status
	}

	sqlDB, err := s.db.DB()
	if err != nil {
		return status
	}

	// 测试连接
	if err := sqlDB.Ping(); err == nil {
		status.Connected = true
	}

	// 获取连接统计
	stats := sqlDB.Stats()
	status.OpenConnections = stats.OpenConnections
	status.InUse = stats.InUse
	status.Idle = stats.Idle
	status.MaxOpen = stats.MaxOpenConnections

	return status
}

// checkDatabaseHealth 检查数据库健康状态
func (s *monitorService) checkDatabaseHealth() ServiceHealth {
	if s.db == nil {
		return ServiceHealth{
			Healthy: false,
			Message: "数据库连接未初始化",
		}
	}

	// 执行简单查询测试
	if err := s.db.Exec("SELECT 1").Error; err != nil {
		return ServiceHealth{
			Healthy: false,
			Message: "数据库查询失败: " + err.Error(),
		}
	}

	// 检查连接池状态
	sqlDB, err := s.db.DB()
	if err != nil {
		return ServiceHealth{
			Healthy: false,
			Message: "无法获取数据库连接: " + err.Error(),
		}
	}

	stats := sqlDB.Stats()
	if stats.OpenConnections == 0 {
		return ServiceHealth{
			Healthy: false,
			Message: "没有可用的数据库连接",
		}
	}

	return ServiceHealth{
		Healthy: true,
		Message: "数据库运行正常",
	}
}

// checkSSEHealth 检查 SSE 服务健康状态
func (s *monitorService) checkSSEHealth() ServiceHealth {
	if s.sseHub == nil {
		return ServiceHealth{
			Healthy: false,
			Message: "SSE 服务未初始化",
		}
	}

	return ServiceHealth{
		Healthy: true,
		Message: "SSE 服务运行正常",
	}
}

// checkEmailServiceHealth 检查邮件服务健康状态
func (s *monitorService) checkEmailServiceHealth() ServiceHealth {
	if s.collector == nil {
		return ServiceHealth{
			Healthy: true,
			Message: "监控收集器未初始化",
		}
	}

	// 获取最近的邮件服务错误
	recentErrors := s.collector.GetRecentErrors(100)
	emailErrors := 0
	for _, err := range recentErrors {
		if err.Operation == "email_sync" || err.Operation == "email_send" {
			emailErrors += err.Count
		}
	}

	// 如果最近有大量错误，认为服务不健康
	if emailErrors > 10 {
		return ServiceHealth{
			Healthy: false,
			Message: "邮件服务最近出现大量错误",
		}
	}

	return ServiceHealth{
		Healthy: true,
		Message: "邮件服务运行正常",
	}
}

// checkTaskServiceHealth 检查任务服务健康状态
func (s *monitorService) checkTaskServiceHealth() ServiceHealth {
	if s.collector == nil {
		return ServiceHealth{
			Healthy: true,
			Message: "监控收集器未初始化",
		}
	}

	taskStatus := s.collector.GetTaskStatus()

	// 如果有大量失败的任务，认为服务不健康
	if taskStatus.Failed > 10 {
		return ServiceHealth{
			Healthy: false,
			Message: "任务服务有大量失败任务",
		}
	}

	// 如果待处理任务积压过多，认为服务不健康
	if taskStatus.Pending > 100 {
		return ServiceHealth{
			Healthy: false,
			Message: "任务队列积压过多",
		}
	}

	return ServiceHealth{
		Healthy: true,
		Message: "任务服务运行正常",
	}
}

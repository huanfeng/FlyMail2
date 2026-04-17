import { api } from '../axios'
import { API_ENDPOINTS } from '../config'
import type {
  MonitorStatus,
  HealthStatus,
  SystemMetrics,
  BusinessMetrics,
  RealtimeStatus
} from '../types'
import { ApiError } from '../ApiError'

class MonitoringService {
  /**
   * Get Prometheus metrics (returns plain text)
   */
  async getMetrics(): Promise<string> {
    const response = await fetch(
      `${API_ENDPOINTS.MONITOR.METRICS}`,
      {
        method: 'GET',
        headers: {
          'Accept': 'text/plain'
        }
      }
    )
    
    if (response.ok) {
      return response.text()
    }
    
    throw new Error('获取监控指标失败')
  }

  /**
   * Get system status in JSON format
   */
  async getStatus() {
    const response = await api.get<{
      timestamp: string
      system: SystemMetrics
      business: BusinessMetrics
      realtime: RealtimeStatus
    }>(API_ENDPOINTS.MONITOR.STATUS)
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get health check status
   */
  async getHealth() {
    const response = await api.get<HealthStatus>(
      API_ENDPOINTS.MONITOR.HEALTH
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get all email monitor status (admin only)
   */
  async getEmailMonitorStatus() {
    const response = await api.get<Record<string, MonitorStatus>>(
      API_ENDPOINTS.EMAIL_MONITOR.STATUS_ALL
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get specific account monitor status (admin only)
   */
  async getAccountMonitorStatus(accountId: number) {
    const response = await api.get<MonitorStatus>(
      API_ENDPOINTS.EMAIL_MONITOR.STATUS(accountId)
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Start monitoring for specific account (admin only)
   */
  async startAccountMonitor(accountId: number) {
    const response = await api.post<null>(
      API_ENDPOINTS.EMAIL_MONITOR.START(accountId)
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Stop monitoring for specific account (admin only)
   */
  async stopAccountMonitor(accountId: number) {
    const response = await api.post<null>(
      API_ENDPOINTS.EMAIL_MONITOR.STOP(accountId)
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Format uptime for display
   */
  formatUptime(seconds: number): string {
    const days = Math.floor(seconds / 86400)
    const hours = Math.floor((seconds % 86400) / 3600)
    const minutes = Math.floor((seconds % 3600) / 60)
    
    const parts = []
    if (days > 0) parts.push(`${days}天`)
    if (hours > 0) parts.push(`${hours}小时`)
    if (minutes > 0) parts.push(`${minutes}分钟`)
    
    return parts.join(' ') || '< 1分钟'
  }

  /**
   * Get health status color
   */
  getHealthStatusColor(status: string): string {
    switch (status.toLowerCase()) {
      case 'healthy':
        return 'green'
      case 'unhealthy':
        return 'red'
      case 'degraded':
        return 'yellow'
      default:
        return 'gray'
    }
  }

  /**
   * Calculate system health score
   */
  calculateHealthScore(metrics: SystemMetrics): number {
    let score = 100
    
    // CPU usage penalty
    if (metrics.cpu_usage > 80) score -= 20
    else if (metrics.cpu_usage > 60) score -= 10
    
    // Memory usage penalty
    if (metrics.memory_usage > 90) score -= 30
    else if (metrics.memory_usage > 70) score -= 15
    
    // DB connections penalty
    if (metrics.db_connections > 80) score -= 10
    
    return Math.max(0, score)
  }

  /**
   * Parse Prometheus metrics to key-value pairs
   */
  parsePrometheusMetrics(text: string): Record<string, number> {
    const metrics: Record<string, number> = {}
    const lines = text.split('\n')
    
    for (const line of lines) {
      if (line.startsWith('#') || !line.trim()) continue
      
      const match = line.match(/^(\w+)(?:\{[^}]*\})?\s+([\d.]+)/)
      if (match) {
        metrics[match[1]] = parseFloat(match[2])
      }
    }
    
    return metrics
  }
}

export const monitoringService = new MonitoringService()
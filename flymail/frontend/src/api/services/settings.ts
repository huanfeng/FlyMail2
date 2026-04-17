import { api } from '../axios'
import { API_ENDPOINTS } from '../config'
import type {
  AppSettings,
  EmailMonitorSettings,
  ScheduledTask,
  CreateScheduledTaskRequest
} from '../types';
import { ApiError } from '../ApiError'

class SettingsService {
  /**
   * Get all settings (admin only)
   */
  async getAllSettings() {
    const response = await api.get<{ settings: Record<string, string>; count: number }>(
      API_ENDPOINTS.SETTINGS.LIST
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Update multiple settings (admin only)
   */
  async updateSettings(settings: Record<string, string>) {
    const response = await api.put<{ updated: number }>(
      API_ENDPOINTS.SETTINGS.UPDATE_BATCH,
      { settings }
    )
    
    if (response.code === 0 && response.data) {
      return response.data.updated
    }
    
    throw new ApiError(response)
  }

  /**
   * Get single setting by key (admin only)
   */
  async getSetting(key: string) {
    const response = await api.get<{ key: string; value: string }>(
      API_ENDPOINTS.SETTINGS.GET(key)
    )
    
    if (response.code === 0 && response.data) {
      return response.data.value
    }
    
    throw new ApiError(response)
  }

  /**
   * Update single setting (admin only)
   */
  async updateSetting(key: string, value: string) {
    const response = await api.put<{ key: string; value: string }>(
      API_ENDPOINTS.SETTINGS.UPDATE(key),
      { value }
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Delete setting (admin only)
   */
  async deleteSetting(key: string) {
    const response = await api.delete<null>(
      API_ENDPOINTS.SETTINGS.DELETE(key)
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Get application settings (admin only)
   */
  async getAppSettings() {
    const response = await api.get<AppSettings>(
      API_ENDPOINTS.SETTINGS.APP
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Update application settings (admin only)
   */
  async updateAppSettings(settings: Partial<AppSettings>) {
    const response = await api.put<AppSettings>(
      API_ENDPOINTS.SETTINGS.UPDATE_APP,
      settings
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get email monitor settings
   */
  async getEmailMonitorSettings() {
    const response = await api.get<EmailMonitorSettings>(
      API_ENDPOINTS.SETTINGS.EMAIL_MONITOR
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Update email monitor settings
   */
  async updateEmailMonitorSettings(settings: Partial<EmailMonitorSettings>) {
    const response = await api.put<EmailMonitorSettings>(
      API_ENDPOINTS.SETTINGS.UPDATE_EMAIL_MONITOR,
      settings
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get scheduled tasks (admin only)
   */
  async getScheduledTasks() {
    const response = await api.get<{ tasks: ScheduledTask[] }>(
      API_ENDPOINTS.SCHEDULED_TASKS.LIST
    )
    
    if (response.code === 0 && response.data) {
      return response.data.tasks
    }
    
    throw new ApiError(response)
  }

  /**
   * Get scheduled task by ID (admin only)
   */
  async getScheduledTask(id: number) {
    const response = await api.get<{ task: ScheduledTask }>(
      API_ENDPOINTS.SCHEDULED_TASKS.GET(id)
    )
    
    if (response.code === 0 && response.data) {
      return response.data.task
    }
    
    throw new ApiError(response)
  }

  /**
   * Create scheduled task (admin only)
   */
  async createScheduledTask(task: CreateScheduledTaskRequest) {
    const response = await api.post<{ task: ScheduledTask }>(
      API_ENDPOINTS.SCHEDULED_TASKS.CREATE,
      task
    )
    
    if (response.code === 0 && response.data) {
      return response.data.task
    }
    
    throw new ApiError(response)
  }

  /**
   * Update scheduled task (admin only)
   */
  async updateScheduledTask(id: number, task: CreateScheduledTaskRequest) {
    const response = await api.put<{ task: ScheduledTask }>(
      API_ENDPOINTS.SCHEDULED_TASKS.UPDATE(id),
      task
    )
    
    if (response.code === 0 && response.data) {
      return response.data.task
    }
    
    throw new ApiError(response)
  }

  /**
   * Delete scheduled task (admin only)
   */
  async deleteScheduledTask(id: number) {
    const response = await api.delete<null>(
      API_ENDPOINTS.SCHEDULED_TASKS.DELETE(id)
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Validate cron expression
   */
  validateCronExpression(expression: string): boolean {
    // Basic cron expression validation
    const cronRegex = /^(\*|([0-9]|1[0-9]|2[0-9]|3[0-9]|4[0-9]|5[0-9])|\*\/([0-9]|1[0-9]|2[0-9]|3[0-9]|4[0-9]|5[0-9])) (\*|([0-9]|1[0-9]|2[0-3])|\*\/([0-9]|1[0-9]|2[0-3])) (\*|([1-9]|1[0-9]|2[0-9]|3[0-1])|\*\/([1-9]|1[0-9]|2[0-9]|3[0-1])) (\*|([1-9]|1[0-2])|\*\/([1-9]|1[0-2])) (\*|([0-6])|\*\/([0-6]))$/
    return cronRegex.test(expression)
  }

  /**
   * Get cron expression description
   */
  getCronDescription(expression: string): string {
    const parts = expression.split(' ')
    if (parts.length !== 5) return '无效的Cron表达式'
    
    const [minute, hour, day, month, weekday] = parts
    
    // Common patterns
    if (expression === '* * * * *') return '每分钟'
    if (expression === '0 * * * *') return '每小时'
    if (expression === '0 0 * * *') return '每天午夜'
    if (expression === '0 0 * * 0') return '每周日午夜'
    if (expression === '0 0 1 * *') return '每月1日午夜'
    
    // Build description
    const desc = []
    
    if (minute !== '*') {
      desc.push(`在第${minute}分钟`)
    }
    
    if (hour !== '*') {
      desc.push(`${hour}点`)
    }
    
    if (day !== '*') {
      desc.push(`每月${day}日`)
    }
    
    if (month !== '*') {
      desc.push(`${month}月`)
    }
    
    if (weekday !== '*') {
      const days = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
      desc.push(days[parseInt(weekday)])
    }
    
    return desc.join(' ') || '自定义时间'
  }
}

export const settingsService = new SettingsService()
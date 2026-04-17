import { api } from '../axios'
import { API_ENDPOINTS } from '../config'
import type {
  Task,
  TaskDetail,
  CreateTaskRequest,
  TaskPageData,
  TaskStatus,
  TaskConfig,
  TaskConfigPageData,
  CreateTaskConfigRequest,
  UpdateTaskConfigRequest,
  TaskLog,
  TaskType
} from '../types'
import { ApiError } from '../ApiError'

interface TaskListParams {
  type?: string
  page?: number
  page_size?: number
  status?: TaskStatus
}

interface TaskConfigListParams {
  task_type?: TaskType
  task_name?: string
  is_active?: boolean
  page?: number
  page_size?: number
}

class TasksService {
  /**
   * Create a new task
   */
  async createTask(task: CreateTaskRequest) {
    const response = await api.post<Task>(
      API_ENDPOINTS.TASKS.CREATE,
      task
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get tasks list
   */
  async getTasks(params?: TaskListParams) {
    const response = await api.get<TaskPageData>(
      API_ENDPOINTS.TASKS.LIST,
      { params }
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get task detail by ID
   */
  async getTask(taskId: string) {
    const response = await api.get<TaskDetail>(
      API_ENDPOINTS.TASKS.GET(taskId)
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Cancel a running or pending task
   */
  async cancelTask(taskId: string) {
    const response = await api.post<null>(
      API_ENDPOINTS.TASKS.CANCEL(taskId)
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Get task status label
   */
  getTaskStatusLabel(status: TaskStatus): string {
    const statusMap: Record<TaskStatus, string> = {
      'pending': '等待中',
      'running': '执行中',
      'completed': '已完成',
      'failed': '失败',
      'cancelled': '已取消'
    }
    
    return statusMap[status] || status
  }

  /**
   * Get task status color
   */
  getTaskStatusColor(status: TaskStatus): string {
    const colorMap: Record<TaskStatus, string> = {
      'pending': 'default',
      'running': 'blue',
      'completed': 'green',
      'failed': 'red',
      'cancelled': 'gray'
    }
    
    return colorMap[status] || 'default'
  }

  /**
   * Get task type label
   */
  getTaskTypeLabel(type: string): string {
    const typeMap: Record<string, string> = {
      'email_sync': '邮件同步',
      'folder_sync': '文件夹同步',
      'account_test': '账户测试',
      'batch_sync': '批量同步',
      'database_backup': '数据库备份'
    }
    
    return typeMap[type] || type
  }

  /**
   * Check if task is in progress
   */
  isTaskInProgress(status: TaskStatus): boolean {
    return status === 'pending' || status === 'running'
  }

  /**
   * Calculate task duration
   */
  getTaskDuration(task: TaskDetail): string | null {
    if (!task.started_at || !task.completed_at) {
      return null
    }
    
    const start = new Date(task.started_at).getTime()
    const end = new Date(task.completed_at).getTime()
    const duration = end - start
    
    if (duration < 1000) {
      return `${duration}ms`
    } else if (duration < 60000) {
      return `${Math.round(duration / 1000)}s`
    } else if (duration < 3600000) {
      return `${Math.round(duration / 60000)}m`
    } else {
      const hours = Math.floor(duration / 3600000)
      const minutes = Math.round((duration % 3600000) / 60000)
      return `${hours}h ${minutes}m`
    }
  }

  /**
   * Format task time
   */
  formatTaskTime(time: string): string {
    const date = new Date(time)
    const now = new Date()
    const diff = now.getTime() - date.getTime()
    
    if (diff < 60000) {
      return '刚刚'
    } else if (diff < 3600000) {
      return `${Math.floor(diff / 60000)}分钟前`
    } else if (diff < 86400000) {
      return `${Math.floor(diff / 3600000)}小时前`
    } else {
      return date.toLocaleDateString('zh-CN', {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
      })
    }
  }

  /**
   * Poll task status until completion
   */
  async pollTaskStatus(
    taskId: string, 
    onUpdate?: (task: TaskDetail) => void,
    pollInterval = 1000,
    maxAttempts = 300 // 5 minutes max
  ): Promise<TaskDetail> {
    let attempts = 0
    
    while (attempts < maxAttempts) {
      const task = await this.getTask(taskId)
      
      if (onUpdate) {
        onUpdate(task)
      }
      
      if (!this.isTaskInProgress(task.status)) {
        return task
      }
      
      await new Promise(resolve => setTimeout(resolve, pollInterval))
      attempts++
    }
    
    throw new Error('任务轮询超时')
  }

  // Task Configuration Management Methods
  
  /**
   * Get task configurations list
   */
  async getTaskConfigs(params?: TaskConfigListParams) {
    const response = await api.get<TaskConfigPageData>(
      API_ENDPOINTS.TASKS.CONFIG_LIST,
      { params }
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get task configuration by ID
   */
  async getTaskConfig(id: number) {
    const response = await api.get<TaskConfig>(
      API_ENDPOINTS.TASKS.CONFIG_GET(id)
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Create task configuration
   */
  async createTaskConfig(data: CreateTaskConfigRequest) {
    const response = await api.post<TaskConfig>(
      API_ENDPOINTS.TASKS.CONFIG_CREATE,
      data
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Update task configuration
   */
  async updateTaskConfig(id: number, data: UpdateTaskConfigRequest) {
    const response = await api.put<TaskConfig>(
      API_ENDPOINTS.TASKS.CONFIG_UPDATE(id),
      data
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Delete task configuration
   */
  async deleteTaskConfig(id: number) {
    const response = await api.delete<null>(
      API_ENDPOINTS.TASKS.CONFIG_DELETE(id)
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Execute task configuration
   */
  async executeTaskConfig(id: number) {
    const response = await api.post<null>(
      API_ENDPOINTS.TASKS.CONFIG_EXECUTE(id)
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Get task execution logs
   */
  async getTaskLogs(id: number, limit: number = 20) {
    const response = await api.get<TaskLog[]>(
      API_ENDPOINTS.TASKS.CONFIG_LOGS(id),
      { params: { limit } }
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }
}

export const tasksService = new TasksService()
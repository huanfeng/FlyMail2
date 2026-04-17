import { API_CONFIG, API_ENDPOINTS, TOKEN_KEYS } from '../config'
import type {
  NewEmailsEvent,
  TaskProgressEvent,
  TaskCountsEvent,
  TaskDetail
} from '../types'
// 内联定义SSE事件处理器类型，避免循环依赖
interface SSEEventHandlersBase {
  onConnected?: (data: { client_id: string; connected_at: string }) => void
  onHeartbeat?: (data: { timestamp: string }) => void
  onNewEmails?: (data: NewEmailsEvent) => void
  onTaskProgress?: (data: TaskProgressEvent) => void
  onTaskCounts?: (data: TaskCountsEvent) => void
}

export type SSEEventType =
  | 'connected'
  | 'heartbeat'
  | 'new_emails'
  | 'task_created'
  | 'task_started'
  | 'task_progress'
  | 'task_counts'
  | 'task_completed'
  | 'task_failed'
  | 'task_cancelled'

export interface SSEHandlers extends SSEEventHandlersBase {
  onTaskCreated?: (data: TaskDetail) => void
  onTaskStarted?: (data: TaskDetail) => void
  onTaskCompleted?: (data: TaskDetail) => void
  onTaskFailed?: (data: TaskDetail) => void
  onTaskCancelled?: (data: TaskDetail) => void
  onError?: (error: Event) => void
  onReconnect?: () => void
}

class SSEService {
  private eventSource: EventSource | null = null
  private handlers: SSEHandlers = {}
  private reconnectTimer: number | null = null
  private reconnectAttempts = 0
  private maxReconnectAttempts = 10
  private reconnectDelay = 3000
  private isConnecting = false
  private shouldReconnect = true

  /**
   * Connect to SSE endpoint
   */
  connect(handlers?: SSEHandlers) {
    if (this.eventSource || this.isConnecting) {
      console.warn('SSE already connected or connecting')
      return
    }

    this.handlers = handlers || {}
    this.shouldReconnect = true
    this.isConnecting = true

    const token = localStorage.getItem(TOKEN_KEYS.ACCESS_TOKEN)
    if (!token) {
      console.error('No access token found')
      this.isConnecting = false
      return
    }

    // 构建 SSE URL
    const baseUrl = API_CONFIG.BASE_URL.startsWith('http')
      ? API_CONFIG.BASE_URL
      : `${window.location.protocol}//${window.location.host}${API_CONFIG.BASE_URL}`

    // 统一使用 URL 参数传递 token（因为 EventSource 不支持自定义请求头）
    const url = `${baseUrl}${API_ENDPOINTS.EVENTS}?token=${encodeURIComponent(token)}`

    const options: EventSourceInit | undefined = import.meta.env.DEV
      ? undefined
      : { withCredentials: false }

    console.log(`SSE start connect`)

    this.eventSource = new EventSource(url, options)

    // Connection opened
    this.eventSource.onopen = () => {
      console.log('SSE connected')
      this.isConnecting = false
      this.reconnectAttempts = 0
      if (this.reconnectTimer) {
        clearTimeout(this.reconnectTimer)
        this.reconnectTimer = null
      }
    }

    // Handle named events
    this.setupEventHandlers()

    // Handle unnamed messages (heartbeat)
    this.eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        if (data.type === 'heartbeat' && this.handlers.onHeartbeat) {
          this.handlers.onHeartbeat(data)
        }
      } catch (error) {
        console.error('Failed to parse SSE message:', error)
      }
    }

    // Handle errors
    this.eventSource.onerror = (error) => {
      console.error('SSE error:', error)
      this.isConnecting = false

      // Check if it's a connection error
      if (this.eventSource?.readyState === EventSource.CLOSED) {
        console.warn('SSE connection closed. This might be due to proxy configuration or backend not supporting SSE.')
      }

      if (this.handlers.onError) {
        this.handlers.onError(error)
      }

      // Attempt to reconnect
      if (this.shouldReconnect && this.reconnectAttempts < this.maxReconnectAttempts) {
        this.scheduleReconnect()
      } else {
        console.warn('SSE: Max reconnection attempts reached. Stopping reconnection.')
        this.disconnect()
      }
    }
  }

  /**
   * Setup handlers for named events
   */
  private setupEventHandlers() {
    if (!this.eventSource) return

    // Connected event
    this.eventSource.addEventListener('connected', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data)
        if (this.handlers.onConnected) {
          this.handlers.onConnected(data)
        }
      } catch (error) {
        console.error('Failed to parse connected event:', error)
      }
    })

    // New emails event
    this.eventSource.addEventListener('new_emails', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data) as NewEmailsEvent
        if (this.handlers.onNewEmails) {
          this.handlers.onNewEmails(data)
        }
      } catch (error) {
        console.error('Failed to parse new_emails event:', error)
      }
    })

    // Task events
    const taskEvents: Array<[SSEEventType, keyof SSEHandlers]> = [
      ['task_created', 'onTaskCreated'],
      ['task_started', 'onTaskStarted'],
      ['task_completed', 'onTaskCompleted'],
      ['task_failed', 'onTaskFailed'],
      ['task_cancelled', 'onTaskCancelled']
    ]

    taskEvents.forEach(([eventType, handlerName]) => {
      this.eventSource!.addEventListener(eventType, (event: MessageEvent) => {
        try {
          const data = JSON.parse(event.data) as TaskDetail
          const handler = this.handlers[handlerName] as ((data: TaskDetail) => void) | undefined
          if (handler) {
            handler(data)
          }
        } catch (error) {
          console.error(`Failed to parse ${eventType} event:`, error)
        }
      })
    })

    // Task progress event
    this.eventSource.addEventListener('task_progress', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data) as TaskProgressEvent
        if (this.handlers.onTaskProgress) {
          this.handlers.onTaskProgress(data)
        }
      } catch (error) {
        console.error('Failed to parse task_progress event:', error)
      }
    })

    // Task counts event
    this.eventSource.addEventListener('task_counts', (event: MessageEvent) => {
      try {
        const data = JSON.parse(event.data) as TaskCountsEvent
        if (this.handlers.onTaskCounts) {
          this.handlers.onTaskCounts(data)
        }
      } catch (error) {
        console.error('Failed to parse task_counts event:', error)
      }
    })
  }

  /**
   * Schedule reconnection attempt
   */
  private scheduleReconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
    }

    this.reconnectAttempts++
    const delay = Math.min(
      this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1),
      30000 // Max 30 seconds
    )

    console.log(`Scheduling SSE reconnect in ${delay}ms (attempt ${this.reconnectAttempts})`)

    this.reconnectTimer = window.setTimeout(() => {
      if (this.handlers.onReconnect) {
        this.handlers.onReconnect()
      }
      this.connect(this.handlers)
    }, delay)
  }

  /**
   * Update event handlers
   */
  updateHandlers(handlers: Partial<SSEHandlers>) {
    this.handlers = { ...this.handlers, ...handlers }
  }

  /**
   * Disconnect from SSE
   */
  disconnect() {
    this.shouldReconnect = false

    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }

    if (this.eventSource) {
      this.eventSource.close()
      this.eventSource = null
    }

    this.isConnecting = false
    this.reconnectAttempts = 0
    console.log('SSE disconnected')
  }

  /**
   * Check if connected
   */
  isConnected(): boolean {
    return this.eventSource?.readyState === EventSource.OPEN
  }

  /**
   * Get connection state
   */
  getState(): 'connecting' | 'open' | 'closed' {
    if (this.isConnecting) return 'connecting'
    if (!this.eventSource) return 'closed'

    switch (this.eventSource.readyState) {
      case EventSource.CONNECTING:
        return 'connecting'
      case EventSource.OPEN:
        return 'open'
      default:
        return 'closed'
    }
  }
}

// Export singleton instance
export const sseService = new SSEService()
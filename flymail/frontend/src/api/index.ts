// Export all API services and types

import { authService } from './services/auth'
import { accountsService } from './services/accounts'
import { foldersService } from './services/folders'
import { emailsService } from './services/emails'
import { tasksService } from './services/tasks'
import { monitoringService } from './services/monitoring'
import { settingsService } from './services/settings'
import { sseService } from './services/sse'
import { notifyService } from './services/notify'

// Export types
export * from './types'

// Export configuration
export { API_CONFIG, API_ENDPOINTS, TOKEN_KEYS } from './config'

// Export axios instance and helpers
export { default as axios, api, apiRequest } from './axios'

// Export services
export { authService } from './services/auth'
export { accountsService } from './services/accounts'
export { foldersService, type FolderTreeNode } from './services/folders'
export { emailsService } from './services/emails'
export { tasksService } from './services/tasks'
export { monitoringService } from './services/monitoring'
export { settingsService } from './services/settings'
export { sseService, type SSEEventType, type SSEHandlers } from './services/sse'
export { notifyService } from './services/notify'

// Re-export as default for convenience
const apiServices = {
  auth: authService,
  accounts: accountsService,
  folders: foldersService,
  emails: emailsService,
  tasks: tasksService,
  monitoring: monitoringService,
  settings: settingsService,
  sse: sseService,
  notify: notifyService,
}

export default apiServices
// API Configuration

/**
 * 动态构建 API 基础 URL
 * 支持三种配置方式：
 * 1. VITE_API_BASE_URL: 完整的 API 基础 URL（优先级最高）
 * 2. VITE_API_PORT + VITE_API_PATH: 使用当前主机名 + 指定端口和路径
 * 3. 默认使用相对路径 /api/v1
 */
function getBaseUrl(): string {
  // 如果直接配置了完整的 BASE_URL，直接使用
  if (import.meta.env.VITE_API_BASE_URL) {
    return import.meta.env.VITE_API_BASE_URL;
  }

  // 如果配置了端口，则动态构建 URL
  if (import.meta.env.VITE_API_PORT) {
    const protocol = window.location.protocol;
    const hostname = window.location.hostname;
    const port = import.meta.env.VITE_API_PORT;
    const path = import.meta.env.VITE_API_PATH || '/api/v1';

    return `${protocol}//${hostname}:${port}${path}`;
  }

  // 默认使用相对路径
  return '/api/v1';
}

export const API_CONFIG = {
  BASE_URL: getBaseUrl(),
  TIMEOUT: 30000,
  RETRY_COUNT: 3,
  RETRY_DELAY: 1000,
}

export const API_ENDPOINTS = {
  // Authentication
  AUTH: {
    LOGIN: '/auth/login',
    REFRESH: '/auth/refresh',
    ME: '/auth/me',
    UPDATE_CREDENTIALS: '/auth/admin/credentials',
  },

  // Email Accounts
  ACCOUNTS: {
    LIST: '/accounts',
    CREATE: '/accounts',
    GET: (id: number) => `/accounts/${id}`,
    UPDATE: (id: number) => `/accounts/${id}`,
    DELETE: (id: number) => `/accounts/${id}`,
    SEND: (id: number) => `/accounts/${id}/send`,
    SYNC: (id: number) => `/accounts/${id}/sync`,
    TEST: (id: number) => `/accounts/${id}/test`,
    TEMP_TEST: '/accounts/temp_test',
    SYNC_ALL: '/accounts/sync-all',
    SYNC_HISTORY: (id: number) => `/accounts/${id}/sync/history`,
  },

  // Folders
  FOLDERS: {
    LIST: (accountId: number) => `/accounts/${accountId}/folders`,
    SYNC: (accountId: number) => `/accounts/${accountId}/folders/sync`,
  },

  // Emails
  EMAILS: {
    LIST: '/emails',
    GET: (id: number) => `/emails/${id}`,
    DELETE: (id: number) => `/emails/${id}`,
    GET_FLAGS: (id: number) => `/emails/${id}/flags`,
    UPDATE_FLAGS: (id: number) => `/emails/${id}/flags`,
    BATCH_FLAGS: '/emails/batch/flags',
    BATCH_DELETE: '/emails/batch',
  },

  // Tasks
  TASKS: {
    CREATE: '/tasks',
    LIST: '/tasks',
    GET: (taskId: string) => `/tasks/${taskId}`,
    CANCEL: (taskId: string) => `/tasks/${taskId}/cancel`,
    // Task Config Management
    CONFIG_LIST: '/tasks',
    CONFIG_CREATE: '/tasks',
    CONFIG_GET: (id: number) => `/tasks/${id}`,
    CONFIG_UPDATE: (id: number) => `/tasks/${id}`,
    CONFIG_DELETE: (id: number) => `/tasks/${id}`,
    CONFIG_EXECUTE: (id: number) => `/tasks/${id}/execute`,
    CONFIG_LOGS: (id: number) => `/tasks/${id}/logs`,
  },

  // Settings
  SETTINGS: {
    LIST: '/settings',
    UPDATE_BATCH: '/settings',
    GET: (key: string) => `/settings/${key}`,
    UPDATE: (key: string) => `/settings/${key}`,
    DELETE: (key: string) => `/settings/${key}`,
    APP: '/settings/app',
    UPDATE_APP: '/settings/app',
    EMAIL_MONITOR: '/settings/email-monitor',
    UPDATE_EMAIL_MONITOR: '/settings/email-monitor',
  },

  // Email Monitor
  EMAIL_MONITOR: {
    STATUS_ALL: '/email-monitor/status',
    STATUS: (id: number) => `/email-monitor/accounts/${id}/status`,
    START: (id: number) => `/email-monitor/accounts/${id}/start`,
    STOP: (id: number) => `/email-monitor/accounts/${id}/stop`,
  },

  // Scheduled Tasks
  SCHEDULED_TASKS: {
    LIST: '/scheduled-tasks',
    CREATE: '/scheduled-tasks',
    GET: (id: number) => `/scheduled-tasks/${id}`,
    UPDATE: (id: number) => `/scheduled-tasks/${id}`,
    DELETE: (id: number) => `/scheduled-tasks/${id}`,
  },

  // Monitoring
  MONITOR: {
    METRICS: '/monitor/metrics',
    STATUS: '/monitor/status',
    HEALTH: '/monitor/health',
  },

  // SSE
  EVENTS: '/events',
}

// Token storage keys
export const TOKEN_KEYS = {
  ACCESS_TOKEN: 'flymail_access_token',
  REFRESH_TOKEN: 'flymail_refresh_token',
}
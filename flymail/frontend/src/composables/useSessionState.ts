import { onMounted, onUnmounted } from 'vue'

export interface SessionState {
  scrollPositions: Record<string, number>
  expandedFolders: Set<number>
  emailListFilter: {
    search?: string
    sortBy?: string
    sortOrder?: 'asc' | 'desc'
  }
  panelSizes?: number[]
  lastActivity?: number
}

const SESSION_KEY = 'flymail_session_state'
const STORAGE_INTERVAL = 5000 // 5秒自动保存一次
const SESSION_TIMEOUT = 30 * 60 * 1000 // 30分钟会话超时

export const useSessionState = () => {
  let saveTimer: ReturnType<typeof setInterval> | null = null
  let sessionState: SessionState = {
    scrollPositions: {},
    expandedFolders: new Set(),
    emailListFilter: {},
    lastActivity: Date.now()
  }

  // 从SessionStorage加载状态
  const loadState = (): SessionState | null => {
    try {
      const stored = sessionStorage.getItem(SESSION_KEY)
      if (!stored) return null
      
      const parsed = JSON.parse(stored)
      
      // 检查会话是否过期
      if (parsed.lastActivity && Date.now() - parsed.lastActivity > SESSION_TIMEOUT) {
        sessionStorage.removeItem(SESSION_KEY)
        return null
      }
      
      // 恢复Set类型
      if (parsed.expandedFolders) {
        parsed.expandedFolders = new Set(parsed.expandedFolders)
      }
      
      return parsed
    } catch (error) {
      console.error('Failed to load session state:', error)
      return null
    }
  }

  // 保存状态到SessionStorage
  const saveState = (state?: Partial<SessionState>) => {
    try {
      if (state) {
        sessionState = { ...sessionState, ...state, lastActivity: Date.now() }
      }
      
      // 转换Set为数组以便序列化
      const toSave = {
        ...sessionState,
        expandedFolders: Array.from(sessionState.expandedFolders)
      }
      
      sessionStorage.setItem(SESSION_KEY, JSON.stringify(toSave))
    } catch (error) {
      console.error('Failed to save session state:', error)
    }
  }

  // 保存滚动位置
  const saveScrollPosition = (key: string, position: number) => {
    sessionState.scrollPositions[key] = position
    saveState()
  }

  // 恢复滚动位置
  const restoreScrollPosition = (key: string): number => {
    return sessionState.scrollPositions[key] || 0
  }

  // 保存展开的文件夹
  const saveExpandedFolders = (folders: Set<number>) => {
    sessionState.expandedFolders = folders
    saveState()
  }

  // 恢复展开的文件夹
  const getExpandedFolders = (): Set<number> => {
    return sessionState.expandedFolders
  }

  // 保存邮件列表过滤器
  const saveEmailListFilter = (filter: SessionState['emailListFilter']) => {
    sessionState.emailListFilter = filter
    saveState()
  }

  // 恢复邮件列表过滤器
  const getEmailListFilter = (): SessionState['emailListFilter'] => {
    return sessionState.emailListFilter
  }

  // 保存面板大小
  const savePanelSizes = (sizes: number[]) => {
    sessionState.panelSizes = sizes
    saveState()
  }

  // 恢复面板大小
  const getPanelSizes = (): number[] | undefined => {
    return sessionState.panelSizes
  }

  // 清除会话状态
  const clearState = () => {
    sessionStorage.removeItem(SESSION_KEY)
    sessionState = {
      scrollPositions: {},
      expandedFolders: new Set(),
      emailListFilter: {},
      lastActivity: Date.now()
    }
  }

  // 初始化 - 在组件挂载时调用
  const initialize = () => {
    const loaded = loadState()
    if (loaded) {
      sessionState = loaded
    }
    
    // 设置定期保存
    saveTimer = setInterval(() => {
      saveState()
    }, STORAGE_INTERVAL)
  }

  // 清理 - 在组件卸载时调用
  const cleanup = () => {
    if (saveTimer) {
      clearInterval(saveTimer)
      saveTimer = null
    }
    saveState() // 最后保存一次
  }

  // 在组件中使用的便捷方法
  const useAutoSave = () => {
    onMounted(() => {
      initialize()
    })
    
    onUnmounted(() => {
      cleanup()
    })
  }

  return {
    loadState,
    saveState,
    saveScrollPosition,
    restoreScrollPosition,
    saveExpandedFolders,
    getExpandedFolders,
    saveEmailListFilter,
    getEmailListFilter,
    savePanelSizes,
    getPanelSizes,
    clearState,
    initialize,
    cleanup,
    useAutoSave
  }
}
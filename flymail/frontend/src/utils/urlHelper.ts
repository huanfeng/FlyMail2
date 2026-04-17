import type { AppState } from '@/composables/useUrlDrivenState'

// URL路径常量
export const MAIL_ROUTES = {
  MAIN: '/main',
  VIEW: '/view'
} as const

// 虚拟文件夹列表
export const VIRTUAL_FOLDERS = [
  'all-inbox', 'all-unread', 'all-starred',
  'all-sent', 'all-drafts', 'all-trash'
] as const

/**
 * 构建主页面URL (#/main)
 * @param state 应用状态
 * @returns URL字符串
 */
export const buildMainUrl = (state: Partial<AppState>): string => {
  const params = new URLSearchParams()

  // 账户参数
  if (state.accountId !== undefined) {
    params.set('a', String(state.accountId === null ? 0 : state.accountId))
  }

  // 文件夹参数
  if (state.virtualFolder) {
    params.set('f', state.virtualFolder)
  } else if (state.folderId !== undefined && state.folderId !== null) {
    params.set('f', String(state.folderId))
  }

  // 邮件参数
  if (state.mailId) {
    params.set('m', String(state.mailId))
  }

  // 视图状态参数
  const views: string[] = []
  if (state.settingsOpen) views.push('settings')
  if (state.composeOpen) views.push('compose')
  if (views.length > 0) {
    params.set('view', views.join(','))
  }

  // 撰写相关参数
  if (state.composeOpen && state.composeMode && state.composeMode !== 'new') {
    params.set('compose', state.composeMode)
  }
  if (state.composeMailId) {
    params.set('composeId', String(state.composeMailId))
  }

  const queryString = params.toString()
  return queryString ? `${MAIL_ROUTES.MAIN}?${queryString}` : MAIL_ROUTES.MAIN
}

/**
 * 构建单页邮件查看URL (#/view?id=xxx)
 * @param mailId 邮件ID
 * @returns URL字符串
 */
export const buildMailViewUrl = (mailId: number): string => {
  return `${MAIL_ROUTES.VIEW}?id=${mailId}`
}

/**
 * 解析主页面URL
 * @param search URL查询字符串 (如: "?a=3&f=30&m=1234")
 * @returns 解析出的应用状态
 */
export const parseMainUrl = (search: string): Partial<AppState> => {
  const params = new URLSearchParams(search)
  const state: Partial<AppState> = {}

  console.log('🔍 [URLHelper] parseMainUrl - 输入查询:', search)

  // 解析账户参数
  const accountParam = params.get('a')
  if (accountParam !== null) {
    const accountId = parseInt(accountParam)
    if (!isNaN(accountId)) {
      state.accountId = accountId === 0 ? null : accountId
      console.log('📊 [URLHelper] 解析账户ID:', state.accountId)
    }
  }

  // 解析文件夹参数
  const folderParam = params.get('f')
  if (folderParam !== null) {
    console.log('📁 [URLHelper] 解析文件夹参数:', folderParam)

    // 检查是否为虚拟文件夹
    if (VIRTUAL_FOLDERS.includes(folderParam as any)) {
      state.virtualFolder = folderParam
      state.folderId = null
      console.log('✨ [URLHelper] 识别为虚拟文件夹:', folderParam)
    } else {
      const folderId = parseInt(folderParam)
      if (!isNaN(folderId)) {
        state.folderId = folderId
        state.virtualFolder = ""
        console.log('📁 [URLHelper] 识别为普通文件夹:', folderId)
      }
    }
  }

  // 解析邮件参数
  const mailParam = params.get('m')
  if (mailParam !== null) {
    const mailId = parseInt(mailParam)
    if (!isNaN(mailId)) {
      state.mailId = mailId
      console.log('📧 [URLHelper] 解析邮件ID:', mailId)
    }
  }

  // 解析视图状态
  const viewParam = params.get('view')
  if (viewParam) {
    const views = viewParam.split(',')
    state.settingsOpen = views.includes('settings')
    state.composeOpen = views.includes('compose')
    console.log('👁️ [URLHelper] 解析视图状态:', {
      settingsOpen: state.settingsOpen,
      composeOpen: state.composeOpen
    })
  }

  // 解析撰写模式
  const composeParam = params.get('compose')
  if (composeParam && ['reply', 'replyAll', 'forward'].includes(composeParam)) {
    state.composeMode = composeParam as AppState['composeMode']
    console.log('✍️ [URLHelper] 解析撰写模式:', state.composeMode)
  } else if (state.composeOpen) {
    state.composeMode = 'new'
  }

  // 解析撰写邮件ID
  const composeIdParam = params.get('composeId')
  if (composeIdParam) {
    const composeMailId = parseInt(composeIdParam)
    if (!isNaN(composeMailId)) {
      state.composeMailId = composeMailId
      console.log('✍️ [URLHelper] 解析撰写邮件ID:', composeMailId)
    }
  }

  console.log('🎯 [URLHelper] parseMainUrl - 解析结果:', state)
  return state
}

/**
 * 解析单页邮件查看URL
 * @param search URL查询字符串 (如: "?id=1234")
 * @returns 邮件ID
 */
export const parseMailViewUrl = (search: string): number | null => {
  const params = new URLSearchParams(search)
  const idParam = params.get('id')

  if (idParam) {
    const mailId = parseInt(idParam)
    if (!isNaN(mailId)) {
      console.log('📧 [URLHelper] 解析单页邮件ID:', mailId)
      return mailId
    }
  }

  console.warn('⚠️ [URLHelper] 无效的单页邮件URL:', search)
  return null
}

/**
 * 检查URL是否为主页面路径
 * @param path URL路径
 * @returns boolean
 */
export const isMainRoute = (path: string): boolean => {
  return path === MAIL_ROUTES.MAIN || path.startsWith(MAIL_ROUTES.MAIN + '?')
}

/**
 * 检查URL是否为单页邮件查看路径
 * @param path URL路径
 * @returns boolean
 */
export const isMailViewRoute = (path: string): boolean => {
  return path === MAIL_ROUTES.VIEW || path.startsWith(MAIL_ROUTES.VIEW + '?')
}

/**
 * 验证状态对象是否有效
 * @param state 应用状态
 * @returns boolean
 */
export const validateState = (state: Partial<AppState>): boolean => {
  // 基本验证规则
  if (state.accountId !== undefined && state.accountId !== null && state.accountId < 0) {
    console.warn('⚠️ [URLHelper] 无效的账户ID:', state.accountId)
    return false
  }

  if (state.folderId !== undefined && state.folderId !== null && state.folderId < 0) {
    console.warn('⚠️ [URLHelper] 无效的文件夹ID:', state.folderId)
    return false
  }

  if (state.mailId !== undefined && state.mailId < 0) {
    console.warn('⚠️ [URLHelper] 无效的邮件ID:', state.mailId)
    return false
  }

  // 虚拟文件夹验证
  if (state.virtualFolder && !VIRTUAL_FOLDERS.includes(state.virtualFolder as any)) {
    console.warn('⚠️ [URLHelper] 无效的虚拟文件夹:', state.virtualFolder)
    return false
  }

  return true
}

/**
 * 生成默认状态URL（所有收件箱）
 * @returns 默认主页面URL
 */
export const getDefaultMainUrl = (): string => {
  return buildMainUrl({
    accountId: null,
    virtualFolder: 'all-inbox'
  })
}

/**
 * 比较两个状态是否相同
 * @param state1 状态1
 * @param state2 状态2
 * @returns boolean
 */
export const isSameState = (state1: Partial<AppState>, state2: Partial<AppState>): boolean => {
  return (
    state1.accountId === state2.accountId &&
    state1.folderId === state2.folderId &&
    state1.virtualFolder === state2.virtualFolder &&
    state1.mailId === state2.mailId &&
    state1.settingsOpen === state2.settingsOpen &&
    state1.composeOpen === state2.composeOpen &&
    state1.composeMode === state2.composeMode &&
    state1.composeMailId === state2.composeMailId
  )
}
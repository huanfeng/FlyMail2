import { ref, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useMailApiStore } from '@/stores/mailApi'
import { useSettingsStore } from '@/stores/settings'
import {
  buildMainUrl,
  parseMainUrl,
  isMainRoute,
  validateState,
  getDefaultMainUrl,
  isSameState,
  buildMailViewUrl
} from '@/utils/urlHelper'

// 应用状态接口定义
export interface AppState {
  accountId?: number | null
  folderId?: number | null
  virtualFolder?: string
  mailId?: number
  composeOpen: boolean
  composeMode: 'new' | 'reply' | 'replyAll' | 'forward'
  composeMailId?: number
  settingsOpen: boolean
}

// URL驱动状态管理器接口
export interface UrlDrivenStateManager {
  // 当前状态（只读，从URL解析）
  readonly state: AppState

  // 状态更新方法（通过更新URL实现）
  updateState: (updates: Partial<AppState>, strategy?: 'push' | 'replace' | 'auto') => void
  selectAccount: (accountId: number) => void
  selectFolder: (folderId: number) => void
  selectVirtualFolder: (virtualFolder: string) => void
  selectMail: (mailId: number) => void
  clearMail: () => void
  openSettings: () => void
  closeSettings: () => void
  openCompose: (mode?: AppState['composeMode'], mailId?: number) => void
  closeCompose: () => void

  // 原子操作
  selectAccountAndFolder: (accountId: number, folderId: number) => void

  // 单页邮件查看
  openMailView: (mailId: number) => void

  // 初始化和清理
  initialize: () => void
  dispose: () => void
}

// 提供给组件使用的状态管理器接口（通过provide/inject）
export interface StateManager {
  state: import('vue').ComputedRef<AppState>
  selectAccount: (accountId: number) => void
  selectFolder: (folderId: number) => void
  selectVirtualFolder: (virtualFolder: string) => void
  selectMail: (mailId: number) => void
  clearMail: () => void
  openSettings: () => void
  closeSettings: () => void
  openCompose: (mode?: AppState['composeMode'], mailId?: number) => void
  closeCompose: () => void
  selectAccountAndFolder: (accountId: number, folderId: number) => void
  openMailView: (mailId: number) => void
}

export const useUrlDrivenState = (): UrlDrivenStateManager => {
  const route = useRoute()
  const router = useRouter()
  const mailStore = useMailApiStore()
  const settingsStore = useSettingsStore()

  // 是否正在处理URL变化（防止循环）
  const isProcessingUrlChange = ref(false)

  // 当前状态（从URL解析）
  const currentState = ref<AppState>({
    accountId: undefined,
    folderId: undefined,
    virtualFolder: undefined,
    mailId: undefined,
    composeOpen: false,
    composeMode: 'new',
    composeMailId: undefined,
    settingsOpen: false
  })

  // 解析URL并更新状态
  const parseAndUpdateState = () => {
    if (!isMainRoute(route.path)) {
      console.log('🚫 [UrlDrivenState] 非主页面路径，跳过状态解析:', route.path)
      return
    }

    isProcessingUrlChange.value = true

    try {
      const search = route.fullPath.split('?')[1] || ''
      const urlState = parseMainUrl(search)

      // 验证状态有效性
      if (!validateState(urlState)) {
        console.warn('⚠️ [UrlDrivenState] URL状态无效，重定向到默认状态')
        router.replace(getDefaultMainUrl())
        return
      }

      // 更新内部状态
      const newState: AppState = {
        accountId: urlState.accountId,
        folderId: urlState.folderId,
        virtualFolder: urlState.virtualFolder,
        mailId: urlState.mailId,
        composeOpen: urlState.composeOpen || false,
        composeMode: urlState.composeMode || 'new',
        composeMailId: urlState.composeMailId,
        settingsOpen: urlState.settingsOpen || false
      }

      // 只有状态真正改变时才更新
      if (!isSameState(currentState.value, newState)) {
        console.log('🔄 [UrlDrivenState] 状态变化:', {
          从: currentState.value,
          到: newState
        })

        currentState.value = newState

        // 同步到stores
        syncToStores(newState)
      }
    } finally {
      // 延迟重置标志，确保同步操作完成
      nextTick(() => {
        setTimeout(() => {
          isProcessingUrlChange.value = false
        }, 50)
      })
    }
  }

    // 验证URL状态中的ID是否存在
  const validateUrlState = async (state: AppState) => {
    // 等待store初始化完成
    if (!mailStore.isInitialized) {
      console.log('📍 [UrlDrivenState] 等待mailStore初始化完成...')
      // 简单等待，最多3秒
      let waitCount = 0
      while (!mailStore.isInitialized && waitCount < 30) {
        await new Promise(resolve => setTimeout(resolve, 100))
        waitCount++
      }

      if (!mailStore.isInitialized) {
        console.warn('⚠️ [UrlDrivenState] mailStore初始化超时')
        return { valid: false, reason: 'Store初始化超时' }
      }
    }

            // 如果没有任何账户，允许继续（显示空状态）
    if (mailStore.accounts.length === 0) {
      return { valid: true, reason: '' }
    }

    // 验证账户ID
    if (state.accountId !== undefined && state.accountId !== null) {
      const accountExists = mailStore.accounts.some(account => account.account_id === state.accountId)
      if (!accountExists) {
        return { valid: false, reason: `账户 ${state.accountId} 不存在` }
      }
    }

    // 验证文件夹ID
    if (state.folderId !== undefined && state.folderId !== null) {
      const folder = mailStore.folders.find(f => f.folder_id === state.folderId)
      if (!folder) {
        return { valid: false, reason: `文件夹 ${state.folderId} 不存在` }
      }

      // 如果同时指定了账户，验证文件夹是否属于该账户
      if (state.accountId !== undefined && state.accountId !== null) {
        if (folder.account_id !== state.accountId) {
          return { valid: false, reason: `文件夹 ${state.folderId} 不属于账户 ${state.accountId}` }
        }
      }
    }

    // 如果有邮件ID，验证邮件是否存在（暂时跳过API调用验证）
    if (state.mailId !== undefined) {
      // TODO: 可以在这里添加邮件存在性验证
      // 由于需要API调用，暂时跳过，让邮件详情组件处理不存在的情况
    }

    return { valid: true, reason: '' }
  }

  // 同步状态到stores
  const syncToStores = async (state: AppState) => {
    console.log('🔗 [UrlDrivenState] 同步到stores:', state)

    try {
      // 处理虚拟文件夹
      if (state.virtualFolder) {
        console.log('✨ [UrlDrivenState] 选择虚拟文件夹:', state.virtualFolder)
        // 如果切换到虚拟文件夹，且URL中没有邮件ID，清空当前邮件
        if (state.mailId === undefined && mailStore.currentEmail) {
          console.log('🧹 [UrlDrivenState] 切换到虚拟文件夹时清空当前邮件')
          mailStore.currentEmail = null
        }
        mailStore.selectVirtualFolder(state.virtualFolder)
      } else {
        // 验证URL中的ID是否存在
        const validationResult = await validateUrlState(state)
        if (!validationResult.valid) {
          console.warn('⚠️ [UrlDrivenState] URL状态验证失败:', validationResult.reason)
          // 重定向到默认状态（使用 replace 避免在历史记录中留下无效URL）
          router.replace(getDefaultMainUrl())
          return
        }
        // 从虚拟文件夹切换到普通文件夹时，确保清除虚拟文件夹状态
        if (mailStore.selectedVirtualFolder && !state.virtualFolder) {
          console.log('🔄 [UrlDrivenState] 从虚拟文件夹切换到普通文件夹，清除虚拟文件夹状态')
          mailStore.selectedVirtualFolder = null
        }

        // 检查是否需要同时设置账户和文件夹（原子性操作）
        const needAccountChange = state.accountId !== undefined && state.accountId !== mailStore.selectedAccountId
        const needFolderChange = state.folderId !== undefined && state.folderId !== null && state.folderId !== mailStore.selectedFolderId
        const hasValidFolder = state.folderId !== undefined && state.folderId !== null

        // 如果切换了账户或文件夹，且URL中没有邮件ID，清空当前邮件
        if ((needAccountChange || needFolderChange) && state.mailId === undefined && mailStore.currentEmail) {
          console.log('🧹 [UrlDrivenState] 切换账户/文件夹时清空当前邮件')
          mailStore.currentEmail = null
        }

        if (needAccountChange && hasValidFolder) {
          // 同时需要切换账户和文件夹，使用原子性操作
          console.log('🎯 [UrlDrivenState] 原子性操作 - 同时设置账户和文件夹:', {
            accountId: state.accountId,
            folderId: state.folderId
          })

          if (state.accountId !== null && state.accountId !== undefined &&
            state.folderId !== null && state.folderId !== undefined) {
            // 先直接设置账户ID（不触发自动选择）
            mailStore.selectAccount(state.accountId, true)
            // 然后设置文件夹ID
            await mailStore.selectFolder(state.folderId)
          }
        } else {
          // 分别处理账户和文件夹选择
          if (needAccountChange) {
            console.log('👤 [UrlDrivenState] 选择账户:', state.accountId)
            if (state.accountId !== null && state.accountId !== undefined) {
              await mailStore.selectAccount(state.accountId, false)
            }
          }

          if (needFolderChange) {
            console.log('📁 [UrlDrivenState] 选择文件夹:', state.folderId)
            if (state.folderId !== null && state.folderId !== undefined) {
              await mailStore.selectFolder(state.folderId)
            }
          } else if (hasValidFolder) {
            // 文件夹ID相同但可能需要刷新（如从虚拟文件夹切换过来）
            console.log('📁 [UrlDrivenState] 强制刷新邮件列表:', state.folderId)
            await mailStore.fetchEmails(1)
          }
        }
      }

      // 处理邮件选择
      if (state.mailId !== undefined && state.mailId !== mailStore.currentEmail?.email_id) {
        console.log('📧 [UrlDrivenState] 加载邮件:', state.mailId)
        try {
          await mailStore.loadEmailDetail(state.mailId)
        } catch (error) {
          console.warn('⚠️ [UrlDrivenState] 邮件加载失败，清空邮件选择:', state.mailId, error)
          // 邮件不存在或加载失败时，清空邮件ID
          // 直接更新URL以避免循环
          updateUrl({ mailId: undefined }, 'replace')
        }
      }

      // 处理设置弹窗
      if (state.settingsOpen !== settingsStore.isSettingsOpen) {
        console.log('⚙️ [UrlDrivenState] 更新设置状态:', state.settingsOpen)
        if (state.settingsOpen) {
          settingsStore.openSettings()
        } else {
          settingsStore.closeSettings()
        }
      }

      // 处理撰写邮件弹窗
      // TODO: 实现撰写邮件弹窗状态管理

    } catch (error) {
      console.error('❌ [UrlDrivenState] 同步到stores失败:', error)
    }
  }

  // 更新URL的不同策略
  type UrlUpdateStrategy = 'push' | 'replace' | 'auto'

  // 更新URL
  const updateUrl = (newState: Partial<AppState>, strategy: UrlUpdateStrategy = 'auto') => {
    // 合并当前状态和新状态
    const mergedState = { ...currentState.value, ...newState }

    // 构建新URL
    const newUrl = buildMainUrl(mergedState)
    console.log('🌐 [UrlDrivenState] 更新URL:', newUrl)

    // 决定使用 push 还是 replace
    let shouldReplace = false

    if (strategy === 'replace') {
      shouldReplace = true
    } else if (strategy === 'push') {
      shouldReplace = false
    } else {
      // 'auto' 模式：智能判断
      shouldReplace = determineReplaceStrategy(newState, mergedState)
    }

    // 导航到新URL
    if (shouldReplace) {
      console.log('🔄 [UrlDrivenState] 使用 replace 策略')
      router.replace(newUrl)
    } else {
      console.log('➡️ [UrlDrivenState] 使用 push 策略（创建历史记录）')
      router.push(newUrl)
    }
  }

  // 智能判断是否应该使用 replace
  const determineReplaceStrategy = (newState: Partial<AppState>, _mergedState: AppState): boolean => {
    // 关闭弹窗操作使用 replace（返回到之前的状态）
    if (newState.settingsOpen === false || newState.composeOpen === false) {
      return true
    }

    // 如果是清除邮件选择（从邮件详情返回列表），使用 replace
    if (newState.mailId === undefined && currentState.value.mailId !== undefined) {
      return true
    }

    // 如果是修正状态（比如设置了不存在的ID），使用 replace
    if (newState.accountId === null && currentState.value.accountId !== null) {
      return true
    }

    // 如果是在同一个页面内的微小状态变化（比如只更新composeMode），使用 replace
    const significantChanges = [
      'accountId', 'folderId', 'virtualFolder', 'mailId', 'settingsOpen', 'composeOpen'
    ]
    const hasSignificantChange = significantChanges.some(key =>
      newState[key as keyof AppState] !== undefined &&
      newState[key as keyof AppState] !== currentState.value[key as keyof AppState]
    )

    if (!hasSignificantChange) {
      return true
    }

    // 其他情况默认使用 push（创建历史记录）
    return false
  }

  // 公共接口实现
  const updateState = (updates: Partial<AppState>, strategy: UrlUpdateStrategy = 'auto') => {
    console.log('🔄 [UrlDrivenState] updateState:', updates)
    updateUrl(updates, strategy)
  }

  const selectAccount = (accountId: number) => {
    console.log('👤 [UrlDrivenState] selectAccount:', accountId)
    // 切换账户时清空邮件选择
    updateState({ accountId, virtualFolder: undefined, mailId: undefined }, 'push')
  }

  const selectFolder = (folderId: number) => {
    console.log('📁 [UrlDrivenState] selectFolder:', folderId)

    // 自动推断文件夹所属的账户ID
    const folder = mailStore.folders.find(f => f.folder_id === folderId)
    if (folder && folder.account_id) {
      // 切换文件夹时清空邮件选择
      updateState({
        accountId: folder.account_id,
        folderId,
        virtualFolder: undefined,
        mailId: undefined
      }, 'push')
    } else {
      console.warn('⚠️ [UrlDrivenState] 无法找到文件夹或账户ID:', folderId)
      // 切换文件夹时清空邮件选择
      updateState({ folderId, virtualFolder: undefined, mailId: undefined }, 'push')
    }
  }

  const selectVirtualFolder = (virtualFolder: string) => {
    console.log('✨ [UrlDrivenState] selectVirtualFolder:', virtualFolder)
    // 切换虚拟文件夹时清空邮件选择
    updateState({ virtualFolder, folderId: undefined, accountId: null, mailId: undefined }, 'push')
  }

  const selectMail = (mailId: number) => {
    console.log('📧 [UrlDrivenState] selectMail:', mailId)
    updateState({ mailId }, 'push')
  }

  const clearMail = () => {
    console.log('🔙 [UrlDrivenState] clearMail')
    updateState({ mailId: undefined }, 'replace')
  }

  const openSettings = () => {
    console.log('⚙️ [UrlDrivenState] openSettings')
    updateState({ settingsOpen: true }, 'push')
  }

  const closeSettings = () => {
    console.log('⚙️ [UrlDrivenState] closeSettings')
    updateState({ settingsOpen: false }, 'replace')
  }

  const openCompose = (mode: AppState['composeMode'] = 'new', mailId?: number) => {
    console.log('✍️ [UrlDrivenState] openCompose:', { mode, mailId })
    updateState({
      composeOpen: true,
      composeMode: mode,
      composeMailId: mailId
    }, 'push')
  }

  const closeCompose = () => {
    console.log('✍️ [UrlDrivenState] closeCompose')
    updateState({
      composeOpen: false,
      composeMailId: undefined
    }, 'replace')
  }

  const selectAccountAndFolder = (accountId: number, folderId: number) => {
    console.log('🎯 [UrlDrivenState] selectAccountAndFolder:', { accountId, folderId })
    // 同时切换账户和文件夹时清空邮件选择
    updateState({
      accountId,
      folderId,
      virtualFolder: undefined,
      mailId: undefined
    }, 'push')
  }

  const openMailView = (mailId: number) => {
    console.log('📖 [UrlDrivenState] openMailView:', mailId)
    const viewUrl = buildMailViewUrl(mailId)
    router.push(viewUrl)
  }

  // 监听URL变化
  let urlWatcher: (() => void) | null = null

  const initialize = () => {
    console.log('🚀 [UrlDrivenState] 初始化')

    // 立即解析当前URL
    parseAndUpdateState()

    // 监听URL变化
    urlWatcher = watch(
      () => route.fullPath,
      (newPath, oldPath) => {
        console.log('🌐 [UrlDrivenState] URL变化:', { 从: oldPath, 到: newPath })

        // 只处理主页面的URL变化
        if (isMainRoute(route.path)) {
          parseAndUpdateState()
        }
      },
      { immediate: false }
    )
  }

  const dispose = () => {
    console.log('🧹 [UrlDrivenState] 清理')
    if (urlWatcher) {
      urlWatcher()
      urlWatcher = null
    }
  }

  return {
    // 只读状态
    get state() {
      return currentState.value
    },

    // 状态更新方法
    updateState,
    selectAccount,
    selectFolder,
    selectVirtualFolder,
    selectMail,
    clearMail,
    openSettings,
    closeSettings,
    openCompose,
    closeCompose,
    selectAccountAndFolder,
    openMailView,

    // 生命周期
    initialize,
    dispose
  }
}
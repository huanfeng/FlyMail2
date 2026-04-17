<script lang="ts" setup>
import { ref, onMounted, computed, provide, onUnmounted, watch } from 'vue'
import { cn } from '@/lib/utils'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import { TooltipProvider } from '@/components/ui/tooltip'
import { Button } from '@/components/ui/button'
import { AlertCircle, X } from 'lucide-vue-next'
import AccountTree from '@/components/mail/AccountTree.vue'
import MailList from '@/components/mail/MailList.vue'
import MailDisplay from '@/components/mail/MailDisplay.vue'
import UserProfile from '@/components/mail/UserProfile.vue'
import SettingsDialog from '@/components/mail/SettingsDialog.vue'
import ComposeDialog from '@/components/mail/ComposeDialog.vue'
import { useMailApiStore } from '@/stores/mailApi'
import { useUrlDrivenState, type StateManager } from '@/composables/useUrlDrivenState'
import { useSessionState } from '@/composables/useSessionState'

const mailStore = useMailApiStore()
const urlStateManager = useUrlDrivenState()
const { useAutoSave, getPanelSizes, savePanelSizes } = useSessionState()

// 全局错误处理
const globalError = ref<string | null>(null)
const errorTimeout = ref<ReturnType<typeof setTimeout> | null>(null)

// 监听 mailStore 的错误状态
watch(() => mailStore.error, (newError) => {
  if (newError) {
    showGlobalError(newError)
  }
}, { immediate: true })

// 显示全局错误
function showGlobalError(message: string) {
  globalError.value = message

  // 清除之前的定时器
  if (errorTimeout.value) {
    clearTimeout(errorTimeout.value)
  }

  // 5秒后自动关闭
  errorTimeout.value = setTimeout(() => {
    globalError.value = null
  }, 5000)
}

// 手动关闭错误
function dismissError() {
  globalError.value = null
  // 同时清除 mailStore 的错误状态
  mailStore.clearError()

  if (errorTimeout.value) {
    clearTimeout(errorTimeout.value)
    errorTimeout.value = null
  }
}

// 使用计算属性从URL驱动状态管理器获取状态
const composeOpen = computed({
  get: () => urlStateManager.state.composeOpen,
  set: (value) => value ? urlStateManager.openCompose() : urlStateManager.closeCompose()
})

const composeMode = computed(() => urlStateManager.state.composeMode)
const composeEmail = computed(() => {
  if (urlStateManager.state.composeMailId) {
    // 使用any类型暂时避免类型不匹配问题
    return mailStore.emails.find(e => e.email_id === urlStateManager.state.composeMailId) as any
  }
  return undefined
})

// 创建状态管理器实例，提供给子组件使用
const stateManager: StateManager = {
  state: computed(() => urlStateManager.state),
  selectAccount: urlStateManager.selectAccount,
  selectFolder: urlStateManager.selectFolder,
  selectVirtualFolder: urlStateManager.selectVirtualFolder,
  selectMail: urlStateManager.selectMail,
  clearMail: urlStateManager.clearMail,
  openSettings: urlStateManager.openSettings,
  closeSettings: urlStateManager.closeSettings,
  openCompose: urlStateManager.openCompose,
  closeCompose: urlStateManager.closeCompose,
  selectAccountAndFolder: urlStateManager.selectAccountAndFolder,
  openMailView: urlStateManager.openMailView
}

// 提供给子组件使用
provide('stateManager', stateManager)

// 使用会话状态恢复面板大小
const savedPanelSizes = getPanelSizes()
const defaultLayout = savedPanelSizes || [20, 35, 45]
const isCollapsed = ref(false)
const navCollapsedSize = 4

// 监听面板大小变化
const handlePanelResize = (sizes: number[]) => {
  savePanelSizes(sizes)
}

// 启用自动保存
useAutoSave()

function onCollapse() {
  isCollapsed.value = true
}

function onExpand() {
  isCollapsed.value = false
}

// Initialize on mount
onMounted(async () => {
  console.log('🚀 [MailView] 开始初始化')

  // 首先初始化URL驱动状态管理器
  urlStateManager.initialize()

  // 然后初始化mailStore
  await mailStore.initialize()

  console.log('✅ [MailView] 初始化完成')
})

// Cleanup on unmount
onUnmounted(() => {
  console.log('🧹 [MailView] 清理URL状态管理器')
  urlStateManager.dispose()

  // 清理错误定时器
  if (errorTimeout.value) {
    clearTimeout(errorTimeout.value)
  }
})
</script>

<template>
  <TooltipProvider :delay-duration="200">
    <!-- 全局错误提示 -->
    <Transition enter-active-class="transition-all duration-300 ease-out"
      enter-from-class="opacity-0 transform -translate-y-full" enter-to-class="opacity-100 transform translate-y-0"
      leave-active-class="transition-all duration-300 ease-in" leave-from-class="opacity-100 transform translate-y-0"
      leave-to-class="opacity-0 transform -translate-y-full">
      <div v-if="globalError" class="fixed top-6 left-1/2 transform -translate-x-1/2 z-50 max-w-lg w-full mx-4">
        <div
          class="relative w-full rounded-lg border-2 border-red-500 dark:border-red-400 p-4 shadow-2xl backdrop-blur-sm bg-white dark:bg-gray-900">
          <div class="flex items-start gap-3">
            <AlertCircle class="h-5 w-5 text-red-500 dark:text-red-400 flex-shrink-0 mt-0.5" />
            <div class="flex-1 text-sm font-medium text-red-700 dark:text-red-300 leading-relaxed">
              {{ globalError }}
            </div>
            <Button variant="ghost" size="icon"
              class="h-8 w-8 hover:bg-red-100 dark:hover:bg-red-900/30 text-red-500 dark:text-red-400 hover:text-red-600 dark:hover:text-red-300 rounded-md flex items-center justify-center flex-shrink-0"
              @click="dismissError">
              <X class="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
    </Transition>

    <ResizablePanelGroup id="mail-panels" direction="horizontal" class="h-full items-stretch"
      @update:sizes="handlePanelResize">
      <!-- Left Panel - Account Tree -->
      <ResizablePanel id="account-panel" :default-size="defaultLayout[0]" :collapsed-size="navCollapsedSize" collapsible
        :min-size="15" :max-size="30" :class="cn(isCollapsed && 'min-w-[50px] transition-all duration-300 ease-in-out')"
        @expand="onExpand" @collapse="onCollapse">
        <div class="flex flex-col h-full">
          <UserProfile :is-collapsed="isCollapsed" />
          <div class="flex-1 overflow-hidden">
            <AccountTree :is-collapsed="isCollapsed" />
          </div>
        </div>
      </ResizablePanel>

      <ResizableHandle with-handle />

      <!-- Middle Panel - Mail List -->
      <ResizablePanel id="list-panel" :default-size="defaultLayout[1]" :min-size="25" :max-size="45">
        <MailList />
      </ResizablePanel>

      <ResizableHandle with-handle />

      <!-- Right Panel - Mail Display -->
      <ResizablePanel id="content-panel" :default-size="defaultLayout[2]" :min-size="30">
        <MailDisplay />
      </ResizablePanel>
    </ResizablePanelGroup>

    <!-- Settings Dialog -->
    <SettingsDialog />

    <!-- Compose Dialog -->
    <ComposeDialog v-model:open="composeOpen" :mode="composeMode" :original-email="composeEmail" />
  </TooltipProvider>
</template>
<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { Bell, Plus, RefreshCw, MoreVertical, Edit2, Trash2, Power, PowerOff, TestTube, GripVertical } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { notifyService } from '@/api'
import type { NotifyChannel, NotifyChannelType } from '@/api/types'
import { getErrorMessage } from '@/utils/error'
import NotifyChannelDialog from './NotifyChannelDialog.vue'
import { VueDraggable } from 'vue-draggable-plus'
import { useToast } from '@/components/ui/toast'

const { t } = useI18n()
const { toast } = useToast()

// 状态管理
const channels = ref<NotifyChannel[]>([])
const isLoading = ref(false)
const error = ref<string | null>(null)
const isRefreshing = ref(false)

// 对话框状态
const dialogOpen = ref(false)
const channelToEdit = ref<NotifyChannel | null>(null)
const deleteDialogOpen = ref(false)
const channelToDelete = ref<NotifyChannel | null>(null)
const isDeleting = ref(false)
const toggleDialogOpen = ref(false)
const channelToToggle = ref<NotifyChannel | null>(null)
const isToggling = ref<string | null>(null)
const isTesting = ref<string | null>(null)

// 渠道类型映射
const channelTypeLabels = computed<Record<NotifyChannelType, string>>(() => ({
  webhook: t('settings.notifications.channels.webhook'),
  feishu: t('settings.notifications.channels.feishu'),
  wecom: t('settings.notifications.channels.wecom'),
  telegram: t('settings.notifications.channels.telegram'),
  email: t('settings.notifications.channels.email'),
  dingtalk: t('settings.notifications.channels.dingtalk'),
  slack: t('settings.notifications.channels.slack')
}))

// 渠道类型颜色
const channelTypeColors: Record<NotifyChannelType, string> = {
  webhook: 'secondary',
  feishu: 'default',
  wecom: 'default', 
  telegram: 'default',
  email: 'default',
  dingtalk: 'default',
  slack: 'default'
}

// 计算属性：启用的渠道数量
const enabledChannelsCount = computed(() => {
  return channels.value.filter(c => c.enabled).length
})

// 加载通知渠道列表
async function loadChannels() {
  isLoading.value = true
  error.value = null
  try {
    const response = await notifyService.getChannels()
    if (response.code === 0 && response.data) {
      channels.value = response.data.channels || []
    } else {
      throw new Error(response.message || t('settings.notifications.channels.loadFailed'))
    }
  } catch (err) {
    error.value = getErrorMessage(err) || t('settings.notifications.channels.loadFailed')
  } finally {
    isLoading.value = false
  }
}

// 刷新列表
async function refreshChannels() {
  isRefreshing.value = true
  await loadChannels()
  isRefreshing.value = false
}

// 添加渠道
function handleAdd() {
  channelToEdit.value = null
  dialogOpen.value = true
}

// 编辑渠道
function handleEdit(channel: NotifyChannel) {
  channelToEdit.value = channel
  dialogOpen.value = true
}

// 渠道保存成功回调
function handleChannelSaved() {
  dialogOpen.value = false
  channelToEdit.value = null
  refreshChannels()
}

// 确认切换渠道状态
function confirmToggle(channel: NotifyChannel) {
  channelToToggle.value = channel
  toggleDialogOpen.value = true
}

// 切换渠道状态
async function toggleChannelStatus() {
  if (!channelToToggle.value || isToggling.value === channelToToggle.value.id) return

  isToggling.value = channelToToggle.value.id
  try {
    await notifyService.updateChannel(channelToToggle.value.id, {
      enabled: !channelToToggle.value.enabled
    })
    await refreshChannels()
    toggleDialogOpen.value = false
    channelToToggle.value = null
  } catch (err) {
    error.value = getErrorMessage(err) || t('settings.notifications.channels.toggleFailed')
  } finally {
    isToggling.value = null
  }
}

// 确认删除渠道
function confirmDelete(channel: NotifyChannel) {
  channelToDelete.value = channel
  deleteDialogOpen.value = true
}

// 删除渠道
async function deleteChannel() {
  if (!channelToDelete.value || isDeleting.value) return

  isDeleting.value = true
  try {
    await notifyService.deleteChannel(channelToDelete.value.id)
    await refreshChannels()
    deleteDialogOpen.value = false
    channelToDelete.value = null
  } catch (err) {
    error.value = getErrorMessage(err) || t('settings.notifications.channels.deleteFailed')
  } finally {
    isDeleting.value = false
  }
}

// 测试渠道
async function testChannel(channel: NotifyChannel) {
  if (isTesting.value === channel.id) return
  
  isTesting.value = channel.id
  try {
    const response = await notifyService.testChannel(channel.id)
    if (response.code === 0) {
      toast({
        description: t('settings.notifications.channels.testSuccess'),
      })
    } else {
      throw new Error(response.message || t('settings.notifications.channels.testFailed'))
    }
  } catch (err) {
    error.value = getErrorMessage(err) || t('settings.notifications.channels.testFailed')
  } finally {
    isTesting.value = null
  }
}

// 拖拽结束处理
async function handleDragEnd() {
  // 获取新的顺序
  const channelIds = channels.value.map(c => c.id)
  try {
    await notifyService.updateChannelOrder(channelIds)
  } catch (err) {
    error.value = getErrorMessage(err) || t('settings.notifications.channels.updateOrderFailed')
    // 重新加载以恢复原始顺序
    await loadChannels()
  }
}

// 获取状态徽章
function getStatusBadge(channel: NotifyChannel) {
  if (channel.enabled) {
    return { text: t('settings.notifications.channels.enabled'), variant: 'default' as const }
  } else {
    return { text: t('settings.notifications.channels.disabled'), variant: 'secondary' as const }
  }
}

// 格式化配置显示
function formatConfig(channel: NotifyChannel): string {
  const config = channel.config
  switch (channel.type) {
    case 'webhook':
      return config.webhook_url || t('settings.notifications.channels.notConfigured')
    case 'feishu':
      return config.app_id ? `AppID: ${config.app_id}` : t('settings.notifications.channels.notConfigured')
    case 'wecom':
      return config.corp_id ? `${t('settings.notifications.channels.corpId')}: ${config.corp_id}` : t('settings.notifications.channels.notConfigured')
    case 'telegram':
      return config.bot_token ? `Bot: ${config.bot_token.substring(0, 10)}...` : t('settings.notifications.channels.notConfigured')
    case 'email':
      return config.smtp_host ? `SMTP: ${config.smtp_host}` : t('settings.notifications.channels.notConfigured')
    case 'dingtalk':
      return config.webhook_url || t('settings.notifications.channels.notConfigured')
    case 'slack':
      return config.channel ? `${t('settings.notifications.channels.channel')}: ${config.channel}` : t('settings.notifications.channels.notConfigured')
    default:
      return t('settings.notifications.channels.unknownType')
  }
}

// 组件挂载时加载数据
onMounted(() => {
  loadChannels()
})
</script>

<template>
  <div class="space-y-6">
    <!-- 头部操作栏 -->
    <div class="flex items-center justify-between">
      <h3 class="text-lg font-medium">{{ t('settings.tabs.notifications') }}</h3>
      <div class="flex gap-2">
        <Button variant="outline" size="sm" @click="refreshChannels" :disabled="isRefreshing">
          <RefreshCw :class="{ 'animate-spin': isRefreshing }" class="h-4 w-4 mr-2" />
          {{ t('common.refresh') }}
        </Button>
        <Button size="sm" @click="handleAdd">
          <Plus class="h-4 w-4 mr-2" />
          {{ t('settings.notifications.channels.addChannel') }}
        </Button>
      </div>
    </div>

    <Separator />

    <!-- 统计信息 -->
    <div class="grid gap-4 md:grid-cols-3">
      <Card>
        <CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle class="text-sm font-medium">{{ t('settings.notifications.channels.totalChannels') }}</CardTitle>
          <Bell class="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div class="text-2xl font-bold">{{ channels.length }}</div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle class="text-sm font-medium">{{ t('settings.notifications.channels.enabledChannels') }}</CardTitle>
          <Power class="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div class="text-2xl font-bold">{{ enabledChannelsCount }}</div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader class="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle class="text-sm font-medium">{{ t('settings.notifications.channels.channelTypes') }}</CardTitle>
          <Bell class="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div class="text-2xl font-bold">{{ new Set(channels.map(c => c.type)).size }}</div>
        </CardContent>
      </Card>
    </div>

    <!-- 错误提示 -->
    <Alert v-if="error" variant="destructive">
      <AlertDescription>{{ error }}</AlertDescription>
    </Alert>

    <!-- 加载状态 -->
    <div v-if="isLoading" class="space-y-3">
      <Skeleton class="h-20 w-full" />
      <Skeleton class="h-20 w-full" />
      <Skeleton class="h-20 w-full" />
    </div>

    <!-- 渠道列表 -->
    <div v-else-if="channels.length > 0" class="space-y-3">
      <VueDraggable
        v-model="channels"
        :animation="200"
        handle=".drag-handle"
        @end="handleDragEnd"
      >
        <div
          v-for="channel in channels"
          :key="channel.id"
          class="rounded-lg border p-4 hover:bg-accent/50 transition-colors"
        >
          <div class="flex items-start justify-between">
            <div class="flex items-start gap-3 flex-1 min-w-0">
              <!-- 拖拽手柄 -->
              <div class="drag-handle cursor-move mt-1">
                <GripVertical class="h-5 w-5 text-muted-foreground" />
              </div>
              
              <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2 mb-2">
                  <div class="font-medium truncate">{{ channel.name }}</div>
                  <Badge :variant="channelTypeColors[channel.type] as any">
                    {{ channelTypeLabels[channel.type] }}
                  </Badge>
                  <Badge :variant="getStatusBadge(channel).variant">
                    {{ getStatusBadge(channel).text }}
                  </Badge>
                </div>
                <div class="text-sm text-muted-foreground mb-1">
                  {{ formatConfig(channel) }}
                </div>
                <div class="text-xs text-muted-foreground">
                  {{ t('settings.notifications.channels.retry') }}: {{ channel.max_retries || 3 }} | 
                  {{ t('settings.notifications.channels.interval') }}: {{ channel.retry_interval || 30 }} {{ t('settings.notifications.channels.seconds') }}
                  <span v-if="channel.events && channel.events.length > 0">
                    | {{ channel.events.length }} {{ t('settings.notifications.channels.subscribedEvents') }}
                  </span>
                </div>
              </div>
            </div>

            <DropdownMenu>
              <DropdownMenuTrigger as-child>
                <Button variant="ghost" size="icon" class="h-8 w-8">
                  <MoreVertical class="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem @click="handleEdit(channel)">
                  <Edit2 class="mr-2 h-4 w-4" />
                  {{ t('common.edit') }}
                </DropdownMenuItem>
                <DropdownMenuItem @click="testChannel(channel)" :disabled="isTesting === channel.id">
                  <TestTube class="mr-2 h-4 w-4" />
                  {{ t('common.test') }}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem @click="confirmToggle(channel)" :disabled="isToggling === channel.id">
                  <Power v-if="!channel.enabled" class="mr-2 h-4 w-4" />
                  <PowerOff v-else class="mr-2 h-4 w-4" />
                  {{ channel.enabled ? t('common.disable') : t('common.enable') }}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  @click="confirmDelete(channel)"
                  class="text-destructive focus:text-destructive"
                >
                  <Trash2 class="mr-2 h-4 w-4" />
                  {{ t('common.delete') }}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </VueDraggable>
    </div>

    <!-- 空状态 -->
    <div v-else class="text-center py-12">
      <Bell class="mx-auto h-12 w-12 text-muted-foreground mb-4" />
      <h3 class="text-lg font-medium mb-2">{{ t('settings.notifications.channels.noChannels') }}</h3>
      <p class="text-sm text-muted-foreground mb-4">
        {{ t('settings.notifications.channels.noChannelsDesc') }}
      </p>
      <Button size="sm" @click="handleAdd">
        <Plus class="h-4 w-4 mr-2" />
        {{ t('settings.notifications.channels.addFirstChannel') }}
      </Button>
    </div>

    <!-- 添加/编辑渠道对话框 -->
    <NotifyChannelDialog
      v-model:open="dialogOpen"
      :channel="channelToEdit"
      @saved="handleChannelSaved"
    />

    <!-- 删除确认对话框 -->
    <AlertDialog v-model:open="deleteDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{{ t('settings.notifications.channels.confirmDelete') }}</AlertDialogTitle>
          <AlertDialogDescription>
            {{ t('settings.notifications.channels.confirmDeleteDesc', { name: channelToDelete?.name }) }}
            <br />
            <strong>{{ t('settings.notifications.channels.confirmDeleteWarning') }}</strong>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction
            @click="deleteChannel"
            :disabled="isDeleting"
            class="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            {{ isDeleting ? t('common.deleting') : t('common.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>

    <!-- 启用/禁用确认对话框 -->
    <AlertDialog v-model:open="toggleDialogOpen">
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {{ channelToToggle?.enabled ? t('settings.notifications.channels.confirmDisable') : t('settings.notifications.channels.confirmEnable') }}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {{ channelToToggle?.enabled 
              ? t('settings.notifications.channels.confirmDisableDesc')
              : t('settings.notifications.channels.confirmEnableDesc')
            }}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{{ t('common.cancel') }}</AlertDialogCancel>
          <AlertDialogAction @click="toggleChannelStatus" :disabled="isToggling === channelToToggle?.id">
            {{ isToggling === channelToToggle?.id ? t('common.processing') : t('common.confirm') }}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
</template>
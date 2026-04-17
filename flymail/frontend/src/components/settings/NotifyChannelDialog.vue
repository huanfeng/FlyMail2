<script lang="ts" setup>
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { TestTube, Loader2 } from 'lucide-vue-next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { notifyService } from '@/api'
import type { NotifyChannel, NotifyChannelType, CreateNotifyChannelRequest, UpdateNotifyChannelRequest } from '@/api/types'
import { getErrorMessage } from '@/utils/error'
import { useToast } from '@/components/ui/toast'

const { t } = useI18n()
const { toast } = useToast()

const props = defineProps<{
  channel?: NotifyChannel | null
}>()

const emit = defineEmits<{
  saved: []
}>()

const open = defineModel<boolean>('open', { default: false })

// 表单数据
const formData = ref<{
  name: string
  type: NotifyChannelType
  enabled: boolean
  max_retries: number
  retry_interval: number
  config: Record<string, any>
}>({
  name: '',
  type: 'webhook',
  enabled: true,
  max_retries: 3,
  retry_interval: 30,
  config: {}
})

// 状态
const isSubmitting = ref(false)
const isTesting = ref(false)
const error = ref<string | null>(null)

// 渠道类型选项
const channelTypeOptions = computed(() => [
  { value: 'webhook', label: t('settings.notifications.channels.webhook') },
  { value: 'feishu', label: t('settings.notifications.channels.feishu') },
  { value: 'wecom', label: t('settings.notifications.channels.wecom') },
  { value: 'telegram', label: t('settings.notifications.channels.telegram') },
  { value: 'email', label: t('settings.notifications.channels.email') },
  { value: 'dingtalk', label: t('settings.notifications.channels.dingtalk') },
  { value: 'slack', label: t('settings.notifications.channels.slack') },
])

// 计算属性：是否为编辑模式
const isEditMode = computed(() => !!props.channel)

// 计算属性：对话框标题
const dialogTitle = computed(() => isEditMode.value ? t('settings.notifications.channels.editChannel') : t('settings.notifications.channels.addChannel'))

// 计算属性：当前选中的渠道类型标签
const selectedTypeLabel = computed(() => {
  const option = channelTypeOptions.value.find(opt => opt.value === formData.value.type)
  return option?.label || formData.value.type
})

// 监听 channel 属性变化
watch(() => props.channel, (newChannel) => {
  if (newChannel) {
    formData.value = {
      name: newChannel.name,
      type: newChannel.type,
      enabled: newChannel.enabled,
      max_retries: newChannel.max_retries || 3,
      retry_interval: newChannel.retry_interval || 30,
      config: { ...newChannel.config }
    }
  } else {
    resetForm()
  }
}, { immediate: true })

// 监听类型变化，重置配置
watch(() => formData.value.type, (newType, oldType) => {
  if (newType !== oldType && !isEditMode.value) {
    formData.value.config = getDefaultConfig(newType)
  }
})

// 获取默认配置
function getDefaultConfig(type: NotifyChannelType): Record<string, any> {
  switch (type) {
    case 'webhook':
    case 'dingtalk':
      return { webhook_url: '' }
    case 'feishu':
      return { webhook_url: '', app_id: '', app_secret: '' }
    case 'wecom':
      return { webhook_url: '', corp_id: '', agent_id: '', secret: '' }
    case 'telegram':
      return { bot_token: '', chat_id: '' }
    case 'email':
      return {
        smtp_host: '',
        smtp_port: 587,
        smtp_user: '',
        smtp_password: '',
        from_email: '',
        to_emails: []
      }
    case 'slack':
      return { webhook_url: '', bot_token: '', channel: '' }
    default:
      return {}
  }
}

// 重置表单
function resetForm() {
  formData.value = {
    name: '',
    type: 'webhook',
    enabled: true,
    max_retries: 3,
    retry_interval: 30,
    config: getDefaultConfig('webhook')
  }
  error.value = null
}

// 验证表单
function validateForm(): boolean {
  if (!formData.value.name.trim()) {
    error.value = t('settings.notifications.channels.channelNameRequired')
    return false
  }

  // 根据类型验证必填字段
  const config = formData.value.config
  switch (formData.value.type) {
    case 'webhook':
    case 'dingtalk':
      if (!config.webhook_url) {
        error.value = t('settings.notifications.channels.webhookUrlRequired')
        return false
      }
      break
    case 'feishu':
      if (!config.webhook_url && (!config.app_id || !config.app_secret)) {
        error.value = t('settings.notifications.channels.appCredentialsRequired')
        return false
      }
      break
    case 'wecom':
      if (!config.webhook_url && (!config.corp_id || !config.agent_id || !config.secret)) {
        error.value = t('settings.notifications.channels.corpConfigRequired')
        return false
      }
      break
    case 'telegram':
      if (!config.bot_token || !config.chat_id) {
        error.value = t('settings.notifications.channels.botTokenRequired')
        return false
      }
      break
    case 'email':
      if (!config.smtp_host || !config.smtp_user || !config.from_email) {
        error.value = t('settings.notifications.channels.smtpConfigRequired')
        return false
      }
      if (!config.to_emails || config.to_emails.length === 0) {
        error.value = t('settings.notifications.channels.recipientRequired')
        return false
      }
      break
    case 'slack':
      if (!config.webhook_url && (!config.bot_token || !config.channel)) {
        error.value = t('settings.notifications.channels.slackConfigRequired')
        return false
      }
      break
  }

  return true
}

// 提交表单
async function handleSubmit() {
  if (!validateForm()) return

  isSubmitting.value = true
  error.value = null

  try {
    if (isEditMode.value && props.channel) {
      // 编辑模式
      const updateData: UpdateNotifyChannelRequest = {
        name: formData.value.name,
        enabled: formData.value.enabled,
        max_retries: formData.value.max_retries,
        retry_interval: formData.value.retry_interval,
        config: formData.value.config
      }
      await notifyService.updateChannel(props.channel.id, updateData)
    } else {
      // 新增模式
      const createData: CreateNotifyChannelRequest = {
        name: formData.value.name,
        type: formData.value.type,
        enabled: formData.value.enabled,
        max_retries: formData.value.max_retries,
        retry_interval: formData.value.retry_interval,
        config: formData.value.config
      }
      await notifyService.createChannel(createData)
    }

    emit('saved')
    open.value = false
    resetForm()
  } catch (err) {
    error.value = getErrorMessage(err) || t('settings.notifications.channels.saveFailed')
  } finally {
    isSubmitting.value = false
  }
}

// 处理邮件地址输入
function handleEmailsInput(event: Event) {
  const target = event.target as HTMLTextAreaElement
  const emails = target.value.split(/[,\n]/).map(e => e.trim()).filter(e => e)
  formData.value.config.to_emails = emails
}

// 获取邮件地址显示文本
function getEmailsText(): string {
  return formData.value.config.to_emails?.join('\n') || ''
}

// 测试通知渠道
async function handleTest() {
  if (!props.channel) return

  isTesting.value = true
  error.value = null

  try {
    const response = await notifyService.testChannel(props.channel.id)
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
    isTesting.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogContent class="sm:max-w-[600px] flex flex-col h-[70vh] max-h-[70vh]">
      <!-- 固定的标题栏 -->
      <DialogHeader class="flex-shrink-0">
        <DialogTitle>{{ dialogTitle }}</DialogTitle>
        <DialogDescription>
          {{ t('settings.notifications.channels.description') }}
        </DialogDescription>
      </DialogHeader>

      <!-- 可滚动的内容区域 -->
      <div class="flex-1 overflow-y-auto px-1 -mx-1 py-4 space-y-6">
        <!-- 错误提示 -->
        <Alert v-if="error" variant="destructive">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>

        <!-- 基本信息 -->
        <div class="space-y-4">
          <div class="grid gap-2">
            <Label for="name">{{ t('settings.notifications.channels.channelName') }}</Label>
            <Input
              id="name"
              v-model="formData.name"
              :placeholder="t('settings.notifications.channels.channelNamePlaceholder')"
              :disabled="isSubmitting"
            />
          </div>

          <div class="grid gap-2">
            <Label for="type">{{ t('settings.notifications.channels.channelType') }}</Label>
            <Select
              :modelValue="formData.type"
              @update:modelValue="(value) => formData.type = value as NotifyChannelType"
              :disabled="isEditMode || isSubmitting"
            >
              <SelectTrigger id="type">
                <span>{{ selectedTypeLabel }}</span>
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="option in channelTypeOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div class="flex items-center space-x-2">
            <Switch id="enabled" v-model:checked="formData.enabled" :disabled="isSubmitting" />
            <Label for="enabled">{{ t('settings.notifications.channels.enableChannel') }}</Label>
          </div>
        </div>

        <!-- 高级设置 -->
        <Tabs default-value="config" class="w-full flex flex-col flex-1">
          <TabsList class="grid w-full grid-cols-2">
            <TabsTrigger value="config">{{ t('settings.notifications.channels.channelConfig') }}</TabsTrigger>
            <TabsTrigger value="advanced">{{ t('settings.notifications.channels.advancedSettings') }}</TabsTrigger>
          </TabsList>

          <!-- 渠道配置 -->
          <TabsContent value="config" class="space-y-4 mt-4">
            <!-- Webhook 配置 -->
            <template v-if="formData.type === 'webhook' || formData.type === 'dingtalk'">
              <div class="grid gap-2">
                <Label for="webhook_url">{{ t('settings.notifications.channels.webhookUrl') }}</Label>
                <Input
                  id="webhook_url"
                  v-model="formData.config.webhook_url"
                  type="url"
                  placeholder="https://..."
                  :disabled="isSubmitting"
                />
              </div>
              <div v-if="formData.type === 'dingtalk'" class="grid gap-2">
                <Label for="sign_secret">{{ t('settings.notifications.channels.signSecret') }}</Label>
                <Input
                  id="sign_secret"
                  v-model="formData.config.sign_secret"
                  type="password"
                  placeholder="SEC..."
                  :disabled="isSubmitting"
                />
              </div>
            </template>

            <!-- 飞书配置 -->
            <template v-else-if="formData.type === 'feishu'">
              <div class="grid gap-2">
                <Label for="feishu_webhook">{{ t('settings.notifications.channels.webhookUrlRecommended') }}</Label>
                <Input
                  id="feishu_webhook"
                  v-model="formData.config.webhook_url"
                  type="url"
                  placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..."
                  :disabled="isSubmitting"
                />
              </div>
              <div class="text-xs text-center text-muted-foreground my-2">{{ t('settings.notifications.channels.orUseAppCredentials') }}</div>
              <div class="grid gap-2">
                <Label for="app_id">{{ t('settings.notifications.channels.appId') }}</Label>
                <Input
                  id="app_id"
                  v-model="formData.config.app_id"
                  placeholder="cli_..."
                  :disabled="isSubmitting"
                />
              </div>
              <div class="grid gap-2">
                <Label for="app_secret">{{ t('settings.notifications.channels.appSecret') }}</Label>
                <Input
                  id="app_secret"
                  v-model="formData.config.app_secret"
                  type="password"
                  placeholder="..."
                  :disabled="isSubmitting"
                />
              </div>
            </template>

            <!-- 企业微信配置 -->
            <template v-else-if="formData.type === 'wecom'">
              <div class="grid gap-2">
                <Label for="wecom_webhook">{{ t('settings.notifications.channels.webhookUrlRecommended') }}</Label>
                <Input
                  id="wecom_webhook"
                  v-model="formData.config.webhook_url"
                  type="url"
                  placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..."
                  :disabled="isSubmitting"
                />
              </div>
              <div class="text-xs text-center text-muted-foreground my-2">{{ t('settings.notifications.channels.orUseAppConfig') }}</div>
              <div class="grid gap-2">
                <Label for="corp_id">{{ t('settings.notifications.channels.corpId') }}</Label>
                <Input
                  id="corp_id"
                  v-model="formData.config.corp_id"
                  placeholder="ww..."
                  :disabled="isSubmitting"
                />
              </div>
              <div class="grid gap-2">
                <Label for="agent_id">{{ t('settings.notifications.channels.agentId') }}</Label>
                <Input
                  id="agent_id"
                  v-model="formData.config.agent_id"
                  placeholder="1000002"
                  :disabled="isSubmitting"
                />
              </div>
              <div class="grid gap-2">
                <Label for="wecom_secret">{{ t('settings.notifications.channels.secret') }}</Label>
                <Input
                  id="wecom_secret"
                  v-model="formData.config.secret"
                  type="password"
                  placeholder="..."
                  :disabled="isSubmitting"
                />
              </div>
            </template>

            <!-- Telegram 配置 -->
            <template v-else-if="formData.type === 'telegram'">
              <div class="grid gap-2">
                <Label for="bot_token">{{ t('settings.notifications.channels.botToken') }}</Label>
                <Input
                  id="bot_token"
                  v-model="formData.config.bot_token"
                  type="password"
                  placeholder="123456:ABC-DEF..."
                  :disabled="isSubmitting"
                />
              </div>
              <div class="grid gap-2">
                <Label for="chat_id">{{ t('settings.notifications.channels.chatId') }}</Label>
                <Input
                  id="chat_id"
                  v-model="formData.config.chat_id"
                  placeholder="-1001234567890"
                  :disabled="isSubmitting"
                />
              </div>
            </template>

            <!-- 邮件配置 -->
            <template v-else-if="formData.type === 'email'">
              <div class="grid gap-2">
                <Label for="smtp_host">{{ t('settings.notifications.channels.smtpServer') }}</Label>
                <Input
                  id="smtp_host"
                  v-model="formData.config.smtp_host"
                  placeholder="smtp.gmail.com"
                  :disabled="isSubmitting"
                />
              </div>
              <div class="grid gap-2">
                <Label for="smtp_port">{{ t('settings.notifications.channels.smtpPort') }}</Label>
                <Input
                  id="smtp_port"
                  v-model.number="formData.config.smtp_port"
                  type="number"
                  placeholder="587"
                  :disabled="isSubmitting"
                />
              </div>
              <div class="grid gap-2">
                <Label for="smtp_user">{{ t('settings.notifications.channels.smtpUsername') }}</Label>
                <Input
                  id="smtp_user"
                  v-model="formData.config.smtp_user"
                  placeholder="your-email@gmail.com"
                  :disabled="isSubmitting"
                />
              </div>
              <div class="grid gap-2">
                <Label for="smtp_password">{{ t('settings.notifications.channels.smtpPassword') }}</Label>
                <Input
                  id="smtp_password"
                  v-model="formData.config.smtp_password"
                  type="password"
                  placeholder="..."
                  :disabled="isSubmitting"
                />
              </div>
              <div class="grid gap-2">
                <Label for="from_email">{{ t('settings.notifications.channels.fromEmail') }}</Label>
                <Input
                  id="from_email"
                  v-model="formData.config.from_email"
                  type="email"
                  placeholder="noreply@example.com"
                  :disabled="isSubmitting"
                />
              </div>
              <div class="grid gap-2">
                <Label for="to_emails">{{ t('settings.notifications.channels.toEmails') }}</Label>
                <Textarea
                  id="to_emails"
                  :value="getEmailsText()"
                  @input="handleEmailsInput"
                  placeholder="admin@example.com&#10;alert@example.com"
                  class="min-h-[100px]"
                  :disabled="isSubmitting"
                />
              </div>
            </template>

            <!-- Slack 配置 -->
            <template v-else-if="formData.type === 'slack'">
              <div class="grid gap-2">
                <Label for="slack_webhook">{{ t('settings.notifications.channels.webhookUrlRecommended') }}</Label>
                <Input
                  id="slack_webhook"
                  v-model="formData.config.webhook_url"
                  type="url"
                  placeholder="https://hooks.slack.com/services/..."
                  :disabled="isSubmitting"
                />
              </div>
              <div class="text-xs text-center text-muted-foreground my-2">{{ t('settings.notifications.channels.orUseBotToken') }}</div>
              <div class="grid gap-2">
                <Label for="slack_token">{{ t('settings.notifications.channels.botToken') }}</Label>
                <Input
                  id="slack_token"
                  v-model="formData.config.bot_token"
                  type="password"
                  placeholder="xoxb-..."
                  :disabled="isSubmitting"
                />
              </div>
              <div class="grid gap-2">
                <Label for="slack_channel">{{ t('settings.notifications.channels.channel') }}</Label>
                <Input
                  id="slack_channel"
                  v-model="formData.config.channel"
                  placeholder="#general"
                  :disabled="isSubmitting"
                />
              </div>
            </template>
          </TabsContent>

          <!-- 高级设置 -->
          <TabsContent value="advanced" class="space-y-4">
            <div class="grid gap-2">
              <Label for="max_retries">{{ t('settings.notifications.channels.maxRetries') }}</Label>
              <Input
                id="max_retries"
                v-model.number="formData.max_retries"
                type="number"
                min="0"
                max="10"
                :disabled="isSubmitting"
              />
            </div>
            <div class="grid gap-2">
              <Label for="retry_interval">{{ t('settings.notifications.channels.retryInterval') }}</Label>
              <Input
                id="retry_interval"
                v-model.number="formData.retry_interval"
                type="number"
                min="1"
                max="3600"
                :disabled="isSubmitting"
              />
            </div>
          </TabsContent>
        </Tabs>
      </div>

      <!-- 固定的底部按钮栏 -->
      <DialogFooter class="flex-shrink-0 pt-4 border-t">
        <div class="flex items-center justify-between w-full">
          <div>
            <Button
              v-if="isEditMode && formData.enabled"
              variant="outline"
              size="sm"
              @click="handleTest"
              :disabled="isSubmitting || isTesting"
            >
              <TestTube v-if="!isTesting" class="mr-2 h-4 w-4" />
              <Loader2 v-if="isTesting" class="mr-2 h-4 w-4 animate-spin" />
              {{ isTesting ? t('settings.notifications.channels.testing') : t('settings.notifications.channels.testConnection') }}
            </Button>
          </div>
          <div class="flex gap-2">
            <Button variant="outline" @click="open = false" :disabled="isSubmitting">
              {{ t('settings.notifications.channels.cancel') }}
            </Button>
            <Button @click="handleSubmit" :disabled="isSubmitting">
              {{ isSubmitting ? t('settings.notifications.channels.saving') : t('settings.notifications.channels.save') }}
            </Button>
          </div>
        </div>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
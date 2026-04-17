<script lang="ts" setup>
import { ref, computed, watch } from 'vue'
import { Edit, Save, Loader2, AlertCircle, Eye, EyeOff, Wifi, Check, X } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { detectEmailProvider } from '@/utils/emailProviders'
import { accountsService } from '@/api'
import type { EmailAccount, CreateAccountRequest } from '@/api/types'
import { getErrorMessage } from '@/utils/error'

interface Props {
  account: EmailAccount
  standalone?: boolean
  open?: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  accountUpdated: [account: EmailAccount]
  'update:open': [value: boolean]
}>()

const { t } = useI18n()

const open = computed({
  get: () => props.standalone && props.open !== undefined ? props.open : internalOpen.value,
  set: (value) => {
    if (props.standalone && props.open !== undefined) {
      emit('update:open', value)
    } else {
      internalOpen.value = value
    }
  }
})

const internalOpen = ref(false)
const isLoading = ref(false)
const isTesting = ref(false)
const error = ref<string | null>(null)
const testResult = ref<{ imap: boolean, smtp: boolean, supports_idle: boolean } | null>(null)
const showPassword = ref(false)

// 表单数据
const formData = ref<CreateAccountRequest>({
  name: '',
  email: '',
  type: 'imap',
  imap_server: '',
  imap_port: 993,
  imap_ssl: true,
  smtp_server: '',
  smtp_port: 587,
  smtp_ssl: true,
  username: '',
  password: '',
  signature: '',
  auto_reply: false,
  auto_reply_message: ''
})

// 检测到的邮箱提供商
const detectedProvider = computed(() => {
  return detectEmailProvider(formData.value.email)
})

// 初始化表单数据
function initForm() {
  formData.value = {
    name: props.account.name,
    email: props.account.email,
    type: props.account.type as 'smtp' | 'imap' | 'oauth',
    imap_server: props.account.imap_server || '',
    imap_port: props.account.imap_port || 993,
    imap_ssl: props.account.imap_ssl ?? true,
    smtp_server: props.account.smtp_server || '',
    smtp_port: props.account.smtp_port || 587,
    smtp_ssl: props.account.smtp_ssl ?? true,
    username: props.account.username || '',
    password: '', // Password is empty, user needs to re-enter
    signature: props.account.signature || '',
    auto_reply: props.account.auto_reply ?? false,
    auto_reply_message: props.account.auto_reply_message || ''
  }
}

// 监听对话框打开状态
watch(open, (isOpen) => {
  if (isOpen) {
    initForm()
    error.value = null
    testResult.value = null
  }
})

// 在 standalone 模式下，监听 account 变化
watch(() => props.account, () => {
  if (props.standalone && open.value) {
    initForm()
  }
}, { immediate: true })

// 验证表单
const isFormValid = computed(() => {
  return formData.value.name &&
         formData.value.email &&
         formData.value.imap_server &&
         formData.value.smtp_server &&
         formData.value.username
})

// 是否可以测试连接
const canTestConnection = computed(() => {
  return isFormValid.value && formData.value.password
})

async function updateAccount() {
  if (!isFormValid.value) {
    error.value = t('account.edit.requiredFields')
    return
  }

  isLoading.value = true
  error.value = null

  try {
    // 创建更新数据的副本
    const updateData = { ...formData.value }

    // 如果密码为空，删除密码字段
    if (!updateData.password) {
      delete updateData.password
    }

    await accountsService.updateAccount(props.account.account_id, updateData)

    // 获取更新后的账户信息
    const updatedAccount = await accountsService.getAccount(props.account.account_id)

    emit('accountUpdated', updatedAccount)
    open.value = false
  } catch (err) {
    error.value = getErrorMessage(err) || t('account.edit.updateFailed')
  } finally {
    isLoading.value = false
  }
}

async function testConnection() {
  if (!canTestConnection.value) {
    error.value = t('account.edit.testRequiresPassword')
    return
  }

  isTesting.value = true
  error.value = null
  testResult.value = null

  try {
    const testData = { ...formData.value }
    const result = await accountsService.testTempAccount(testData)
    testResult.value = {
      imap: result.imap,
      smtp: result.smtp,
      supports_idle: result.supports_idle
    }
  } catch (err) {
    error.value = getErrorMessage(err) || t('account.edit.testFailed')
  } finally {
    isTesting.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child v-if="!props.standalone">
      <Button variant="ghost" size="sm" class="gap-2">
        <Edit class="h-3 w-3" />
        {{ t('common.edit') }}
      </Button>
    </DialogTrigger>
    <DialogContent class="sm:max-w-3xl flex flex-col max-h-[90vh]">
      <!-- 固定的标题栏 -->
      <DialogHeader class="flex-shrink-0">
        <DialogTitle>{{ t('account.edit.title') }}</DialogTitle>
        <DialogDescription>
          {{ t('account.edit.subtitle') }}
        </DialogDescription>
      </DialogHeader>

      <!-- 可滚动的内容区域 -->
      <div class="flex-1 overflow-y-auto pr-2 -mr-2">
        <!-- 错误提示 -->
        <Alert v-if="error" variant="destructive" class="mb-4">
          <AlertCircle class="h-4 w-4" />
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>

        <!-- 基本信息和认证信息 -->
        <Card class="mb-4">
          <CardHeader>
            <CardTitle class="text-base">{{ t('account.edit.basicInfo') }}</CardTitle>
          </CardHeader>
          <CardContent class="space-y-4">
            <div class="grid grid-cols-2 gap-4">
              <div class="space-y-2">
                <Label for="edit_name">{{ t('account.edit.displayName') }}</Label>
                <Input
                  id="edit_name"
                  v-model="formData.name"
                  :placeholder="t('account.edit.displayNamePlaceholder')"
                />
              </div>
              <div class="space-y-2">
                <Label for="edit_email">{{ t('account.edit.email') }}</Label>
                <Input
                  id="edit_email"
                  v-model="formData.email"
                  type="email"
                  placeholder="your.email@example.com"
                />
              </div>
            </div>

            <div class="grid grid-cols-2 gap-4">
              <div class="space-y-2">
                <Label for="edit_username">{{ t('account.edit.username') }}</Label>
                <Input
                  id="edit_username"
                  v-model="formData.username"
                  :placeholder="t('account.edit.usernameHint')"
                />
              </div>
              <div class="space-y-2">
                <Label for="edit_password">{{ t('account.edit.password') }}</Label>
                <div class="relative">
                  <Input
                    id="edit_password"
                    v-model="formData.password"
                    :type="showPassword ? 'text' : 'password'"
                    :placeholder="t('account.edit.passwordHint')"
                    class="pr-10"
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    class="absolute right-0 top-0 h-full w-10"
                    @click="showPassword = !showPassword"
                  >
                    <Eye v-if="showPassword" class="h-4 w-4" />
                    <EyeOff v-else class="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </div>

            <div v-if="detectedProvider">
              <div class="text-sm text-muted-foreground">
                {{ detectedProvider.description }}
              </div>
              <div v-if="detectedProvider.authType === 'app_password'" class="text-sm text-amber-600 mt-1">
                <AlertCircle class="h-3 w-3 inline mr-1" />
                {{ t('account.edit.appPasswordRequired') }}
              </div>
            </div>
          </CardContent>
        </Card>

        <!-- 服务器设置 -->
        <Card class="mb-4">
          <CardHeader>
            <CardTitle class="text-base">{{ t('account.edit.serverSettings') }}</CardTitle>
          </CardHeader>
          <CardContent class="space-y-4">
            <!-- IMAP 设置 -->
            <div class="space-y-2">
              <Label class="text-sm font-medium">{{ t('account.edit.imapTitle') }}</Label>
              <div class="grid grid-cols-12 gap-2 items-end">
                <div class="col-span-6 space-y-1">
                  <Label class="text-xs text-muted-foreground">{{ t('account.edit.serverAddress') }}</Label>
                  <Input
                    v-model="formData.imap_server"
                    placeholder="imap.example.com"
                  />
                </div>
                <div class="col-span-3 space-y-1">
                  <Label class="text-xs text-muted-foreground">{{ t('account.edit.port') }}</Label>
                  <Input
                    v-model.number="formData.imap_port"
                    type="number"
                    placeholder="993"
                  />
                </div>
                <div class="col-span-3 flex items-center gap-2 pb-2">
                  <Switch id="edit_imap_ssl" v-model="formData.imap_ssl" />
                  <Label for="edit_imap_ssl" class="text-sm">{{ t('account.edit.useSSL') }}</Label>
                </div>
              </div>
            </div>

            <!-- SMTP 设置 -->
            <div class="space-y-2">
              <Label class="text-sm font-medium">{{ t('account.edit.smtpTitle') }}</Label>
              <div class="grid grid-cols-12 gap-2 items-end">
                <div class="col-span-6 space-y-1">
                  <Label class="text-xs text-muted-foreground">{{ t('account.edit.serverAddress') }}</Label>
                  <Input
                    v-model="formData.smtp_server"
                    placeholder="smtp.example.com"
                  />
                </div>
                <div class="col-span-3 space-y-1">
                  <Label class="text-xs text-muted-foreground">{{ t('account.edit.port') }}</Label>
                  <Input
                    v-model.number="formData.smtp_port"
                    type="number"
                    placeholder="587"
                  />
                </div>
                <div class="col-span-3 flex items-center gap-2 pb-2">
                  <Switch id="edit_smtp_ssl" v-model="formData.smtp_ssl" />
                  <Label for="edit_smtp_ssl" class="text-sm">{{ t('account.edit.useSSL') }}</Label>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <!-- 邮件设置 -->
        <Card class="mb-4">
          <CardHeader>
            <CardTitle class="text-base">{{ t('account.edit.emailSettings') }}</CardTitle>
          </CardHeader>
          <CardContent class="space-y-4">
            <div class="space-y-2">
              <Label for="edit_signature">{{ t('account.edit.signature') }}</Label>
              <Textarea
                id="edit_signature"
                v-model="formData.signature"
                :placeholder="t('account.edit.signaturePlaceholder')"
                class="min-h-[80px]"
              />
            </div>
          </CardContent>
        </Card>

        <!-- 测试结果 -->
        <Card v-if="testResult" class="mb-4">
          <CardHeader>
            <CardTitle class="text-base">{{ t('account.edit.testResult') }}</CardTitle>
          </CardHeader>
          <CardContent>
            <div class="space-y-2 text-sm">
              <div class="flex items-center gap-2">
                <Check v-if="testResult.imap" class="h-4 w-4 text-green-600" />
                <X v-else class="h-4 w-4 text-red-600" />
                <span>{{ testResult.imap ? t('account.edit.imapSuccess') : t('account.edit.imapFailed') }}</span>
              </div>
              <div class="flex items-center gap-2">
                <Check v-if="testResult.smtp" class="h-4 w-4 text-green-600" />
                <X v-else class="h-4 w-4 text-red-600" />
                <span>{{ testResult.smtp ? t('account.edit.smtpSuccess') : t('account.edit.smtpFailed') }}</span>
              </div>
              <div class="flex items-center gap-2">
                <Check v-if="testResult.supports_idle" class="h-4 w-4 text-green-600" />
                <X v-else class="h-4 w-4 text-amber-600" />
                <span>{{ testResult.supports_idle ? t('account.edit.supportsIdle') : t('account.edit.requiresPolling') }}</span>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <!-- 固定的操作按钮栏 -->
      <div class="flex justify-end gap-2 pt-4 border-t flex-shrink-0">
        <Button variant="outline" @click="open = false">
          {{ t('common.cancel') }}
        </Button>
        <Button
          variant="outline"
          @click="testConnection"
          :disabled="!canTestConnection || isTesting || isLoading"
        >
          <Loader2 v-if="isTesting" class="mr-2 h-4 w-4 animate-spin" />
          <Wifi v-else class="mr-2 h-4 w-4" />
          {{ isTesting ? t('common.testing') : t('account.edit.testConnection') }}
        </Button>
        <Button
          @click="updateAccount"
          :disabled="!isFormValid || isLoading"
        >
          <Loader2 v-if="isLoading" class="mr-2 h-4 w-4 animate-spin" />
          <Save v-else class="mr-2 h-4 w-4" />
          {{ isLoading ? t('common.saving') : t('account.edit.saveChanges') }}
        </Button>
      </div>
    </DialogContent>
  </Dialog>
</template>
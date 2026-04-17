<script lang="ts" setup>
import { ref, computed, watch } from 'vue'
import { Plus, AlertCircle, Check, Loader2, RotateCcw, X } from 'lucide-vue-next'
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
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Switch } from '@/components/ui/switch'
import Steps from '@/components/ui/Steps.vue'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { detectEmailProvider, EMAIL_PROVIDERS, type EmailProvider, getProviderSetupNote } from '@/utils/emailProviders'
import { accountsService } from '@/api'
import type { CreateAccountRequest, EmailAccount } from '@/api/types'
import { getErrorMessage } from '@/utils/error'

// 接收 size 属性
interface Props {
  size?: 'default' | 'sm' | 'lg' | 'icon'
}

const props = withDefaults(defineProps<Props>(), {
  size: 'default'
})

const { t } = useI18n()
const open = ref(false)
const currentStep = ref(1)
const isTestingConnection = ref(false)
const error = ref<string | null>(null)
const testResult = ref<any>(null)

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

// 监听SSL状态变化，自动调整端口
watch(() => formData.value.imap_ssl, (useSSL) => {
  if (useSSL) {
    if (formData.value.imap_port === 143) {
      formData.value.imap_port = 993
    }
  } else {
    if (formData.value.imap_port === 993) {
      formData.value.imap_port = 143
    }
  }
})

watch(() => formData.value.smtp_ssl, (useSSL) => {
  if (useSSL) {
    if (formData.value.smtp_port === 25) {
      formData.value.smtp_port = 587
    }
  } else {
    if (formData.value.smtp_port === 587 || formData.value.smtp_port === 465) {
      formData.value.smtp_port = 25
    }
  }
})

// 检测到的邮箱提供商
const detectedProvider = ref<EmailProvider | null>(null)

// 邮箱后缀提示
const emailSuggestions = ref<string[]>([])
const showSuggestions = ref(false)
const selectedSuggestionIndex = ref(-1)

// 处理邮箱输入框失焦，延迟隐藏建议
const handleEmailBlur = () => {
  setTimeout(() => {
    showSuggestions.value = false
    selectedSuggestionIndex.value = -1
  }, 200)
}

// 处理键盘导航
const handleKeydown = (event: KeyboardEvent) => {
  if (!showSuggestions.value || emailSuggestions.value.length === 0) return

  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      selectedSuggestionIndex.value = Math.min(
        selectedSuggestionIndex.value + 1,
        emailSuggestions.value.length - 1
      )
      break
    case 'ArrowUp':
      event.preventDefault()
      selectedSuggestionIndex.value = Math.max(selectedSuggestionIndex.value - 1, -1)
      break
    case 'Enter':
      event.preventDefault()
      if (selectedSuggestionIndex.value >= 0) {
        selectEmailSuggestion(emailSuggestions.value[selectedSuggestionIndex.value])
      }
      break
    case 'Escape':
      showSuggestions.value = false
      selectedSuggestionIndex.value = -1
      break
  }
}

// 步骤配置
const steps = computed(() => [
  { title: t('account.add.steps.basic.title'), description: t('account.add.steps.basic.desc') },
  { title: t('account.add.steps.server.title'), description: t('account.add.steps.server.desc') },
  { title: t('account.add.steps.test.title'), description: t('account.add.steps.test.desc') }
])

// 监听邮箱地址变化，自动检测提供商
watch(() => formData.value.email, (email) => {
  if (email) {
    const provider = detectEmailProvider(email)
    detectedProvider.value = provider

    if (provider) {
      // 自动填充服务器配置
      formData.value.imap_server = provider.imap.server
      formData.value.imap_port = provider.imap.port
      formData.value.imap_ssl = provider.imap.ssl
      formData.value.smtp_server = provider.smtp.server
      formData.value.smtp_port = provider.smtp.port
      formData.value.smtp_ssl = provider.smtp.ssl

      // 自动设置用户名
      if (!formData.value.username) {
        formData.value.username = email
      }

      // 自动设置显示名称
      if (!formData.value.name) {
        const localPart = email.split('@')[0]
        formData.value.name = `${localPart} (${provider.name})`
      }
    }
  }
})

// 验证当前步骤
const canProceedToNext = computed(() => {
  switch (currentStep.value) {
    case 1:
      return formData.value.email && formData.value.name && formData.value.username && formData.value.password
    case 2:
      return formData.value.imap_server && formData.value.smtp_server
    default:
      return true
  }
})

function nextStep() {
  if (canProceedToNext.value && currentStep.value < 3) {
    currentStep.value++
  }
}

function prevStep() {
  if (currentStep.value > 1) {
    currentStep.value--
  }
}

async function testConnection() {
  isTestingConnection.value = true
  error.value = null
  testResult.value = null

  try {
    // 使用临时测试接口
    const result = await accountsService.testTempAccount(formData.value)
    testResult.value = {
      imap: result.imap,
      smtp: result.smtp,
      supports_idle: result.supports_idle
    }

    if (!testResult.value.imap || !testResult.value.smtp) {
      error.value = t('account.add.testFailedTitle')
    }
  } catch (err) {
    error.value = getErrorMessage(err) || t('account.add.testFailed')
  } finally {
    isTestingConnection.value = false
  }
}

// 添加账户（不需要测试）
const isAddingAccount = ref(false)

async function addAccount() {
  isAddingAccount.value = true
  error.value = null

  try {
    const account = await accountsService.createAccount(formData.value)
    emit('accountAdded', account)
    resetForm()
    open.value = false
  } catch (err) {
    error.value = getErrorMessage(err) || t('msg.account.create_failed')
  } finally {
    isAddingAccount.value = false
  }
}

function resetForm() {
  currentStep.value = 1
  error.value = null
  testResult.value = null
  detectedProvider.value = null
  showSuggestions.value = false
  emailSuggestions.value = []
  isAddingAccount.value = false
  formData.value = {
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
  }
}

// 生成邮箱后缀提示
function generateEmailSuggestions(partialEmail: string) {
  const atIndex = partialEmail.indexOf('@')
  if (atIndex === -1) {
    showSuggestions.value = false
    selectedSuggestionIndex.value = -1
    return
  }

  const username = partialEmail.substring(0, atIndex)
  const partialDomain = partialEmail.substring(atIndex + 1).toLowerCase()

  if (username.length === 0) {
    showSuggestions.value = false
    selectedSuggestionIndex.value = -1
    return
  }

  // 获取所有可能的域名
  const allDomains = EMAIL_PROVIDERS.reduce((acc, provider) => {
    acc.push(...provider.domains)
    return acc
  }, [] as string[])

  // 过滤匹配的域名
  const matchedDomains = allDomains.filter(domain =>
    domain.toLowerCase().startsWith(partialDomain)
  )

  if (matchedDomains.length > 0 && partialDomain.length > 0) {
    emailSuggestions.value = matchedDomains.slice(0, 5).map(domain => `${username}@${domain}`)
    showSuggestions.value = true
    selectedSuggestionIndex.value = -1
  } else {
    showSuggestions.value = false
    selectedSuggestionIndex.value = -1
  }
}

// 选择邮箱提示
function selectEmailSuggestion(suggestion: string) {
  formData.value.email = suggestion
  showSuggestions.value = false
  selectedSuggestionIndex.value = -1
}

// 重置服务器设置
function resetServerSettings() {
  if (detectedProvider.value) {
    formData.value.imap_server = detectedProvider.value.imap.server
    formData.value.imap_port = detectedProvider.value.imap.port
    formData.value.imap_ssl = detectedProvider.value.imap.ssl
    formData.value.smtp_server = detectedProvider.value.smtp.server
    formData.value.smtp_port = detectedProvider.value.smtp.port
    formData.value.smtp_ssl = detectedProvider.value.smtp.ssl
  }
}

const emit = defineEmits<{
  accountAdded: [account: EmailAccount]
}>()
</script>

<template>
  <Dialog v-model:open="open" @update:open="resetForm">
    <DialogTrigger as-child>
      <Button class="gap-2" :size="props.size">
        <Plus class="h-4 w-4" />
        {{ t('account.add.title') }}
      </Button>
    </DialogTrigger>
    <DialogContent class="sm:max-w-3xl flex flex-col h-[80vh] max-h-[80vh]">
      <!-- 固定的标题栏 -->
      <DialogHeader class="flex-shrink-0">
        <DialogTitle>{{ t('account.add.title') }}</DialogTitle>
        <DialogDescription>
          {{ t('account.add.subtitle') }}
        </DialogDescription>
      </DialogHeader>

      <!-- 步骤指示器 -->
      <div class="my-4 flex-shrink-0">
        <Steps :current="currentStep" :steps="steps" />
      </div>

      <!-- 可滚动的内容区域 -->
      <div class="flex-1 overflow-y-auto px-1 -mx-1">
        <!-- 错误提示 -->
        <Alert v-if="error" variant="destructive" class="mb-4">
          <AlertCircle class="h-4 w-4" />
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>

        <!-- 步骤1: 基本信息 -->
        <div v-if="currentStep === 1" class="space-y-6">
          <div class="space-y-2">
            <Label for="email">{{ t('account.add.fields.email') }}</Label>
            <div class="relative">
              <Input id="email" v-model="formData.email" type="email" placeholder="your.email@example.com"
                :class="{ 'border-green-500': detectedProvider }" @input="generateEmailSuggestions(formData.email)"
                @focus="generateEmailSuggestions(formData.email)" @blur="handleEmailBlur" @keydown="handleKeydown" />

              <!-- 邮箱后缀提示 -->
              <div v-if="showSuggestions && emailSuggestions.length > 0"
                class="absolute z-50 w-full bg-white border border-gray-200 rounded-md shadow-lg max-h-40 overflow-y-auto top-full mt-1">
                <div v-for="(suggestion, index) in emailSuggestions" :key="suggestion"
                  class="px-3 py-2 hover:bg-gray-100 cursor-pointer text-sm"
                  :class="{ 'bg-blue-100': index === selectedSuggestionIndex }"
                  @click="selectEmailSuggestion(suggestion)" @mouseenter="selectedSuggestionIndex = index">
                  {{ suggestion }}
                </div>
              </div>
            </div>

            <div v-if="detectedProvider" class="flex items-center gap-2 text-sm text-green-600">
              <Check class="h-3 w-3" />
              {{ t('account.add.fields.emailRecognized', { provider: detectedProvider.name }) }}
            </div>

            <!-- 简化的帮助提示 -->
            <div v-if="detectedProvider?.authType === 'app_password'"
              class="text-sm text-amber-600 bg-amber-50 p-3 rounded-lg">
              {{ detectedProvider.name }} {{ getProviderSetupNote(detectedProvider) }}
              <a v-if="detectedProvider.helpUrl" :href="detectedProvider.helpUrl" target="_blank"
                class="text-amber-700 hover:text-amber-800 underline ml-1">
                查看文档 →
              </a>
            </div>
          </div>

          <div class="space-y-2">
            <Label for="name">{{ t('account.add.fields.displayName') }}</Label>
            <Input id="name" v-model="formData.name" :placeholder="t('account.add.fields.displayNamePlaceholder')" />
          </div>

          <!-- 认证信息 -->
          <div class="space-y-2">
            <Label for="username">{{ t('account.add.fields.username') }}</Label>
            <Input id="username" v-model="formData.username" :placeholder="t('account.add.fields.usernameHint')" />
          </div>

          <div class="space-y-2">
            <Label for="password">{{ t('account.add.fields.password') }} *</Label>
            <Input id="password" v-model="formData.password" type="password"
              :placeholder="detectedProvider?.authType === 'app_password' ? t('account.add.fields.appPasswordPlaceholder') : t('account.add.fields.passwordPlaceholder')"
              required />
          </div>

        </div>

        <!-- 步骤2: 服务器设置 -->
        <div v-if="currentStep === 2" class="space-y-6">
          <!-- 服务器设置 -->
          <Card>
            <CardHeader>
              <div class="flex items-center justify-between">
                <CardTitle class="text-base">{{ t('account.add.fields.serverSettings') }}</CardTitle>
                <Button v-if="detectedProvider" variant="outline" size="sm" @click="resetServerSettings" class="gap-2">
                  <RotateCcw class="h-3 w-3" />
                  {{ t('common.reset') }}
                </Button>
              </div>
            </CardHeader>
            <CardContent class="space-y-4">
              <!-- IMAP 设置 -->
              <div class="space-y-2">
                <Label class="text-sm font-medium">{{ t('account.add.fields.imapSettings') }}</Label>
                <div class="grid grid-cols-12 gap-2 items-end">
                  <div class="col-span-6 space-y-1">
                    <Label class="text-xs text-muted-foreground">{{ t('account.edit.serverAddress') }}</Label>
                    <Input v-model="formData.imap_server" placeholder="imap.example.com" />
                  </div>
                  <div class="col-span-3 space-y-1">
                    <Label class="text-xs text-muted-foreground">{{ t('account.edit.port') }}</Label>
                    <Input v-model.number="formData.imap_port" type="number" placeholder="993" />
                  </div>
                  <div class="col-span-3 flex items-center gap-2 pb-2">
                    <Switch id="imap_ssl" v-model="formData.imap_ssl" />
                    <Label for="imap_ssl" class="text-sm">{{ t('account.edit.useSSL') }}</Label>
                  </div>
                </div>
              </div>

              <!-- SMTP 设置 -->
              <div class="space-y-2">
                <Label class="text-sm font-medium">{{ t('account.add.fields.smtpSettings') }}</Label>
                <div class="grid grid-cols-12 gap-2 items-end">
                  <div class="col-span-6 space-y-1">
                    <Label class="text-xs text-muted-foreground">{{ t('account.edit.serverAddress') }}</Label>
                    <Input v-model="formData.smtp_server" placeholder="smtp.example.com" />
                  </div>
                  <div class="col-span-3 space-y-1">
                    <Label class="text-xs text-muted-foreground">{{ t('account.edit.port') }}</Label>
                    <Input v-model.number="formData.smtp_port" type="number" placeholder="587" />
                  </div>
                  <div class="col-span-3 flex items-center gap-2 pb-2">
                    <Switch id="smtp_ssl" v-model="formData.smtp_ssl" />
                    <Label for="smtp_ssl" class="text-sm">{{ t('account.edit.useSSL') }}</Label>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>

        <!-- 步骤3: 配置确认 -->
        <div v-if="currentStep === 3" class="space-y-4">
          <div class="text-center">
            <div class="text-lg font-medium mb-2">{{ t('account.add.fields.configConfirm') }}</div>
            <div class="text-sm text-muted-foreground">
              {{ t('account.add.fields.confirmMessage') }}
            </div>
          </div>

          <!-- 配置摘要 -->
          <Card>
            <CardHeader>
              <CardTitle class="text-base">{{ t('account.add.fields.configSummary') }}</CardTitle>
            </CardHeader>
            <CardContent class="space-y-2 text-sm">
              <div><strong>{{ t('account.add.fields.email') }}:</strong> {{ formData.email }}</div>
              <div><strong>{{ t('account.add.fields.displayName') }}:</strong> {{ formData.name }}</div>
              <div><strong>IMAP:</strong> {{ formData.imap_server }}:{{ formData.imap_port }} ({{ formData.imap_ssl ?
                'SSL/TSL' : t('account.add.fields.noSsl') }})</div>
              <div><strong>SMTP:</strong> {{ formData.smtp_server }}:{{ formData.smtp_port }} ({{ formData.smtp_ssl ?
                'SSL/TSL' : t('account.add.fields.noSsl') }})</div>
            </CardContent>
          </Card>

          <!-- 测试结果 -->
          <Card v-if="testResult">
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
                  <span>{{ testResult.supports_idle ? t('account.edit.supportsIdle') : t('account.edit.requiresPolling')
                    }}</span>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>

      <!-- 导航按钮 -->
      <div class="flex justify-between pt-4 border-t flex-shrink-0">
        <Button variant="outline" @click="prevStep" :disabled="currentStep === 1">
          {{ t('common.back') }}
        </Button>

        <div class="flex gap-2">
          <!-- 第三步的特殊按钮 -->
          <template v-if="currentStep === 3">
            <Button variant="outline" @click="testConnection" :disabled="isTestingConnection || isAddingAccount">
              <Loader2 v-if="isTestingConnection" class="mr-2 h-4 w-4 animate-spin" />
              {{ isTestingConnection ? t('common.testing') : t('account.edit.testConnection') }}
            </Button>
            <Button @click="addAccount" :disabled="isTestingConnection || isAddingAccount">
              <Loader2 v-if="isAddingAccount" class="mr-2 h-4 w-4 animate-spin" />
              <Plus class="mr-2 h-4 w-4" />
              {{ isAddingAccount ? t('account.add.adding') : t('account.add.addAccount') }}
            </Button>
          </template>

          <!-- 其他步骤的下一步按钮 -->
          <Button v-else @click="nextStep" :disabled="!canProceedToNext || currentStep === 3">
            {{ t('common.next') }}
          </Button>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
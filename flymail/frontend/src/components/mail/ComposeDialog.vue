<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Separator } from '@/components/ui/separator'
import { X, Send, Paperclip, Minimize2, Maximize2 } from 'lucide-vue-next'
import { useMailApiStore } from '@/stores/mailApi'
import { inject } from 'vue'
import type { Email } from '@/types/email'
import { useI18n } from 'vue-i18n'
import { useApiMessage } from '@/hooks/useApiMessage'
import type { UrlDrivenStateManager } from '@/composables/useUrlDrivenState'

interface Props {
  open: boolean
  mode?: 'new' | 'reply' | 'replyAll' | 'forward'
  originalEmail?: Email
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:open', value: boolean): void
  (e: 'sent'): void
}>()

const mailStore = useMailApiStore()
const stateManager = inject<UrlDrivenStateManager>('stateManager')!
const { t } = useI18n()
const { showWarning, showSuccess, showError } = useApiMessage()

// Form data
const to = ref('')
const cc = ref('')
const bcc = ref('')
const subject = ref('')
const content = ref('')
const attachments = ref<File[]>([])

// UI state
const showCc = ref(false)
const showBcc = ref(false)
const isMinimized = ref(false)
const isSending = ref(false)

// Computed dialog open state
const isOpen = computed({
  get: () => props.open,
  set: (value) => {
    emit('update:open', value)
    // 通过状态管理器更新状态
    if (!value) {
      stateManager.closeCompose()
    }
  }
})

// Watch for mode and original email changes
watch([() => props.mode, () => props.originalEmail], ([newMode, email]) => {
  if (!email) return

  switch (newMode) {
    case 'reply':
      to.value = email.from.email
      subject.value = `Re: ${email.subject.replace(/^Re:\s*/i, '')}`
      content.value = `\n\n---\n${email.from.name} <${email.from.email}> ${t('mail.compose.wrote')}\n\n${email.textBody || email.body}`
      break

    case 'replyAll':
      to.value = email.from.email
      cc.value = email.cc?.map(addr => addr.email).join(', ') || ''
      subject.value = `Re: ${email.subject.replace(/^Re:\s*/i, '')}`
      content.value = `\n\n---\n${email.from.name} <${email.from.email}> ${t('mail.compose.wrote')}\n\n${email.textBody || email.body}`
      showCc.value = true
      break

    case 'forward':
      subject.value = `Fwd: ${email.subject.replace(/^Fwd:\s*/i, '')}`
      content.value = `\n\n--- ${t('mail.compose.forwardedMessage')} ---\n${t('mail.compose.from')}: ${email.from.name} <${email.from.email}>\n${t('mail.compose.date')}: ${new Date(email.createdAt).toLocaleString()}\n${t('mail.compose.subject')}: ${email.subject}\n${t('mail.compose.to')}: ${email.to.map(addr => `${addr.name} <${addr.email}>`).join(', ')}\n\n${email.textBody || email.body}`
      break
  }
}, { immediate: true })

// Reset form when dialog opens with new mode
watch(() => props.open, (newValue) => {
  if (newValue && props.mode === 'new') {
    resetForm()
  }
})

const resetForm = () => {
  to.value = ''
  cc.value = ''
  bcc.value = ''
  subject.value = ''
  content.value = ''
  attachments.value = []
  showCc.value = false
  showBcc.value = false
}

const handleSend = async () => {
  if (!to.value.trim() || !subject.value.trim()) {
    showWarning(t('mail.compose.validation'))
    return
  }

  isSending.value = true
  try {
    // 构建邮件数据
    const emailData = {
      to: to.value.split(',').map(email => ({ email: email.trim() })),
      cc: cc.value ? cc.value.split(',').map(email => ({ email: email.trim() })) : undefined,
      bcc: bcc.value ? bcc.value.split(',').map(email => ({ email: email.trim() })) : undefined,
      subject: subject.value,
      body: content.value,
      textBody: content.value,
      accountId: mailStore.selectedAccountId || 0,
      folderId: mailStore.selectedFolderId || 0,
      replyToId: props.mode === 'reply' || props.mode === 'replyAll' ? props.originalEmail?.id : undefined
    }

    // 发送邮件 - TODO: 实现实际的发送功能
    console.log('Sending email:', emailData)
    // await mailStore.sendEmail(emailData)

    // 模拟发送成功
    setTimeout(() => {
      emit('sent')
      isOpen.value = false
      resetForm()
      showSuccess(t('mail.compose.sendSuccess'))
    }, 1000)
  } catch (error) {
    console.error('Failed to send email:', error)
    showError(t('mail.compose.sendFailed'))
  } finally {
    isSending.value = false
  }
}

const handleAttachment = () => {
  // 文件上传逻辑
  const input = document.createElement('input')
  input.type = 'file'
  input.multiple = true
  input.onchange = (e) => {
    const files = (e.target as HTMLInputElement).files
    if (files) {
      attachments.value.push(...Array.from(files))
    }
  }
  input.click()
}

const removeAttachment = (index: number) => {
  attachments.value.splice(index, 1)
}
</script>

<template>
  <Dialog v-model:open="isOpen">
    <DialogContent
      :class="[
        'max-w-3xl h-[80vh] flex flex-col',
        isMinimized && 'h-auto'
      ]"
    >
      <DialogHeader class="flex-shrink-0">
        <div class="flex items-center justify-between">
          <DialogTitle>
            {{ mode === 'new' ? t('mail.compose.newEmail') :
               mode === 'reply' ? t('mail.compose.reply') :
               mode === 'replyAll' ? t('mail.compose.replyAll') :
               mode === 'forward' ? t('mail.compose.forward') : t('mail.compose.title') }}
          </DialogTitle>
          <div class="flex items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              @click="isMinimized = !isMinimized"
            >
              <Minimize2 v-if="!isMinimized" class="h-4 w-4" />
              <Maximize2 v-else class="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              @click="isOpen = false"
            >
              <X class="h-4 w-4" />
            </Button>
          </div>
        </div>
      </DialogHeader>

      <div v-if="!isMinimized" class="flex-1 flex flex-col overflow-hidden">
        <div class="space-y-3 flex-shrink-0 px-6">
          <!-- 收件人 -->
          <div class="flex items-center gap-2">
            <Label class="w-16 text-right">{{ t('mail.compose.to') }}</Label>
            <Input
              v-model="to"
              :placeholder="t('mail.compose.toPlaceholder')"
              class="flex-1"
            />
          </div>

          <!-- 抄送 -->
          <div v-if="showCc" class="flex items-center gap-2">
            <Label class="w-16 text-right">{{ t('mail.compose.cc') }}</Label>
            <Input
              v-model="cc"
              :placeholder="t('mail.compose.toPlaceholder')"
              class="flex-1"
            />
          </div>

          <!-- 密送 -->
          <div v-if="showBcc" class="flex items-center gap-2">
            <Label class="w-16 text-right">{{ t('mail.compose.bcc') }}</Label>
            <Input
              v-model="bcc"
              :placeholder="t('mail.compose.toPlaceholder')"
              class="flex-1"
            />
          </div>

          <!-- 添加抄送/密送按钮 -->
          <div v-if="!showCc || !showBcc" class="flex items-center gap-2 ml-[72px]">
            <Button
              v-if="!showCc"
              variant="link"
              size="sm"
              @click="showCc = true"
            >
              {{ t('mail.compose.addCc') }}
            </Button>
            <Button
              v-if="!showBcc"
              variant="link"
              size="sm"
              @click="showBcc = true"
            >
              {{ t('mail.compose.addBcc') }}
            </Button>
          </div>

          <!-- 主题 -->
          <div class="flex items-center gap-2">
            <Label class="w-16 text-right">{{ t('mail.compose.subject') }}</Label>
            <Input
              v-model="subject"
              :placeholder="t('mail.compose.subjectPlaceholder')"
              class="flex-1"
            />
          </div>
        </div>

        <Separator class="my-3" />

        <!-- 邮件内容 -->
        <div class="flex-1 px-6 overflow-hidden">
          <Textarea
            v-model="content"
            :placeholder="t('mail.compose.contentPlaceholder')"
            class="w-full h-full resize-none"
          />
        </div>

        <!-- 附件 -->
        <div v-if="attachments.length > 0" class="px-6 py-2 border-t">
          <div class="flex items-center gap-2 flex-wrap">
            <span class="text-sm text-muted-foreground">{{ t('mail.compose.attachments') }}</span>
            <div
              v-for="(file, index) in attachments"
              :key="index"
              class="flex items-center gap-1 px-2 py-1 bg-secondary rounded text-sm"
            >
              <Paperclip class="h-3 w-3" />
              <span>{{ file.name }}</span>
              <Button
                variant="ghost"
                size="icon"
                class="h-4 w-4 p-0"
                @click="removeAttachment(index)"
              >
                <X class="h-3 w-3" />
              </Button>
            </div>
          </div>
        </div>

        <!-- 操作按钮 -->
        <div class="flex items-center justify-between px-6 py-3 border-t">
          <Button
            variant="ghost"
            size="sm"
            @click="handleAttachment"
          >
            <Paperclip class="h-4 w-4 mr-2" />
            {{ t('mail.compose.addAttachment') }}
          </Button>

          <div class="flex items-center gap-2">
            <Button
              variant="outline"
              @click="isOpen = false"
            >
              {{ t('common.cancel') }}
            </Button>
            <Button
              :disabled="isSending || !to.trim() || !subject.trim()"
              @click="handleSend"
            >
              <Send class="h-4 w-4 mr-2" />
              {{ isSending ? t('mail.compose.sending') : t('mail.compose.send') }}
            </Button>
          </div>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
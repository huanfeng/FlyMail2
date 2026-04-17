<script setup lang="ts">
import { ref, onMounted, computed, watch, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ArrowLeft, Download, Reply, ReplyAll, Forward, Archive, Trash2, Star, ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { useMailApiStore } from '@/stores/mailApi'
import { parseMailViewUrl } from '@/utils/urlHelper'
import { formatDate } from '@/utils/date'

const route = useRoute()
const router = useRouter()
const mailStore = useMailApiStore()
const { t } = useI18n()

const isLoading = ref(false)
const error = ref<string | null>(null)
const showShortcuts = ref(false)

// 从URL解析邮件ID
const mailId = computed(() => {
  return parseMailViewUrl(route.fullPath.split('?')[1] || '')
})

// 解析发件人信息
const parseFromAddress = (from: string) => {
  const match = from.match(/^(.+?)\s*<(.+?)>$/)
  if (match) {
    return {
      name: match[1].trim(),
      email: match[2].trim()
    }
  }
  return {
    name: from,
    email: from
  }
}

// 当前邮件
const currentEmail = computed(() => mailStore.currentEmail)

// 加载邮件详情
const loadEmail = async (id: number) => {
  if (!id) return

  isLoading.value = true
  error.value = null

  try {
    await mailStore.loadEmailDetail(id)
  } catch (err) {
    error.value = t('error.loadMailFailed')
    console.error('Failed to load email:', err)
  } finally {
    isLoading.value = false
  }
}

// 返回主页面
const goBack = () => {
  router.push('/main')
}

// 邮件操作
const toggleStar = async () => {
  if (currentEmail.value) {
    await mailStore.toggleStar(currentEmail.value.email_id)
  }
}

const archiveEmail = async () => {
  if (currentEmail.value) {
    // TODO: 实现归档功能
    console.log('Archive email:', currentEmail.value.email_id)
  }
}

const deleteEmail = async () => {
  if (currentEmail.value) {
    await mailStore.deleteEmail(currentEmail.value.email_id)
    goBack()
  }
}

const replyEmail = (mode: 'reply' | 'replyAll' | 'forward') => {
  if (currentEmail.value) {
    // 跳转到主页面并打开撰写窗口
    const params = new URLSearchParams()
    params.set('view', 'compose')
    params.set('compose', mode)
    params.set('composeId', String(currentEmail.value.email_id))
    router.push(`/main?${params.toString()}`)
  }
}

const downloadAttachment = (attachmentId: string) => {
  // TODO: 实现附件下载
  console.log('Download attachment:', attachmentId)
}

// 邮件导航
const navigatePrevious = async () => {
  const success = await mailStore.navigateToPreviousEmail()
  if (success && mailStore.currentEmail) {
    // 更新URL
    const params = new URLSearchParams()
    params.set('view', 'mail')
    params.set('mailId', String(mailStore.currentEmail.email_id))
    router.push(`/mail?${params.toString()}`)
  }
}

const navigateNext = async () => {
  const success = await mailStore.navigateToNextEmail()
  if (success && mailStore.currentEmail) {
    // 更新URL
    const params = new URLSearchParams()
    params.set('view', 'mail')
    params.set('mailId', String(mailStore.currentEmail.email_id))
    router.push(`/mail?${params.toString()}`)
  }
}

// 监听URL变化
watch(mailId, (newId) => {
  if (newId) {
    loadEmail(newId)
  }
}, { immediate: true })

// 键盘快捷键处理
const handleKeyboard = (e: KeyboardEvent) => {
  // 如果正在输入，忽略快捷键
  if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) {
    return
  }

  switch (e.key) {
    case 'Escape':
      goBack()
      break
    case 'r':
      if (!e.ctrlKey && !e.metaKey) {
        replyEmail('reply')
      }
      break
    case 'a':
      if (!e.ctrlKey && !e.metaKey) {
        replyEmail('replyAll')
      }
      break
    case 'f':
      if (!e.ctrlKey && !e.metaKey) {
        replyEmail('forward')
      }
      break
    case 's':
      if (!e.ctrlKey && !e.metaKey) {
        toggleStar()
      }
      break
    case 'Delete':
    case 'd':
      if (!e.ctrlKey && !e.metaKey) {
        deleteEmail()
      }
      break
    case '?':
      showShortcuts.value = !showShortcuts.value
      break
    case 'ArrowLeft':
    case 'p':
      if (!e.ctrlKey && !e.metaKey) {
        navigatePrevious()
      }
      break
    case 'ArrowRight':
    case 'n':
      if (!e.ctrlKey && !e.metaKey) {
        navigateNext()
      }
      break
  }
}

// 初始化
onMounted(async () => {
  await mailStore.initialize()
  if (mailId.value) {
    loadEmail(mailId.value)
  }
  
  // 添加键盘事件监听
  window.addEventListener('keydown', handleKeyboard)
})

// 清理
onUnmounted(() => {
  window.removeEventListener('keydown', handleKeyboard)
})
</script>

<template>
  <div class="flex flex-col h-screen bg-background">
    <!-- 顶部操作栏 -->
    <div class="flex items-center justify-between p-4 border-b">
      <div class="flex items-center gap-2">
        <Button variant="ghost" size="sm" @click="goBack">
          <ArrowLeft class="h-4 w-4 mr-2" />
          {{ t('common.back') }}
        </Button>
        <Separator orientation="vertical" class="h-6" />
        <h1 class="text-lg font-semibold">{{ t('mail.detail.title') }}</h1>
      </div>

      <div class="flex items-center gap-2" v-if="currentEmail">
        <Button variant="ghost" size="sm" @click="navigatePrevious">
          <ChevronLeft class="h-4 w-4" />
        </Button>
        <Button variant="ghost" size="sm" @click="navigateNext">
          <ChevronRight class="h-4 w-4" />
        </Button>
        <Separator orientation="vertical" class="h-6" />
        <Button variant="ghost" size="sm" @click="toggleStar">
          <Star
            class="h-4 w-4"
            :class="currentEmail.is_starred ? 'fill-yellow-400 text-yellow-400' : ''"
          />
        </Button>
        <Button variant="ghost" size="sm" @click="() => replyEmail('reply')">
          <Reply class="h-4 w-4" />
        </Button>
        <Button variant="ghost" size="sm" @click="() => replyEmail('replyAll')">
          <ReplyAll class="h-4 w-4" />
        </Button>
        <Button variant="ghost" size="sm" @click="() => replyEmail('forward')">
          <Forward class="h-4 w-4" />
        </Button>
        <Separator orientation="vertical" class="h-6" />
        <Button variant="ghost" size="sm" @click="archiveEmail">
          <Archive class="h-4 w-4" />
        </Button>
        <Button variant="ghost" size="sm" @click="deleteEmail">
          <Trash2 class="h-4 w-4" />
        </Button>
      </div>
    </div>

    <!-- 邮件内容区 -->
    <ScrollArea class="flex-1">
      <div class="max-w-4xl mx-auto p-6">
        <!-- 错误状态 -->
        <Alert v-if="error" variant="destructive" class="mb-4">
          <AlertDescription>{{ error }}</AlertDescription>
        </Alert>

        <!-- 加载状态 -->
        <div v-if="isLoading" class="space-y-4">
          <Skeleton class="h-8 w-3/4" />
          <Skeleton class="h-4 w-1/2" />
          <Skeleton class="h-4 w-full" />
          <Skeleton class="h-32 w-full" />
        </div>

        <!-- 邮件内容 -->
        <div v-else-if="currentEmail" class="space-y-6">
          <!-- 邮件头部 -->
          <div class="space-y-4">
            <div class="flex items-start justify-between">
              <h1 class="text-2xl font-bold">
                {{ currentEmail.subject || t('mail.detail.noSubject') }}
              </h1>
              <div class="flex items-center gap-2">
                <Badge v-if="currentEmail.is_starred" variant="secondary">
                  <Star class="h-3 w-3 mr-1 fill-yellow-400 text-yellow-400" />
                  {{ t('mail.viewer.star') }}
                </Badge>
                <Badge v-if="!currentEmail.is_read" variant="default">
                  {{ t('mail.unread') }}
                </Badge>
              </div>
            </div>

            <!-- 发件人信息 -->
            <div class="space-y-2">
              <div class="flex items-center justify-between">
                <div>
                  <div class="font-medium">
                    {{ parseFromAddress(currentEmail.from).name }}
                  </div>
                  <div class="text-sm text-muted-foreground">
                    {{ parseFromAddress(currentEmail.from).email }}
                  </div>
                </div>
                <div class="text-sm text-muted-foreground">
                  {{ formatDate(currentEmail.date) }}
                </div>
              </div>

              <!-- 收件人信息 -->
              <div class="text-sm text-muted-foreground">
                <span class="font-medium">{{ t('mail.compose.to') }}</span>
                {{ currentEmail.to || t('common.none') }}
              </div>
            </div>
          </div>

          <Separator />

          <!-- 附件列表 -->
          <div v-if="currentEmail.attachments && currentEmail.attachments.length > 0" class="space-y-2">
            <h3 class="text-sm font-medium">{{ t('mail.viewer.attachments') }}</h3>
            <div class="flex flex-wrap gap-2">
              <Button
                v-for="attachment in currentEmail.attachments"
                :key="attachment.attachment_id"
                variant="outline"
                size="sm"
                @click="downloadAttachment(String(attachment.attachment_id))"
              >
                <Download class="h-4 w-4 mr-2" />
                {{ attachment.filename }} ({{ Math.round(attachment.size / 1024) }}KB)
              </Button>
            </div>
            <Separator />
          </div>

          <!-- 邮件正文 -->
          <div class="prose prose-sm max-w-none dark:prose-invert">
            <div
              v-if="currentEmail.body_html"
              v-html="currentEmail.body_html"
              class="email-content"
            />
            <pre
              v-else-if="currentEmail.body"
              class="whitespace-pre-wrap font-sans text-sm bg-transparent border-none p-0"
            >{{ currentEmail.body }}</pre>
            <div v-else class="text-muted-foreground italic">
              {{ t('mail.viewer.emptyContent') }}
            </div>
          </div>
        </div>

        <!-- 无邮件状态 -->
        <div v-else-if="!isLoading && !error" class="text-center py-12">
          <div class="text-muted-foreground">
            {{ t('mail.detail.notFound') }}
          </div>
          <Button variant="outline" class="mt-4" @click="goBack">
            {{ t('mail.detail.backToMail') }}
          </Button>
        </div>
      </div>
    </ScrollArea>
  </div>

  <!-- 快捷键帮助对话框 -->
  <Dialog v-model:open="showShortcuts">
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{{ t('mail.shortcuts.title') }}</DialogTitle>
      </DialogHeader>
      <div class="grid grid-cols-2 gap-4 text-sm">
        <div class="flex justify-between">
          <span class="text-muted-foreground">{{ t('mail.shortcuts.reply') }}</span>
          <kbd class="px-2 py-1 text-xs border rounded">R</kbd>
        </div>
        <div class="flex justify-between">
          <span class="text-muted-foreground">{{ t('mail.shortcuts.replyAll') }}</span>
          <kbd class="px-2 py-1 text-xs border rounded">A</kbd>
        </div>
        <div class="flex justify-between">
          <span class="text-muted-foreground">{{ t('mail.shortcuts.forward') }}</span>
          <kbd class="px-2 py-1 text-xs border rounded">F</kbd>
        </div>
        <div class="flex justify-between">
          <span class="text-muted-foreground">{{ t('mail.shortcuts.star') }}</span>
          <kbd class="px-2 py-1 text-xs border rounded">S</kbd>
        </div>
        <div class="flex justify-between">
          <span class="text-muted-foreground">{{ t('mail.shortcuts.delete') }}</span>
          <kbd class="px-2 py-1 text-xs border rounded">D</kbd>
        </div>
        <div class="flex justify-between">
          <span class="text-muted-foreground">{{ t('mail.shortcuts.back') }}</span>
          <kbd class="px-2 py-1 text-xs border rounded">Esc</kbd>
        </div>
        <div class="flex justify-between">
          <span class="text-muted-foreground">{{ t('mail.shortcuts.help') }}</span>
          <kbd class="px-2 py-1 text-xs border rounded">?</kbd>
        </div>
        <div class="flex justify-between">
          <span class="text-muted-foreground">{{ t('mail.shortcuts.previous') }}</span>
          <kbd class="px-2 py-1 text-xs border rounded">P / ←</kbd>
        </div>
        <div class="flex justify-between">
          <span class="text-muted-foreground">{{ t('mail.shortcuts.next') }}</span>
          <kbd class="px-2 py-1 text-xs border rounded">N / →</kbd>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>

<style scoped>
/* 邮件内容样式 */
.email-content {
  color: var(--color-text-primary);
}

.email-content :deep(a) {
  color: var(--color-primary);
  text-decoration: underline;
}

.email-content :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 6px;
}

.email-content :deep(blockquote) {
  border-left: 4px solid var(--color-border);
  padding-left: 1rem;
  font-style: italic;
  color: var(--color-text-muted);
}

.email-content :deep(table) {
  border: 1px solid var(--color-border);
  border-radius: 6px;
}

.email-content :deep(td),
.email-content :deep(th) {
  border: 1px solid var(--color-border);
  padding: 0.5rem 0.75rem;
}

.email-content :deep(th) {
  background-color: var(--color-bg-muted);
  font-weight: 500;
}
</style>
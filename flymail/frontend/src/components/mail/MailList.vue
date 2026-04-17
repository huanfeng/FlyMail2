<script lang="ts" setup>
import { computed, ref, onMounted, onUnmounted, watch, nextTick, inject } from 'vue'
import { formatDistanceToNow } from 'date-fns'
import { zhCN, enUS } from 'date-fns/locale'
import { Search, ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { useMailApiStore } from '@/stores/mailApi'
import { Skeleton } from '@/components/ui/skeleton'
import { debounce } from 'lodash-es'
import type { UrlDrivenStateManager } from '@/composables/useUrlDrivenState'

const mailStore = useMailApiStore()
const { t, locale } = useI18n()

// 注入状态管理器来处理邮件选择
const stateManager = inject<UrlDrivenStateManager>('stateManager')!

const searchQuery = ref('')
const scrollAreaRef = ref<InstanceType<typeof ScrollArea> | null>(null)
const scrollViewportRef = ref<HTMLElement | null>(null)
const isLoadingMore = ref(false)

// Debounced search
const debouncedSearch = debounce(async (query: string) => {
  await mailStore.searchEmails(query)
}, 500)

// Extract sender name from email address
function getSenderName(from: string): string {
  // Parse format: "Name <email@example.com>" or just "email@example.com"
  const match = from.match(/^([^<]+)\s*<([^>]+)>$/)
  if (match) {
    return match[1].trim()
  }
  // If no name, use the part before @ as name
  return from.split('@')[0]
}

// Get email preview text (first 100 characters of body)
function getEmailPreview(email: any): string {
  // 如果有 preview 字段，使用它
  if (email.preview) {
    return email.preview
  }
  // 如果有 body 字段（通常只在详情中存在），使用它的前100个字符
  if (email.body) {
    return email.body.substring(0, 100).replace(/\n/g, ' ').trim()
  }
  // 邮件列表中没有预览内容时，返回空字符串，避免与标题重复
  return ''
}

const filteredMails = computed(() => {
  return mailStore.emails
})

// Pagination controls
function goToPreviousPage() {
  if (mailStore.currentPage > 1) {
    mailStore.goToPage(mailStore.currentPage - 1)
  }
}

function goToNextPage() {
  if (mailStore.currentPage < mailStore.totalPages) {
    mailStore.goToPage(mailStore.currentPage + 1)
  }
}

function goToPage(page: number) {
  if (page >= 1 && page <= mailStore.totalPages) {
    mailStore.goToPage(page)
  }
}

// Page selector options with format
const pageOptions = computed(() => {
  const options = []
  const total = mailStore.totalPages
  for (let i = 1; i <= total; i++) {
    options.push({
      value: i,
      label: `${i}/${total}`
    })
  }
  return options
})

// Handle email selection
function selectMail(emailId: number) {
  stateManager.selectMail(emailId)
}

// Watch search query
watch(searchQuery, (newQuery) => {
  debouncedSearch(newQuery)
})

// Handle infinite scroll
async function handleScroll() {
  try {
    if (!scrollViewportRef.value) return

    const scrollElement = scrollViewportRef.value
    const scrollBottom = scrollElement.scrollHeight - scrollElement.scrollTop - scrollElement.clientHeight

    if (scrollBottom < 100 && mailStore.hasMore && !isLoadingMore.value && !mailStore.isLoadingEmails) {
      isLoadingMore.value = true
      try {
        await mailStore.loadMore()
      } finally {
        isLoadingMore.value = false
      }
    }
  } catch (error) {
    console.error('Error in handleScroll:', error)
  }
}

// Setup scroll event listener
function setupScrollListener() {
  nextTick(() => {
    if (scrollAreaRef.value) {
      // Find the viewport element inside ScrollArea
      const viewport = scrollAreaRef.value.$el?.querySelector('[data-reka-scroll-area-viewport]')
      if (viewport) {
        scrollViewportRef.value = viewport as HTMLElement
        viewport.addEventListener('scroll', handleScroll)
      }
    }
  })
}

// Cleanup scroll event listener
function cleanupScrollListener() {
  if (scrollViewportRef.value) {
    scrollViewportRef.value.removeEventListener('scroll', handleScroll)
  }
}

onMounted(() => {
  setupScrollListener()
})

onUnmounted(() => {
  cleanupScrollListener()
})


// Watch for folder changes is not needed here
// because selectFolder in mailApi store already calls fetchEmails

// Get folder name for display
const currentFolderName = computed(() => {
  if (mailStore.selectedFolder) {
    return mailStore.getLocalizedName(mailStore.selectedFolder)
  }
  return t('mail.list.inbox')
})

// Get unread count
const unreadCount = computed(() => mailStore.unreadCount)

// Total email count for display
const totalEmailCount = computed(() => mailStore.totalEmails)

// Get date-fns locale based on current locale
const dateFnsLocale = computed(() => locale.value === 'zh' ? zhCN : enUS)
</script>

<template>
  <!-- Email list container - always shown -->
  <div class="flex flex-col h-full">
    <div class="flex items-center h-[52px] px-4 gap-4">
      <h1 class="text-base font-semibold">{{ currentFolderName }}<span v-if="unreadCount > 0" class="ml-2 text-sm text-muted-foreground">({{ unreadCount }})</span></h1>
      <div class="flex-1 max-w-md relative ml-auto">
        <Search class="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
        <Input
          v-model="searchQuery"
          :placeholder="t('mail.list.searchPlaceholder')"
          class="pl-8 h-9"
        />
      </div>
    </div>

    <Separator />

    <div class="flex-1 overflow-hidden">
      <ScrollArea
        ref="scrollAreaRef"
        class="h-full flex"
      >
        <div class="flex flex-col p-4 pt-2">

          <!-- Loading state for initial load -->
          <div v-if="mailStore.isLoadingEmails && filteredMails.length === 0" class="space-y-2">
            <Skeleton class="h-20 w-full" />
            <Skeleton class="h-20 w-full" />
            <Skeleton class="h-20 w-full" />
          </div>

          <!-- Empty state -->
          <div v-else-if="!mailStore.isLoadingEmails && filteredMails.length === 0" class="text-center py-8 text-muted-foreground">
            {{ searchQuery ? t('mail.list.noResults') : t('mail.list.noEmails') }}
          </div>
          <button
            v-for="mail in filteredMails"
            :key="mail.email_id"
            :class="cn(
              'flex flex-col items-start gap-2 rounded-lg border p-3 text-left text-sm transition-all mb-2',
              mailStore.currentEmail?.email_id === mail.email_id ? 'bg-muted' : 'hover:bg-accent',
            )"
            @click="selectMail(mail.email_id)"
          >
            <div class="flex w-full flex-col gap-1">
              <div class="flex items-center">
                <div class="flex items-center gap-2">
                  <div class="font-semibold">{{ getSenderName(mail.from) }}</div>
                  <span v-if="!mail.is_read" class="flex h-2 w-2 rounded-full bg-blue-600" />
                </div>
                <div class="ml-auto text-xs text-muted-foreground">
                  {{ formatDistanceToNow(new Date(mail.date), { addSuffix: true, locale: dateFnsLocale }) }}
                </div>
              </div>
              <div class="text-xs font-medium">{{ mail.subject }}</div>
            </div>
            <div v-if="getEmailPreview(mail)" class="line-clamp-2 text-xs text-muted-foreground">
              {{ getEmailPreview(mail) }}
            </div>
            <div class="flex items-center gap-2">
              <span v-if="mail.is_starred" class="text-yellow-500">
                ⭐
              </span>
            </div>
          </button>
        </div>
      </ScrollArea>
    </div>

    <!-- Pagination controls -->
    <div class="border-t px-3 py-2 flex items-center justify-between">
      <div class="text-xs text-muted-foreground">
        {{ t('mail.list.totalCount', { count: totalEmailCount }) }}
      </div>
      <div class="flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon"
          class="h-7 w-7"
          :disabled="mailStore.currentPage <= 1 || mailStore.isLoadingEmails"
          @click="goToPreviousPage"
        >
          <ChevronLeft class="h-4 w-4" />
        </Button>
        <Select
          :model-value="String(mailStore.currentPage)"
          @update:model-value="(value) => goToPage(Number(value))"
          :disabled="mailStore.isLoadingEmails"
        >
          <SelectTrigger class="w-[70px] h-7 text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent class="max-h-[200px]">
            <SelectItem
              v-for="page in pageOptions"
              :key="page.value"
              :value="String(page.value)"
              class="text-xs"
            >
              {{ page.label }}
            </SelectItem>
          </SelectContent>
        </Select>
        <Button
          variant="ghost"
          size="icon"
          class="h-7 w-7"
          :disabled="mailStore.currentPage >= mailStore.totalPages || mailStore.isLoadingEmails"
          @click="goToNextPage"
        >
          <ChevronRight class="h-4 w-4" />
        </Button>
      </div>
    </div>
  </div>
</template>
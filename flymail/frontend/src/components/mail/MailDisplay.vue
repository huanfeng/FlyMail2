<script lang="ts" setup>
import { computed, ref } from 'vue'
import { format } from 'date-fns'
import { zhCN, enUS } from 'date-fns/locale'
import { useI18n } from 'vue-i18n'
import {
  Archive,
  ArchiveX,
  Clock,
  Forward,
  MoreVertical,
  Reply,
  ReplyAll,
  Trash2,
  Star,
} from 'lucide-vue-next'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { ScrollArea } from '@/components/ui/scroll-area'
// import type { Attachment } from '@/api/types'
// import type { AttachmentDownloadInfo } from '@/types/components'
import { Input } from '@/components/ui/input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useMailApiStore } from '@/stores/mailApi'
import { Skeleton } from '@/components/ui/skeleton'
import { Paperclip } from 'lucide-vue-next'

const mailStore = useMailApiStore()
const { t, locale } = useI18n()

const currentMail = computed(() => mailStore.currentEmail)
const isLoadingDetail = computed(() => mailStore.isLoadingDetail)
const emailIframe = ref<HTMLIFrameElement | null>(null)

// Get date-fns locale based on current locale
const dateFnsLocale = computed(() => locale.value === 'zh' ? zhCN : enUS)

const mailInitials = computed(() => {
  if (!currentMail.value) return ''
  // Extract name from email format "Name <email@example.com>"
  const match = currentMail.value.from.match(/^([^<]+)\s*<([^>]+)>$/)
  const name = match ? match[1].trim() : currentMail.value.from.split('@')[0]
  return name
    .split(' ')
    .map(n => n[0])
    .join('')
    .toUpperCase()
})

const senderName = computed(() => {
  if (!currentMail.value) return ''
  const match = currentMail.value.from.match(/^([^<]+)\s*<([^>]+)>$/)
  return match ? match[1].trim() : currentMail.value.from.split('@')[0]
})

const senderEmail = computed(() => {
  if (!currentMail.value) return ''
  const match = currentMail.value.from.match(/^([^<]+)\s*<([^>]+)>$/)
  return match ? match[2] : currentMail.value.from
})

const isStarred = computed(() => {
  if (!currentMail.value) return false
  return currentMail.value.is_starred || false
})

function handleReply() {
  // TODO: Implement reply functionality
  console.log('Reply to:', currentMail.value?.from)
}

function handleReplyAll() {
  // TODO: Implement reply all functionality
  console.log('Reply all')
}

function handleForward() {
  // TODO: Implement forward functionality
  console.log('Forward')
}

function handleQuickReply(event: Event) {
  const target = event.target as HTMLInputElement
  const message = target.value.trim()
  
  if (message) {
    // TODO: Implement quick reply sending
    console.log('Quick reply:', message)
    target.value = ''
  }
}

function handleArchive() {
  // TODO: Implement archive functionality
  console.log('Archive')
}

async function handleTrash() {
  if (currentMail.value) {
    await mailStore.deleteEmail(currentMail.value.email_id)
  }
}

async function toggleStar() {
  if (currentMail.value) {
    await mailStore.toggleStar(currentMail.value.email_id)
  }
}

async function markAsUnread() {
  if (currentMail.value) {
    await mailStore.markAsRead(currentMail.value.email_id, false)
  }
}

function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 Bytes'
  
  const k = 1024
  const sizes = ['Bytes', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

// Sanitize HTML content to prevent XSS
function sanitizeHtml(html: string): string {
  // Create a temporary element to parse HTML
  const temp = document.createElement('div')
  temp.innerHTML = html
  
  // Remove dangerous elements
  const dangerousTags = ['script', 'style', 'link', 'meta', 'object', 'embed', 'iframe', 'form', 'input', 'button']
  dangerousTags.forEach(tag => {
    const elements = temp.querySelectorAll(tag)
    elements.forEach(el => el.remove())
  })
  
  // Remove all event handlers and javascript: URLs
  const allElements = temp.querySelectorAll('*')
  allElements.forEach(element => {
    // Remove all attributes starting with 'on'
    Array.from(element.attributes).forEach(attr => {
      if (attr.name.startsWith('on')) {
        element.removeAttribute(attr.name)
      }
    })
    
    // Clean href attributes with javascript:
    if (element.tagName === 'A' && element.getAttribute('href')) {
      const href = element.getAttribute('href')
      if (href && href.toLowerCase().includes('javascript:')) {
        element.setAttribute('href', '#')
      }
    }
  })
  
  return temp.innerHTML
}


// Create a complete HTML document for iframe to isolate styles
const sanitizedHtmlWithStyles = computed(() => {
  if (!currentMail.value?.body_html) return ''
  
  const html = sanitizeHtml(currentMail.value.body_html)
  
  // Create a complete HTML document with base styles
  return `
    <!DOCTYPE html>
    <html>
    <head>
      <meta charset="utf-8">
      <meta name="viewport" content="width=device-width, initial-scale=1">
      <style>
        body {
          font-family: system-ui, -apple-system, sans-serif;
          font-size: 14px;
          line-height: 1.6;
          color: #374151;
          margin: 0;
          padding: 0;
          word-wrap: break-word;
        }
        
        /* Reset common email styles */
        table { border-collapse: collapse; }
        img { max-width: 100%; height: auto; }
        a { color: #3b82f6; text-decoration: underline; }
        blockquote { 
          margin: 0 0 1rem 0; 
          padding-left: 1rem; 
          border-left: 4px solid #e5e7eb; 
        }
        pre { 
          background: #f3f4f6; 
          padding: 1rem; 
          border-radius: 0.375rem; 
          overflow-x: auto; 
        }
        
        /* Dark mode support */
        @media (prefers-color-scheme: dark) {
          body { color: #e5e7eb; }
          blockquote { border-left-color: #4b5563; }
          pre { background: #1f2937; }
        }
      </style>
    </head>
    <body>
      ${html}
    </body>
    </html>
  `
})

// Handle iframe load event
function handleIframeLoad() {
  // Use window.setTimeout to ensure proper context
  window.setTimeout(resizeIframe, 100)
}

// Resize iframe to fit content
function resizeIframe() {
  if (!emailIframe.value) return
  
  const iframe = emailIframe.value
  const doc = iframe.contentDocument
  
  if (doc && doc.body) {
    // Reset height to auto to get accurate measurement
    iframe.style.height = 'auto'
    
    // Get the actual content height
    const contentHeight = Math.max(
      doc.body.scrollHeight,
      doc.body.offsetHeight,
      doc.documentElement.clientHeight,
      doc.documentElement.scrollHeight,
      doc.documentElement.offsetHeight
    )
    
    // Add a small buffer for padding
    iframe.style.height = (contentHeight + 20) + 'px'
    
    // Also set max-height to prevent excessive height
    iframe.style.maxHeight = '800px'
    iframe.style.overflowY = 'auto'
  }
}

// Download attachment
function downloadAttachment(attachment: any) {
  // Construct download URL
  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api/v1'
  const downloadUrl = `${baseUrl}/emails/${currentMail.value?.email_id}/attachments/${attachment.attachment_id}/download`
  
  // Create a temporary link and trigger download
  const link = document.createElement('a')
  link.href = downloadUrl
  link.download = attachment.filename
  
  // Add auth token to the request
  const token = localStorage.getItem('access_token')
  if (token) {
    link.href += `?token=${encodeURIComponent(token)}`
  }
  
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}
</script>

<template>
  <div class="flex h-full flex-col">
    <!-- Toolbar -->
    <div class="flex items-center p-2">
      <div class="flex items-center gap-2">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" :disabled="!currentMail" @click="handleArchive">
              <Archive class="h-4 w-4" />
              <span class="sr-only">{{ t('mail.viewer.archive') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('mail.viewer.archive') }}</TooltipContent>
        </Tooltip>
        
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" :disabled="!currentMail">
              <ArchiveX class="h-4 w-4" />
              <span class="sr-only">{{ t('mail.viewer.spam') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('mail.viewer.markAsSpam') }}</TooltipContent>
        </Tooltip>
        
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" :disabled="!currentMail" @click="handleTrash">
              <Trash2 class="h-4 w-4" />
              <span class="sr-only">{{ t('mail.viewer.delete') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('mail.viewer.delete') }}</TooltipContent>
        </Tooltip>
        
        <Separator orientation="vertical" class="mx-1 h-6 bg-border" />
        
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" :disabled="!currentMail">
              <Clock class="h-4 w-4" />
              <span class="sr-only">{{ t('mail.viewer.snooze') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('mail.viewer.snooze') }}</TooltipContent>
        </Tooltip>
        
        <Tooltip>
          <TooltipTrigger as-child>
            <Button 
              variant="ghost" 
              size="icon" 
              :disabled="!currentMail"
              @click="toggleStar"
            >
              <Star class="h-4 w-4" :class="isStarred && 'fill-current'" />
              <span class="sr-only">{{ t('mail.viewer.star') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ isStarred ? t('mail.viewer.unstar') : t('mail.viewer.addStar') }}</TooltipContent>
        </Tooltip>
      </div>
      
      <div class="ml-auto flex items-center gap-2">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" :disabled="!currentMail" @click="handleReply">
              <Reply class="h-4 w-4" />
              <span class="sr-only">{{ t('mail.viewer.reply') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('mail.viewer.reply') }}</TooltipContent>
        </Tooltip>
        
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" :disabled="!currentMail" @click="handleReplyAll">
              <ReplyAll class="h-4 w-4" />
              <span class="sr-only">{{ t('mail.viewer.replyAll') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('mail.viewer.replyAll') }}</TooltipContent>
        </Tooltip>
        
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" :disabled="!currentMail" @click="handleForward">
              <Forward class="h-4 w-4" />
              <span class="sr-only">{{ t('mail.viewer.forward') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('mail.viewer.forward') }}</TooltipContent>
        </Tooltip>
      </div>
      
      <Separator orientation="vertical" class="mx-2 h-6 bg-border" />
      
      <DropdownMenu>
        <DropdownMenuTrigger as-child>
          <Button variant="ghost" size="icon" :disabled="!currentMail">
            <MoreVertical class="h-4 w-4" />
            <span class="sr-only">{{ t('mail.viewer.more') }}</span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem @click="markAsUnread">{{ t('mail.viewer.markAsUnread') }}</DropdownMenuItem>
          <DropdownMenuItem>{{ t('mail.viewer.addLabel') }}</DropdownMenuItem>
          <DropdownMenuItem>{{ t('mail.viewer.muteConversation') }}</DropdownMenuItem>
          <DropdownMenuItem>{{ t('mail.viewer.print') }}</DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
    
    <Separator />
    
    <!-- Mail Content Container -->
    <div class="flex-1 flex flex-col overflow-hidden">
      <!-- Loading State -->
      <div v-if="isLoadingDetail && !currentMail" class="flex-1 flex items-center justify-center">
        <div class="space-y-4 w-full max-w-2xl px-8">
          <Skeleton class="h-4 w-full" />
          <Skeleton class="h-4 w-3/4" />
          <Skeleton class="h-32 w-full" />
          <Skeleton class="h-4 w-full" />
          <Skeleton class="h-4 w-5/6" />
        </div>
      </div>
      
      
      <!-- Mail Content -->
      <div v-else-if="currentMail" class="flex-1 flex flex-col min-h-0">
        <!-- Email Header -->
        <div class="p-4 border-b flex-shrink-0">
          <h2 class="text-lg font-semibold mb-2">{{ currentMail.subject }}</h2>
          <div class="flex items-start gap-4">
            <Avatar class="h-10 w-10">
              <AvatarImage v-if="false" src="" />
              <AvatarFallback>{{ mailInitials }}</AvatarFallback>
            </Avatar>
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2">
                <div class="font-medium text-sm">{{ senderName }}</div>
                <div class="text-xs text-muted-foreground">{{ senderEmail }}</div>
              </div>
              <div class="text-xs text-muted-foreground mt-1">
                {{ t('mail.viewer.to') }} {{ currentMail.to }}
                <span v-if="currentMail.cc"> · {{ t('mail.viewer.cc') }} {{ currentMail.cc }}</span>
              </div>
            </div>
            <div class="text-xs text-muted-foreground">
              {{ currentMail.date ? format(new Date(currentMail.date), 'PPpp', { locale: dateFnsLocale }) : '' }}
            </div>
          </div>
        </div>
        
        <!-- Email Body with scroll -->
        <ScrollArea class="flex-1 overflow-hidden">
          <div class="p-4">
            <!-- HTML Content with isolated styles -->
            <div v-if="currentMail.body_html" class="email-content-wrapper">
              <iframe 
                ref="emailIframe"
                :srcdoc="sanitizedHtmlWithStyles"
                class="w-full border-0"
                sandbox="allow-popups allow-popups-to-escape-sandbox"
                @load="handleIframeLoad"
              />
            </div>
            <!-- Plain Text Content -->
            <div v-else-if="currentMail.body" class="whitespace-pre-wrap text-sm">
              {{ currentMail.body }}
            </div>
            <!-- Empty Content -->
            <div v-else class="text-sm text-muted-foreground">
              {{ t('mail.viewer.emptyContent') }}
            </div>
            
            <!-- Attachments -->
            <div v-if="currentMail.attachments && currentMail.attachments.length > 0" class="mt-4 pt-4 border-t">
              <h4 class="text-sm font-medium mb-2">{{ t('mail.viewer.attachments') }} ({{ currentMail.attachments.length }})</h4>
              <div class="space-y-2">
                <div v-for="attachment in currentMail.attachments" 
                     :key="attachment.attachment_id"
                     class="flex items-center gap-2 p-2 rounded-md border hover:bg-accent cursor-pointer transition-colors"
                     @click="downloadAttachment(attachment)"
                     :title="`${t('mail.viewer.download')} ${attachment.filename}`"
                >
                  <Paperclip class="h-4 w-4" />
                  <span class="text-sm flex-1">{{ attachment.filename }}</span>
                  <span class="text-xs text-muted-foreground">{{ formatFileSize(attachment.size) }}</span>
                </div>
              </div>
            </div>
          </div>
        </ScrollArea>
        
        <!-- Quick Reply Bar -->
        <div class="border-t bg-background flex-shrink-0">
          <div class="flex items-center gap-2 p-3">
            <Input 
              class="flex-1"
              :placeholder="t('mail.viewer.quickReply')"
              @keyup.enter="handleQuickReply"
            />
            <Button size="sm" variant="ghost" @click="handleReply">
              <Reply class="h-4 w-4 mr-1" />
              {{ t('mail.viewer.fullReply') }}
            </Button>
            <Button size="sm" variant="ghost" @click="handleReplyAll">
              <ReplyAll class="h-4 w-4 mr-1" />
              {{ t('mail.viewer.replyAll') }}
            </Button>
            <Button size="sm" variant="ghost" @click="handleForward">
              <Forward class="h-4 w-4 mr-1" />
              {{ t('mail.viewer.forward') }}
            </Button>
          </div>
        </div>
      </div>
      
      <!-- Empty State -->
      <div v-else class="flex-1 flex items-center justify-center p-8 text-center text-muted-foreground">
        <div>
          <p class="text-lg font-medium">{{ t('mail.viewer.noEmailSelected') }}</p>
          <p class="text-sm">{{ t('mail.viewer.selectEmailPrompt') }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Iframe wrapper ensures proper sizing */
.email-content-wrapper {
  width: 100%;
}

.email-content-wrapper iframe {
  width: 100%;
  min-height: 100px;
  height: auto;
  border: none;
  display: block;
  overflow-y: auto;
}

.email-content :deep(table) {
  border-collapse: collapse;
  width: 100%;
}

.email-content :deep(table td),
.email-content :deep(table th) {
  border: 1px solid hsl(var(--border));
  padding: 0.5rem;
}
</style>
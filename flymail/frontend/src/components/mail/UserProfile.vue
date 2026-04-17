<script lang="ts" setup>
import { computed, inject } from 'vue'
import { Settings, LogOut } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useAuthStore } from '@/stores/auth'
import { authService } from '@/api'
import type { UrlDrivenStateManager } from '@/composables/useUrlDrivenState'

interface UserProfileProps {
  isCollapsed: boolean
}

defineProps<UserProfileProps>()
const authStore = useAuthStore()
const { t } = useI18n()

// 注入状态管理器
const stateManager = inject<UrlDrivenStateManager>('stateManager')!

// Get user from auth store
const user = computed(() => authStore.user || {
  username: 'User',
  email: 'user@example.com',
  is_admin: false
})

const userInitials = computed(() => {
  const name = user.value.username || 'U'
  return name
    .split(' ')
    .map(n => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2)
})

function handleSettingsClick() {
  stateManager.openSettings()
}

function handleLogout() {
  authService.logout()
}
</script>

<template>
  <div class="border-b">
    <div :class="cn('flex items-center h-[52px] px-2', isCollapsed && 'justify-center')">
      <Avatar :class="cn('h-8 w-8', isCollapsed && 'h-8 w-8')">
        <AvatarFallback>{{ userInitials }}</AvatarFallback>
      </Avatar>

      <div v-if="!isCollapsed" class="flex-1 min-w-0 ml-3">
        <div class="text-sm font-medium truncate">{{ user.username }}</div>
        <div class="text-xs text-muted-foreground truncate">{{ user.email }}</div>
      </div>

      <div v-if="!isCollapsed" class="flex gap-1 ml-auto">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" class="h-8 w-8" @click="handleSettingsClick">
              <Settings class="h-4 w-4" />
              <span class="sr-only">{{ t('common.settings') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('common.settings') }}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" class="h-8 w-8" @click="handleLogout">
              <LogOut class="h-4 w-4" />
              <span class="sr-only">{{ t('common.logout') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{{ t('common.logout') }}</TooltipContent>
        </Tooltip>
      </div>

      <div v-else class="flex flex-col gap-1">
        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" class="h-8 w-8" @click="handleSettingsClick">
              <Settings class="h-4 w-4" />
              <span class="sr-only">{{ t('common.settings') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">{{ t('common.settings') }}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger as-child>
            <Button variant="ghost" size="icon" class="h-8 w-8" @click="handleLogout">
              <LogOut class="h-4 w-4" />
              <span class="sr-only">{{ t('common.logout') }}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent side="right">{{ t('common.logout') }}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { Icon } from '@iconify/vue'
import { computed } from 'vue'
import { cn } from '@/lib/utils'
import type { ToastVariant } from './types'

interface Props {
  title?: string
  description?: string
  variant?: ToastVariant
  duration?: number
  closable?: boolean
  action?: {
    label: string
    onClick: () => void
  }
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  duration: 5000,
  closable: true
})

defineEmits<{
  close: []
}>()

const iconMap = {
  default: '',
  success: 'mdi:check-circle',
  error: 'mdi:alert-circle',
  warning: 'mdi:alert',
  info: 'mdi:information'
}

const icon = computed(() => iconMap[props.variant])

const toastClasses = computed(() => cn(
  'relative pointer-events-auto flex w-full max-w-md rounded-lg border p-4 shadow-lg transition-all',
  'data-[swipe=cancel]:translate-x-0 data-[swipe=end]:translate-x-[var(--radix-toast-swipe-end-x)]',
  'data-[swipe=move]:translate-x-[var(--radix-toast-swipe-move-x)] data-[swipe=move]:transition-none',
  'data-[state=open]:animate-in data-[state=closed]:animate-out data-[swipe=end]:animate-out',
  'data-[state=closed]:fade-out-80 data-[state=closed]:slide-out-to-right-full',
  'data-[state=open]:slide-in-from-top-full data-[state=open]:sm:slide-in-from-bottom-full',
  {
    'bg-background text-foreground border-border': props.variant === 'default',
    'bg-green-50 text-green-900 border-green-200 dark:bg-green-900/20 dark:text-green-100 dark:border-green-800': props.variant === 'success',
    'bg-red-50 text-red-900 border-red-200 dark:bg-red-900/20 dark:text-red-100 dark:border-red-800': props.variant === 'error',
    'bg-yellow-50 text-yellow-900 border-yellow-200 dark:bg-yellow-900/20 dark:text-yellow-100 dark:border-yellow-800': props.variant === 'warning',
    'bg-blue-50 text-blue-900 border-blue-200 dark:bg-blue-900/20 dark:text-blue-100 dark:border-blue-800': props.variant === 'info',
  }
))
</script>

<template>
  <div :class="toastClasses">
    <div class="flex gap-3">
      <Icon v-if="icon" :icon="icon" class="h-5 w-5 shrink-0" />
      <div class="flex-1 space-y-1">
        <p v-if="title" class="text-sm font-semibold leading-none">
          {{ title }}
        </p>
        <p v-if="description" class="text-sm opacity-90">
          {{ description }}
        </p>
      </div>
    </div>
    <div class="flex gap-2 ml-auto pl-4">
      <button
        v-if="action"
        @click="action.onClick"
        class="shrink-0 text-sm font-medium hover:opacity-90 transition-opacity"
      >
        {{ action.label }}
      </button>
      <button
        v-if="closable"
        @click="$emit('close')"
        class="shrink-0 rounded-md p-1 hover:bg-black/5 dark:hover:bg-white/5 transition-colors"
      >
        <Icon icon="mdi:close" class="h-4 w-4" />
      </button>
    </div>
  </div>
</template>
import { ref, readonly } from 'vue'
import type { ToastOptions, ToastItem } from './types'

const toasts = ref<ToastItem[]>([])
let toastId = 0

function generateId(): string {
  return `toast-${++toastId}`
}

function addToast(options: ToastOptions): string {
  const id = generateId()
  const duration = options.duration ?? 5000
  
  const toast: ToastItem = {
    ...options,
    id,
    duration
  }

  toasts.value.push(toast)

  if (duration > 0) {
    const timer = window.setTimeout(() => {
      removeToast(id)
    }, duration)
    toast.timer = timer
  }

  return id
}

function removeToast(id: string) {
  const index = toasts.value.findIndex(t => t.id === id)
  if (index > -1) {
    const toast = toasts.value[index]
    if (toast.timer) {
      window.clearTimeout(toast.timer)
    }
    toasts.value.splice(index, 1)
  }
}

function clearToasts() {
  toasts.value.forEach(toast => {
    if (toast.timer) {
      window.clearTimeout(toast.timer)
    }
  })
  toasts.value = []
}

export function useToast() {
  return {
    toasts: readonly(toasts),
    addToast,
    removeToast,
    clearToasts,
    toast: (options: ToastOptions | string) => {
      if (typeof options === 'string') {
        return addToast({ description: options })
      }
      return addToast(options)
    },
    success: (options: ToastOptions | string) => {
      const opts = typeof options === 'string' ? { description: options } : options
      return addToast({ ...opts, variant: 'success' })
    },
    error: (options: ToastOptions | string) => {
      const opts = typeof options === 'string' ? { description: options } : options
      return addToast({ ...opts, variant: 'error' })
    },
    warning: (options: ToastOptions | string) => {
      const opts = typeof options === 'string' ? { description: options } : options
      return addToast({ ...opts, variant: 'warning' })
    },
    info: (options: ToastOptions | string) => {
      const opts = typeof options === 'string' ? { description: options } : options
      return addToast({ ...opts, variant: 'info' })
    }
  }
}
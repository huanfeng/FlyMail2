export type ToastVariant = 'default' | 'success' | 'error' | 'warning' | 'info'

export interface ToastOptions {
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

export interface ToastItem extends ToastOptions {
  id: string
  timer?: number
}
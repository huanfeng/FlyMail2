import { useI18n } from 'vue-i18n'
import { useToast } from '@/components/ui/toast'
import type { BaseResponse } from '@/api/types'

/**
 * 处理API响应中的消息，自动翻译并显示Toast通知
 */
export function useApiMessage() {
  const { t } = useI18n()
  const toast = useToast()

  /**
   * 翻译消息ID
   * 如果消息是以 "msg." 开头的ID，尝试翻译它
   * 否则返回原始消息
   */
  function translateMessage(message: string): string {
    if (typeof message === 'string' && message.startsWith('msg.')) {
      // 尝试翻译消息ID
      const translated = t(message)
      // 如果翻译存在（不等于原始key），返回翻译
      if (translated !== message) {
        return translated
      }
    }
    return message
  }

  /**
   * 显示成功消息
   */
  function showSuccess(message: string, title?: string) {
    const translatedMessage = translateMessage(message)
    const translatedTitle = title ? translateMessage(title) : undefined

    toast.success({
      title: translatedTitle,
      description: translatedMessage
    })
  }

  /**
   * 显示错误消息
   */
  function showError(message: string, title?: string) {
    const translatedMessage = translateMessage(message)
    const translatedTitle = title ? translateMessage(title) : undefined

    toast.error({
      title: translatedTitle,
      description: translatedMessage
    })
  }

  /**
   * 显示警告消息
   */
  function showWarning(message: string, title?: string) {
    const translatedMessage = translateMessage(message)
    const translatedTitle = title ? translateMessage(title) : undefined

    toast.warning({
      title: translatedTitle,
      description: translatedMessage
    })
  }

  /**
   * 显示信息消息
   */
  function showInfo(message: string, title?: string) {
    const translatedMessage = translateMessage(message)
    const translatedTitle = title ? translateMessage(title) : undefined

    toast.info({
      title: translatedTitle,
      description: translatedMessage
    })
  }

  /**
   * 处理API响应，自动显示成功或错误消息
   */
  function handleResponse<T>(response: BaseResponse<T>, options?: {
    showSuccess?: boolean
    successTitle?: string
    errorTitle?: string
  }): T | null {
    const { showSuccess: shouldShowSuccess = true, successTitle, errorTitle } = options || {}

    if (response.code === 0) {
      // 成功响应
      if (shouldShowSuccess && response.message) {
        showSuccess(response.message, successTitle)
      }
      return response.data
    } else {
      // 错误响应
      const errorMessage = response.error?.details || response.message || t('msg.operation_failed')
      showError(errorMessage, errorTitle)
      return null
    }
  }

  /**
   * 处理错误对象，提取并显示错误消息
   */
  function handleError(error: unknown, title?: string) {
    let errorMessage = t('msg.operation_failed')

    // 处理不同类型的错误
    if (error instanceof Error) {
      errorMessage = error.message
    } else if (typeof error === 'object' && error !== null && 'response' in error) {
      const axiosError = error as any
      const response = axiosError.response?.data as BaseResponse | undefined

      if (response?.error?.details) {
        errorMessage = response.error.details
      } else if (response?.message) {
        errorMessage = response.message
      } else if (axiosError.message) {
        errorMessage = axiosError.message
      }
    } else if (typeof error === 'string') {
      errorMessage = error
    }

    showError(errorMessage, title)
  }

  return {
    translateMessage,
    showSuccess,
    showError,
    showWarning,
    showInfo,
    handleResponse,
    handleError,
    toast
  }
}
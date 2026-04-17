// 错误处理工具函数

import { AxiosError } from 'axios'
import { i18n } from '@/locales'

// 定义基础响应接口，避免循环依赖
interface BaseResponse<T = unknown> {
  code: number
  message: string
  data: T | null
  error?: {
    details?: string
    field?: string
    reason?: string
    suggestion?: string
    error_code?: string
    metadata?: Record<string, unknown>
  } | null
}

/**
 * 从各种类型的错误中提取错误消息
 * @param error - 任意类型的错误
 * @returns 错误消息字符串
 */
export function getErrorMessage(error: unknown): string {
  // 处理 Axios 错误
  if (error instanceof AxiosError) {
    const response = error.response?.data as BaseResponse | undefined
    if (response?.error?.details) {
      return translateMessage(response.error.details)
    }
    if (response?.message) {
      return translateMessage(response.message)
    }
    if (error.message) {
      return error.message
    }
  }
  
  // 处理标准 Error 对象
  if (error instanceof Error) {
    return error.message
  }
  
  // 处理带有 response 属性的对象（兼容旧代码）
  if (typeof error === 'object' && error !== null && 'response' in error) {
    const axiosLikeError = error as any
    const message = axiosLikeError.response?.data?.message || axiosLikeError.message || '请求失败'
    return translateMessage(message)
  }
  
  // 处理字符串错误
  if (typeof error === 'string') {
    return translateMessage(error)
  }
  
  // 默认错误消息
  return i18n.global.t('msg.operation_failed')
}

/**
 * 翻译消息ID
 * 如果消息是以 "msg." 开头的ID，尝试翻译它
 * 否则返回原始消息
 */
function translateMessage(message: string): string {
  if (typeof message === 'string' && message.startsWith('msg.')) {
    // 尝试翻译消息ID
    const translated = i18n.global.t(message)
    // 如果翻译存在（不等于原始key），返回翻译
    if (translated !== message) {
      return translated
    }
  }
  return message
}

/**
 * 判断是否为 Axios 错误
 */
export function isAxiosError(error: unknown): error is AxiosError {
  return error instanceof AxiosError
}

/**
 * 获取 HTTP 状态码
 */
export function getErrorStatusCode(error: unknown): number | undefined {
  if (isAxiosError(error)) {
    return error.response?.status
  }
  return undefined
}

/**
 * 判断是否为网络错误
 */
export function isNetworkError(error: unknown): boolean {
  if (isAxiosError(error)) {
    return !error.response && error.code === 'ERR_NETWORK'
  }
  return false
}

/**
 * 判断是否为超时错误
 */
export function isTimeoutError(error: unknown): boolean {
  if (isAxiosError(error)) {
    return error.code === 'ECONNABORTED' || error.code === 'ERR_TIMEOUT'
  }
  return false
}
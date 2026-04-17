import type { BaseResponse } from './types'

/**
 * 自定义API错误类
 */
export class ApiError extends Error {
  public readonly code: number
  public readonly messageId: string
  public readonly details?: string
  public readonly response?: BaseResponse

  constructor(response: BaseResponse) {
    const message = response.error?.details || response.message || 'msg.operation_failed'
    super(message)
    
    this.name = 'ApiError'
    this.code = response.code
    this.messageId = response.message
    this.details = response.error?.details
    this.response = response
    
    // 保持原型链
    Object.setPrototypeOf(this, ApiError.prototype)
  }
}
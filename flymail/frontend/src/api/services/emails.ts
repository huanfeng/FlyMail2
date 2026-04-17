import { api } from '../axios'
import { API_ENDPOINTS } from '../config'
import type {
  Email,
  EmailDetail,
  EmailFlags,
  UpdateEmailFlagsRequest,
  BatchUpdateFlagsRequest,
  BatchOperationResult,
  VirtualFolder,
  PageData
} from '../types'
import { ApiError } from '../ApiError'

interface EmailListParams {
  page?: number
  page_size?: number
  account_id?: number
  folder_id?: number
  folder?: string
  virtual_folder?: VirtualFolder
  is_read?: boolean
  is_starred?: boolean
  search?: string
}

class EmailsService {
  /**
   * Get emails list with pagination and filters
   */
  async getEmails(params?: EmailListParams) {
    const response = await api.get<PageData<Email>>(
      API_ENDPOINTS.EMAILS.LIST,
      { params }
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get email detail by ID
   */
  async getEmail(id: number) {
    const response = await api.get<EmailDetail>(
      API_ENDPOINTS.EMAILS.GET(id)
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Delete email
   */
  async deleteEmail(id: number) {
    const response = await api.delete<null>(
      API_ENDPOINTS.EMAILS.DELETE(id)
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Get email flags without fetching full content
   */
  async getEmailFlags(id: number) {
    const response = await api.get<EmailFlags>(
      API_ENDPOINTS.EMAILS.GET_FLAGS(id)
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Update email flags (read/starred status)
   */
  async updateEmailFlags(id: number, flags: UpdateEmailFlagsRequest) {
    const response = await api.patch<null>(
      API_ENDPOINTS.EMAILS.UPDATE_FLAGS(id),
      flags
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Mark email as read
   */
  async markAsRead(id: number) {
    return this.updateEmailFlags(id, { is_read: true })
  }

  /**
   * Mark email as unread
   */
  async markAsUnread(id: number) {
    return this.updateEmailFlags(id, { is_read: false })
  }

  /**
   * Toggle email star status
   */
  async toggleStar(id: number, isStarred: boolean) {
    return this.updateEmailFlags(id, { is_starred: isStarred })
  }

  /**
   * Batch update multiple emails flags using new API endpoint
   */
  async batchUpdateFlags(email_ids: number[], flags: Omit<BatchUpdateFlagsRequest, 'email_ids'>) {
    const response = await api.post<BatchOperationResult>(
      API_ENDPOINTS.EMAILS.BATCH_FLAGS,
      { email_ids, ...flags }
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Batch delete multiple emails using new API endpoint
   */
  async batchDelete(email_ids: number[]) {
    const response = await api.delete<BatchOperationResult>(
      API_ENDPOINTS.EMAILS.BATCH_DELETE,
      { data: { email_ids } }
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Legacy batch update method for backwards compatibility
   * @deprecated Use batchUpdateFlags instead
   */
  async legacyBatchUpdateFlags(ids: number[], flags: UpdateEmailFlagsRequest) {
    const promises = ids.map(id => this.updateEmailFlags(id, flags))
    const results = await Promise.allSettled(promises)
    
    const failed = results.filter(r => r.status === 'rejected')
    if (failed.length > 0) {
      console.error('部分邮件更新失败:', failed)
    }
    
    return {
      successful: results.filter(r => r.status === 'fulfilled').length,
      failed: failed.length,
      failed_ids: [],
      errors: failed.map((_, index) => `邮件 ${ids[index]} 更新失败`)
    }
  }

  /**
   * Legacy batch delete method for backwards compatibility
   * @deprecated Use batchDelete instead
   */
  async legacyBatchDelete(ids: number[]) {
    const promises = ids.map(id => this.deleteEmail(id))
    const results = await Promise.allSettled(promises)
    
    const failed = results.filter(r => r.status === 'rejected')
    if (failed.length > 0) {
      console.error('部分邮件删除失败:', failed)
    }
    
    return {
      successful: results.filter(r => r.status === 'fulfilled').length,
      failed: failed.length,
      failed_ids: [],
      errors: failed.map((_, index) => `邮件 ${ids[index]} 删除失败`)
    }
  }

  /**
   * Format email date for display
   */
  formatEmailDate(date: string): string {
    const emailDate = new Date(date)
    const now = new Date()
    const diff = now.getTime() - emailDate.getTime()
    const days = Math.floor(diff / (1000 * 60 * 60 * 24))
    
    if (days === 0) {
      // Today: show time
      return emailDate.toLocaleTimeString('zh-CN', { 
        hour: '2-digit', 
        minute: '2-digit' 
      })
    } else if (days === 1) {
      // Yesterday
      return '昨天'
    } else if (days < 7) {
      // This week: show day of week
      return emailDate.toLocaleDateString('zh-CN', { 
        weekday: 'long' 
      })
    } else if (emailDate.getFullYear() === now.getFullYear()) {
      // This year: show month and day
      return emailDate.toLocaleDateString('zh-CN', { 
        month: 'short', 
        day: 'numeric' 
      })
    } else {
      // Other: show full date
      return emailDate.toLocaleDateString('zh-CN', { 
        year: 'numeric', 
        month: 'short', 
        day: 'numeric' 
      })
    }
  }

  /**
   * Format file size for display
   */
  formatFileSize(bytes: number): string {
    if (bytes === 0) return '0 B'
    
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  /**
   * Extract sender name from email address
   */
  extractSenderName(from: string): string {
    // Handle format: "Name" <email@example.com>
    const match = from.match(/^"?([^"<]+)"?\s*<?([^>]+)>?$/)
    if (match) {
      return match[1].trim()
    }
    
    // If no name, return email without domain
    const emailMatch = from.match(/^([^@]+)@/)
    if (emailMatch) {
      return emailMatch[1]
    }
    
    return from
  }

  /**
   * Extract email address from string
   */
  extractEmailAddress(str: string): string {
    const match = str.match(/<([^>]+)>/)
    return match ? match[1] : str
  }
}

export const emailsService = new EmailsService()
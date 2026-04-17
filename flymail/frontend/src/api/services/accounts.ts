import { api } from '../axios'
import { API_ENDPOINTS } from '../config'
import { ApiError } from '../ApiError'
import type {
  EmailAccount,
  CreateAccountRequest,
  AccountTestResult,
  SendEmailRequest,
  SyncTaskRequest,
  Task,
  TaskPageData
} from '../types';

class AccountsService {
  /**
   * Get all email accounts for current user
   */
  async getAccounts() {
    const response = await api.get<{ accounts: EmailAccount[]; count: number }>(
      API_ENDPOINTS.ACCOUNTS.LIST
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get specific account by ID
   */
  async getAccount(id: number) {
    const response = await api.get<EmailAccount>(
      API_ENDPOINTS.ACCOUNTS.GET(id)
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Create new email account
   */
  async createAccount(account: CreateAccountRequest) {
    const response = await api.post<{ account: EmailAccount }>(
      API_ENDPOINTS.ACCOUNTS.CREATE,
      account
    )
    
    if (response.code === 0 && response.data) {
      return response.data.account
    }
    
    throw new ApiError(response)
  }

  /**
   * Update email account
   */
  async updateAccount(id: number, account: CreateAccountRequest) {
    const response = await api.put<null>(
      API_ENDPOINTS.ACCOUNTS.UPDATE(id),
      account
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Delete email account
   */
  async deleteAccount(id: number) {
    const response = await api.delete<null>(
      API_ENDPOINTS.ACCOUNTS.DELETE(id)
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Send email using specific account
   */
  async sendEmail(accountId: number, email: SendEmailRequest) {
    const response = await api.post<null>(
      API_ENDPOINTS.ACCOUNTS.SEND(accountId),
      email
    )
    
    if (response.code === 0) {
      return true
    }
    
    throw new ApiError(response)
  }

  /**
   * Create sync task for specific account
   */
  async syncAccount(accountId: number, options?: SyncTaskRequest) {
    const response = await api.post<Task>(
      API_ENDPOINTS.ACCOUNTS.SYNC(accountId),
      options
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Test account connection
   */
  async testAccount(accountId: number) {
    const response = await api.post<AccountTestResult>(
      API_ENDPOINTS.ACCOUNTS.TEST(accountId)
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Test account configuration without saving
   */
  async testTempAccount(account: CreateAccountRequest) {
    const response = await api.post<{ test_result: AccountTestResult }>(
      API_ENDPOINTS.ACCOUNTS.TEMP_TEST,
      account
    )
    
    if (response.code === 0 && response.data) {
      return response.data.test_result
    }
    
    throw new ApiError(response)
  }

  /**
   * Sync all accounts
   */
  async syncAllAccounts(options?: SyncTaskRequest) {
    const response = await api.post<Task>(
      API_ENDPOINTS.ACCOUNTS.SYNC_ALL,
      options
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get sync history for specific account
   */
  async getSyncHistory(accountId: number, page = 1, pageSize = 20) {
    const response = await api.get<TaskPageData>(
      API_ENDPOINTS.ACCOUNTS.SYNC_HISTORY(accountId),
      {
        params: {
          page,
          page_size: pageSize
        }
      }
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }
}

export const accountsService = new AccountsService()
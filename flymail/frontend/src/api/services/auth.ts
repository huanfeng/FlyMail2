import { api } from '../axios'
import { API_ENDPOINTS, TOKEN_KEYS } from '../config'
import type { LoginRequest, RefreshRequest, LoginResponse, User, UpdateCredentialsRequest } from '../types'
import { useAuthStore } from '../../stores/auth'
import { ApiError } from '../ApiError'

class AuthService {
  /**
   * User login
   */
  async login(credentials: LoginRequest) {
    const response = await api.post<LoginResponse>(
      API_ENDPOINTS.AUTH.LOGIN,
      credentials
    )
    
    if (response.code === 0 && response.data) {
      // Save tokens
      localStorage.setItem(TOKEN_KEYS.ACCESS_TOKEN, response.data.access_token)
      localStorage.setItem(TOKEN_KEYS.REFRESH_TOKEN, response.data.refresh_token)
      
      // Update auth store
      const authStore = useAuthStore()
      authStore.setUser(response.data.user)
      
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Refresh access token
   */
  async refresh(refreshToken: string) {
    const response = await api.post<LoginResponse>(
      API_ENDPOINTS.AUTH.REFRESH,
      { refresh_token: refreshToken } as RefreshRequest
    )
    
    if (response.code === 0 && response.data) {
      // Update tokens
      localStorage.setItem(TOKEN_KEYS.ACCESS_TOKEN, response.data.access_token)
      localStorage.setItem(TOKEN_KEYS.REFRESH_TOKEN, response.data.refresh_token)
      
      // Update auth store
      const authStore = useAuthStore()
      authStore.setUser(response.data.user)
      
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get current user info
   */
  async getCurrentUser() {
    const response = await api.get<User>(API_ENDPOINTS.AUTH.ME)
    
    if (response.code === 0 && response.data) {
      // Update auth store
      const authStore = useAuthStore()
      authStore.setUser(response.data)
      
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Logout
   */
  logout() {
    const authStore = useAuthStore()
    authStore.logout()
  }

  /**
   * Check if user is authenticated
   */
  isAuthenticated(): boolean {
    return !!localStorage.getItem(TOKEN_KEYS.ACCESS_TOKEN)
  }

  /**
   * Get stored access token
   */
  getAccessToken(): string | null {
    return localStorage.getItem(TOKEN_KEYS.ACCESS_TOKEN)
  }

  /**
   * Get stored refresh token
   */
  getRefreshToken(): string | null {
    return localStorage.getItem(TOKEN_KEYS.REFRESH_TOKEN)
  }

  /**
   * Update admin credentials (username, email, password)
   */
  async updateCredentials(data: UpdateCredentialsRequest) {
    const response = await api.put(API_ENDPOINTS.AUTH.UPDATE_CREDENTIALS, data)
    
    if (response.code === 0) {
      // If username or email was updated, update the auth store
      if (data.username || data.email) {
        await this.getCurrentUser()
      }
      
      return response.data
    }
    
    throw new ApiError(response)
  }
}

export const authService = new AuthService()
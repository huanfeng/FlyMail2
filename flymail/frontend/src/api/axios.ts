import axios, { type AxiosInstance, AxiosError, type AxiosRequestConfig } from 'axios'
import { API_CONFIG, TOKEN_KEYS, API_ENDPOINTS } from './config'
import type { BaseResponse } from './types'
import { useAuthStore } from '../stores/auth'

// Create axios instance
const axiosInstance: AxiosInstance = axios.create({
  baseURL: API_CONFIG.BASE_URL,
  timeout: API_CONFIG.TIMEOUT,
  headers: {
    'Content-Type': 'application/json',
  },
})

// Token refresh promise to prevent multiple refresh requests
let refreshTokenPromise: Promise<BaseResponse<{ access_token: string; refresh_token: string }>> | null = null

// Request interceptor
axiosInstance.interceptors.request.use(
  (config) => {
    // Add auth token to requests
    const token = localStorage.getItem(TOKEN_KEYS.ACCESS_TOKEN)
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// Response interceptor
axiosInstance.interceptors.response.use(
  (response) => {
    // Return data directly for successful responses
    return response.data
  },
  async (error: AxiosError<BaseResponse>) => {
    const originalRequest = error.config as AxiosRequestConfig & { _retry?: boolean }

    // Handle 401 Unauthorized
    if (error.response?.status === 401 && !originalRequest._retry) {
      originalRequest._retry = true

      // Skip refresh for login and refresh endpoints
      if (
        originalRequest.url === API_ENDPOINTS.AUTH.LOGIN ||
        originalRequest.url === API_ENDPOINTS.AUTH.REFRESH
      ) {
        const authStore = useAuthStore()
        authStore.logout()
        return Promise.reject(error)
      }

      // Refresh token
      if (!refreshTokenPromise) {
        const refreshToken = localStorage.getItem(TOKEN_KEYS.REFRESH_TOKEN)
        if (refreshToken) {
          refreshTokenPromise = axiosInstance
            .post<BaseResponse<{ access_token: string; refresh_token: string }>>(API_ENDPOINTS.AUTH.REFRESH, { refresh_token: refreshToken })
            .then((response) => {
              const data = response as unknown as BaseResponse<{ access_token: string; refresh_token: string }>
              if (data.code === 0 && data.data) {
                localStorage.setItem(TOKEN_KEYS.ACCESS_TOKEN, data.data.access_token)
                localStorage.setItem(TOKEN_KEYS.REFRESH_TOKEN, data.data.refresh_token)
                return data
              }
              throw new Error('Token refresh failed')
            })
            .catch((err) => {
              const authStore = useAuthStore()
              authStore.logout()
              throw err
            })
            .finally(() => {
              refreshTokenPromise = null
            })
        } else {
          const authStore = useAuthStore()
          authStore.logout()
          return Promise.reject(error)
        }
      }

      try {
        const tokenData = await refreshTokenPromise
        originalRequest.headers!.Authorization = `Bearer ${tokenData.data?.access_token}`
        return axiosInstance(originalRequest)
      } catch (refreshError) {
        return Promise.reject(refreshError)
      }
    }

    // Handle other errors
    if (error.response) {
      const responseData = error.response.data
      
      // Log error for debugging
      console.error('API Error:', {
        status: error.response.status,
        message: responseData?.message,
        error: responseData?.error
      })
    } else if (error.request) {
      console.error('Network Error:', '无法连接到服务器')
    } else {
      console.error('Request Error:', error.message)
    }

    return Promise.reject(error)
  }
)

// Export configured axios instance
export default axiosInstance

// Helper function for making API requests
export async function apiRequest<T = unknown>(
  config: AxiosRequestConfig
): Promise<BaseResponse<T>> {
  try {
    const response = await axiosInstance(config)
    return response as unknown as BaseResponse<T>
  } catch (error) {
    throw error
  }
}

// Helper functions for common HTTP methods
export const api = {
  get: <T = unknown>(url: string, config?: AxiosRequestConfig) => 
    apiRequest<T>({ ...config, method: 'GET', url }),
  
  post: <T = unknown, D = unknown>(url: string, data?: D, config?: AxiosRequestConfig) => 
    apiRequest<T>({ ...config, method: 'POST', url, data }),
  
  put: <T = unknown, D = unknown>(url: string, data?: D, config?: AxiosRequestConfig) => 
    apiRequest<T>({ ...config, method: 'PUT', url, data }),
  
  patch: <T = unknown, D = unknown>(url: string, data?: D, config?: AxiosRequestConfig) => 
    apiRequest<T>({ ...config, method: 'PATCH', url, data }),
  
  delete: <T = unknown>(url: string, config?: AxiosRequestConfig) => 
    apiRequest<T>({ ...config, method: 'DELETE', url }),
}
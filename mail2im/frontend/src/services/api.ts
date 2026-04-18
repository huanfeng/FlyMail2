import http from './http';
import { useAuthStore } from '../stores/auth';
import { pinia } from '../stores';

let refreshPromise: Promise<string | null> | null = null;

http.interceptors.request.use((config) => {
  const auth = useAuthStore(pinia);
  auth.loadFromStorage();
  if (auth.accessToken) {
    config.headers = config.headers || {};
    config.headers.Authorization = `Bearer ${auth.accessToken}`;
  }
  return config;
});

http.interceptors.response.use(
  (response) => {
    // 解包统一响应格式: {code, message, data} → 提取 data
    const body = response.data;
    if (body && typeof body === 'object' && 'code' in body && body.code === 0) {
      response.data = body.data;
    }
    return response;
  },
  async (error) => {
    const { response, config } = error;
    if (!response || !config) {
      return Promise.reject(error);
    }

    // 适配统一错误格式: {code, message, error: {details}} → 兼容旧的 .error 字符串读法
    const body = response.data;
    if (body && typeof body === 'object' && 'code' in body && body.code !== 0) {
      response.data.error = body.message || body.error?.details || 'unknown error';
    }

    const status = response.status;
    const originalRequest: any = config;
    const skipRetry = originalRequest._skipAuthRetry;
    const url = config.url || '';
    const noRetryAuthPaths = ['/auth/login', '/auth/setup', '/auth/refresh'];
    const isNoRetryAuth = noRetryAuthPaths.some((p) => url.includes(p));

    if (status === 401 && !originalRequest._retry && !isNoRetryAuth && !skipRetry) {
      originalRequest._retry = true;
      const auth = useAuthStore(pinia);
      auth.loadFromStorage();

      if (!refreshPromise) {
        refreshPromise = auth.refresh().finally(() => {
          refreshPromise = null;
        });
      }

      const newToken = await refreshPromise;
      if (newToken) {
        originalRequest.headers = originalRequest.headers || {};
        originalRequest.headers.Authorization = `Bearer ${newToken}`;
        return http(originalRequest);
      }

      auth.clear();
    }

    return Promise.reject(error);
  }
);

export default http;

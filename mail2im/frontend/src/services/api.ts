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
  (response) => response,
  async (error) => {
  const { response, config } = error;
  if (!response || !config) {
    return Promise.reject(error);
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

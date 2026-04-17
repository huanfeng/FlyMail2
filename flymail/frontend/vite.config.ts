import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    host: true,
    proxy: {
      // 代理所有以 /api 开头的请求到后端服务器
      '/api': {
        target: 'http://localhost:8080/', // 后端服务地址
        changeOrigin: true, // 改变请求头中的host为target的host
        secure: false, // 如果是https接口，需要配置这个参数
        // rewrite: (path) => path.replace(/^\/api/, '') // 如果后端不需要/api前缀，取消注释这行
        timeout: 0,
        // 配置SSE支持
        configure: (proxy, _) => {
          proxy.on('proxyReq', (proxyReq, req, _res) => {
            if (req.url?.includes('/v1/events')) {
              // SSE 相关头
              proxyReq.setHeader('Accept', 'text/event-stream')
              proxyReq.setHeader('Cache-Control', 'no-cache')
              proxyReq.setHeader('Connection', 'keep-alive')
            }
          })
          proxy.on('proxyRes', (proxyRes, req, _res) => {
            // SSE specific handling
            if (req.url?.includes('/v1/events')) {
              proxyRes.headers['content-type'] = 'text/event-stream'
              proxyRes.headers['cache-control'] = 'no-cache'
              proxyRes.headers['connection'] = 'keep-alive'
              proxyRes.headers['access-control-allow-origin'] = '*'
              // Remove compression for SSE
              delete proxyRes.headers['content-encoding']
              delete proxyRes.headers['content-length']
            }
          })
        }
      }
    },
  }
})
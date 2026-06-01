import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

// 前端构建产物输出到后端 embed 目录（flymail/backend/web/dist）。
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: path.resolve(__dirname, '../backend/web/dist'),
    emptyOutDir: true,
  },
  server: {
    // FlyMail 专属开发端口（避开 Vite 默认 5173，减少与其它项目冲突）。
    port: Number(process.env.FLYMAIL_WEB_PORT) || 5390,
    strictPort: true,
    proxy: {
      '/api': {
        target: process.env.API_HOST || 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})

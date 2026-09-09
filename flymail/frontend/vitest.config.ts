import { defineConfig } from 'vitest/config'
import path from 'path'

export default defineConfig({
  resolve: {
    // 复用 vite.config.ts 中的 @ alias，使测试文件可以用 @/ 路径
    alias: { '@': path.resolve(__dirname, './src') },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    // 含 .tsx：组件级用例（如信任名单分区）也要跑。
    include: ['src/**/*.test.ts', 'src/**/*.test.tsx'],
  },
})

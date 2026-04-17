# FlyMailPlus Frontend

Vue 3 + TypeScript + Vite + Shadcn-vue 邮件客户端前端

## 功能特性

- 🔐 JWT 认证和自动刷新
- 📧 多账户邮箱管理
- 📁 文件夹树形结构
- 🔄 实时邮件同步（SSE）
- 🎨 主题定制器
- 📱 响应式设计

## 技术栈

- Vue 3 + Composition API
- TypeScript
- Vite
- Tailwind CSS
- Shadcn-vue 组件库
- Pinia 状态管理
- Axios HTTP 客户端
- Radix Vue 无障碍组件

## 快速开始

### 安装依赖

```bash
pnpm install
```

### 配置环境变量

复制 `.env.example` 到 `.env` 并修改配置：

```bash
cp .env.example .env
```

### 启动开发服务器

```bash
pnpm dev
```

### 构建生产版本

```bash
pnpm build
```

## 项目结构

```
src/
├── api/              # API 服务层
│   ├── services/     # 各模块服务
│   ├── types.ts      # TypeScript 类型定义
│   └── config.ts     # API 配置
├── components/       # 组件
│   ├── ui/          # 基础 UI 组件
│   └── mail/        # 邮件相关组件
├── composables/     # 组合式函数
├── views/           # 页面组件
├── stores/          # Pinia 状态管理
├── router/          # 路由配置
└── lib/             # 工具函数
```

## 默认账户

- 用户名: admin
- 密码: admin123

## API 文档

API 服务层基于后端的 OpenAPI 规范自动生成类型，提供完整的 TypeScript 支持。

详细使用说明请查看 [API 文档](./src/api/README.md)

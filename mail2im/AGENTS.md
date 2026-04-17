# Repository Guidelines

- 请总是用简体中文回复

## 项目结构与模块组织
- `backend/` 包含 Go API/worker 入口 `cmd/server/main.go`，业务逻辑集中在 `internal/`（API handlers、config/provider loader、event bus、dispatcher/channels）；数据写入 `backend/mail2im.db`，二进制附件存放在 `backend/storage/attachments/`，Janitor 默认保留 30 天。
- `backend/config/providers.json` 提供默认 provider 配置，部署时结合环境变量调整；日志输出走标准输出，可由外部进程采集。
- `frontend/` 为 Vue 3 + TypeScript + Vite 应用，`src/` 存放视图（Dashboard、Emails、Channels 等）、共享组件如 `PageHeader.vue`、路由与 API 帮助方法 `src/services/api.ts`；构建产物输出到 `dist/`，静态资源放在 `public/`。
- `docs/` 记录设计与进度，`start_dev.sh` 一键启动前后端开发环境；根目录还有 `walkthrough.md`、`GEMINI.md` 供调试参考。

## 构建、测试与开发命令
- 全栈开发：直接运行 `./start_dev.sh`（默认 `PORT=8080`、`FRONTEND_PORT=8008`、`VITE_API_BASE_URL=/api`，可用环境变量覆盖），脚本会并行启动两端。
- 后端：`cd backend && go run cmd/server/main.go` 进行本地调试；发布前用 `go build ./cmd/server` 生成二进制；若需要热重载可搭配 `air` 等外部工具；确保 Go 版本 ≥ 1.25.4。
- 前端：首次进入 `cd frontend && pnpm install`（需 Node 18+、pnpm）；开发时 `pnpm dev --port 8008 --host ::`；产线构建 `pnpm build`（包含类型检查）；用 `pnpm preview --host --port 4173` 验证打包结果。
- 本地运行需保证 `backend/mail2im.db` 与 `storage/attachments/` 可写；生产场景建议将两者挂载至持久卷，并通过系统服务/容器托管。

## 代码风格与命名约定
- Go：统一使用 `gofmt -w ./...`，默认 tab 缩进；包名使用小写单数；导出符号写注释；显式处理错误，将业务逻辑沉淀到 `internal/core`/`dispatcher` 而非 handler，避免在路由层做过多计算。
- 前端：优先 `<script setup lang="ts">`，组件 PascalCase（如 `Emails.vue`、`PageHeader.vue`），路由与文件路径用 kebab-case；HTTP 调用集中在 `src/services/api.ts`，复用在 `main.ts` 注册的 PrimeVue 组件；共享样式放在 `src/style.css`，页面私有样式可用模块 CSS（如 `Accounts.module.css`），保持 2 空格缩进。

## 测试指引
- 当前无自动化套件，补充 Go 表驱动测试放置于同包 `*_test.go`，使用 `go test ./...` 执行；涉及 dispatcher、队列或数据库的逻辑建议添加集成测试或最小化模拟。
- UI 可视需要引入 Vitest/组件测试；在此之前以 `pnpm build` 加手工冒烟为主，覆盖登录/导航、邮件详情展示、搜索过滤、渠道创建与测试流程，并确认国际化文案、时区/日期格式展示正确；调度或事件流改动可用 `/api/debug/test-event` 与 Debug 视图做回归。

## 提交与 Pull Request
- 遵循现有提交格式：`feat: ...`、`fix: ...`、`chore: ...`，主题句保持简短祈使语（英文或中英均可），分支名称可沿用 `feature/`、`fix/` 前缀。
- PR 需说明改动范围、风险与 DB/配置影响，关联 issue/工单；UI 变更附截图或动图；注明 API 协议或环境变量变动（如 `PORT`、`CORS_ORIGINS`、`FRONTEND_PORT`、`VITE_API_BASE_URL`）；合入前至少跑通 `pnpm build` 与关键手测，避免无关格式化噪音。
- 除非明确表示需要提交代码, 否则不要主动提交

## 安全与配置提示
- 不要将密钥入库；避免提交 `mail2im.db` 快照或附件内容，`config/providers.json` 中的敏感字段需脱敏；生产环境请将 `ENV=production` 以启用限制性 CORS。
- 生产环境设置合理的 `CORS_ORIGINS`，避免过宽；避免记录凭据日志或明文令牌；API 入口校验请求并在输出 `/api/emails/:id/html` 时确保 HTML 已被净化；部署时可搭配反向代理（如 Nginx）做 access logging 与 rate limit。

## 前端规则
- 所有的 localStorage 数据的 key, 都使用统一的前缀 mail2im_

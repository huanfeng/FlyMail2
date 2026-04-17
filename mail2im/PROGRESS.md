# Mail2IM 开发任务清单

## Phase 1: 核心引擎与调试基座 (Core & Debug)
- [x] 设计数据库模型 (Account, Proxy, Config) <!-- id: 10 -->
- [x] 实现 EventBus 事件总线 <!-- id: 11 -->
- [x] 实现 DebugService (内存状态管理) <!-- id: 12 -->
- [x] 实现前端调试面板 (/dev) <!-- id: 13 -->
- [x] **[NEW] 重构前端为 Sakai 风格 Admin 布局** <!-- id: 17 -->
- [x] **[NEW] 优化 Topbar 功能 (主题/语言/暗色模式)** <!-- id: 18 -->
- [x] 实现多账户 Worker 与 IDLE/轮询逻辑 <!-- id: 14 -->
- [x] 实现代理管理 (CRUD) <!-- id: 15 -->
- [x] 实现批量账户创建 API <!-- id: 16 -->

## Phase 2: 通知分发系统 (Notification Dispatcher)
- [x] 设计通知渠道接口 (Channel Interface) <!-- id: 20 -->
- [x] 实现基础渠道 (Email, Telegram, Discord) <!-- id: 21 -->
- [x] 实现国内渠道 (企业微信, 飞书, 钉钉) <!-- id: 22 -->
- [x] 实现策略引擎 (静默, 过滤, 优先级) <!-- id: 23 -->
- [x] 实现附件处理与下载服务 <!-- id: 24 -->
- [x] 实现数据清理任务 (Janitor) <!-- id: 25 -->

## Phase 3: Web 查看器与高级功能 (Web & Advanced)
- [x] **[NEW] 实现代理管理页面 (Proxies.vue)** <!-- id: 34 -->
- [x] **[NEW] 实现账户管理页面 (Accounts.vue)** <!-- [x] Implement Email List & Detail View <!-- id: 3 -->
    - [x] Backend: Add Email model and storage logic
    - [x] Backend: Implement Email API (List, Detail, HTML)
    - [x] Frontend: Create Emails.vue and EmailDetail.vue
    - [x] Frontend: Update routes and navigation

- [x] Implement IM Forwarding Channels (Refactor) <!-- id: 4 -->
    - [x] Backend: Add Channel model and migration
    - [x] Backend: Implement Channel CRUD API
    - [x] Backend: Update Dispatcher to load channels from DB
    - [x] Frontend: Create Channels.vue (List & Wizard)
    - [x] Frontend: Add route and menu item
- [/] 实现单封邮件查看页 <!-- id: 30 -->
- [ ] 集成 AI 翻译 API <!-- id: 31 -->
- [ ] 实现 OAuth2 授权流程 <!-- id: 32 -->

## Phase 4: 授权与商业化 (Licensing)
- [ ] 实现设备指纹采集 <!-- id: 40 -->
- [ ] 实现定期上报与授权验证逻辑 <!-- id: 41 -->
- [ ] 实现功能特性开关 <!-- id: 42 -->

## Phase 5: 扩展性与维护 (Extensibility)
- [ ] 完善 API 文档 <!-- id: 50 -->
- [ ] 系统监控与日志 <!-- id: 51 -->

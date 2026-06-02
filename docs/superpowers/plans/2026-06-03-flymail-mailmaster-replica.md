# FlyMail 完全复刻 MailMaster 模板 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development。步骤用 `- [ ]` 跟踪。

**Goal:** 把 FlyMail 前端**完全复刻** MailMaster 模板的布局、组件、视觉与动效,并接 FlyMail 真实后端数据(替换 MailMaster 的 seed 数据);保留中文字体(不复刻英文字体)。

**Architecture:** MailMaster 是 React 单页(自解包 bundle,源码已解包到 `.dev/mailmaster/`)。复刻=把它的 CSS 设计系统整体作为 FlyMail 样式底座 + 按其结构重写组件 + 接 FlyMail 数据。主题切换用 `data-theme`+`data-mode`(+保留 `.dark` 兼容 shadcn)。

**蓝本源码(只读参考,subagent 必读):**
- `.dev/mailmaster/app.css` — 完整设计系统 + 全部组件 CSS + 动画(fadeIn/pop/toastIn/slide-in/resizer)。
- `.dev/mailmaster/src_extracted/00_*.js` 数据种子(EN)、`01_*` 数据种子(ZH)、`02_*` Reader、`03_*` Sidebar、`04_*` Icon 集、`05_*` 邮件列表、`06_*` 通知&设置整页、`07_*` Compose 模态、`08_*` 顶层 App(视图导航/语言/拖拽/TWEAK_DEFAULTS)。
- `.dev/mailmaster/index.full.html` — 模板 head(字体 + CSS 引入)。

**FlyMail 真实数据(已具备的 hooks,见 `src/lib/queries.ts`):** useAccounts / useFolders / useInfiniteMessages / useMessageDetail / useMarkRead / useToggleFlag / useSend / useDrafts(+CRUD) / useSettings / useSetAccountEnabled / useAccountStats / 账户 CRUD / useChangePassword / useTriggerSync / useSyncStatus；附件 `lib/attachments.ts`；SSE `useRealtimeSync`；主题 `lib/theme.ts`。

**MailMaster 有但 FlyMail 后端暂无的功能** → 复刻 UI、能接的接、不能接的合理降级(不报错、不留死按钮):线程会话(我们是单封 → 渲染为单条 thread)、labels 标签、notifications 通知中心(整页 UI 保留,数据暂空/隐藏入口)、tweaks 面板(布局微调,可做前端 localStorage)、two-slide 布局模式(可做)。**注意降级要克制**:宁可隐藏入口也不要放无效按钮。

---

## Phase A：CSS 底座 + 主题属性（基础）

### Task A1: 移植 MailMaster app.css 为 FlyMail 样式底座

**Files:** `src/index.css`、`src/lib/theme.ts`、`src/main.tsx`

- [ ] **Step 1:** 把 `.dev/mailmaster/app.css` 的内容(去掉 `@font-face` 与开头的语言注释)并入 `src/index.css`:保留顶部 `@import "tailwindcss";` 与 shadcn `@theme inline`/`:root`/`.dark`(供残留 shadcn 组件);追加 MailMaster 的全部规则(`:root` 令牌、`[data-theme][data-mode]` 9×2 主题、`.app/.col/.col-resize`、sidebar/list/reader/compose/settings/notif/toast 等全部组件类、`@keyframes fadeIn/pop/toastIn`、滚动条、`::selection`)。
- [ ] **Step 2: 字体改中文栈**:把 MailMaster 的 `--font-display: 'Libre Caslon Text'...`、`--font-body: 'Inter Tight'...` 改为 FlyMail 中文友好栈:
  - `--font-display: "PingFang SC","Microsoft YaHei",-apple-system,"Segoe UI",serif;`(标题,可略带 serif fallback)
  - `--font-body: -apple-system,"Segoe UI","Microsoft YaHei","PingFang SC","Hiragino Sans GB",sans-serif;`
  - `--font-mono`: 保留等宽栈。**不引入** Google Fonts / 英文字体 @font-face。
- [ ] **Step 3: theme.ts 适配 data-mode**:`applyTheme` 同时设 `documentElement.dataset.theme=tone`、`dataset.mode=mode`、`classList.toggle('dark', mode==='dark')`。`getTheme` 不变(mode+tone,默认 light+slate)。TONES 已含 9 调。`main.tsx` 仍 `initTheme()`。
- [ ] **Step 4:** `pnpm build` 通过;`pnpm test`(theme.test 可能需把断言从只查 .dark 改为同时查 data-mode,更新之)。
- [ ] **Step 5: 提交** `feat(flymail-web): 移植 MailMaster 完整 CSS 设计系统(中文字体)+ 主题 data-mode`

---

## Phase B：App 壳 + Sidebar + Icon 集

### Task B1: Icon 组件

**Files:** Create `src/components/ui/Icon.tsx`

- [ ] 参考 `.dev/mailmaster/src_extracted/04_*.js`,把其 stroke-based 16px 图标集移植为 React 组件 `<Icon name size stroke/>`(TS 化,name 联合类型)。供下面组件统一用。提交 `feat(flymail-web): MailMaster 图标集`。

### Task B2: App 壳(三栏 + 可拖拽)+ Sidebar

**Files:** `src/components/mail/AppLayout.tsx`(重写为 .app/.col 结构 + .col-resize)、`src/components/mail/AccountSidebar.tsx`(重写)、`src/pages/Shell.tsx`(适配)

- [ ] **Step 1: AppLayout** 按 `app.css` 的 `.app/.col.sidebar/.col.list/.col.reader/.col-resize` 重写:三栏 + 两个拖拽手柄(沿用现有 pointer capture + localStorage 记忆宽度,但 DOM/类名改为 MailMaster 的 `.col-resize`,拖拽时加 `.dragging` 与 body `.is-resizing`)。宽度用 CSS 变量 `--sidebar-w/--list-w` 由状态注入。
- [ ] **Step 2: Sidebar** 参考 `03_*.js` 重写:`.sidebar-head`(brand-mark + brand-name + brand-dot)、`.compose-btn`(写邮件,接 onCompose)、`.sidebar-scroll`(账户树:`.account-row` 可展开 + `.folder-list/.folder-row` 文件夹,接 useAccounts/useFolders/选中态/未读 count;labels 暂隐藏或留静态;`.side-section-label` 分组标题 + add 按钮接添加账户)、`.sidebar-foot`(当前用户/设置入口齿轮)。视图导航(mail/notif/settings)做成 state:notif/settings 切换到整页(Phase E),mail 为默认三栏。
- [ ] **Step 3:** 接 Shell 现有选中逻辑(account/folder/message via URL params)。`pnpm build`+tsc。
- [ ] **Step 4: 提交** `feat(flymail-web): 复刻 App 壳(可拖拽三栏)+ Sidebar`

---

## Phase C：邮件列表

### Task C1: MailList 复刻

**Files:** `src/components/mail/MailList.tsx`(重写)

- [ ] 参考 `05_*.js` + `app.css` 的 `.list-head/.search-bar/.filter-chips/.mail-list/.mail-item(+.mail-item-row 紧凑)/.avatar-sq/.mi-*`。重写为:
  - `.list-head`(标题 + 副标题计数)。
  - `.search-bar`(搜索框 + ⌘K kbd;**搜索后端暂无 → 先做前端按已加载列表过滤**,标注后续接后端)。
  - `.filter-chips`(全部/未读/星标 等,接前端过滤)。
  - `.mail-list` 虚拟滚动(沿用 @tanstack/react-virtual + useInfiniteMessages 无限加载)+ `.mail-item` 卡片(头像方块 acct-pip、未读点、发件人/时间、主题、预览、tags、悬停星标);保留"紧凑/卡片"两样式(list-prefs)——紧凑映射到 `.mail-item-row`。
  - 接 useToggleFlag(星标)、选中态(.selected)、未读(.unread)。
- [ ] `pnpm build`+tsc+test(date-group/list-prefs 保留)。提交 `feat(flymail-web): 复刻邮件列表(搜索/筛选chips/卡片+紧凑/虚拟滚动)`。

---

## Phase D：Reader（阅读区）

### Task D1: Reader 复刻

**Files:** `src/components/mail/Reader.tsx`(重写)

- [ ] 参考 `02_*.js` + `app.css` 的 `.reader-empty/.reader-toolbar/.tb-btn/.reader-inner/.reader-subject/.thread-msg/.thread-head/.thread-body/.thread-attach/.attach-card/.reply-box/.pill-btn`。重写为:
  - 空态 `.reader-empty`(标题 + 提示 + 快捷键表)。
  - 工具条 `.reader-toolbar`(归档/删除占位降级、星标、标未读、回复、转发——只放能接的:星标 useToggleFlag、标未读 useMarkRead、回复/转发 onReply/onForward;删除/归档后端暂无 → 隐藏或禁用带 tooltip)。
  - 正文区 `.reader-inner`:`.reader-subject` 大标题 + meta;**单封消息渲染为一个 `.thread-msg`**(thread-head 头像/发件人/收件人/时间 + thread-body)。HTML 正文仍用**沙箱 iframe**(保留 cidHtml/processedHtml/showImages/远程图拦截/内联图);纯文本走 `.thread-body p`。
  - 附件 `.thread-attach/.attach-card`(接 lib/attachments:下载/预览,过滤 is_inline)。
  - `.reply-box`(点开触发 onReply 打开 Compose,或内联快捷回复——先做点击打开 Compose)。
- [ ] 保留按需正文抓取(useMessageDetail)。`pnpm build`+tsc。提交 `feat(flymail-web): 复刻 Reader(工具条/会话视图/沙箱正文/附件卡片/回复)`。

---

## Phase E：Compose 模态 + Settings/Notifications 整页

### Task E1: Compose 模态

**Files:** `src/components/mail/ComposeDialog.tsx`(重写为 MailMaster 风)

- [ ] 参考 `07_*.js` + `.compose-window/.compose-bar(最小化)/.compose-head/.compose-row/.compose-textarea/.compose-foot`。重写撰写窗(右下浮窗 + 最小化条 + pop 动画),接 useSend/useCreateDraft/useUpdateDraft(保留富文本或纯文本正文、收件人/抄送/密送、从账户选择、回复/转发预填 compose-prefill)。提交 `feat(flymail-web): 复刻 Compose 浮窗(最小化/动画)`。

### Task E2: Settings 整页 + 主题网格 + Notifications 整页(降级)

**Files:** `src/components/settings/*`(重写为 MailMaster 风)、Shell 视图切换

- [ ] **Settings** 参考 `06_*.js` + `.fullpage/.settings-grid/.settings-nav/.settings-block/.theme-grid-large/.theme-card/.toggle/.lang-toggle/.account-card`。做整页设置(或模态 `.settings-dialog`,二选一,优先整页贴合 MailMaster):分区 外观(主题卡片网格 9 调 + 亮/暗 mode-toggle + 语言)、账户(account-card 列表 + 增删改/启停/连接测试,接现有)、邮件(同步深度/轮询间隔/远程图/列表样式)、安全(改密)。主题卡片实时 applyTheme。
- [ ] **Notifications** 整页 `.notif-*`:后端暂无通知数据 → 渲染整页框架 + 空态(notif-empty),侧栏 bell 入口可暂隐藏或显示但空。**克制降级**。
- [ ] `pnpm build`+tsc+test。提交 `feat(flymail-web): 复刻 设置整页(主题网格)+ 通知页(降级)`。

---

## Phase F：动效/Toast/快捷键 + i18n + 收尾

### Task F1: 动效 + Toast + 快捷键 + i18n

- [ ] Toast 组件(`.toast`+toastIn,用于发送成功/操作反馈)。pop/fadeIn 已随 CSS 生效(模态/浮窗)。slide-in reader(two-slide 布局)可选实现。
- [ ] 快捷键(参考 08:c 撰写、j/k 上下、r 回复、/ 搜索、g 切换等)——做核心几个。
- [ ] i18n:补齐所有新文案 zh/en 对齐;校验 JSON、key 集合相等。
- [ ] 提交 `feat(flymail-web): Toast + 快捷键 + i18n 收尾`。

---

## 最终审查 + 收尾

- [ ] `pnpm test` 全绿、`tsc --noEmit`、`pnpm build` 通过;i18n zh/en key 对齐脚本校验。
- [ ] code-reviewer/designer 审查:与 MailMaster 视觉/动效一致度、多主题×亮暗、虚拟列表/无限加载未回归、降级处无死按钮、无写死颜色(全用令牌)。
- [ ] 真机自测清单交用户:三栏拖拽、主题网格切 9 调×亮暗即时变、列表卡片/紧凑/搜索/筛选、Reader 会话视图+附件、Compose 浮窗+最小化、设置整页、动效(模态 pop / toast)。
- [ ] superpowers:finishing-a-development-branch(合并 main 并删分支)。

## 已知取舍
- 字体用中文栈(不复刻 MailMaster 英文字体)。
- 线程会话渲染为单封(后端无线程);labels/notifications/tweaks 为 UI 复刻 + 数据降级。
- 搜索先前端过滤已加载项,后端全文搜索后续(属 P0,另做)。
- 删除/移动/归档后端暂无 → 工具条相应按钮隐藏或禁用,不放死按钮。

# FlyMail P1 前端打磨（多色调主题 + 可拖拽三栏 + 无限加载/虚拟滚动 + 视觉打磨）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development。步骤用 `- [ ]` 复选框跟踪。

**Goal:** 把 FlyMail 前端从"能用"提升到"好用好看"：以 MailMaster 为布局模板做多色调主题系统(默认 Notion/slate 中性色)、三栏可拖拽+记忆、邮件列表无限加载+虚拟滚动+两种样式可切换、整体视觉打磨。

**Architecture:** 纯前端里程碑(后端已支持 `GET /folders/:fid/messages?before_uid=&limit=` 游标分页,无需改后端)。主题系统用 `<html data-theme="<tone>" class="dark?">` + CSS 变量(移植 MailMaster 的 9 套色调亮/暗,**保留 FlyMail 现有中文字体栈,不复刻 MailMaster 英文字体**)。三栏用 `react-resizable-panels`(autoSaveId 持久化)。列表用 `@tanstack/react-virtual` + TanStack Query `useInfiniteQuery`(before_uid 游标)。列表两种样式(Notion 紧凑+日期分组 / 卡片)由设置切换。

**Tech Stack:** React19 + Tailwind4 + CSS 变量主题、react-resizable-panels、@tanstack/react-virtual、@tanstack/react-query useInfiniteQuery、vitest。

**参考素材:**
- 布局/色调模板：`D:\Download\MailMaster_standalone.html`(超大文件,**勿全读**;用 grep 抽 CSS)。其 `:root` 与 `[data-theme="warm|slate|sky|rose|mint|lavender|coral|butter|aqua"]`(各含亮/暗,暗色用 `[data-theme=X].dark` 或 `.dark`)定义全套设计令牌。FlyMail 当前暖色即 MailMaster 的 warm 调。
- 审美目标：仓库根 `Notion_Mail_ScreenShot_*.png`(干净中性、高密度列表、按日期分组、弱头像)。Notion 中性对应 slate 调,设为默认。

---

## 文件结构

- 修改 `src/index.css` — 移植 MailMaster 设计令牌(9 色调亮/暗 + 扩展令牌 --bg-sunk/--bg-active/--ink-4/--accent-soft/--shadow-*),保留中文 --font-body。
- 重写 `src/lib/theme.ts` — 管理 tone + mode,应用 data-theme + dark,持久化;导出 THEMES 列表。
- 新建 `src/lib/list-prefs.ts` — 列表样式偏好(compact|card)读写 localStorage。
- 新建 `src/lib/date-group.ts` — 邮件按日期分组(今天/昨天/本周/更早或月份)。
- 修改 `src/components/settings/GeneralSection.tsx` — 色调选择器(色块)+ 亮/暗。
- 修改 `src/components/settings/MailSection.tsx`(或 General)— 列表样式切换。
- 修改 `src/components/mail/AppLayout.tsx` — react-resizable-panels 三栏可拖拽+记忆。
- 重写 `src/components/mail/MailList.tsx` — 虚拟滚动 + 无限加载 + 两种样式 + 日期分组(compact)+ 骨架/空态。
- 修改 `src/lib/queries.ts` — useMessages 改 useInfiniteQuery(before_uid 游标);保留 useMessagesFlat 便于消费。
- 修改 `src/pages/Shell.tsx` — 适配无限数据(flatten)+ 传 loadMore/hasNext。
- 修改 `src/components/mail/Reader.tsx`、`AccountSidebar.tsx` — 视觉打磨(头部/激活态/间距/层次)。
- 修改 `src/locales/zh.json`/`en.json` — 主题色调名、列表样式、加载更多等文案。
- 新建测试：`src/lib/theme.test.ts`、`src/lib/date-group.test.ts`、`src/lib/list-prefs.test.ts`。

---

## Phase A：多色调主题系统

### Task A1: 移植 MailMaster 设计令牌到 index.css

**Files:** Modify `src/index.css`

- [ ] **Step 1: 从 MailMaster 抽取令牌**。用:
  ```
  grep -oE "(:root|\.dark|\[data-theme=\"[a-z]+\"\](\.dark)?)[[:space:]]*\{[^}]*\}" "D:/Download/MailMaster_standalone.html"
  ```
  得到 `:root`(基础+warm 亮)、各 `[data-theme=X]`(亮)、`.dark` 与 `[data-theme=X].dark`(暗)块。注意输出中的 `\n` 是字面转义,需还原为换行。
- [ ] **Step 2: 写入 index.css**。在现有 `@theme inline`/shadcn `:root`/`.dark` 之后,追加 FlyMail 设计令牌层:
  - `:root` 设为**基础令牌 + 默认(slate 中性)调**的亮色值(把 MailMaster slate 亮色值作为 :root 默认,使无 data-theme 时即 Notion 风)。包含:--bg/--bg-alt/--bg-sunk/--bg-hover/--bg-active/--surface、--ink/--ink-2/--ink-3/--ink-4、--rule/--rule-strong、--accent/--accent-soft/--accent-wash/--accent-ink、--shadow-sm/md/lg、--sidebar-w/--list-w。
  - **保留**现有 `--font-body`(中文字体栈),**不要**引入 MailMaster 的 Libre Caslon/Inter Tight。
  - 为每个色调写 `[data-theme="warm"] { ...亮色覆盖... }` … `[data-theme="aqua"]`(9 个),值取自 MailMaster 对应块。
  - 暗色:`.dark { ...slate 暗... }` 作默认暗;每色调 `[data-theme="X"].dark { ...该调暗... }`(9 个)。
  - 兼容性:保留旧变量名(--accent-color 当前代码在用)——可加 `--accent-color: var(--accent);` 等别名,避免大面积改组件。先 grep 现有组件用到的变量名(--accent-color/--accent-wash/--bg/--bg-alt/--surface/--ink*/--rule/--bg-hover),确保新令牌全覆盖或提供别名。
- [ ] **Step 3: 验证** `pnpm build` 通过;构建后页面无样式崩溃(后续任务里跑 dev 视觉确认)。
- [ ] **Step 4: 提交** `feat(flymail-web): 移植 MailMaster 多色调设计令牌(默认 slate 中性)`

### Task A2: theme.ts 主题状态 + 应用

**Files:** Rewrite `src/lib/theme.ts`；Test `src/lib/theme.test.ts`

设计:主题 = { mode: 'light'|'dark', tone: ToneId }。应用到 `document.documentElement`:`setAttribute('data-theme', tone)` + `classList.toggle('dark', mode==='dark')`。持久化 localStorage(`flymail_theme_mode`、`flymail_theme_tone`)。默认 mode='light'、tone='slate'。

- [ ] **Step 1: 写失败测试** `theme.test.ts`（vitest + jsdom）：
  - `applyTheme({mode:'dark',tone:'sky'})` → `document.documentElement.getAttribute('data-theme')==='sky'` 且 `classList.contains('dark')`。
  - `getTheme()` 默认返回 {mode:'light',tone:'slate'}(localStorage 空时)。
  - set 后再 get 持久化一致。
- [ ] **Step 2: 实现**
  ```ts
  export type ThemeMode = 'light' | 'dark'
  export type ToneId = 'slate'|'warm'|'sky'|'rose'|'mint'|'lavender'|'coral'|'butter'|'aqua'
  export interface ThemePref { mode: ThemeMode; tone: ToneId }

  export const TONES: { id: ToneId; nameKey: string; swatch: string }[] = [
    { id: 'slate', nameKey: 'settings.general.tone.slate', swatch: '#64748b' },
    // ...其余 8 个，swatch 取该调 --accent 亮色值
  ]
  const MODE_KEY = 'flymail_theme_mode', TONE_KEY = 'flymail_theme_tone'
  export function getTheme(): ThemePref { /* 读 localStorage，缺省 light/slate，校验合法值 */ }
  export function applyTheme(t: ThemePref): void { /* setAttribute + classList + 持久化 */ }
  export function initTheme(): void { applyTheme(getTheme()) }
  ```
  在 `main.tsx`(或入口)调用 `initTheme()`(确认现有入口在哪初始化主题,替换之)。
- [ ] **Step 3: 测试通过** `pnpm test src/lib/theme.test.ts`。
- [ ] **Step 4: 提交** `feat(flymail-web): theme.ts 多色调+亮暗 状态与持久化`

### Task A3: 设置里的色调选择器

**Files:** Modify `src/components/settings/GeneralSection.tsx`

- [ ] **Step 1:** 读现有 GeneralSection(当前语言 + 亮/暗 toggle)。加"色调"分组:渲染 TONES 色块按钮(选中描边 var(--accent)),点选 `applyTheme({mode, tone})` 并存。亮/暗 toggle 复用现有,改为走 theme.ts。所有文本走 i18n。
- [ ] **Step 2: 类型检查** tsc。
- [ ] **Step 3: 提交** `feat(flymail-web): 设置-通用 增加色调选择器`

---

## Phase B：三栏可拖拽 + 记忆

### Task B1: react-resizable-panels 接入 AppLayout

**Files:** Modify `src/components/mail/AppLayout.tsx`

- [ ] **Step 1:** `pnpm add react-resizable-panels`。
- [ ] **Step 2:** 重写 AppLayout 用 `PanelGroup direction="horizontal" autoSaveId="flymail-layout"`,三个 `Panel`(sidebar/list/reader)+ 两个 `PanelResizeHandle`。约束:sidebar defaultSize≈18% minSize 12% maxSize 30%;list defaultSize≈30% minSize 20% maxSize 45%;reader 余下 minSize 30%。ResizeHandle 渲染 1px 分隔 + hover 加宽高亮(用 --rule/--accent)。保留 sidebar/list 背景(--bg-alt)。autoSaveId 自动持久化到 localStorage。
  - 注意:react-resizable-panels 用百分比;原 248/380px 换算为合理默认百分比即可。
- [ ] **Step 3: 验证** tsc + build;拖拽分隔条可调宽、刷新后保持(dev 手测)。
- [ ] **Step 4: 提交** `feat(flymail-web): 三栏可拖拽调宽 + 宽度记忆(react-resizable-panels)`

---

## Phase C：列表无限加载 + 虚拟滚动 + 两种样式

### Task C1: useInfiniteMessages（before_uid 游标）

**Files:** Modify `src/lib/queries.ts`

- [ ] **Step 1:** 新增 `useInfiniteMessages(folderId)` 用 `useInfiniteQuery`：
  ```ts
  queryKey: ['messages', folderId]
  queryFn: ({ pageParam }) => GET /folders/{folderId}/messages?limit=50&before_uid={pageParam ?? 0}
  initialPageParam: 0
  getNextPageParam: (lastPage) => lastPage.length < 50 ? undefined : lastPage[lastPage.length-1].uid
  ```
  注意游标用**列表项的 uid**(MessageListItem 含 uid;确认 types.ts 有 uid 字段,无则后端 dto 已返回则补类型)。导出一个 flatten 辅助(把 pages 拍平为 MessageListItem[])。
  **保留 query key `['messages', folderId]`**,确保现有失效逻辑(标已读/同步/SSE invalidate ['messages'])继续生效。
- [ ] **Step 2:** 验证 tsc。(无限加载行为在 C2 + Shell 接好后整体手测。)
- [ ] **Step 3: 提交** `feat(flymail-web): useInfiniteMessages 游标无限加载`

### Task C2: MailList 虚拟滚动 + 两种样式 + 日期分组 + 骨架

**Files:** Rewrite `src/components/mail/MailList.tsx`；新建 `src/lib/date-group.ts`、`src/lib/list-prefs.ts`；Test `date-group.test.ts`、`list-prefs.test.ts`

- [ ] **Step 1:** `pnpm add @tanstack/react-virtual`。
- [ ] **Step 2: list-prefs.ts**：`getListStyle(): 'compact'|'card'`(localStorage `flymail_list_style`,默认 'compact')、`setListStyle()`。写测试。
- [ ] **Step 3: date-group.ts**：`groupByDate(items)` → 有序分组 [{label, items}]，label = 今天/昨天/本周/本月/更早 或 年月(用相对时间;纯函数,传入"现在"便于测试)。写测试覆盖边界。
- [ ] **Step 4: 重写 MailList**：
  - props 改为接收 flatten 后的 messages + `hasNextPage`/`fetchNextPage`/`isFetchingNextPage` + loading。
  - 用 `useVirtualizer`(动态/估算行高:compact ~52px、card ~76px)。滚动到接近底部(virtualizer range 末尾或 onScroll 阈值)且 hasNextPage 时调 fetchNextPage。
  - **两种样式**:`compact`(单/双行:未读小圆点 + 发件人 · 主题,右侧日期/附件/星标;按 date-group 插入分组标题行)；`card`(沿用当前头像卡片三行,精修)。由 getListStyle 决定。
  - 加载首屏:骨架(3-5 条灰条占位,用 --bg-sunk/动画),替换原 "…"。空态/底部"没有更多"提示。
  - 选中/悬停态用 --accent-wash/--bg-hover;未读用 --ink 粗体 + 圆点,已读 --ink-2。
  - 虚拟列表 + 分组标题:可把分组标题也作为虚拟行(flatten 成 [{type:'header'|'item'}]),或 compact 用分组、card 不分组。实现自行权衡,保证虚拟化正确。
- [ ] **Step 5:** `pnpm test`(date-group/list-prefs 测试)+ tsc。
- [ ] **Step 6: 提交** `feat(flymail-web): 邮件列表 虚拟滚动+无限加载+紧凑/卡片两样式+日期分组+骨架`

### Task C3: Shell 接线无限数据 + 列表样式设置

**Files:** Modify `src/pages/Shell.tsx`、`src/components/settings/MailSection.tsx`(或 General)

- [ ] **Step 1: Shell**：用 useInfiniteMessages 取数据,flatten 后传 MailList(含 fetchNextPage/hasNextPage/isFetchingNextPage)。保持现有"打开自动标已读"等逻辑对 flatten 列表生效。
- [ ] **Step 2: 设置**：MailSection(或 General)加"列表样式"选择(紧凑/卡片),调 setListStyle;切换后列表即时生效(可用一个轻量事件或状态提升:简单做法是设置项写 localStorage 并触发 Shell 重渲染——可用一个 React state + storage 事件,或把 listStyle 提升到 Shell 经 context/prop)。优先简单:listStyle 放 Shell state,初值 getListStyle(),设置项通过回调更新并持久化。
- [ ] **Step 3:** tsc + build。
- [ ] **Step 4: 提交** `feat(flymail-web): Shell 接无限加载 + 列表样式可切换`

---

## Phase D：视觉打磨

### Task D1: Reader / Sidebar / 全局层次打磨

**Files:** Modify `src/components/mail/Reader.tsx`、`AccountSidebar.tsx`，必要时全局样式

- [ ] **Step 1: Reader**：头部精修(主题大字 + 发件人/收件人/日期排版,可加发件人头像;操作工具条 回复/转发/星标/未读 用图标按钮整齐排列);正文区留白与最大宽度;附件区卡片化;加载用骨架、错误态清晰。
- [ ] **Step 2: Sidebar**：账户/文件夹层次(分组标题 --ink-3 小字、激活项 --accent-wash + --accent-ink、未读计数右对齐 badge);写邮件按钮主色;间距统一。用新 --shadow-*/--rule 令牌。
- [ ] **Step 3:** tsc + build;dev 手测亮/暗 + 多色调下观感一致(无写死颜色)。
- [ ] **Step 4: 提交** `feat(flymail-web): Reader/Sidebar 视觉打磨(层次/间距/状态/工具条)`

---

## Phase E：i18n + 收尾

### Task E1: i18n + 全量校验

**Files:** Modify `src/locales/zh.json`/`en.json`

- [ ] **Step 1:** 加文案:`settings.general.tone.*`(9 色调名:石板/暖/天空/玫瑰/薄荷/薰衣草/珊瑚/奶油/水蓝)、`settings.general.toneTitle`、`settings.mail.listStyle`/`compact`/`card`、`list.loadingMore`/`list.noMore`、reader 工具条等新文案。JSON 无重复 key/无引号内嵌,校验解析。
- [ ] **Step 2:** `pnpm test`(全部前端测试)、`pnpm exec tsc --noEmit`、`pnpm build` 全过。
- [ ] **Step 3: 提交** `feat(flymail-web): P1 i18n 文案`

---

## 最终审查 + 收尾

- [ ] `pnpm test` 全绿、`tsc --noEmit` 通过、`pnpm build` 通过。
- [ ] 派 code-reviewer(或 designer)审查:主题令牌覆盖完整(无组件写死颜色导致换调失效)、虚拟列表+无限加载正确(无重复/漏项/抖动)、resizable 持久化、列表两样式切换、亮暗×多色调组合观感、可访问性(对比度/焦点)。
- [ ] superpowers:finishing-a-development-branch(合并 main 并删分支)。
- [ ] 真机自测清单交用户:设置里切换色调(默认 slate)/亮暗;拖拽三栏并刷新保持;邮件列表滚到底自动加载更多、切换紧凑/卡片样式;整体观感对照 Notion Mail 截图。

## 已知限制 / 取舍

- 多色调取自 MailMaster 9 调;字体保留中文栈(不复刻英文字体)。
- 虚拟滚动 + 分组标题混排需保证测量正确;compact 分组、card 可不分组以简化。
- 无限加载基于 before_uid 游标(后端已支持);本地已同步邮件之外的更早邮件需先同步入库才出现(与同步深度相关)。
- 列表样式/主题为前端 localStorage 偏好(未存后端,换设备不同步)——后续可入 settings 表。

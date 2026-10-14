# FlyMail 第八轮 UI/UE 打磨设计（商业化体验）

日期：2026-10-14
范围：`flymail/frontend`，P0 硬伤 7 条 + P1 层级提升 6 条。
审查方式：本机 Docker 实例（镜像与 main `1577ae4` 同步）用 Chromium 在
1440×900 / 1280×720 / 1024×768 / 390×844 四种视口、亮暗两模式、slate/warm/sky/mint
四种色调下共截 43 张图，逐张比对；再回到代码核对成因。

前七轮（见 `ui-audit.md`）已把可访问性、状态覆盖、撤销、对话框这些**机制**做扎实了。
这一轮不动机制，只处理**一眼看得见的排版与层级问题**——它们才是现在拉低
「商业产品感」的东西。

**基线事实（动手前记录，避免事后归因错误）：**
- `pnpm test`：942 项，1 项既有失败（`MailList.test.tsx › 分组打乱顺序后，方向键打开的是相邻那一行本身`），与本轮无关；本轮结束时若仍失败，单独说明。
- `pnpm lint`：既有 17 错误 3 警告；本轮判据是**本轮改动的文件不新增问题**。
- 后端通知流里「同步异常 / 已恢复」每 10 分钟震荡一次（冒烟账户 DNS 失败），
  属后端缺滞回判定，**不在本轮范围**。

---

## 一、贯穿本轮的三条做法

1. **宽度判据用容器查询，不用视口断点。** 阅读区在三栏桌面下也可能只有 400px
   （1024 平板、或用户把列表拖宽），视口断点判不准。`.col.reader` / `.reader-slide` /
   `.col.list` 设为 `container-type: inline-size`，窄栏规则写 `@container`。
   视口断点只保留三档：**1024（紧凑桌面）/ 768（移动单栏）/ 480（小屏）**，
   现有 720 与 820 两处并入 768。
2. **键盘提示只给键盘用户看。** `(pointer: coarse)` 下一律不显示 `kbd` 提示
   （搜索框、写邮件按钮、阅读区空态都适用）。
3. **凡新写的判定逻辑进 `lib/` 做纯函数并配测试**；组件只做渲染。

---

## 二、P0 硬伤

### 1. 搜索框快捷键提示折行（`MailList` 顶栏）

**现象**：默认 380px 列宽下 `Ctrl K` 断成两行挤在搜索框右缘，首屏即见；
触摸设备也显示。
**成因**：`.search-input .kbd` 没写 `white-space: nowrap`，且无任何宽度/指针守卫。
**做法**：
- `.search-input .kbd { white-space: nowrap; flex: 0 0 auto }`
- `@media (pointer: coarse) { .search-input .kbd, .compose-btn .kbd { display: none } }`
- `@container (max-width: 400px) { .search-input .kbd { display: none } }`（列栏容器）
**验证**：390 与 1440 截图里不再出现两行提示；测试读 CSS 断言这三条规则存在。

### 2. 阅读区工具条在窄栏折成两行（`ReaderToolbar`）

**现象**：≤1024 时「删除 / 更多」掉到第二行；手机上更甚。
**做法**：工具条在窄容器下切换为**图标模式**：
`@container reader (max-width: 560px) { .tb-btn > span { display: none } .tb-btn { padding: 0 8px } }`。
带文字的按钮补 `aria-label`（现在只有 `title`，文字隐藏后读屏会丢名字）。
保留 `flex-wrap` 作为更窄时的最后兜底。
**验证**：1024 与 390 截图工具条单行；a11y 测试断言每颗 `tb-btn` 有可访问名。

### 3. 会话卡头部发件人被截成 `chip3…`（`ThreadReader` / `Reader`）

**现象**：右侧「收件箱 2026/9/17 19:38:11」等宽全时间戳 + 文件夹 chip 占 200px，
1024 宽下发件人只剩 5 个字符。
**成因**：`formatDate` 是 `toLocaleString()` 全量输出；`.ti-side` 不收缩。
**做法**：
- 新增 `lib/mail-format.ts#formatShortDate(dateStr, now = new Date())`：
  今天 → `HH:mm`；本年 → `M月D日`（跟随 locale 的 `{month:'short',day:'numeric'}`）；
  其它 → `YYYY/M/D`。完整时间放 `title`。
- 折叠行与展开头都用短日期；`.ti-folder` chip 在 `@container (max-width: 480px)` 下隐藏
  （它只在聚合/搜索视图有信息量，窄屏优先保发件人）。
- 单封 `Reader` 的 `.th-time` 同样处理。
**验证**：`formatShortDate` 三分支单测（含跨年、NaN 回退原串）；1024 截图发件人完整。

### 4. 同一账户出现两个「已删除」（`AccountSidebar`）

**现象**：Gmail 账户 `[Gmail]/已删除邮件` 与 `已删除` 都归为 `trash`，
i18n 把两者都显示成「已删除」。
**做法**：新增 `lib/mail-format.ts#folderLabels(folders, t): Map<id, label>`：
按 `type` 分组，非 custom 类型**第一个**用 i18n 名，其余用服务器 `display_name`。
侧栏、右键菜单、同步进度都改走这张表。
**验证**：单测「两个 trash → 一个『已删除』一个原名」「单个 trash 不变」。

### 5. 账户健康提示条压在内容上（`HealthBanner`）

**现象**：`position: fixed` 在底部中央，叠在列表/阅读区分界上，设置弹窗开着时还压在弹窗上。
**做法**：从浮层改为**列表栏顶部的内联横幅**（Outlook 的做法）：
`Shell` 把 `<HealthBanner/>` 放进 `list` 插槽、位于 `MailList` 之上；
CSS 改为静态块、允许文字换行、`border-bottom` 分隔，去掉 `z-index`。
断线横幅 `.conn-banner` 保持浮层不动（它是秒级瞬态，不同性质）。
**验证**：截图在设置弹窗打开时横幅被遮罩盖住；`HealthBanner` 既有测试不变。

### 6. 手机端没有写邮件入口（`Shell` / `AppLayout`）

**做法**：`Shell` 在 `list` 插槽末尾加 `.compose-fab`（圆形 56px、`--primary` 底、
`edit` 图标、`aria-label=写邮件`），桌面 `display: none`，≤768 显示；
挂在 `.col.list` 内，阅读面板激活时随列表栏一起隐藏，不需要额外状态。
**验证**：390 截图右下角有 FAB；组件测试断言移动端存在、点击调用 `openCompose`。

### 7. 通知中心计数自相矛盾（`NotificationsPage`）

**现象**：「547 条未读 · 共 30 条」，30 是已加载条数。
**做法**：只显示未读数，i18n 键 `notif.unreadSummary`（zh「{{count}} 条未读」/ en「{{count}} unread」），
删掉 `isZh` 分支。
**验证**：`keys.test.ts` 自动校验键存在；组件测试断言文案。

---

## 三、P1 层级与可扫视性

### 8. 发件人头像按色相区分（列表行 ×3、`ThreadReader`、`Reader`）

**现象**：所有头像同一个 `--accent` 底色，扫视列表时无法区分发件人。
**做法**：
- 新增 `lib/avatar-color.ts#avatarHue(key): 1..8`（FNV-1a 哈希地址，稳定）。
- `index.css` 定义 `--avatar-1..8`（oklch，L≈0.52 C≈0.11，白字对比 ≥ 4.5），
  亮暗同一套（判据是「字对底」而不是「底对页」）。
- `.avatar-sq[data-hue]` 取对应变量；`key = from_addr` 小写，缺地址用名字。
- 账户色点 `.acct-pip` 保留。骨架屏、账户头像不动。
**验证**：`avatarHue` 单测（稳定、范围）；CSS 测试断言 8 个令牌齐全并按公式算白字对比。

### 9. 侧栏自定义文件夹折叠（`AccountSidebar`）

**现象**：10 个账户时 `Google/G+`、`[Gmail]/重要` 等全部平铺，侧栏成墙。
**做法**：新增 `lib/sidebar-folders.ts#splitFolders(folders, activeId, showMore)`：
系统文件夹常显；custom 文件夹 **≥ 3 个**时折进「更多文件夹 (N)」一行，
展开状态按账户记在 localStorage（复用 `sidebar-expand` 的存取套路）；
**当前激活的 custom 文件夹即使折叠也常显**，免得选中项消失。
**验证**：纯函数单测四个分支；截图侧栏高度明显缩短。

### 10. 断点收口 + 1024 紧凑档

**做法**：`720px` 与 `820px` 的两块并入 `768px`；新增 `@media (max-width: 1024px)`：
侧栏 `flex-basis: min(var(--sidebar-w), 200px)`、列表 `min(var(--list-w), 340px)`，
给阅读区让出宽度（1024 下阅读区从 396 → 484px）。
**验证**：CSS 测试断言不再存在 720/820 断点；1024 截图三栏比例。

### 11. 监控统计卡数值折行（`MonitoringSection`）

**做法**：数值 `font-size: 20px; font-variant-numeric: tabular-nums`，
长字符串（版本号）`overflow-wrap: anywhere`，网格 `minmax(150px, 1fr)`。
**验证**：截图无折行。

### 12. 撰写器发件人改为项目统一的下拉（`ComposeDialog`）

**现象**：原生 `<select>` 灰底，与全站 `DropMenu` 风格不一致。
**做法**：多地址时用 `DropMenu` 包一个 `.compose-select` 外观的触发按钮，
按钮带 `aria-labelledby` 指向「发件人」标签；选项用 `CtxMenuItem`。
**验证**：`ComposeDialog` 既有测试里涉及 select 的用例改为点菜单项；行为不变。

### 13. 登录页品牌感

**做法**：logo 底色改 `--primary` / 字 `--primary-foreground`（与写邮件按钮同一对令牌，
全主题对比已验证）；页面背景加一层极淡的 `accent-wash` 径向渐变。
**验证**：亮暗截图各一张。

---

## 四、不做的事

- 不改列表行高与 `estimateSize` 常量（虚拟滚动精度依赖它）。
- 不重做侧栏为图标栏（P2，另立里程碑）。
- 不动后端通知震荡。
- 不提交 git（前端 `CLAUDE.md` 规定需明确指示）。

## 五、验收

1. `pnpm build`（含 `tsc -b`）绿；`pnpm test` 除既有那 1 项外全绿；
   本轮触及文件 `pnpm lint` 零新增。
2. 重新跑同一套截图脚本，逐条对照上面 13 个「验证」项。
3. 独立 code-reviewer 过一遍，再修。

---

## 六、验收记录（2026-10-14）

实现完成，未提交 git（按前端 `CLAUDE.md` 等待明确指示）。
证据：`pnpm build` 绿；`pnpm test` 970 项中 969 通过，唯一失败是动手前就存在的
`MailList.test.tsx › 分组打乱顺序后，方向键打开的是相邻那一行本身`（与本轮无关）；
`pnpm lint` 仍为基线的 20 条（17 错 3 警），本轮触及文件零新增。
镜像用最终代码重建后，在 1440 / 1024 / 390 三种视口重新截图逐条对照：

| # | 条目 | 结果 | 证据 |
|---|---|---|---|
| 1 | 搜索框 kbd 折行 | ✅ | 1440 下 `Ctrl K` 单行在框内；390 触摸视口不显示；有输入时清除按钮可见。补了 `.search-input input { min-width: 0 }`——`<input>` 的固有最小宽度会把 kbd 挤出框外，这是第一次截图才发现的 |
| 2 | 阅读工具条折行 | ✅ | 1024 与 390 下单行图标模式；`ReaderToolbar.test.tsx` 钉每颗按钮有 aria-label |
| 3 | 会话卡发件人被截 | ✅ | 1024 下 `chip3@example.com` 完整，日期显示 `9月17日`；`formatShortDate` 四分支单测 |
| 4 | 两个「已删除」 | ✅ | 侧栏显示 `[Gmail]/已删除邮件` 与 `已删除`。**规则改为「同类型多个时全部用原名」**：第一版「第二个起用原名」在真实数据上仍撞名，第二个的原名恰好就叫「已删除」 |
| 5 | 健康横幅压内容 | ✅ | 内联在列表栏顶部，文字可换行，设置弹窗遮罩能盖住；手机同样。注：冒烟账户重启后暂未再故障，截图用同标记注入核验样式，组件逻辑由 `health-banner.test.tsx` 覆盖 |
| 6 | 手机无写信入口 | ✅ | 390 列表页右下角 FAB，点击打开全屏撰写 |
| 7 | 通知计数矛盾 | ✅ | 「557 条未读」；`NotificationsPage.test.tsx` 钉文案不含「共/total」 |
| 8 | 头像同色 | ✅ | 列表/会话卡/阅读头 8 色相，亮暗均可辨；CSS 测试按公式钉白字对比 ≥ 4.5 |
| 9 | 侧栏文件夹成墙 | ✅ | Gmail 账户 10 个自定义文件夹折成「更多文件夹 (10) · 22」，展开后全显；`splitFolders` 四分支单测 |
| 10 | 断点收口 | ✅ | 只剩 1024/768/480；1024 下侧栏 200 / 列表 340，阅读区明显变宽 |
| 11 | 监控卡折行 | ✅ | 「352.3 MB」「dev-preview」单行 |
| 12 | 发件人原生 select | ✅ | DropMenu 风格下拉，10 个账户单列展示；`ComposeDialog.test.tsx` 钉触发按钮与 aria-labelledby |
| 13 | 登录页品牌感 | ✅ | logo 用 primary 对令牌，顶部极淡晕染 |

**实现中的两条裁定**（都已写进代码注释与测试）：
- `formatShortDate` 多了可选 `locale` 参数，组件传 `i18n.language`：本年分支的月日格式要跟界面语言，不跟 node/浏览器 locale（测试在 en-US 环境下炸出来的）。
- 列表容器隐藏 kbd 的阈值从 400 降到 340：默认列宽 380 下 kbd 已单行放得下，400 会让桌面默认永远看不到提示。

**顺手发现、未在本轮处理：**
- 既有失败用例 `分组打乱顺序后…` 需单独看（与日期分组无关的断言差异）。
- 通知流「同步异常 / 已恢复」每 10 分钟震荡（后端缺滞回），本轮未动。

### 独立审查后的修复（同日）

独立 code-reviewer（opus）给出 0 Critical / 3 Important / 8 Minor，全部按 TDD 处理：

- **Esc 连关撰写器**（新引入）：发件人下拉是撰写器里第一个 radix 菜单，radix 在捕获阶段处理 Esc 并
  `preventDefault` 但不阻止冒泡，全局快捷键随后把撰写器也关了。`useKeyboardShortcuts` 的 Esc 分支
  加 `if (e.defaultPrevented) return`——顺带修掉了阅读区「更多」菜单按 Esc 会关掉当前邮件的同类老问题。
- **文件夹名只在侧栏统一**：移动菜单 6 处、列表标题、同步进度、文件夹 chip 全部改走 `folderLabel`；
  `folderLabels` 改按「账户 + 类型」计数（MailList 的列表跨账户，两个账户各一个 trash 不算重复）。
  结构性测试扫描 `src/components` 不允许再出现 `t(\`folder.${…}\`)`。
- **1024 档拖拽死区**：CSS 用 `min()` 封顶列宽，但拖拽状态上限仍按窗口宽算，状态涨到 500 列宽不动。
  `AppLayout.maxOf` 在 ≤1024 时同样封顶（200 / 340，与 CSS 同一组常量）。
- 两条 Minor 按用户影响升级并修复：发件人按钮可访问名丢当前值（相对原生 select 的 a11y 退化）；
  手机 FAB 盖住列表最后一行与「没有更多」（列表底部留白 88px）。
- `DropMenu` 增加 `disabled` 透传到 radix Trigger（发送中不能改发件人）；补 `ComposeFab` 组件测试与
  「选菜单项能换发件人」测试。

**延后的 Minor（未改）：** 阅读头短日期不带精确时间（只在 title）；列表行 `relTime` 与阅读区
`formatShortDate` 两套日期规则；翻译按钮 aria-label 随状态变又带 aria-pressed（既有）；title 与
aria-label 重复播报快捷键。

最终：`pnpm test` 978 项中 977 通过（仅既有 1 项失败）；lint 基线 20 不变；`pnpm build` 绿；
镜像重建后浏览器核验 Esc / 移动菜单 / 手机底部三项均符合预期。

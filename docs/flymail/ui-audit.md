# FlyMail 前端 UI 审查清单

审查时间：2026-09-11
范围：`flymail/frontend/src` 全量，两路独立审查（设计/交互维度 + 可访问性与状态覆盖维度）。

共发现 41 条。本轮已处理 16 条，其余 25 条记录在此，按优先级与改动成本排期。

行号以 `d882840`（前端操作流程优化）之后、`ui-fixes` 之前的代码为准；本轮改过的文件行号已经变了，
正文里会标注"已处理"。

---

## 一、本轮已处理（16 条）

### 撤销机制的五条缺陷

这五条都是 `d882840`（处理后自动前进 + 撤销）自己引入的。把确认框换成撤销条，
等于把安全性从"堵在路上"换成"5 秒内用户得看见、听见、够得着"——这三个前提当时一个都没验证。

| 问题 | 后果 | 修法 |
|---|---|---|
| `flush()` 不收起 toast | 删完切文件夹，撤销按钮变哑巴：点了没反应，用户以为撤销成功，邮件已永久删除 | `useToast()` 暴露 `dismiss`，强制落地时同步收起 |
| 撤销哑火是静默的 | 同上，且将来任何绕过 dismiss 的新路径都会复发 | `undo()` 返回布尔，失败时给出「撤销窗口已过」 |
| 关标签页/关窗丢操作 | React 卸载清理不执行，删除请求永不发出；UI 显示删了，重开又回来 | `beforeunload` 兜底落地 |
| toast 动作按钮对比度 1.4–2.6:1 | 反色容器里用了为正常底色设计的 `--accent`，暗色下几乎看不见 | 改为与正文同色 + 常驻下划线 |
| 撤销对键盘/读屏不可用 | 按钮在屏幕底部要 Tab 穿过整个虚拟列表；live region 与内容同时插入 DOM，读屏不播报 | 绑 `Ctrl/⌘+Z`；播报区改为常驻 |

### 其余 P0

- **触摸端隐形删除按钮** — `.mi-star`/`.mi-del` 用 `opacity:0` 而非 `display:none`，
  元素照常接收点击，而全项目 `@media (hover:hover)` 守卫数为 0。手机上列表行右侧
  14–54px 躺着两个看不见的按钮，点下去就是删除。`.account-row-actions` 同理，
  还导致移动端完全没有手动同步入口。→ 加指针类型守卫，粗指针下常驻显示。
- **列表请求失败被渲染成空收件箱** — 只读 `isLoading` 不读 `isError`，
  后端 500 / 断网 / 令牌失效全都落进空态分支，显示"这个文件夹里还没有邮件"。
  → `MailList` 加 `error`/`onRetry`，错误分支排在空态之前。
- **三个自制浮层无焦点管理** — 设置/通知/速查表都是裸 `div` + backdrop，
  Tab 会走到遮罩背后去操作看不见的元素，关闭后焦点落回 `body`。
  → 新增 `hooks/useFocusTrap.ts`，补 `role="dialog"` 与 `aria-modal`。
- **Esc 被两处同时消费** — 浮层各自的 `document` 监听与快捷键的 `window` 监听都触发，
  关浮层连带关掉当前邮件。→ Esc 层级收口到 `useKeyboardShortcuts`，`overlayOpen` 时让位。
- **浮层打开时单键快捷键未屏蔽** — 屏蔽判据只有"焦点在输入框"，而浮层里焦点通常落在按钮上。
  设置面板开着时按 `#` 会删掉背后的邮件，撤销条又在用户视线之外。
  → 新增 `overlayOpen` 选项，接入 `settingsOpen || notifOpen || dialogOpen || drawerOpen`。
- **撰写器丢弃零保护** — 9 处 `window.confirm` 全用在可恢复的小操作上，
  而"关掉写了一半的邮件"这个唯一真正会丢工作的操作零确认。
  → 三选一守卫（保存草稿 / 丢弃 / 继续写），Esc 经 `COMPOSE_CLOSE_EVENT` 走同一条路。

### P1

- **换搜索关键词不清空选择** — `sourceKey` 里搜索态固定写作 `'search'`（为保住搜索框焦点），
  于是换词时选择留着：工具栏显示"已选 5 封"而列表无一高亮，批量删除会删掉看不见的邮件。
  → 拆出 `selectionKey`（多含 `debouncedQuery`），滚动重置与选择清空分开。
- **聚合视图看不出邮件属于哪个账户** — 所有头像底色写死同一个 `var(--accent)`；
  `.acct-pip`（头像角上的账户色点）样式早已写好但全项目 0 处引用，是死代码。
  → 新增 `lib/account-color.ts`，聚合/搜索视图启用色点。
- **焦点可见性只覆盖 3 个选择器** — 工具栏、侧栏、设置的 15 个分区导航全都没有焦点环。
  → 用 `:where()` 加零特异性全局兜底，个别控件仍可覆盖。
- **`--danger-wash` / `--danger-ink` 从未定义** — 靠 `var(--x, 回退值)` 苟活，
  暗色主题下是一块亮粉底。整套令牌缺语义色这一层。
  → 补 danger/success/warning 明暗两套；同时补 `color-scheme`（原生控件跟随明暗）、
  `prefers-reduced-motion` 降级、`index.html` 内联主题引导（消除暗色刷新白闪）、
  未存偏好时跟随 `prefers-color-scheme`。
- **无账户时没有引导** — 新用户首次登录看到标题为空、内容为"暂无邮件"的界面，
  唯一入口是侧栏一个 `+` 图标。→ 空态按上下文分派，给「添加账户」按钮；
  筛选筛空时一并给「清除筛选」出口。

### 计划外发现

- **`t('common.undo')` 是悬空键** — `common` 命名空间根本不存在，i18next 取不到键就原样返回键名，
  **撤销按钮上一直显示的是字面量 `common.undo`**。两路审查都没抓到，因为它们读代码而不是跑界面；
  类型检查、lint、466 项测试也全都放行——`t()` 的签名是 `(key: string) => string`，
  任何字符串都合法，取不到键还"成功"返回了一个字符串，失败是静默的。
- **固化为测试** — 新增 `src/locales/keys.test.ts`，扫描全部源码里的 `t('...')` 字面量
  与键位目录的 `descKey/titleKey/nameKey/labelKey`，比对语言文件。
  已做正向验证：放回旧 bug 立刻失败并指名文件与键。
  注意这与既有的"zh/en 键对齐"是两件事——前者防两份语言文件漂移，后者防引用悬空。

---

## 二、待处理（25 条）

### P1 — 建议下一轮

**1. 翻页失败后列表静默卡死**
`MailList` 的 `shouldLoadMore` 守卫先写 `loadedLenRef.current = itemCount` 再发请求，
第 2 页失败后 `itemCount` 没变而 ref 已推进，守卫从此恒为 false。
用户继续下滚不再触发任何请求，底部既不显示"加载中"也不显示"没有更多"，
列表看起来就是在第 50 封处戛然而止。
→ 读 `isFetchNextPageError`，底部渲染「加载失败 · 重试」，点击时回退 ref 再 `fetchNextPage()`。

**2. 后台刷新完全不可见**
三种 loading 只区分了两种：首次加载（`isLoading`）与翻页（`isFetchingNextPage`），
缺 `isFetching && !isLoading`。而后台刷新在这个应用里非常频繁——
`useFolders` 带 30 秒轮询、SSE 推送会 invalidate、同步完成后一次性 invalidate 五个 key。
用户会在毫无预期的时刻看到列表整体换内容。
搜索防抖的 300ms 同理：链路带 `keepPreviousData`，换关键词时列表停在上一个关键词的结果上，
用户可能对着旧结果做操作。
→ 标题栏副标题旁加细进度指示。

**3. 新邮件到达无播报**
`useRealtimeSync` 收到 SSE 后只 invalidate 缓存。视觉用户能看到未读徽标跳变，
读屏用户完全无感知。→ 复用本轮已修好的常驻 live region。

**4. 列表缺 ARIA 语义**
每行是孤立的 `role="button"`，外层容器没有 `role="list"`/`"listbox"`。
读屏读不出"第 12 项，共 340 项"，也读不出列表边界。
批量选中态只有 CSS class 没有 `aria-selected`；行内复选框的 `aria-label` 写死英文 `"select"`，
读出来是"select 复选框"，不说明选的是哪封邮件（同时违反 i18n 规则）。
附带：虚拟化 + 每行 `tabIndex={0}`，Tab 序列只含视口内已渲染的 5~20 行且随滚动变化。

**5. aria-label 缺失 / 硬编码**
`AppLayout` 三个分栏手柄的 `aria-label` 是**硬编码中文**，英文界面下读屏念中文；
`MailList` 星标按钮写死 `'Star'`/`'Unstar'`；
`ComposeDialog` 多个纯图标按钮只有 `title`（部分读屏配置下不作为可访问名），
丢弃按钮连 `title` 都没有；From/To/Cc/Bcc/主题行的 `<label>` 没有 `htmlFor`，
`<select>`/`<input>` 没有 `id`，两者未关联。
→ 样板是 `composer/EditorToolbar.tsx` 的 `ToolButton`（`title` + `aria-label` + `aria-pressed`）。

**6. 分栏手柄是个假承诺**
`AppLayout` 与 `MailList` 的四个手柄都有 `role="separator" aria-orientation="vertical"`，
却只挂了 `onPointerDown`：不可聚焦、无 `aria-valuenow`、无方向键处理。
向读屏宣告了"这是可调节的分隔符"却无法操作，比不加更糟。
侧栏/列表/发件人列宽在设置里有滑块替代，但**双栏模式浮动面板宽度只有拖拽一条路**。

**7. 长操作没有进度表达**
首次同步几千封是分钟级操作，全部反馈是账户行里一个 11px 的圆点在转，
且条件写成 `syncing && active`——同步非当前账户时屏幕上零变化。
`syncStatus.phase` 已经取到了但没有任何一处用在 UI 上。
SSE 连接状态也不外露（`useRealtimeSync(): void`），合盖唤醒 / 后端重启后 UI 静默停止收信。
发送与 10MB 附件上传同样零进度。

**8. 侧栏与草稿列表仍无 error 态**
本轮只修了 `MailList`。`useAccounts` / `useFolders` / `useAggregateCounts` / `useDrafts`
都只解构 `data = []`，请求失败 = 一个账户都没有的界面。全项目也没有 ErrorBoundary。

**9. 设置页 8 处 window.confirm 仍在**
删账户 / 别名 / 黑名单 / 通知渠道 / 规则 / 信任发件人，以及会话内删单封。
原生 confirm 在 Wails 桌面壳里外观完全脱离应用，且不跟随主题。
→ 统一改为 toast + 撤销，或至少换成走 `.ctx-menu` 令牌的自绘弹层。

**10. 批量「移动到」菜单与全局菜单不同源**
`MailList` 里是完全内联样式的手搓下拉：硬编码 `boxShadow`（不是 `var(--shadow-md)`）、
靠 `onMouseEnter/Leave` 手改 `style.background` 模拟 hover。
而 `.ctx-menu`/`.ctx-item` 是权威样式，`DropMenu` 组件和 `CtxMenuItem` 模型都是现成的。
同一应用里两种菜单外观。→ 替换成 `<DropMenu>` 净删约 35 行。

**11. 登录页是另一套视觉语言**
`Login.tsx` 用 lucide-react 图标 + shadcn Card/Button/Input + Tailwind 语义类；
应用内部用自绘 16px stroke 图标集 + MailMaster 令牌 + 手写像素间距。
连圆角尺度都不同（shadcn 的 `--radius` 派生 vs 裸像素）。
用户看到的第一屏不长得像这个应用。顺带：为 3 个图标引入了整个 lucide-react。

**12. 触摸目标偏小 + 窄屏断点只做了一半**
`.icon-btn` 28×28、`.lt-btn` 30×30、`.rt-btn` 26×26、`.chip-clear` 22×22、
账户同步按钮 22×22（图标 11px）、搜索清除按钮 20×20。
而 `max-width:768px` 块只改了间距与字号，没有任何一处放大触摸目标。
三栏退化到单栏本身做得不错（抽屉 + `data-mobile-pane`），缺的是退化之后的"手指尺度"。
附带：汉堡菜单用的是三点图标（隐喻错误），返回键用 `chevron-right` 加 `scaleX(-1)` 镜像
——都是图标集缺项的代偿。

**13. 撰写器校验错误显示在会滚走的位置**
`validationError` 渲染在 `.compose-body`（`overflow-y:auto`）最底部、编辑器之后，
而发送按钮在固定的 `.compose-foot`。写完长正文点发送，错误出现在滚动区看不见的地方，
按钮表现为"没反应"。→ 移到 `.compose-foot` 内或 `.compose-head` 下方吸顶。

**14. 会话手风琴折叠行没有展开指示符**
`.ti-head` 只有 `cursor:pointer` 和 hover 底色，没有 caret/chevron，
而侧栏账户行有（`.account-row .caret`，展开时 rotate 90°）。同一应用里两种折叠容器。
另外折叠/展开是瞬时的，一条 10 封的会话点开时整个滚动区会跳。

### P2 — 锦上添花

**15.** 超长附件名撑破附件卡（`.attach-card .ac-name` 无截断，而撰写器侧的 `.attach-chip .ac-name` 有，两处不一致）；且无 `title`，截断后看不到全名。

**16.** 超长无空格主题横向溢出（`.reader-subject` 缺 `overflow-wrap: anywhere`）。列表行侧有 ellipsis，没问题。

**17.** "无主题"两处文案不一致：列表行用字面量 `'—'`，阅读区用 `t('list.noSubject')`。同一封邮件点开前后显示不同。

**18.** 大量附件无折叠：`MessageBody` 无条件全量渲染，50 个附件就堆 50 张卡片。

**19.** 通知筛选 tab 缺 `role="tablist"/"tab"/aria-selected`，读屏读成六个孤立按钮，也无方向键切换。

**20.** `.mail-item` 焦点环用 `outline-offset: -2px`，与 `selected`/`batch-selected` 背景视觉混淆，深色主题对比不足。

**21.** 附件下载是 `void` 调用，异常被吞——下载失败时界面毫无反应；且接收侧无大小门槛，200MB 附件无二次确认无进度。

**22.** `NotificationsPage` 的 `dayLabel` 缺 NaN 守卫（同文件的 `fmtTime` 与 `MailList` 的 `relTime` 都有），非法 `created_at` 静默落入"更早"分组。

**23.** 约 150 行死 CSS：`.quick-theme` 整块（73 行，被移除的快速主题气泡）、`.settings-panel`/`.settings-section`/`.theme-swatches`、`.compose-textarea`（已换 Tiptap）、`.nf-pip`、`.label-dot`、`.kbd-pill`。在一个 2224 行的单文件 CSS 里，这些残留会让后来者分不清哪套是现行语言。

**24.** 主题色板有三份副本：`index.css` 的权威定义、`SettingsDialog` 预览卡里的 54 个 hex、`lib/theme.ts` 的 `TONES.swatch`。改一次主题要改三处，必然漂移。→ 预览卡改为挂 `data-theme`/`data-mode` 让它自己继承令牌。

**25.** 撰写器细节：最小化条的展开箭头是手画的内联 `<svg>`（图标集里有 `chevron-up`，这是全项目唯一一处绕开图标组件的地方）；**展开按钮的 `title` 写成了 `t('compose.minimize')`，文案是反的**；From 下拉带 14 行内联样式而 `.settings-field > select` 是现成的。

**26.** 列表底部恒挂 12px 空白条（`hasNextPage && !isFetchingNextPage` 时渲染 `null` 但外层 padding 照常生效），滚到底时像"还有一行没加载出来"；翻页加载态是纯文字而首屏有完整骨架，同一列表两种表达强度。

**27.** 应用外壳收尾：favicon 仍是 Vite 脚手架默认的 `/vite.svg`；`<title>` 是静态的，未读数没反映到标签页标题（桌面端已有托盘角标，浏览器端缺这一层）。

**28.** 正文 iframe 强制白底（刻意为之，邮件 HTML 假定白底，合理），但暗色下阅读等于盯一块白板。设置页已有隐私分区，可加一个「暗化正文」开关。

**29.** 阅读区日期显示两遍：meta 行与 `.th-time` 相距约 40px 都是 `formatDate(detail.date)`。那条 meta 行目前只承载这一条重复信息，白占一整行视觉预算。→ 换成所属账户/文件夹（会话视图的 `.ti-folder` 已经这么做了）。

---

## 三、两条贯穿性的根因

审查是两路独立进行的，却得出了同一个判断：**项目不缺设计体系，缺的是贯彻**。
18 组色调令牌 + shadcn 桥接是完整的，`AccountDialog` 用 radix 做对了对话框，
`EditorToolbar` 的 `ToolButton` 有完整 aria，搜索空态给了明确出口——
每一类问题都能在仓库里找到一个做对了的样板，新代码只是没照着走。

**根因一：把两种语义绑在同一个标志上。**
`sourceKey` 同时表达"滚动该重置"和"选择该清空"，为保搜索框焦点牺牲了后者；
快捷键屏蔽用"焦点在输入框"代表"被浮层遮挡"；
撤销窗口的存在与否同时记在 `pending` 和 toast 上。
三处的修法都是拆开，而不是给原标志打补丁。

**根因二：照着"看起来像"写，而不是照着"语义上是"写。**
比照样板手写 div 更快，而且当场看不出区别。
`role="separator"` 那条最典型：加了 ARIA 角色却没实现该角色承诺的交互，
读屏据此告诉用户"这里可以调节"，用户却操作不了——比不加更糟。

---

## 四、建议的推进顺序

1. **P1 第 1、2、8 条**（翻页失败、后台刷新不可见、侧栏/草稿 error 态）
   ——都是"数据状态没有如实反映到界面"，改动面集中在几个 query 消费点。
2. **P1 第 4、5、6 条**（ARIA 语义、aria-label、分栏手柄）
   ——本轮补完了焦点这一层，这三条补完语义那一层，a11y 才算成体系。
3. **P2 第 23、24 条**打包做（死 CSS + 色板三合一）
   ——加起来能删约 200 行并消除主题漂移的结构性风险，只碰两个文件、不动交互逻辑，回归成本最低。
4. **P1 第 11、12 条**（登录页视觉统一、触摸尺度）
   ——面向"第一印象"与移动端，可作为独立的一轮。

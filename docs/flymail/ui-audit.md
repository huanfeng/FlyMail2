# FlyMail 前端 UI 审查清单

审查时间：2026-09-11
范围：`flymail/frontend/src` 全量，两路独立审查（设计/交互维度 + 可访问性与状态覆盖维度）。

共发现 41 条。第一轮处理 16 条，第二轮 3 条，第三轮 3 条，做浏览器通知时顺带修掉第 3、27 条，
第四轮 4 条（第 11、12、23、24 条），其余 13 条记录在此，按优先级与改动成本排期。

行号以 `d882840`（前端操作流程优化）之后、`ui-fixes` 之前的代码为准；改过的文件行号已经变了，
正文里会标注"已处理"。**编号一律不重排**：已处理的条目保留原编号并就地标注，
免得后续讨论里"第 8 条"指向两个不同的东西。

---

## 一、第一轮已处理（16 条）

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

## 二、第二轮已处理（3 条）——数据状态如实反映到界面

第一轮把 `MailList` 的首屏错误态补上了，但"数据状态没有如实反映到界面"这个根因
还剩三个出口没堵：翻页失败、后台刷新、侧栏与草稿的加载失败。三条都属于
**故障与正常状态在界面上同形**，共用同一种修法：把 query 已经知道的状态往下传，
并让错误分支排在空态之前。

### 1. 翻页失败后列表静默卡死（原第 1 条）

守卫 `shouldLoadMore` 先写 `loadedLenRef.current = itemCount` 再发请求，
第 2 页失败后 `itemCount` 没变而 ref 已推进，守卫从此恒为 false——
用户继续下滚不再触发任何请求，底部既不显示"加载中"也不显示"没有更多"。

修法与原方案（"点击时回退 ref"）不同：**把 `nextPageError` 作为守卫的一条独立输入**，
失败期间一律不自动翻页。理由是回退 ref 之后错误仍在，而自动翻页的判据
"最末可见行接近底部"不受失败影响，下一次滚动就会再次发请求——那是按帧重发。
有了这条守卫，重试就不必动 ref：重试成功时 `itemCount` 增长，
`lastLoadedCount !== messageCount` 自然放行后续翻页。

- `lib/list-guards.ts`：`LoadMoreInput` 加 `nextPageError`，守卫早退
- `MailList`：底部状态条抽成 `.list-foot`，失败分支**排在"没有更多"之前**
  （两者都表现为列表不再增长，混在一起就是把故障说成数据的尽头）
- `Shell`：`isFetchNextPageError` 按会话/单封两套链路取，重试复用 `loadMore`

#### 这一条差点做成了负数

第一版改完，代码审查指出：`nextPageError` 为真时 `error` 必然也非真空，
于是首屏错误态会接管整个屏幕——**屏幕上那 50 封邮件被一整页"加载失败"换掉**，
底部新写的「加载失败 · 重试」出现在一张空列表的下面。比不修更糟。

用 `InfiniteQueryObserver` 实测确认（`lib/query-semantics.test.ts` 固化了这几条）：

```
翻页失败后: status='error'  isError=true  error=Error  data.pages=1(还在)
            isFetchNextPageError=true  isLoadingError=false  isLoading=false
```

`status` 是**整个 query 的**，翻页失败、后台重取失败都会把它置为 `'error'` 并填上
`error`，而已加载的页原样留在缓存里。判据必须是 `isLoadingError`（= `isError` 且没有数据）。
同一个错误因此影响了四处，一并改掉：

| 位置 | 原判据 | 现判据 | 不改会怎样 |
|---|---|---|---|
| `Shell` 的 `messagesError` | `source.error` | `isLoadingError ? error : null` | 第 2 页失败 → 整屏错误面板吃掉已加载的邮件 |
| `DraftsList` | `isError` | `isLoadingError` | 发完信重取失败 → 好端端的草稿列表被换成错误面板 |
| `Shell` → `MailList` 的账户错误 | `accountsQuery.error` | 同上 | 账户增删后重取失败 → 邮件列表被整页错误替掉 |
| `AccountSidebar` 的两处 `SideError` | 裸 `error` | 同上 | `useFolders` 30 秒轮询抖一下 → 完整列表上方挂一行红色"加载失败" |

`MailList` 那边还加了一道组件级不变量：整屏错误态叠加 `itemCount === 0`，
即便将来有人又传了裸 `error` 下来，也不会掀掉已有内容。一道表达语义、一道兜住下次。

**为什么第一版的测试是绿的**：`MailList.test.tsx` 构造了 `{ nextPageError: true }`
而 `error` 留空——这个组合在真实链路里**根本不可达**。测试是照着实现写的，
不是照着真实状态组合写的，于是完美复现了实现的错误假设。
这和上一轮 `t('common.undo')` 悬空键是同一类失败：**绿灯来自没有被检验的前提**。
补法是两条一起上——`query-semantics.test.ts` 把第三方状态位的真实语义钉死，
`MailList.test.tsx` 补一条 `{ nextPageError: true, error: Error }` 断言列表容器仍在。

### 2. 后台刷新完全不可见（原第 2 条）

三种 loading 只区分了两种，缺 `isFetching && !isLoading`——
而这一种恰恰最频繁：`useFolders` 30 秒轮询、SSE invalidate、同步完成后批量刷新、
搜索防抖期间靠 `keepPreviousData` 停在上一个关键词的结果上。

顺带纠正清单原文的一处事实错误：第 2 条写"搜索防抖的 300ms 同理，链路带 `keepPreviousData`"。
核对 `lib/queries.ts` 后确认，无限查询链路（`useInfiniteMessages` / `useInfiniteSearch` /
会话三条）**既没有 `refetchInterval` 也没有 `placeholderData`**；`keepPreviousData` 只在
`useThreadMessages` 与 `useMessageDetail` 上，`refetchInterval: 30_000` 只在
`useFolders` / `useAggregateCounts` / `useAccountUnread` / `useNotificationUnread` 上。
换搜索词 = 新 queryKey = 全新查询 → `isLoading` 为真 → 走骨架屏，`refreshing` 恰恰为假。
真正让这条 bar 亮起来的是各处 `invalidate(['messages'])`：同步完成后的批量刷新、SSE 推送、
以及删除/移动/标记已读等 mutation 的收尾。注释与文档都按核对结果改了。

→ `.list-refresh-bar`：压在标题栏下沿的 2px 不确定进度条。
**绝对定位不占布局**是刻意的——这个应用里刷新太频繁，任何占位的指示都会让标题栏反复抖动。
`prefers-reduced-motion` 下单独处理：全局那条规则会把无限动画压成一帧、滑块停在随机位置，
所以改为整条常亮。

### 8. 侧栏与草稿列表仍无 error 态（原第 8 条）

三处请求失败时 `data` 都回落成空数组，与"一个账户都没有""这个账户没有文件夹"
"一封草稿都没有"完全同形。

- `AccountSidebar`：新增 `SideError` 行（danger 语义色 + 重试），账户列表与
  当前账户的文件夹列表各一路；`foldersError` 只传给激活账户——其余账户压根没发这个请求
- `DraftsList`：错误分支排在空态之前，复用 `.list-error` 版式
- `ErrorBoundary`：新增并挂在 `main.tsx` 最外层（连 provider 自身的渲染异常也兜住）。
  开关用独立的 `hasError` 布尔而不是 `error != null`——`throw null` / `throw undefined`
  也是合法抛出，拿 error 当判据时那种情况回退界面不出现而子树继续抛。
  没有它时任何组件抛异常都会让 React 卸载整棵树，而"白屏"与"网络断了""服务挂了"
  在用户眼里无法区分。只兜渲染期异常——事件处理器与 async 里的异常 React 不会传过来，
  那些路径各自用 toast / 错误态表达
- `errorText()` 从 `MailList` 提到 `lib/format.ts`，列表、草稿、崩溃界面三处共用

### 复审又抓出的第五处同形

`noAccounts` 第一次修成 `accountsQuery.isSuccess && accounts.length === 0`，仍然是错的：
后台重取失败时 `status` 变 `'error'`（`isSuccess` 转假）而缓存里的 `[]` 还在
（`isLoadingError` 也就为假），于是 `noAccounts` 与 `error` **两头落空**，
主栏落进通用空态「暂无邮件」——既没有错误也没有添加入口。
触发路径恰恰在新手身上：0 账户的用户点「添加账户」成功 →
`useCreateAccount` 的 onSuccess `invalidateQueries(['accounts'])` → 这次重取失败。

→ 判据要与 status 无关，只问「收到过一份列表吗」：`accountsQuery.data != null && accounts.length === 0`。
这与 `isLoadingError` 内部的 `hasData` 是同一个概念，两处口径统一。

**同一个判据错了两次**（先是 `accounts.length === 0`，再是 `isSuccess && ...`），
两次都是把"有没有数据"和"最近一次请求成不成功"混为一谈——
react-query 的 `status` 描述的是后者，而界面要问的几乎总是前者。

### 收尾时发现的第四处同形

`Shell` 把 `noAccounts={accounts.length === 0}` 传给 `MailList` 驱动新手引导。
账户请求失败时 `accounts` 同样是空数组，而此时消息链路多半处于禁用状态
（没有 `folderId`，既不 loading 也没有 error、`itemCount` 为 0）——
于是一次 500 在中间栏显示成「还没有邮箱账户 · 添加一个账户就可以开始收信了」。
侧栏刚修好的错误行与它同屏，两边说的话还互相矛盾。

→ `noAccounts` 改为 `accountsQuery.isSuccess && accounts.length === 0`（确实取到空列表才算），
并把 `accountsQuery.error` 接进 `MailList` 的 `error`（账户取不到时邮件列表本来就无从谈起）。
这条是**修完前三条之后重看 diff 才发现的**：同一个根因在第四个地方以同样的形状存在，
而它不在两路审查的 41 条里——说明"data 回落成空数组"这个模式值得将来单独扫一遍。

**未纳入**：`useAggregateCounts` / `useAccountUnread` / `useNotificationUnread` 失败时
徽标不显示（渲染条件是 `> 0`），不会把故障说成别的东西，且它们都带 30 秒轮询会自愈，
再加一条错误行只是噪音。

### 本轮的测试

- `list-guards.test.ts`：翻页失败不自动重发 / 重试成功后恢复自动翻页
- `MailList.test.tsx`（新建）：失败态给出重试入口且压过"没有更多"、
  真到底才说"没有更多"、刷新条随 `refreshing` 出现与消失、首屏失败不渲染空态
- `DraftsList.test.tsx`（新建）：失败不落进空态、重试后正常显示、真空仍走空态
- `ErrorBoundary.test.tsx`（新建）：抛错给回退界面而非白屏、重试可恢复、正常时不介入
- `query-semantics.test.ts`（新建）：把 react-query 的错误状态位语义钉死——
  翻页失败会把整个 query 置为 error 且数据仍在、`isLoadingError` 能分开两种失败
- 六组都做了**正向验证**：把修复逐条回退，对应用例立刻失败（这是上一轮
  `t('common.undo')` 悬空键的教训——读代码的审查看不出静默失败）
- 独立代码审查跑在实现之后、提交之前，抓出了上面那条 P0。**这一轮的最大教训不是
  某个 API 用错了，而是"我写的测试验证了我自己的假设"**：测试和实现出自同一个
  理解，理解错了两边一起错，绿灯毫无信息量。凡是依赖第三方状态语义的判据，
  要么实测钉死，要么让另一双眼睛看。

### 复审带出的三处打磨

- **进度条延迟 200ms 才出现**：同步收尾一次 invalidate 多个 key，本地接口几十毫秒返回，
  不加阈值这条 2px 的线只会反复亮灭——比不显示更烦人。
- **翻页失败补播报**：`.list-foot-retry` 是失败后唯一的出口，而读屏用户滚到底看不到它。
  加了常驻的 `sr-only` live region（常驻是上一轮的教训：与内容同时插入 DOM 时读屏不播报）。
- **侧栏错误行等重取落地再报**：gcTime 内切回一个上次取数失败过的账户，
  refetch-on-mount 还没落地就会先闪一下红行，自愈后又消失 → 判据加 `&& !isFetching`。

顺带按复审意见删掉两条**没有区分度**的测试：`ErrorBoundary` 的「子树正常时完全不介入」
（去掉被测组件照样绿）与 `list-guards` 的「重试成功后恢复自动翻页」
（`shouldLoadMore` 是无状态纯函数，这条与既有用例等价）。绿灯不等于证据。

**需人工确认**：进度条在真实浏览器里的观感（2px 是否过细、200ms 阈值是否合适），
以及暗色主题下 `.side-error` 与 `.list-foot-retry` 的对比度。

---

## 三、第三轮已处理（3 条）——a11y 的语义层

第一轮补完了焦点这一层（焦点环、焦点陷阱、Esc 层级），这一轮补语义层：
**界面对读屏说的话，要和它实际能做的事对得上**。三条各自是这句话的一个侧面——
说了「这是列表」却说不出第几项、说了「这是可调节的分隔符」却调不动、
说的是硬编码的另一种语言。

### 6. 分栏手柄是个假承诺（原第 6 条）

四个手柄（侧栏 / 列表 / 浮动面板 / 发件人列）都是 `role="separator"` 加一个
`onPointerDown`：向读屏宣告了可调节，却不可聚焦、没有 `aria-valuenow`、不接受方向键。
**比不加这个角色更糟**——读屏据此告诉用户这里能调，用户却调不动。
双栏模式下浮动面板的宽度更是只有拖拽一条路（另外三个在设置里还有滑块）。

→ 新增 `components/ui/ResizeHandle.tsx`，四处共用：`tabIndex=0`、
`aria-valuenow/min/max`、`aria-valuetext`（读屏默认把 valuenow 念成百分比，
对宽度没有意义）、方向键按 16px 调整、Shift 走 64px、Home / End 到端点。

值的语义留在调用方：组件只报告**水平位移增量**，由调用方决定它如何映射到宽度——
浮动面板右锚定，向左拖才是变宽，符号与另外三个相反。这样三栏形态下
「按窗口宽度动态收紧上限」那条规则不必挤进组件里。顺带删掉了 AppLayout 里
两套几乎相同的 pointer 样板。

**落盘时机跟着换了一次**：原来是「松手时读 `wRef.current`」，而键盘没有「松手」
这个时刻——按一次键就落盘的话，ref 还没跟上，写进去的是上一个值。
改成对宽度状态挂防抖 effect，拖拽与键盘两条路共用同一个时机，
那个在 render 期赋值的 `wRef` 也一并删掉（它正是 `react-hooks/refs` 拦的东西）。

⚠ 这个改法带进来一个新的自激风险：落盘 effect 依赖 `[w]`，而 `saveLayoutWidths`
会把值广播回 `LAYOUT_EVENT` 监听器，`detail` 每次都是新对象——直接 `setW(detail)`
就是「落盘 → 广播 → 新引用 → 再落盘」转个不停。监听器改为按值比较后返回 `prev`
（React 对同一引用不重渲染）。`AppLayout.test.tsx` 专门钉住了这一条。

### 4. 列表缺 ARIA 语义（原第 4 条）

每行是孤立的 `role="button"`，外层容器没有列表角色；读屏读不出「第 12 项，共 340 项」，
也读不出列表边界。虚拟化让问题更重一层：DOM 里只有视口内那十几行，
**读屏据 DOM 推断出的计数必然是错的**。

- `.mail-rows` 容器加 `role="list"` + 名称；每个条目行的定位层是 `listitem`，
  带显式的 `aria-posinset` / `aria-setsize`——这正是这两个属性存在的理由
- 行模型 `RowItem` 带上 `pos`（跳过分组标题的条目序号）
- 分组标题行让位给 `role="presentation"` + 内部 `role="heading"`，
  读屏可以把「今天 / 更早」当标题跳转（严格说 list 的直接子元素都该是 listitem，
  这里用可导航性换了一点规范洁癖）
- **roving tabindex**：原先每行 `tabIndex=0`，Tab 序列只含视口内那十几行、
  还随滚动变化——键盘用户穿过列表要按几十次，穿过什么取决于他滚到了哪。
  改为整份列表只占一个停留点（落在当前打开的那封，没有则第一条），
  进去之后方向键走，Home / End 到首尾，行为与既有的 j / k 一致
- 焦点跟随停留点，但**只在焦点本来就在列表里时才动**——否则 j / k 会把焦点
  从搜索框抢过来；等一帧让虚拟化把目标行渲染出来再聚焦
- 行内复选框的 `aria-label` 原是写死的英文 `"select"`，改为点出具体哪一封

批量选中态没有另加 `aria-selected`：`aria-selected` 在 `role="button"` 上无效，
而选中与否本来就由行内复选框的 `checked` 表达，只要它说得出选的是哪一封。

### 5. aria-label 缺失 / 硬编码（原第 5 条）

- `AppLayout` 三个手柄的 `aria-label` 是**硬编码中文**（英文界面下读屏念中文）→ 走 i18n
- `MailList` 星标按钮写死 `'Star'` / `'Unstar'` → 复用已有的 `ctx.star` / `ctx.unstar`
- `ComposeDialog` 的纯图标按钮只有 `title`（部分读屏配置下不作为可访问名），
  **丢弃按钮连 `title` 都没有** → 四个图标按钮 + 附件移除 + 附件 + 丢弃全部补齐
- From/To/Cc/Bcc/主题的 `<label>` 没有 `htmlFor`、控件没有 `id` → 用 `useId` 生成前缀关联
  （From 行在单账户时渲染的是只读 `span`，那种情况不给 `htmlFor`——
  指向不存在的 id 等于没关联）

顺手修掉清单第 25 条里的两项：最小化条的展开按钮 `title` 写的是
`compose.minimize`（方向正好相反），以及那处手画的内联 `<svg>`（图标集里本就有 `chevron-up`）。

### 本轮的测试

- `ResizeHandle.test.tsx`（新建）：可聚焦 + 值语义、方向键步长与 Shift 加速、
  Home / End、无关按键不拦截
- `AppLayout.test.tsx`（新建）：同值广播不再落盘（自激循环）、变更走防抖
- `MailList.test.tsx` 新增一组：虚拟化下每行仍说得出「第几项，共几项」、
  整份列表只占一个 Tab 停留点、停留点跟着当前打开的那封走、
  方向键与 Home / End、行内按钮的键盘语义不被抢
- 全部做了正向验证（逐条回退修复，对应用例立刻失败）

**这一轮的测试写起来先撞了一堵墙**：`@tanstack/virtual` 量滚动容器用的是
`offsetWidth / offsetHeight` 而不是 `getBoundingClientRect`，jsdom 里前者恒为 0，
于是虚拟列表一行都不渲染——所有关于行的断言都会落空成「没有行所以没问题」。
先 stub 了 `getBoundingClientRect`，测试照样绿，但绿得没有意义；
读了 virtual-core 的 `getRect` 才找对地方。**又一次印证：绿灯本身不是证据。**

### 复审抓出的六条

这一轮的独立审查收获比前两轮都大，其中第一条是真错。

**1（P1）两套编号体系被当成一套。** `pos` 数的是 `groupByDate` 的**输出**顺序，
而 `rovingPos` 与 `moveRoving` 回 `filtered` / `threads` 数的是**输入**顺序。
两者相等的前提是「分组是输入的保序展平」——这个前提会破：`date-group.ts` 把日期
解析不出来的条目归进 `earlier` 组，而该组不在 `fixedKinds` 里、是在固定分组之后
按 Map 插入顺序发出的；后端返回顺序不是严格日期降序时（跨页游标遇到同一时刻、
聚合视图跨账户归并）同样会破。后果是方向键**打开 A 而焦点落到 B**，
Tab 停留点也落在别的行上。
→ 序号一律从 `itemRows`（rows 里的条目行）数，`moveRoving` 连 payload 都从同一个
数组取，不再回输入数组取第二次。整类消除，而不是补一个特例。

**2（P2）带焦点的行被移除时焦点掉到 body。** 焦点跟随的守卫原本是「提交后读
`document.activeElement` 还在不在列表里」，而删除当前邮件时那个节点在同一次提交里
就被摘掉、焦点已经回到 body——于是补不上焦点，用户按一次删除就被踢回页面顶端，
正是 roving tabindex 要解决的问题的反面。
→ 改为在容器上用 focus / blur 维护一个 ref。`blur` 的 `relatedTarget` 为 `null` 时
**保持原值**：它既可能是焦点真的去了 body，也可能是带焦点的行刚被删掉，
后者必须保住状态。
（写这条的测试时先踩了个坑：删**中间**一封复现不出来——虚拟行的 key 是索引，
React 复用了同一个 DOM 节点，焦点根本没离开过。删最后一封才会真的摘掉节点。
先写的那版测试回退修复后照样绿，等于没测。）

**3（P2 次要）跳跃移动时目标行还没渲染。** 方向键那条路自己 `scrollToIndex`，
但 j / k 与「删除后自动前进跨过多行」不经过 `moveRoving`，目标行超出 overscan 时
查不到 → 焦点原地不动。→ 焦点跟随 effect 里也先滚再聚焦。

**4 方向键拦截判据有盲区。** 原判据实际含义是「焦点正落在当前 roving 行上」，
焦点意外落到非 roving 行时方向键完全失效且毫无反馈。
→ 改为「容器本身或任意一行」，仍不误伤行内按钮（按钮不是行）。
（审查同时确认了 `CtxMenu` 用 `Trigger asChild` 不插包裹层，
`role="listitem"` 仍是 `role="list"` 的直接子元素。）

**5 落盘的读-改-写竞态。** `saveLayoutWidths` 原本接整份对象，而 MailList 是
`{...loadLayoutWidths(), senderCol}`——按 localStorage 快照写整份。
先拖发件人列、在它的 200ms 防抖到期前去拖侧栏：拖拽期间 AppLayout 每帧重置自己的
计时器、永远不落盘，而 MailList 的计时器照常到期，广播里带着**旧的 sidebar** →
侧栏在拖拽中途弹回旧宽度。
→ `saveLayoutWidths` 改收 `Partial<LayoutWidths>` 并在函数内部合并，
两个所有者各写各的那份。审查同时确认除了已加的值比较外没有别的循环路径，
`SettingsDialog` 只写不听、不成环。

**6 卸载丢最后 200ms。** 两个 effect 的 cleanup 只 `clearTimeout`，桌面端（Wails）
关窗没有第二次机会，而「拖完就关」正是常见的收尾动作。
→ 另挂 `pagehide` flush（在 cleanup 里直接写不行：它每次变更都跑，等于废掉防抖）。

**另外两条 P3**：`aria-valuemax` 传的是静态上限，而 Home / End 走的是按窗口宽度
收紧后的 `maxOf`——读屏被告知「最大 420」，按 End 却停在别处，`aria-valuenow`
永远够不到 `aria-valuemax`，已改为传 `maxOf`。两个悬空文案键：
`list.ariaDateGroup` 加了没用（分组标题最后用的是 `role="heading"`）已删，
`layout.resizeHint` 挂成了 `aria-describedby`——手柄可聚焦了，
但用户没有任何途径知道方向键能调它。

补的测试：`layout-prefs.test.ts`（新建，各写各份 / 广播合并后的完整宽度 / 夹紧 /
损坏回落）、`AppLayout.test.tsx` 加上限一致性、`MailList.test.tsx` 加「分组打乱顺序后
停留点与方向键仍对得上」「带焦点的行被移除后焦点跟到新停留点」「焦点不在列表里时不抢」。
同样逐条做了正向验证。

### 复审的第二轮：同一个竞态还剩一条通路

把落盘的读-改-写堵住之后，**CSS 变量 `--sender-col-w` 上还有同形的一条**：
`AppLayout` 的 `[w]` effect 与 `MailList` 的 `[senderCol]` effect 都在写它。
复现与落盘那条一模一样——先拖发件人列（MailList 立刻写新值、防抖挂起，AppLayout 手上
那份还是旧的），在防抖到期前去拖侧栏 → AppLayout 的 effect 每帧触发、把变量写回旧值 →
列宽在侧栏拖拽过程中一直弹回去。
→ AppLayout 只写自己管的三个变量；`--sender-col-w` 保留**挂载时**按存盘值写一次兜底
（否则 MailList 挂载前 CSS 走 `var(--sender-col-w, 150px)`，存了别的宽度的用户会看到
列宽先窄后宽闪一下），之后交给 MailList。

**教训**：「一个值两个所有者」修的时候要把**所有通路**一起数一遍。
我只堵了 localStorage 那条，CSS 变量这条原样留着——同一个根因，同一个形状，
少走一步就等于没修。

### `role="list"` 里夹 `presentation` 的取舍：推翻重做

第一版把分组标题行设成 `role="presentation"`，想用「规范洁癖」换「标题可导航」。
复审指出这笔交易不划算：`presentation` 只移除该元素本身、**不移除子树**，
于是 `heading` 在可访问性树里成了 `list` 的直接子元素，而 ARIA 1.2 规定 `list` 的
required owned element 只能是 `listitem`（或 `group`）——这是确凿违例，
各家读屏对「list 里混进非 listitem」的规整策略不一致，有丢掉整个列表语义的先例。

→ 改成 `listitem > heading`（完全合法）。计数不受影响：条目的「第几项、共几项」
靠显式的 `posinset`/`setsize`，不靠读屏数 DOM；分组标题那个 listitem 不带这两个属性，
因此不占条目编号。测试也跟着改成断言「带 posinset 的那些 listitem」——
更准确地表达了意图。

### `pagehide` 不是可靠的最后一次回调

按 Page Lifecycle 的模型，页面被丢弃/终止前唯一可以指望的是
`visibilitychange` → `hidden`；`pagehide` 与 `beforeunload` 在多种终止路径上都可能不触发
——而那恰恰包括 WebView2 关窗，正是我加它时想覆盖的那个场景。
→ 两个事件都挂。多写一次 localStorage 无害，漏写一次就是用户刚调的宽度白调了。

### 复审第三轮：我那个「折中」自己就是个缺陷

焦点归属原先用 `blur` 判断，`relatedTarget` 为 null 时**保持原值**——当时的理由是
「它既可能是焦点真去了 body，也可能是带焦点的行刚被删掉，后者必须保住」，
并顺手写了一句「前者残留 true 无害」。**那句是错的**：点一下阅读区正文这类不可聚焦的
空白之后，下一次 j / k 或删除前进会把焦点**连同滚动位置**一起拽回列表，
而用户正在那边读信。j/k 与「删除后自动前进」恰恰就是改变 active 的那两件事。

→ 换成 document 上捕获阶段的 `pointerdown`：点空白一定有一次落在列表外的 pointerdown，
行被删除则一次都没有——**两种来源从此不再同形**，不必在 blur 时猜。
Tab 进入列表那条路仍由容器的 `onFocus` 负责，两者互补。

这是个通用教训：**当两种情况在某个信号上同形时，正确做法是换一个能分开它们的信号，
而不是在那个信号上选一边押注。** 押注的那一边总有代价，只是当时没看见。

### `pagehide` + `visibilitychange` 的代价

两个都挂之后，一次页面隐藏会写 4 次盘、广播 4 次；而且 `visibilitychange`
**每次切标签页都触发**，哪怕宽度一个字节都没改。开销可忽略，但它把 `LAYOUT_EVENT`
变成了「切标签页也会响」的事件——今天只有做值比较的监听器在听，无害；
哪天有谁订阅它做实事，就会收到一堆莫名其妙的唤醒。

→ 加一个 `pending` 标志，让 flush 名副其实：它的本意是「把**还没到期的**改动补上」，
而不是「无条件再写一次」。依赖变化时 effect 重跑、`pending` 自然复位。

### 第四轮：pointerdown 只堵住了一半

换成 `pointerdown` 之后还剩一个入口，而且是读信时**最常点的地方**：
阅读区正文是**非同源沙箱 iframe**（M12 去掉 `allow-same-origin` 之后），
事件不跨文档边界——点邮件正文时顶层 document 一次 `pointerdown` 都不会触发，
`focusInListRef` 原样保持 true。容器的 `onFocus` 也救不了：
焦点移到的是 `<iframe>` 元素本身，那在列表外，不会冒泡成容器的 focusin。

→ 在焦点跟随 effect 里加一条**不依赖任何事件形状**的现实核对：

```ts
const active = document.activeElement
if (active && active !== document.body && !rowsRef.current?.contains(active)) return
```

放行 `body` 是关键：行被删除时 `activeElement` 正是回落到 body，那一路必须补焦点。
三条路径因此各归其位——删除行走 body 分支放行，点空白由 pointerdown 置 false，
点 iframe / 别栏控件由这条守卫拦住。

**这条与前一轮是同一个教训的下一层**：换信号解决了「blur 分不开两种来源」，
但新信号自己也有盲区（跨文档边界）。真正整类关闭问题的，是那个在**判断发生的那一刻**
去问「现实到底是什么样」的守卫——它不关心用户是怎么把焦点移走的。

顺带把标题层级补齐：`.list-title` 加 `role="heading" aria-level={2}`，
分组标题的 level 3 从此有了上一级。**用属性而不是换成 `<h2>`**——
后者会带进 UA 的默认 margin / font-size，那套 `font-display / 20px` 得再重置一遍。

### 按现状接受的两处

- **分组标题的 listitem 不带 posinset/setsize**：AT 会按 DOM 中的兄弟位置自己算，
  虚拟化下于是可能念成「列表项 4，共 15」，与相邻条目的「第 300 项，共 5000」并排。
  信息没丢（标题文本照念），只是数字不连贯。给标题也编号本身也是将就，不再折腾。
- **焦点落在非 roving 行时方向键按 `rovingPos ± 1` 算**，而不是按脚下那一行。
  窗口很窄（点一行就会让它变成 active 从而变成 roving），且比从前「整个哑掉」好得多。

**需人工确认**：读屏（NVDA / VoiceOver）实际念出的列表计数与分组标题；
方向键导航在长列表里的滚动跟随观感；手柄聚焦态在明暗两套主题下是否够显眼。

---

## 四、第四轮已处理（4 条）——把两套语言并成一套

前三轮修的是行为（数据状态、焦点、语义）。这一轮修的是**同一件事有两套写法**：
色板有三份副本、登录页是另一套视觉语言、尺寸一半写在样式表一半写在内联 style。
四条里没有一条改变功能，但每一条都在消除"改一处要记得改另一处"的结构。

### 23. 约 170 行死 CSS

删掉的：`.quick-theme` 整块连同 `.qt-*`（被移除的快速主题气泡）、旧浮层版设置面板
（`.settings-panel` / `.settings-section` / `.settings-label` / `.theme-swatch*` / `.sw-*`，
现行的是 `.sd-*` 那一套）、`.settings-grid` / `.settings-nav` 及其媒体查询、
`.compose-textarea`（已换 Tiptap）、`.kbd-pill`、`.nf-pip`、`.nf-meta-row`、`.kind-sec`、
`.label-dot`、`.mono`、`.sidebar-foot .avatar`、`.tb-menu-wrap`、`.reader-embedded`。

判据是脚本扫的：index.css 里 323 个类选择器，逐个在 `src/**/*.{ts,tsx}` 与 `index.html`
里按**完整词**匹配（`avatar-sq` 不算 `avatar` 的引用）。45 个零引用里人工排掉三类：
Tiptap/ProseMirror 运行时注入的（`.ProseMirror-selectednode` / `.selectedCell` /
`.tableWrapper` / `.column-resize-handle`）、我的正则从注释里误提的（`.fp-*` / `.notif-*`
这种通配写法）、只在注释里出现的（`.reply-box` / `.reply-actions` 的"为什么移除"说明，
那正是注释该留下的东西）。同时扫了一遍模板拼接的 className，确认没有 `` `xx-${...}` ``
这类拼出来的类名会让"零引用"的判断失真。

### 24. 主题色板三份副本合成一份

原来：`index.css` 的 `[data-theme][data-mode]` 令牌（权威，18 组）、`SettingsDialog`
预览卡里 `THEME_PREVIEW` 的 54 个 hex、`lib/theme.ts` 的 `TONES.swatch` 9 个 hex。
改一次主题要改三处，而漂移了**没有任何东西会报错**——预览卡显示的颜色与点下去
真正生效的颜色不一致，只有肉眼能发现。（动手前先比对了一次：当时刚好还没漂移，
所以这次改造是等价替换，不改变任何现有观感。）

修法不是"让两处引用同一份常量"，而是让预览卡**直接用那份令牌**：

```tsx
<div className="tc-preview" data-theme={id} data-mode={mode}>
```

index.css 里的选择器本来就写成 `[data-theme="x"][data-mode="y"]` 而不是 `:root[...]`，
挂在任意子树上都会在那个子树内重新定义整套令牌。于是 `.tc-preview` / `.tc-side` /
`.tc-accent` / `.tc-line` 全部改用 `var(--bg)` / `var(--bg-alt)` / `var(--accent)` /
`var(--rule-strong)`，TSX 里一个颜色都不写。属性挂在预览区而不是整张卡上——
卡片外框与名称要跟随**当前**主题，只有那块预览是"别的主题长什么样"。

顺带两处：`.tc-line` 与 `.tc-side` 的边框原本是硬编码的 `rgba(0,0,0,0.08)`，
暗色预览里几乎看不见，现在跟着预览主题走；`[data-mode="light"]` 补了一条
`color-scheme: light` 与 dark 那条对称——它原先只写在 `:root` 上，而 `color-scheme`
是继承属性，子树挂了 `[data-mode="light"]` 也拿不到它。**这条今天不修任何可见问题**
（预览卡的 mode 总是取自当前模式，与 html 上的一致），修的是那个承诺本身：
既然说「挂上这两个属性就得到一整套主题」，就不该留一项只在 `:root` 上成立。

`TONES` 也不再带 `swatch` 字段（全项目除了测试没有第二个引用），
新增的 `lib/theme-tokens.test.ts` 钉住这个结构：18 组令牌齐全（预览卡不写颜色，
缺一个就是一块透明，界面上只表现为"这套主题的预览有点怪"）、源码里不出现
主题特征色的 hex、亮暗两条 `color-scheme` 对称。

### 11. 登录页并入应用的视觉语言

`Login.tsx` 原先是 shadcn 的 `Card`/`Button`/`Input` + Tailwind 语义类 + lucide 图标，
与应用内部的令牌、16px 自绘图标集、手写像素间距全都对不上，连圆角尺度都是两套
（shadcn 的 `--radius` 派生 vs 裸像素）。用户看到的第一屏不长得像这个应用。

改为同源：同一批令牌、同一套 `Icon`、同一档圆角与阴影，新增 `.login-*` 一组样式。
控件尺寸按"手指够得着"定（输入 42px、主按钮 44px）——登录页没有密度压力，
这里不该沿用列表那套 28px 的紧凑尺度。

顺带补齐三处 a11y（都是改写时才显出来的）：

- `<label>` 用 `useId` 关联（原先靠写死的 `id="username"` / `id="password"`）
- 密码可见性按钮原是 `tabIndex={-1}` 的裸 button，**没有任何可访问名**——
  读屏只报"按钮"，键盘够不着。现在有 `aria-label` + `aria-pressed`，并且可聚焦
- 错误/限流消息原是条件渲染，改为常驻的 `role="status"` live region
  （与内容同时插入 DOM 的 live region 读屏不播报——这是第一轮就学到的）

`lucide-react` 随之从依赖里移除（DraftsList 的两个图标也换成自绘图标集），
`Icon` 集补了 `eye` / `eye-off`；没人引用的 `components/ui/card.tsx` 一并删掉。
`components/ui/{button,input,label}.tsx` 仍有四个对话框在用，保留。

新增 `pages/Login.test.tsx` 六条：label 真的关联到存在的 id、可见性按钮的可访问名
与可聚焦、点击后 type 与 `aria-pressed` 同步、消息区空着时也在 DOM 里、
401 落进那个消息区、429 时提交按钮禁用。

### 12. 触摸尺度

判据用 `pointer: coarse` 而不是窄屏宽度——**"手指有多粗"和"窗口有多宽"是两件事**：
平板横屏比 768px 宽得多，而桌面上把窗口拖窄的人用的仍是鼠标。
（同一个判据在 `.mi-del` / `.account-row-actions` 的 hover 守卫上已经用过，
第 12 条原文说的"`max-width:768px` 块只改了间距与字号"，缺的正是这个维度。）

`.icon-btn` 36×36、`.lt-btn` 36×36、`.rt-btn` 34、`.chip-clear` 30、`.tb-btn` 高 38，
搜索框同时放宽内边距——两个按钮被放大后需要更多呼吸空间。

**做法是放大视觉尺寸而不是用伪元素扩命中区**：搜索框里那两个按钮只隔几像素，
给它们各铺一块 44px 的透明命中区会互相重叠，结果是点前一个触发后一个。

其中三处按钮的尺寸原本写在 `style={{ width: 20, height: 20 }}` 里——
**内联样式优先级最高，媒体查询改不动它**。改成 `.icon-btn.compact` / `.icon-btn.mini`
两个修饰类，尺寸的归属才回到样式表。这是第 12 条里唯一动到 TSX 的地方，
也是"同一件事两套写法"在这一轮的第三个面孔。

`.mi-star` / `.mi-del` 没有放进去：它们是绝对定位（`right: 14px` / `40px`）且不带宽高，
给了尺寸就要连着重算两个 right——34px 宽时两者会叠掉 8px。粗指针下它们已经常驻显示
（第一轮修的是"看不见却能点"），尺寸单独排。

### 复审抓出的五条

**1（高）登录主按钮绕过了仓库已有的那层桥。** 我照着 `.compose-btn` 写了
`background: var(--accent); color: white`，而 `index.css` 顶部早就为「品牌色作按钮底」
分好了亮暗两套：`--primary` 亮色取深调的 `--accent-ink` 配白字、暗色取亮调的
`--accent` 配深底色字。直接用 `--accent` 配死白字，**9 套主题的暗色全部**
（slate 暗 `#b6bdc8` 对白字 ≈ 1.9:1）与亮色里的 butter / warm / coral / aqua
都达不到 AA 的 4.5:1——「登录」二字糊在按钮底色上，而这是第一屏。

扫了一遍发现同一个写法在**四个地方**各犯了一次：`.brand-mark`（侧栏品牌方块）、
`.compose-btn`（撰写新邮件）、`.pill-btn.primary`（对话框主按钮），加上新写的
`.login-submit`。四处一起改成 `var(--primary)` / `var(--primary-foreground)`，
并加了一条测试扫 CSS：任何规则里同时出现 `background: var(--accent)` 与
写死的白字就失败。

这条是「根因二（照着看起来像写，而不是照着语义上是写）」的又一例，
而且是**照着一个本身就写错了的样板抄**——那三处比登录页早得多。

**2（中高）`.mi-star` / `.mi-del` 在粗指针下反而叠得更狠。** 我在第一版的注释里写
「它们不带宽高，所以没放进放大块」——**那句是错的**：它们的 class 是
`mi-star icon-btn`，宽高正是从 `.icon-btn` 来的，`@media (pointer: coarse)`
里那条 36px 直接命中。卡片模式下两者绝对定位、间隔 26px，放大后重叠 10px
（28px 时只叠 2px），而粗指针下它们是常驻可见的——**点删除键靠右那一侧会变成加星标**。

我想避免的正是这个结果（块首注释写着「铺 44px 命中区会互相重叠，点前一个触发后一个」），
只是换了个方式发生了。→ 尺寸与 `right` 一起定（32px + 12/48），
并把「不重叠」做成算术测试（`lib/touch-targets.test.ts`）：从 CSS 里读出尺寸与 right
自己算一遍，不依赖布局引擎。三种回退都验过会失败——包括「把尺寸改小来避免重叠」
这种把问题掉个头的修法。

**3（中）第 12 条只做了一半。** 漏掉的恰好是不可逆操作：`DraftsList` 的
「立即发送」「删除草稿」是 Tailwind `p-1` ≈ 21px，没带 `.icon-btn` 所以不受放大影响。

而查这两个按钮时发现了更要紧的：它们的容器写的是
`opacity-0 group-hover:opacity-100`——**与第一轮修掉的「触摸端隐形删除按钮」完全同形**。
触摸设备 `:hover` 永不触发，而 `opacity: 0` 的元素照常接收点击，
于是草稿列表每一行右侧躺着两个看不见的不可逆操作。
→ 改用 `.draft-actions`，显隐守卫与 `.mi-star` / `.account-row-actions` 同一套，
另加 `:focus-within` 让键盘用户也看得见。
同时补上 `.mi-select input` / `.lt-all input`（16px 复选框 → 20px）与 `.chip` 的高度。

**同一个缺陷在第一轮修过一次，两个月后在另一个组件上原样复现**——
说明"hover 显隐"这个模式值得像第 23 条那样单独扫一遍全项目。

**4（中）新测试只钉了 CSS 那一侧。** `theme-tokens.test.ts` 验的是令牌齐全、
源码无色板副本、`color-scheme` 对称——这些全过，预览卡照样可能什么都不显示：
只要 `data-theme`/`data-mode` 没挂上去或拼错，9 张卡会**全部渲染成当前主题、
看起来一模一样**，而测试全绿。

旧写法的退化形态是卡片消失（`THEME_PREVIEW[id]` 查不到就 `return null`），一眼可见；
新写法的退化形态是「安静地都对但都一样」。→ 导出 `ThemeCard` 并加
`ThemeCard.test.tsx` 五条（属性挂在预览区而非整张卡、九个色调都渲染得出、
预览区内不出现任何内联颜色），`ThemeCardProps.id` 也从 `string` 收紧为 `ToneId`。

**5（低）`--accent-color` 别名在预览子树里取不到被预览的颜色。**
自定义属性的 `var()` 在**声明处**求值：`:root` 上那条 `--accent-color: var(--accent)`
算出来的永远是 html 那层的值，向下继承时不会被子树的 `[data-theme]` 重新解析。
今天预览卡里没人用它，但「挂上这两个属性就得到一整套主题」这个承诺对它是破的。
→ 补一条 `[data-theme][data-mode] { --accent-color: var(--accent); }`，一条覆盖全部 18 组。

**6（低）粗指针下返回键与工具栏首个按钮贴到 0 间距。** 浮动阅读面板左上角的返回键
（`icon-btn reader-slide-close`，尺寸全来自 `.icon-btn`）放大到 36px 后占到 `left 12→48`，
而 `.reader-slide .reader-toolbar` 为它让的 `padding-left` 正好也是 48px——
28px 时还有 8px 缝。不重叠也不会误点，但在最该留余量的触摸场景下反而最紧。
→ `padding-left` 跟着进 coarse 块提到 56px。

**7（低，既有）色板其实还有第四份副本，而且已经漂了。** `index.html` 的两个
`theme-color`：亮那个 `#fbfaf7` 是 **warm** 的 `--bg`，而默认色调是 **slate**（`#f7f8fa`）；
暗那个 `#1a1917` **在整张 9×2 的令牌表里根本不存在**。
第 24 条自称"三合一"时把它漏了，而新加的守卫也盖不到——那条测试的 glob 只扫
`src` 下的 ts/tsx。

这处没法走令牌：`<meta>` 在 CSS 变量之外，浏览器读它时下面的引导脚本还没跑，
所以它只能表达「默认色调 + 跟随系统」，用户选了别的色调或反着设了明暗时会差一档——
那是这个标签的固有限制。→ 值改成 slate 的，并加一条测试把它和令牌绑住
（顺带断言引导脚本的默认调仍是 slate，那是三处必须一致的第三处）。

**「三合一」其实是四合一。** 找副本时我只找了"程序里引用色板的地方"，
没有找"任何写着颜色的地方"——`index.html` 不在 `src/` 下，也不 import 任何东西，
于是整条搜索路径都绕过了它。而它恰恰是漂得最久、最没人看的一处。

### 自己在实现期间抓到的两条

**JSX 注释漏了闭合的 `}`，`tsc` 两次都放行。** 写成 `{/* … */` 之后紧跟一个元素时，
TS 把它解析成「一个包着 `<div>` 的表达式容器」，语法合法、渲染结果也对，
所以 `tsc --noEmit` 干净通过。第一次是 eslint 的解析器报 `Parsing error` 抓到的，
第二次是 vite 的 oxc 在 `vitest` 里报 `Unterminated regular expression` 抓到的。
**同一个笔误犯了两次，而两次都不是被类型检查发现的**——又一条「tsc 绿灯不是证据」。

**给 `color-scheme` 补丁写的理由说过头了。** 我写的是「暗色应用里预览一张亮色卡会
继承到 dark」，但预览卡的 `mode` 恒等于当前模式，那个场景根本不会发生。
那条 CSS 仍该补（让按属性选择的两条对称），但理由改成了实话：
**它今天不修任何可见问题**，修的是那个承诺本身。写注释时顺手编一个听起来合理的
失败场景，比不写注释更坏——下一个人会拿它当事实。

**需人工确认**（jsdom 测不了的）：新登录页在九套主题明暗两套下的观感与对比度；
真机触摸端放大后的列表行是否拥挤；主题预览卡的九张是否确实各不相同。

---

## 五、待处理（13 条）

### P1 — 建议下一轮

**1. 翻页失败后列表静默卡死** — ✅ 已于第二轮处理，见第二节。

**2. 后台刷新完全不可见** — ✅ 已于第二轮处理，见第二节。

**3. 新邮件到达无播报** — ✅ 已随浏览器通知一并处理（见 `docs/flymail/browser-notify.md`）。
Shell 里加了常驻的 sr-only live region，收到 `notify` 事件即播报，
且**不跟随桌面通知开关**——播报不弹窗不出声，没有理由被那个开关关掉。

**4. 列表缺 ARIA 语义** — ✅ 已于第三轮处理，见第三节。

**5. aria-label 缺失 / 硬编码** — ✅ 已于第三轮处理，见第三节。

**6. 分栏手柄是个假承诺** — ✅ 已于第三轮处理，见第三节。

**7. 长操作没有进度表达**
首次同步几千封是分钟级操作，全部反馈是账户行里一个 11px 的圆点在转，
且条件写成 `syncing && active`——同步非当前账户时屏幕上零变化。
`syncStatus.phase` 已经取到了但没有任何一处用在 UI 上。
SSE 连接状态也不外露（`useRealtimeSync(): void`），合盖唤醒 / 后端重启后 UI 静默停止收信。
发送与 10MB 附件上传同样零进度。

**8. 侧栏与草稿列表仍无 error 态** — ✅ 已于第二轮处理，见第二节。

**9. 设置页 8 处 window.confirm 仍在**
删账户 / 别名 / 黑名单 / 通知渠道 / 规则 / 信任发件人，以及会话内删单封。
原生 confirm 在 Wails 桌面壳里外观完全脱离应用，且不跟随主题。
→ 统一改为 toast + 撤销，或至少换成走 `.ctx-menu` 令牌的自绘弹层。

**10. 批量「移动到」菜单与全局菜单不同源**
`MailList` 里是完全内联样式的手搓下拉：硬编码 `boxShadow`（不是 `var(--shadow-md)`）、
靠 `onMouseEnter/Leave` 手改 `style.background` 模拟 hover。
而 `.ctx-menu`/`.ctx-item` 是权威样式，`DropMenu` 组件和 `CtxMenuItem` 模型都是现成的。
同一应用里两种菜单外观。→ 替换成 `<DropMenu>` 净删约 35 行。

**11. 登录页是另一套视觉语言** — ✅ 已于第四轮处理，见第四节。

<details><summary>原文</summary>

`Login.tsx` 用 lucide-react 图标 + shadcn Card/Button/Input + Tailwind 语义类；
应用内部用自绘 16px stroke 图标集 + MailMaster 令牌 + 手写像素间距。
连圆角尺度都不同（shadcn 的 `--radius` 派生 vs 裸像素）。
用户看到的第一屏不长得像这个应用。顺带：为 3 个图标引入了整个 lucide-react。
</details>

**12. 触摸目标偏小 + 窄屏断点只做了一半** — ✅ 已于第四轮处理，见第四节。

<details><summary>原文</summary>

`.icon-btn` 28×28、`.lt-btn` 30×30、`.rt-btn` 26×26、`.chip-clear` 22×22、
账户同步按钮 22×22（图标 11px）、搜索清除按钮 20×20。
而 `max-width:768px` 块只改了间距与字号，没有任何一处放大触摸目标。
三栏退化到单栏本身做得不错（抽屉 + `data-mobile-pane`），缺的是退化之后的"手指尺度"。
附带：汉堡菜单用的是三点图标（隐喻错误），返回键用 `chevron-right` 加 `scaleX(-1)` 镜像
——都是图标集缺项的代偿。
</details>

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

**23.** ✅ 已于第四轮处理（实删 172 行），见第四节。原文：约 150 行死 CSS：`.quick-theme` 整块（73 行，被移除的快速主题气泡）、`.settings-panel`/`.settings-section`/`.theme-swatches`、`.compose-textarea`（已换 Tiptap）、`.nf-pip`、`.label-dot`、`.kbd-pill`。在一个 2224 行的单文件 CSS 里，这些残留会让后来者分不清哪套是现行语言。

**24.** ✅ 已于第四轮处理，见第四节。原文：主题色板有三份副本：`index.css` 的权威定义、`SettingsDialog` 预览卡里的 54 个 hex、`lib/theme.ts` 的 `TONES.swatch`。改一次主题要改三处，必然漂移。→ 预览卡改为挂 `data-theme`/`data-mode` 让它自己继承令牌。

**25.** 撰写器细节：~~最小化条的展开箭头是手画的内联 `<svg>`~~、~~展开按钮的 `title` 文案是反的~~ —— 这两项已随第三轮修掉；仍开着的是 From 下拉带 14 行内联样式而 `.settings-field > select` 是现成的。

**26.** 列表底部恒挂 12px 空白条（`hasNextPage && !isFetchingNextPage` 时渲染 `null` 但外层 padding 照常生效），滚到底时像"还有一行没加载出来"；翻页加载态是纯文字而首屏有完整骨架，同一列表两种表达强度。

**27.** 应用外壳收尾 — ✅ 已随浏览器通知一并处理：`index.html` 换成内联的 FlyMail 图标
（首屏就不再是脚手架默认图，也少一次请求），未读数写进标签页标题并用 canvas 画进站点图标。

**28.** 正文 iframe 强制白底（刻意为之，邮件 HTML 假定白底，合理），但暗色下阅读等于盯一块白板。设置页已有隐私分区，可加一个「暗化正文」开关。

**29.** 阅读区日期显示两遍：meta 行与 `.th-time` 相距约 40px 都是 `formatDate(detail.date)`。那条 meta 行目前只承载这一条重复信息，白占一整行视觉预算。→ 换成所属账户/文件夹（会话视图的 `.ti-folder` 已经这么做了）。

---

## 六、三条真缺陷的共同形状（2026-09-12 补）

三、四轮的代码审查抓出三条我的测试全部放行了的真缺陷。把它们并排看，是同一件事的三个面：

| 缺陷 | 形状 |
|---|---|
| 两套编号体系（`pos` 与 `rovingPos`） | 用了**两个数据源**却假设它们同序 |
| `blur` 的折中 | **一个信号承载两种含义**时选了一边押注 |
| iframe 盲区 | 换了信号，但**新信号自己有覆盖不到的边界** |

三次的正解都不是把条件写得更细，而是**换一个本身就没有歧义的依据**——
同一个数组、事件的源头、判断那一刻的现实。

补条件是在已知的歧义上打补丁，下一个未知的入口照样漏；换依据是把歧义本身消掉。
每次想给判据加一个 `&&` 的时候，先问一句：是这个判据不够细，还是它根本就不该由这个信号来做。

---

## 七、两条贯穿性的根因

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

## 八、建议的推进顺序

1. ~~**P1 第 1、2、8 条**（翻页失败、后台刷新不可见、侧栏/草稿 error 态）~~
   ——✅ 第二轮完成。
2. ~~**P1 第 4、5、6 条**（ARIA 语义、aria-label、分栏手柄）~~ ——✅ 第三轮完成。
3. ~~**P2 第 23、24 条**（死 CSS + 色板三合一）~~ ——✅ 第四轮完成。
4. ~~**P1 第 11、12 条**（登录页视觉统一、触摸尺度）~~ ——✅ 第四轮完成。
5. **P1 第 7、9、13 条**（长操作无进度表达、设置页 8 处 window.confirm、
   撰写器校验错误显示在会滚走的位置）——三条都是"用户做了事却看不到反馈"，
   其中第 7 条（首次同步几千封是分钟级操作而屏幕上只有一个 11px 的点在转）
   在日常使用里最容易被撞到。
6. **P1 第 10、14 条 + P2 的一串**（菜单不同源、手风琴无展开指示符，
   以及第 15~22、25、26、28、29 条）——多是各自独立的小修，可以按一次一批地清。

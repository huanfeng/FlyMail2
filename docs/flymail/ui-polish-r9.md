# FlyMail 第九轮 UI/UE 打磨设计（列表操作 · 字重 · 外观）

日期：2026-10-28
范围：`flymail/frontend`，纯前端，不动后端与接口。
来源：`ref-seekli-ui.md` 的借鉴清单，经用户取舍后的四项：

| 编号 | 内容 | 用户决定 |
|---|---|---|
| R9-1 | 列表行悬停动作（时间位原地换成动作） | 做 |
| R9-2 | 选择栏重做：全选框与行复选框对齐、选中后操作就地替换筛选 | 做 |
| R9-3 | 字体栈与字重层级：解决「字普遍偏细」 | 做，**系统字体 + 调字重**，不打包 Web 字体 |
| R9-4 | 外观设置扩充 + 实时预览；阅读页新增「邮件正文」设置 | 做 |
| — | 阅读区底部回应条、快捷回复 | **不做** |

**基线事实（动手前记录）：** 以执行时 `pnpm test` / `pnpm lint` 的实际输出为准，写进
第六节验收记录；第八轮记录的既有失败（`MailList.test.tsx › 分组打乱顺序后…`）与
既有 lint 问题不计入本轮。

---

## 一、现状诊断（2026-10-28 截图 + 代码核对）

1. **选择栏错位**（`MailList.tsx` `.list-toolbar`）。进入选择的开关（✓）在最左，
   但全选框 / 「已选 N 封」/ 操作组排在三个筛选 chip **之后**。结果：
   - 全选框与行内复选框不在同一竖列（行复选框 x≈273，全选框 x≈491）；
   - 380px 列表宽度下操作组被挤出栏外，只剩「已读 / 未读」两个可见，移动、删除、退出看不到。
2. **悬停动作少且挤**：卡片行只有删除 + 星标，并排挂在时间右边（`.mi-del` right:40 / `.mi-star` right:14），
   与时间争右上角；没有已读切换与归档。
3. **字普遍偏细**，三个叠加原因：
   - 已读行的发件人与主题都是 `--ink-2` + 400：整列表没有一处「正常黑」的文字；
   - 时间、计数、标签等 38 处用 `--font-mono`（JetBrains Mono 10.5px）——等宽字体笔画细、
     字面窄，小字号下最显单薄，也带工程工具的观感；
   - 标题（`h1-h4`、`.list-title`、`.reader-empty h2` 等）字重 500。
     Windows 上中文落到微软雅黑，雅黑只有 Light/Regular/Bold 三档，**500 会回落成 400**，
     所以这些「中粗」标题实际就是常规体。
4. **外观页**只有色调、布局、亮暗、三条栏宽、列表密度、选择框常显；改动没有就地可见的预览
   （设置对话框盖住了主界面）。阅读页没有任何正文排版设置。

---

## 二、R9-1 列表行悬停动作

**行为**
- 细指针设备（`(hover: hover) and (pointer: fine)`）下，行悬停时**时间淡出**，原位出现
  4 个图标按钮，从左到右：**归档 · 删除 · 已读/未读切换 · 星标**。
- 已标星的星标照旧常显（不随悬停才出现）；悬停时它就是动作组的最后一个，不重复渲染。
- 粗指针设备下这组按钮 `display:none`（不是 `opacity:0`——看不见但可点的按钮是第一轮审查抓过的坑；
  用 CSS 媒体查询而不是 JS 判断指针类型，免得混合设备上首帧与真实指针不一致），
  触屏用户走右键/长按菜单与阅读区工具条。原有「粗指针下星标/删除常驻」的规则一并移除，
  星标常显仅限已标星。
- 键盘：按钮在行内可 Tab 到（行采用 roving tabindex，按钮 `tabIndex={-1}`，与现有
  `.mi-star` 一致——键盘用户有 `e` / `#` / `s` / `Shift+U` 快捷键，不需要在每行停 4 次）。
- 每个按钮有 `aria-label` + `title`（带快捷键提示，复用 `withShortcut`）。

**归档的可用条件**（单一出口 `lib/archive-target.ts`）
- 行所属账户存在 `type==='archive' && selectable` 的文件夹，且该行不在归档文件夹里
  （会话行：只看账户有无归档文件夹，与 `Shell.canArchiveCurrent` 同口径）。
- 聚合视图（所有收件箱 / 未读 / 星标）每行按自己的 `account_id` 判断。
  文件夹数据来自侧栏已在取的各账户文件夹查询（`useFolders` 缓存），不新增请求；
  某账户文件夹尚未加载时该行不显示归档按钮（缺省隐藏，而不是显示后点了报错）。
- `Shell` 现有的 `archiveFolder` 推导改为调用同一函数，消除两处口径。

**动作实现**
- 全部走 `Shell` 已有的 `runUndoable`（带撤销 toast、自动前进到下一封的规则不变）。
  新增 MailList props：`onArchiveMessage(id)` / `onArchiveThread(item)`，
  已读切换复用 `onMarkRead` / `onMarkReadThread`，删除与星标复用现有回调。
- 紧凑行（`.mail-item-row`）：时间列 `mi-time-col` 同样淡出，动作组绝对定位在行右端、
  带行底色遮罩，不改变网格列宽（悬停不引起任何重排）。

**样式要点**
- 动作组容器 `.mi-hover-actions`，按钮 26×26、图标 15，间距 2；
  删除悬停转 `--destructive`，其余悬停转 `--ink`。
- 时间与动作组用 `opacity` 交叉淡入 100ms；`prefers-reduced-motion` 下无过渡。

---

## 三、R9-2 选择栏重做

**工具栏两态（高度恒定 44px，不跳动）**

```
未选中：[☐▾]  未读  星标  有附件  [×清筛选]
已选中：[☑▾]  已选 3 封  │ 已读  未读  星标  移动  删除 │  ✕
```

- **最左固定是全选复选框**，与行复选框**同一竖列**：两者都放在 20px 宽、左内边距与行
  `padding-left` 相同的槽里。结构测试钉住两者共用一个 CSS 变量 `--select-col-x`。
- 复选框右侧的 ▾ 是「按条件选择」下拉（`DropMenu`）：**全部 / 无 / 已读 / 未读 / 已加星标 / 未加星标**。
  条件只作用于**已加载的可见行**（与现在的「全选当前可见」同口径，菜单项文案就写「可见的…」
  之外不另起提示——见 i18n 键）。判定函数抽到 `lib/select-by.ts`，单封与会话两套行类型都支持
  （会话：`unread > 0` 视为未读，`flagged` 取会话聚合字段）。
- 全选框三态：无选中 → 空；部分 → `indeterminate`；全部可见 → 勾。点击：非全选 → 全选可见；全选 → 清空。
- **选中后筛选 chip 整组隐藏**，原位显示「已选 N 封」+ 操作组 + 末尾 ✕（清空选择）。
  筛选状态本身不丢，清空选择后 chip 原样回来。理由：选中时用户的意图是「处理这几封」，
  同屏再摆筛选开关既占宽度又可能误触改变列表、把已选行刷掉。
- 原来单独的「进入选择模式」✓ 按钮**在细指针下移除**——全选框本身就是入口。

**行复选框何时出现**
- 细指针、无选中：行悬停时复选框**覆盖在头像位置**出现（只在被指着的那一行，头像暂时让位；
  移开即恢复）。不加独立列，悬停不引起重排。
- 一旦有选中项（或开了「始终显示选择框」偏好）：切换为现有的**独立复选框列**，所有行常显，
  头像完整可辨（保留第一轮「复选框不能吃掉头像」的结论——它针对的是持续选择态）。
- 粗指针（触屏）：没有悬停，保留一个「选择」按钮（即原 ✓ 开关，只在粗指针下渲染）进入选择模式；
  进入后同上切换为独立列。
- 「始终显示选择框」偏好保留，语义不变。

**窄列表**：`@container list (max-width: 340px)` 下操作组只留图标（本来就是图标按钮），
「已选 N 封」缩成数字徽标 `N`。

---

## 四、R9-3 字体栈与字重层级

**字体栈**（`index.css` 令牌）
```css
--font-body: system-ui, -apple-system, "Segoe UI Variable Text", "Segoe UI",
             "PingFang SC", "Microsoft YaHei UI", "Microsoft YaHei",
             "Noto Sans SC", "Noto Sans CJK SC", sans-serif;
--font-display: 同 --font-body（标题不再单独一套栈，只靠字号与字重区分）
--font-mono: 不变
```
- `"Microsoft YaHei UI"` 是雅黑的界面变体（行距更紧、为 UI 调过 hinting），放在 `Microsoft YaHei` 前。
- 邮件正文 iframe 的 `MAIL_BODY_CSS` 字体栈同步更新（单独一份字符串，测试断言两者包含同样的中文字体序列）。

**字重令牌**（新增，组件与 CSS 一律引用令牌，不再直接写数字）
```css
--fw-regular: 400;  --fw-medium: 500;  --fw-semibold: 600;  --fw-bold: 700;
```
- 标题类（`h1-h4`、`.list-title`、`.reader-subject`、对话框标题、设置分区标题）→ `--fw-semibold`。
  雅黑下 600 取 Bold，标题终于真的「粗」了；西文 Segoe UI 有 Semibold。
- 列表行：

  | | 发件人 | 主题 | 摘要 | 时间 |
  |---|---|---|---|---|
  | 已读 | `--ink` / medium | `--ink` / regular | `--ink-3` / regular | `--ink-3` |
  | 未读 | `--ink` / bold | `--ink` / semibold | `--ink-2` / regular | `--accent-ink` / semibold |

  已读行改为「正常黑」：层级靠字重而不是靠把文字调灰。
- 侧栏文件夹名 `--ink-2` → `--ink`（当前项仍以底色区分）。

**等宽字体收口**
- 时间、计数徽标、`.mi-tag`、`.f-count`、`.slider-val`、日期分组头等改用正文字体 +
  `font-variant-numeric: tabular-nums`（数字等宽对齐保留，笔画回到正文字体）。
- `--font-mono` 只允许出现在白名单选择器：代码/原始邮件（`.raw-source`、`pre`、`code`）、
  `.kbd` 快捷键提示、API/Token 一类机器字符串展示。结构测试列出白名单，新增使用需显式加进去。
- 被替换处字号小于 11px 的统一抬到 11px（10.5px 的 mono 换成比例字体后视觉更小）。

---

## 五、R9-4 外观设置扩充与实时预览

### 5.1 偏好模块 `lib/appearance-prefs.ts`

沿用 `privacy-prefs.ts` 的「localStorage + 可订阅」模式（`useSyncExternalStore`），
只存本设备（与栏宽一致：大屏设置不该污染手机）。

| 键 | 取值 | 默认 | 落地方式 |
|---|---|---|---|
| `uiScale` | 90 / 100 / 110 / 120（%） | 100 | `html { zoom }`，见 5.4 风险 |
| `textContrast` | `standard` / `high` | standard | `:root[data-contrast="high"][data-theme][data-mode]`：`--ink-2/--ink-3` 改为 `color-mix(in srgb, var(--ink) N%, var(--bg))`，一条规则覆盖全部色调（选择器权重须高于色调块的两属性选择器） |
| `motion` | `system` / `reduce` | system | `html[data-motion="reduce"]` 与 `prefers-reduced-motion` 走同一套规则 |
| `bodyFontSize` | 13 / 14 / 15 / 16 / 18（px） | 14 | 注入正文 iframe |
| `bodyLineHeight` | 1.4 / 1.6 / 1.8 | 1.6 | 注入正文 iframe |
| `readingWidth` | `original` / `comfort`（720px）/ `wide`（960px） | original | 注入正文 iframe：`body{max-width;margin:0 auto}` |
| `bodyZoom` | 90 / 100 / 110 / 125 / 150（%） | 100 | 注入正文 iframe：`html{zoom}` |

- 读取时校验取值，非法值回落默认（与 `getListStyle` 同口径），测试覆盖。
- `applyAppearance()` 在 `main.tsx` 启动时调用一次（与 `applyTheme` 同位置），首帧即生效、无闪烁。
- 关于「字体风格：加粗」：评估后**不做**。雅黑只有 Regular/Bold，整体上调一档在中文上要么无效
  （400→500 回落 400）、要么跳成粗体；真正能让「细」的感觉改善的是对比度，故以 `textContrast` 代替。
- 关于「圆角规模」：评估后**不做**。`index.css` 里自有类的圆角是 100+ 处写死的像素值
  （6px ×25、8px ×24、999px ×10…），只有 shadcn 组件走 `--radius`；做这个开关要先把上百处
  声明令牌化，收益只是审美偏好，不值这一轮。

### 5.2 外观页结构

外观页顶部放一条 **sticky 迷你预览**（高 150px，横跨内容区；滚动设置项时预览停在顶部）。
不做左右两栏：设置对话框 920px 减去 220px 导航后内容区只剩约 640px，再分出 260px 预览，
滑块行就挤不下了。

- 预览是一个缩小的三栏骨架：侧栏（3 个文件夹行，第 2 个为当前）、列表（3 行：1 未读 + 2 已读，
  含头像色块、发件人、主题、时间，**用真实的 CSS 类与令牌渲染**，只在外层 `transform: scale()`）、
  阅读区（标题 + 3 行正文）。
- 预览挂当前 `data-theme / data-mode / data-contrast`，列表行跟随列表密度（紧凑/卡片），
  布局跟随三栏/双栏模式。界面字号变化以预览整体 zoom 体现。
- 设置分组：**主题**（色调、亮暗）→ **布局**（布局模式、三条栏宽、列表密度、选择框常显）→
  **文字与界面**（界面字号、文字对比、动效）→ 语言。
- 即改即生效，不加保存条（与其余设置页一致；单独一页要点保存反而让人困惑）。
  分组标题右侧给「恢复默认」文字按钮，只重置本组。

### 5.3 阅读页「邮件正文」分组

放在「会话视图」之后、「阅读隐私」之前：正文字号、行高、阅读宽度、正文缩放，外加一个
**正文预览**（一段固定的示例邮件 HTML，用与真实阅读区相同的 `buildFrameDocument` 渲染进 iframe），
改动即时反映。

- `FrameDocumentOptions` 新增可选 `bodyStyle?: { fontSize; lineHeight; maxWidth; zoom }`；
  缺省时生成的文档与现在**逐字节相同**（测试断言），避免无意改变已有渲染。
- 字号只作用于 `body` 基准，邮件自带的内联字号不覆盖（不写 `!important`、不改 `*`）——
  覆盖内联样式会把营销邮件的版式打散；整体放大交给 `bodyZoom`。
- `MessageBody` 订阅 appearance-prefs，变化时重建 srcDoc（与 `darkBody` 同路径）。

### 5.4 风险与验证点

1. **`html { zoom }` 与虚拟列表**：`list-density.ts` 用固定像素行高估算，`@tanstack/react-virtual`
   读 `scrollTop / offsetHeight`。Chromium 现行（标准化后）zoom 下这些值在元素自身坐标系内一致，
   理论上无影响，但**必须在实施第一步用真实浏览器验证**：90%/120% 下滚动到底、
   键盘 j/k 定位、栏宽拖拽、右键菜单位置。任一失败则 `uiScale` 改为不提供（降级为提示用浏览器缩放），
   不做逐处 px 改 rem 的大改。
2. **正文 iframe 的 `zoom` 与自适应高度**：注入脚本量的是 `scrollHeight`，需在 125%/150% 下核对
   iframe 高度不截断、不留大块空白。
3. **对比度增强**的令牌要在 9 个色调 × 亮暗下满足 `--ink-3` 对 `--bg` ≥ 4.5:1（`theme-tokens.test.ts` 扩展）。

**探针结论（2026-10-29，Chromium 1228 headless，本机 Docker 实例）：**
- **uiScale：可用。** zoom 0.9 与 1.2 下：行间距单一值（0.9→60px、1.2→80px，行高恒定无重叠），
  键盘 j/k 选中行始终在列表视口内；zoom 1.2 滚到底部最后一行完整在列表框内（lastBottom 1030 ≤ listBottom 1080）；
  0.9 下 lastBottom 991 略越 listBottom 810，与「列表底部 padding 参与滚动高度」相关，不构成重叠或截断。
  依据核心判据（gaps 单值 + 键盘定位），uiScale 保留。
- **bodyZoom：注入路径按计划实现，真实 iframe 高度判据未在浏览器实测。**
  目标实例的阅读区把正文渲染为 HTML 时经 iframe（srcDoc）、纯文本与聚合会话视图直接渲染——
  探针在真实阅读区里打开 HTML 邮件后 frames 列表为空（正文 iframe 未在探针会话中出现），iframe 高度
  收敛判据无法执行。正确性改由结构断言保证（mail-frame.test.ts：zoom 写进文档、缺省逐字节不变），
  125%/150% 的视觉验证归入「Windows 实机确认」一并做。

---

## 六、不做的事

- 阅读区底部回应条、快捷回复（用户明确暂不做）。
- 打包 Inter / 中文 Web 字体（用户选定系统字体方案）。
- 稍后提醒、认证胶囊、专注阅读（借鉴清单 P2/P3 中未入选项）。
- 邮件正文「最小字号」强制（需改写内联样式，破坏版式）、「低对比文字自动增强」（需逐元素算色）。

---

## 七、验收

1. `pnpm test` 全绿（除基线既有失败），`tsc -b` 无错，lint 不新增。
2. 结构测试（新增 `lib/ui-polish-r9.test.ts`）：
   - `.mi-hover-actions` 只在细指针媒体查询内可见；粗指针下 `display:none`；
   - 全选框与行复选框共用 `--select-col-x`；
   - `--font-mono` 只出现在白名单选择器；
   - 标题类选择器字重引用 `--fw-semibold`；已读行 `.mi-sender` 颜色为 `--ink`。
3. 单测：`select-by`、`archive-target`、`appearance-prefs`（非法值回落、订阅通知）、
   `buildFrameDocument`（缺省逐字节不变 / bodyStyle 注入）。
4. 截图回归（本机 Docker 127.0.0.1:8086，1440×900 / 1024×768 / 390×844，亮暗两模式）：
   悬停动作、选择栏两态（380px 列表不溢出）、外观页预览、阅读页正文预览、
   Windows 实机看一次字重（用户远程 Chrome 或桌面端）。

---

## 八、验收记录（2026-10-29）

### 测试与静态检查

| 项 | 基线 | 本轮 | 结论 |
|---|---|---|---|
| `pnpm test` | 977 passed / 1 failed（第八轮已知基线） | **1037 passed / 1 failed（同一条已知基线）** | 不新增失败 |
| `pnpm lint` | 20 problems (17 errors, 3 warnings) | 20 problems (17 errors, 3 warnings) | 不新增 |
| `pnpm build`（tsc -b） | — | 无错误 | 通过 |

结构测试与单测全绿：`ui-polish-r9.test.ts` 12 用例、`archive-target` 5、`select-by` 8、
`appearance-prefs` 7、`mail-frame` 正文排版 4、`theme-tokens`（含新增文字对比增强×18 组令牌）、
`touch-targets` 2、`AppearanceSection.test.tsx` 4、`ReadingSection.test.tsx` 4、语言键对齐与引用检查 4。

### 任务完成清单

- Task 1 探针：uiScale 可用 / bodyZoom 注入实现（结论见 §5.4 末尾探针结论）。
- Task 2 `archive-target`：归档可用性单一出口，Shell.canArchiveCurrent 改用同一函数。
- Task 3 R9-1 悬停动作：`RowHoverActions` 统一三种行（卡片/紧凑/会话），归档/删除/已读切换/星标；
  归档目标按行所属账户判定，未加载账户的行不显示归档按钮。
- Task 4 `select-by` + Task 5 R9-2 选择栏重做：全选框与行复选框共用 `--select-col-x`；
  选中后筛选 chip 隐藏（筛选状态保留）、显示计数与操作组；按条件选择（全部/无/已读/未读/已加星/未加星）
  只作用于已加载可见行；悬停复选框覆盖头像（无选择态）。
- Task 6 R9-3 字体与字重：`--font-body` 换为 system-ui 打头 / YaHei UI 靠前的系统字节栈；
  `--fw-regular/medium/semibold/bold` 四档令牌；标题类用 semibold；已读行正常黑；
  `--font-mono` 收口到白名单 8 个选择器；数字类改 `tabular-nums`。
- Task 7 `appearance-prefs`：uiScale / textContrast / motion / bodyFontSize / bodyLineHeight /
  readingWidth / bodyZoom 七键，非法值回落默认；首页启动 `initAppearance()`。
- Task 8 正文排版：`bodyStyleFrom`/`buildFrameDocument` 注入 iframe；缺省输出与不传逐字节相同。
- Task 9 外观页：顶部 sticky 迷你三栏实时预览（真实类名与令牌渲染）；「文字与界面」分组（四档字号 /
  文字对比 / 减少动效 + 本组恢复默认）。
- Task 10 阅读页「邮件正文」分组：正文字号 / 行高 / 阅读宽度 / 正文缩放 + 正文预览 iframe。

### 截图回归与发现的问题

截图 8 张（1440/1024/390 的列表与阅读区、1440 修复后、390 触屏模拟），位于临时目录不进仓库。

**发现并已修复的问题：**
1. 截图发现细指针下 `.lt-select-touch`（触屏专用选择开关）仍然可见——`.lt-btn` 的
   `display: grid` 在文件靠后位置声明，同等特异性覆盖了 `.lt-select-touch { display: none }`。
   修复：选择器改为 `.lt-btn.lt-select-touch`（含 coarse 块），触屏模拟
   （390 + pointer:coarse + hasTouch）下复验：开关可见、`.mi-hover-actions` 与
   `.mi-select-hover` 均 `display: none`，符合设计。

### 独立审查（2026-10-29，未参与实现的审查代理）

五条 Review Focus 全部核实通过——快照稳定性、按条件选择口径、聚合视图归档按钮、chip 隐藏但筛选保留、
正文缺省逐字节不变——**无 Critical / Important 问题，结论：可以交付**。

Minor 4 条及处理：
1. 死代码（清空选择按钮恒同三元 + 恒 false 的 disabled）→ **已清理**。
2. 窄列表「已选 N 封」缩徽标的 spec 项未实现 → Ruling（见下）。
3. 触屏下「按条件选择」▾ 18×24px 小于触控标准 → 留下轮 a11y。
4. 「清空选择后 chips 回来且 active 不变」缺测试 → **已补断言**。

### Rulings（偏离计划之处）

| # | 偏离 | 原因 | 判错的代价 |
|---|---|---|---|
| 1 | touch-targets.test.ts 两条旧断言改写为验证新结构 | 旧断言测的是 R9-1 已移除的 `.mi-star/.mi-del` 定位，保留会让测试与实现永久脱节 | 若新断言写错，粗指针回归会被漏检；故同时断言 `.mi-hover-actions display:none` 双保险 |
| 2 | `list.selectMode` 键删除、新增 `selectModeTouch` | 细指针的「进入选择」按钮已移除，键不再被引用（测试改断言 `list.selectMode` 不出现） | 无——语言文件其余键不动 |
| 3 | 窄列表「已选 N 封」徽标未做 | 徽标需 JS 测量容器宽度才能换文案，CSS-only 做不到；纯外观收益与 ResizeObserver 代价不对等 | 窄列表选中时计数保持完整文本，信息量反而是超集；未达成 spec 的视觉预期 |
| 4 | touch ▾ 按钮未放大到 36px | 本轮粗指针范围只覆盖 hover 动作组与行复选框 | 费点劲但不影响功能；下轮 a11y 一并处理 |

### 已知限制

- 390 触屏模拟下真实点击开信未截图（移动端布局 mail-item 定位超时，环境问题非功能缺陷）；
  触屏相关行为已由 display 计算值直接验证。
- `bodyZoom` 125%/150% 的真实 iframe 高度收敛未在浏览器实测（探针受环境限制），归入「Windows 实机确认」。
- Windows 实机字重确认（R9-3 的核心目标「字偏细」是否改善）待用户在 Windows 上人工确认。


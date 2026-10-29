# FlyMail 第九轮 UI/UE 打磨 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 列表行悬停动作、选择栏对齐重做、字体字重层级、外观与正文阅读设置（含实时预览）。

**Architecture:** 全部在 `flymail/frontend`。判定逻辑抽成 `src/lib/` 纯函数配 vitest
（`archive-target` / `select-by` / `appearance-prefs`）；行内三处重复的删除+星标按钮收成一个
`RowHoverActions` 组件；外观偏好沿用 `privacy-prefs` 的「localStorage + 订阅」模式，
通过 `<html>` 上的 `data-*` 属性与 `zoom` 落地；正文排版经 `buildFrameDocument` 的新可选参数注入 iframe。
CSS 结构约束沿用「读 `index.css` 原文断言」的套路，放进新文件 `lib/ui-polish-r9.test.ts`。

**Tech Stack:** React 19 + TypeScript + Tailwind v4（仅令牌桥接，样式主体是 `src/index.css` 手写类）+ radix DropdownMenu（`DropMenu`）+ @tanstack/react-virtual + vitest/jsdom + i18next。

**Spec:** `docs/flymail/ui-polish-r9.md`

## 〇、交接说明（给独立执行的代理，先读完这一节）

本计划写给**没有参与设计讨论**的代理执行。下面是执行所需的全部背景，不依赖任何会话记忆。

**仓库与阅读顺序**
- 仓库根：`/home/dufeng/develop/workspace/go_dev/MailDev`（monorepo：`core/`、`flymail/`、`mail2im/`）。本轮只动 `flymail/frontend/`，外加本计划与 spec 两份文档。
- 先读 spec `docs/flymail/ui-polish-r9.md`（为什么做、做成什么样、不做什么），再读本计划。
  背景参考（可选）：`docs/flymail/ref-seekli-ui.md`（借鉴对象的走查记录）、`docs/flymail/ui-polish-r8.md`（上一轮的做法与验收记录）。
- spec 是权威；计划与 spec 冲突时按 spec，并在 ledger 记 `Ruling:`。

**用户的硬性规则（违反任何一条都要停下来）**
- 与用户沟通用**中文**；代码注释沿用仓库风格（中文、解释「为什么」）。
- **不提交 git**，除非用户明确要求。若用户要求提交：提交信息用中文；**禁止** `git add -A` / `git add .` / `git commit -a`，
  一律 `git commit -F - --only -- <本轮改动的路径…>`；**禁止** `git stash` 与 `git reset --hard`，补救只用 `git reset --mixed`。
  原因：这个仓库常有其它会话并发修改，整体暂存会把别人的未提交工作卷进来。
- 开工前跑一次 `git status --short` 存快照（`$SCRATCH/r9-git-start.txt`）。快照里已经是修改状态的文件不属于你；
  若本轮也要改其中某个文件，先告诉用户。
- 包管理器 `pnpm`；**不要运行 `pnpm dev`**（用户自己管开发服务器）。
- 任何测试服务只监听 `127.0.0.1`，不用 `0.0.0.0`。
- 新文档只放 `docs/flymail/`。

**临时目录**：自建 `SCRATCH=$(mktemp -d /tmp/flymail-r9-XXXX)`，日志、截图脚本、截图都放这里，不进仓库。
截图脚本需要 `playwright-core`：在 `$SCRATCH` 里 `pnpm add playwright-core`（或 `npm i playwright-core`），
浏览器用已装好的 `~/.cache/ms-playwright/chromium-1228/chrome-linux64/chrome`（`executablePath` 指定，`headless: true`）。

**本机测试实例（截图与探针用）**
- Docker 容器 `flymail`（镜像 `flymail:local`），compose 工作目录就是仓库根。访问 `http://127.0.0.1:8086`，账号 `admin`，
  密码 `flymail-local-test`（不对的话看仓库根 `.env` 的 `FLYMAIL_ADMIN_PASS`）。里面已有若干测试账户与邮件（含 Gmail 账户与本地冒烟账户）。
- 改完前端要看效果必须重建镜像（前端产物 embed 进 Go 二进制）：在仓库根 `FLYMAIL_BIND=127.0.0.1 docker compose up -d --build`
  （前端层有改动时约 1–3 分钟）。
- **静态缓存坑**：重建后先确认服务的是新资源——`curl -s http://127.0.0.1:8086/ | grep -o 'assets/[^"]*\.\(js\|css\)'`
  取文件名，再 `curl -s http://127.0.0.1:8086/assets/<名字> | grep -c <本轮新加的类名>` ≥ 1。浏览器上下文每次新建，别复用旧缓存。
- 截图前的登录片段：

```js
await page.goto(BASE + '/login', { waitUntil: 'networkidle' })
await page.locator('input').first().fill('admin')
await page.locator('input[type="password"]').fill('flymail-local-test')
await page.locator('button[type="submit"]').click()
await page.waitForURL((u) => !u.pathname.startsWith('/login'))
await page.waitForTimeout(1500)
```

**这个前端已知的坑（前几轮踩过，执行时对照）**
1. `tsc --noEmit` 对这个 solution 式 tsconfig 是**空检查**，类型判据只认 `pnpm build`（内含 `tsc -b`）。
2. `src/lib/touch-targets.test.ts` 取 `index.css` 里**第一处** `@media (pointer: coarse)` 且不剥注释——新规则要并进文件末尾那个总块，注释里不能出现这个媒体查询的字面量。
3. 测试里 mock 的 `useTranslation` 常常没有 `i18n` 字段；组件新用到 `i18n.language` 时要给相关测试的 mock 补上。
4. radix 菜单在捕获阶段处理 Esc 并 `preventDefault`，但不阻止冒泡；全局快捷键钩子开头已有 `if (e.defaultPrevented) return`，新加的键盘处理照此办理。
5. `DropMenu` 的 `disabled` 必须传给 `DropMenu` 本身（它透传给 radix Trigger），写在子按钮上 radix 不认。
6. 裸写的元素级 CSS 重置会压过 Tailwind 的 `@layer utilities`；新增元素级规则一律放 `@layer base`（见 `index.css` 第 440 行附近的注释）。
7. `.col.list` / `.col.reader` / `.reader-slide` 是 CSS 容器（`list` / `reader`），窄栏规则写 `@container`，不加视口断点。
8. 侧栏只给**展开/激活**的账户取文件夹；`['folders', accountId]` 是共享缓存键。
9. 第八轮遗留的已知失败：`MailList.test.tsx › 分组打乱顺序后，方向键打开的是相邻那一行本身`（动手前就失败，与本轮无关）。

**执行方式**
- 按任务顺序做，每个任务内按步骤先写测试并看它失败，再实现。Task 1 是探针，结论决定 Task 7–10 的范围，必须先做。
- 进度记在 `$SCRATCH/ledger.md`（每完成一个任务一行：任务号、测试命令与结果、偏离计划的 `Ruling:`）。上下文被压缩后以 ledger 和磁盘上的实际文件为准，不要重做已完成的任务。
- 全部任务完成后按 Task 11 做全量验证与独立审查。

**交付时向用户汇报（中文）**
- 每个任务的完成状态与测试结果（数字），与基线的对比；
- 探针结论（uiScale / bodyZoom 是否保留）；
- 所有 `Ruling:`（偏离计划之处、原因、判错的代价）；
- 审查发现及处理，未修的 Minor 列表；
- 截图目录路径与发现的视觉问题；
- 改动文件清单（`git status --short` 与开工快照的差集），明确说明**未提交**。

## Global Constraints

- 工作目录 `flymail/frontend`（下文相对路径 `src/…` 都以它为根）；包管理器 `pnpm`；**不要自己跑 `pnpm dev`**。浏览器验证与重建方法见第〇节。
- **不提交 git**，除非用户明确要求（提交规则见第〇节）。每个任务结束只跑验证。
- 所有用户可见文案走 i18n：`src/locales/zh.json` 与 `en.json` 同步加键；改完 `node -e "JSON.parse(require('fs').readFileSync('src/locales/zh.json','utf8'));JSON.parse(require('fs').readFileSync('src/locales/en.json','utf8'))"` 校验。`src/locales/keys.test.ts` 扫描源码 `t('...')` 字面量比对语言文件——**动态拼接的键它看不见**，一律写字面量。
- 读 CSS 原文只能用 `node:fs`（vitest `css: false`），文件顶部 `/// <reference types="node" />`，**先剥注释再匹配**。
- 新增的 `@media (pointer: coarse)` 规则必须并进 `index.css` 末尾那个总块（`touch-targets.test.ts` 取第一处且不剥注释），注释里不能写这个媒体查询的字面量。
- 新的视口断点只能是 1024 / 768 / 480（`ui-polish-r8.test.ts › R8-10` 钉着）；窄栏规则写 `@container list|reader`。
- 不改 `.mail-item` 行高、`list-density.ts` 常量与 `estimateSize`。
- 判据用 `pnpm build`（`tsc -b`），不是 `tsc --noEmit`。
- 基线：动手前跑一次 `pnpm test > $SCRATCH/r9-base-test.log 2>&1; tail -30` 与 `pnpm lint > $SCRATCH/r9-base-lint.log 2>&1; tail -5`，记录既有失败与 lint 数；本轮判据是**不新增**。
- 测试命令：单文件 `pnpm exec vitest run <file>`；全量 `pnpm test`。

## Review Focus

1. **`useSyncExternalStore` 的快照稳定性**：`getAppearance()` 若每次返回新对象，React 会无限重渲染（error #185 已在本项目出现过一次）。→ Task 7 单测断言两次调用返回同一引用、写入后引用才变。
2. **按条件选择只作用于已加载的可见行**，且「无」= 清空、不是保留旧选择；会话模式与单封模式各用各的 id 类型（字符串/数字不得混用）。→ Task 4 单测覆盖两种行类型。
3. **聚合视图下的归档按钮**：A 账户有归档、B 账户没有；B 的行不得出现归档按钮；已在归档文件夹里的单封不出现。→ Task 2 单测 + Task 3 组件测试。
4. **选中后筛选 chip 隐藏但筛选状态保留**：清空选择后 chip 回来且 `active` 状态不变；选中时列表数据源不能被改变。→ Task 5 组件测试。
5. **正文 iframe 缺省输出逐字节不变**：没有自定义正文偏好的用户，srcDoc 与不传 `bodyStyle` 时一致（避免无意改变全部邮件渲染）。→ Task 8 单测。

---

### Task 1: 探针——`zoom` 在真实浏览器里是否可用（决定 uiScale / bodyZoom 的去留）

**Files:** 无仓库改动；脚本 `$SCRATCH/shot/r9-zoom-spike.mjs`；结论写进 spec §5.4 末尾「探针结论」。

**Interfaces:**
- Produces: 两个布尔结论 `UI_SCALE_OK`、`BODY_ZOOM_OK`，Task 7/8/9/10 按它们决定是否实现对应项。

- [ ] **Step 1: 写探针脚本**

```js
import { chromium } from 'playwright-core'
const EXE = '/home/dufeng/.cache/ms-playwright/chromium-1228/chrome-linux64/chrome'
const BASE = 'http://127.0.0.1:8086'
const browser = await chromium.launch({ executablePath: EXE, headless: true })
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'zh-CN' })
const page = await ctx.newPage()
await page.goto(BASE + '/login', { waitUntil: 'networkidle' })
await page.locator('input').first().fill('admin')
await page.locator('input[type="password"]').fill('flymail-local-test')
await page.locator('button[type="submit"]').click()
await page.waitForURL((u) => !u.pathname.startsWith('/login'))
await page.waitForTimeout(1500)
for (const z of ['0.9', '1.2']) {
  await page.evaluate((z) => { document.documentElement.style.zoom = z }, z)
  await page.waitForTimeout(400)
  // 1) 滚到底：最后一行可见、无重叠（相邻行 top 差 == 行高）
  const r = await page.evaluate(async () => {
    const el = document.querySelector('.mail-list')
    el.scrollTop = el.scrollHeight
    await new Promise((res) => setTimeout(res, 600))
    const rows = [...document.querySelectorAll('.mail-item')].map((n) => n.getBoundingClientRect())
    const gaps = rows.slice(1).map((b, i) => Math.round(b.top - rows[i].top))
    const listBox = el.getBoundingClientRect()
    return { n: rows.length, gaps: [...new Set(gaps)], lastBottom: Math.round(rows.at(-1).bottom), listBottom: Math.round(listBox.bottom) }
  })
  console.log('zoom', z, JSON.stringify(r))
  // 2) j/k 键盘：按 j 三次后当前行在视口内
  await page.locator('.mail-list').click({ position: { x: 5, y: 5 } })
  for (let i = 0; i < 3; i++) await page.keyboard.press('j')
  await page.waitForTimeout(300)
  console.log('active visible', await page.evaluate(() => {
    const a = document.querySelector('.mail-item.selected'); if (!a) return 'none'
    const b = a.getBoundingClientRect(), l = document.querySelector('.mail-list').getBoundingClientRect()
    return b.top >= l.top - 1 && b.bottom <= l.bottom + 1
  }))
  // 3) 右键菜单贴着鼠标
  const row = page.locator('.mail-item').nth(1); const box = await row.boundingBox()
  await page.mouse.click(box.x + 40, box.y + 10, { button: 'right' })
  await page.waitForTimeout(300)
  const mb = await page.locator('[role="menu"]').first().boundingBox()
  console.log('ctx menu offset', mb && Math.round(mb.x - (box.x + 40)), mb && Math.round(mb.y - (box.y + 10)))
  await page.keyboard.press('Escape')
  await page.screenshot({ path: `out/r9-spike-zoom-${z}.png` })
  // 4) 正文 iframe 内 zoom：打开一封，给 iframe 文档设 zoom 1.5，比较 iframe 高度与内容高度
  await page.locator('.mail-item').first().click()
  await page.waitForTimeout(1200)
  const f = page.frames().find((fr) => fr !== page.mainFrame())
  if (f) {
    await f.evaluate(() => { document.documentElement.style.zoom = '1.5' })
    await page.waitForTimeout(800)
    const h = await page.evaluate(() => Math.round(document.querySelector('.reader iframe, iframe').getBoundingClientRect().height))
    const inner = await f.evaluate(() => Math.round(document.documentElement.getBoundingClientRect().height))
    console.log('iframe height', h, 'inner(zoomed) height', inner)
    await page.screenshot({ path: `out/r9-spike-body-${z}.png` })
  }
}
await browser.close()
```

- [ ] **Step 2: 运行并判定**

Run: `mkdir -p $SCRATCH/shot/out && cd $SCRATCH/shot && node r9-zoom-spike.mjs`（截图写到 `$SCRATCH/shot/out/`）
Expected（全部满足即 `UI_SCALE_OK=true`）：两种 zoom 下 `gaps` 只有一个值（行高恒定、无重叠）、`lastBottom <= listBottom`、`active visible true`、右键菜单偏移在 ±4px 内。
`BODY_ZOOM_OK=true` 的判据：`iframe height` 与 `inner(zoomed) height` 相差 ≤ 4px（注入脚本会在 ResizeObserver 回调里重报高度；若不等，再等 1s 重测一次）。看两张截图确认无截断。

- [ ] **Step 3: 把结论写进 spec**

在 `docs/flymail/ui-polish-r9.md` §5.4 末尾追加：

```markdown
**探针结论（YYYY-MM-DD，Chromium 1228）：** uiScale：<可用 / 不可用，失败项与数值>；bodyZoom：<可用 / 不可用>。
```

若任一为「不可用」：对应偏好键从 Task 7 的类型与选项里删除、Task 9/10 不渲染该控件，spec 表格同步删行，ledger 记 Ruling。

---

### Task 2: `archive-target`——归档可用性的单一出口

**Files:**
- Create: `src/lib/archive-target.ts`
- Test: `src/lib/archive-target.test.ts`
- Modify: `src/pages/Shell.tsx`（约 1121–1128 行的 `archiveFolder` / `canArchiveCurrent`）

**Interfaces:**
- Produces:
  - `archiveFolderOf(folders: Folder[]): Folder | null`
  - `archiveTargetForMessage(foldersByAccount: Map<number, Folder[]>, msg: { account_id: number; folder_id: number }): number | null`
  - `archiveTargetForThread(foldersByAccount: Map<number, Folder[]>, item: { account_id: number }): number | null`
  （返回目标文件夹 id；null = 不提供归档）

- [ ] **Step 1: 写失败测试**

```ts
import { describe, it, expect } from 'vitest'
import { archiveFolderOf, archiveTargetForMessage, archiveTargetForThread } from './archive-target'
import type { Folder } from './types'

function f(id: number, account_id: number, type: string, selectable = true): Folder {
  return { id, account_id, path: String(id), display_name: String(id), type, selectable, total_count: 0, unread_count: 0, sort_order: id }
}

describe('archive-target', () => {
  const byAcct = new Map<number, Folder[]>([
    [1, [f(10, 1, 'inbox'), f(11, 1, 'archive')]],
    [2, [f(20, 2, 'inbox')]],
    [3, [f(30, 3, 'archive', false)]],
  ])

  it('取可选的归档文件夹，不可选的不算', () => {
    expect(archiveFolderOf(byAcct.get(1)!)?.id).toBe(11)
    expect(archiveFolderOf(byAcct.get(3)!)).toBeNull()
  })

  it('聚合视图：每行按自己的账户判断', () => {
    expect(archiveTargetForMessage(byAcct, { account_id: 1, folder_id: 10 })).toBe(11)
    expect(archiveTargetForMessage(byAcct, { account_id: 2, folder_id: 20 })).toBeNull()
  })

  it('已在归档里的单封不再提供归档', () => {
    expect(archiveTargetForMessage(byAcct, { account_id: 1, folder_id: 11 })).toBeNull()
  })

  it('账户文件夹尚未加载时不提供（缺省隐藏而不是点了报错）', () => {
    expect(archiveTargetForMessage(byAcct, { account_id: 9, folder_id: 1 })).toBeNull()
    expect(archiveTargetForThread(byAcct, { account_id: 9 })).toBeNull()
  })

  it('会话只看账户有无归档文件夹', () => {
    expect(archiveTargetForThread(byAcct, { account_id: 1 })).toBe(11)
    expect(archiveTargetForThread(byAcct, { account_id: 2 })).toBeNull()
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/archive-target.test.ts`
Expected: FAIL，`Failed to resolve import "./archive-target"`

- [ ] **Step 3: 实现**

```ts
// 归档可用性的单一出口：阅读区工具条（Shell.canArchiveCurrent）与列表行悬停按钮共用。
//
// 两处各自推导过一次就会漂移——例如一处忘了排除 \Noselect 的归档文件夹，
// 按钮亮着、点下去服务器拒绝。
import type { Folder } from './types'

/** 账户的归档文件夹（必须可选）；没有返回 null */
export function archiveFolderOf(folders: Folder[]): Folder | null {
  return folders.find((f) => f.type === 'archive' && f.selectable) ?? null
}

/**
 * 单封邮件的归档目标。账户文件夹没加载到（map 里没有）时返回 null：
 * 宁可暂时不显示按钮，也不显示一个点了才报错的按钮。
 */
export function archiveTargetForMessage(
  foldersByAccount: Map<number, Folder[]>,
  msg: { account_id: number; folder_id: number },
): number | null {
  const folders = foldersByAccount.get(msg.account_id)
  if (!folders) return null
  const target = archiveFolderOf(folders)
  if (target == null || target.id === msg.folder_id) return null
  return target.id
}

/** 会话的归档目标：会话跨文件夹，「整条已在归档」不成立，只看账户有无归档文件夹 */
export function archiveTargetForThread(
  foldersByAccount: Map<number, Folder[]>,
  item: { account_id: number },
): number | null {
  const folders = foldersByAccount.get(item.account_id)
  if (!folders) return null
  return archiveFolderOf(folders)?.id ?? null
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `pnpm exec vitest run src/lib/archive-target.test.ts`
Expected: PASS 5/5

- [ ] **Step 5: Shell 改用同一函数**

`src/pages/Shell.tsx`：

```ts
import { archiveFolderOf } from '@/lib/archive-target'
// …
const archiveFolder = archiveFolderOf(currentFolders)
```

（删除原来的 `currentFolders.find((f) => f.type === 'archive' && f.selectable) ?? null`；`canArchiveCurrent` 表达式不变。）

Run: `pnpm build 2>&1 | tail -5`
Expected: 无 TS 错误。

---

### Task 3: 列表行悬停动作（R9-1）

**Files:**
- Modify: `src/components/mail/MailList.tsx`（`CardRow` / `CompactRow` / `ThreadRow`、Props、行渲染处约 1850–1905 行）
- Modify: `src/pages/Shell.tsx`（`<MailList>` 传新 props；新增 `onArchiveOne` / `onArchiveThread`；构建 `foldersByAccount`）
- Modify: `src/index.css`（`.mi-del` / `.mi-star` 段约 1083–1112 行替换为 `.mi-hover-actions`）
- Modify: `src/locales/zh.json`、`en.json`（`list.archive`、`list.thread.archive`）
- Test: `src/components/mail/MailList.test.tsx`（新 describe）、`src/lib/ui-polish-r9.test.ts`（新建）

**Interfaces:**
- Consumes: Task 2 的 `archiveTargetForMessage` / `archiveTargetForThread`。
- Produces（MailList 新 props）:
  - `archiveTargetOf: (msg: MessageListItem) => number | null`
  - `archiveTargetOfThread: (item: ThreadListItem) => number | null`
  - `onArchiveMessage: (id: number, folderId: number) => void`
  - `onArchiveThread: (item: ThreadListItem, folderId: number) => void`
- Produces（组件）: `RowHoverActions`（MailList.tsx 内部函数组件），props 见 Step 3。

- [ ] **Step 1: 写失败的组件测试**

在 `MailList.test.tsx` 的 `base` 里补四个新 props 的缺省值：

```ts
  archiveTargetOf: () => null,
  archiveTargetOfThread: () => null,
  onArchiveMessage: noop,
  onArchiveThread: noop,
```

新增 describe（`mount` 与 beforeEach/afterEach 照抄「MailList 的数据状态出口」那段）：

```ts
describe('列表行悬停动作', () => {
  // …mount / beforeEach / afterEach 同上…

  function actionsOf(row: Element) {
    return [...row.querySelectorAll('.mi-hover-actions button')].map((b) => b.getAttribute('aria-label'))
  }

  it('有归档目标时四个动作齐全，顺序为 归档/删除/已读切换/星标', async () => {
    await mount({ archiveTargetOf: () => 99 })
    const row = container.querySelector('.mail-item')!
    expect(actionsOf(row)).toEqual(['list.archive', 'reader.delete', 'ctx.markUnread', 'ctx.star'])
  })

  it('没有归档目标的行不出现归档按钮', async () => {
    await mount({ archiveTargetOf: (m) => (m.id === 1 ? 99 : null) })
    const rows = container.querySelectorAll('.mail-item')
    expect(actionsOf(rows[0])).toContain('list.archive')
    expect(actionsOf(rows[1])).not.toContain('list.archive')
  })

  it('未读行的切换按钮是「标为已读」，并把目标值传出去', async () => {
    const calls: [number, boolean][] = []
    const m = { ...base.messages[0], seen: false }
    await mount({ messages: [m], onMarkRead: (id, read) => calls.push([id, read]) })
    const btn = container.querySelector<HTMLButtonElement>('.mi-hover-actions button[aria-label="ctx.markRead"]')!
    await act(async () => btn.click())
    expect(calls).toEqual([[1, true]])
  })

  it('点归档传出目标文件夹，且不打开邮件', async () => {
    const archived: [number, number][] = []
    const opened: number[] = []
    await mount({
      archiveTargetOf: () => 99,
      onArchiveMessage: (id, fid) => archived.push([id, fid]),
      onSelectMessage: (id: number) => opened.push(id),
    })
    const btn = container.querySelector<HTMLButtonElement>('.mi-hover-actions button[aria-label="list.archive"]')!
    await act(async () => btn.click())
    expect(archived).toEqual([[1, 99]])
    expect(opened).toEqual([])
  })

  it('紧凑行与会话行同样渲染动作组', async () => {
    await mount({ listStyle: 'compact', archiveTargetOf: () => 99 })
    expect(actionsOf(container.querySelector('.mail-item')!)).toHaveLength(4)
  })

  it('动作按钮不进 Tab 序列（键盘走快捷键）', async () => {
    await mount({ archiveTargetOf: () => 99 })
    const btns = container.querySelectorAll<HTMLButtonElement>('.mi-hover-actions button')
    expect([...btns].every((b) => b.tabIndex === -1)).toBe(true)
  })
})
```

> 注：`onSelectMessage` 的真实签名以 Props 为准（若带第二个参数，测试回调照收即可）。

新建 `src/lib/ui-polish-r9.test.ts`（头部 helper 与 `ui-polish-r8.test.ts` 第 1–21 行相同）：

```ts
describe('R9-1 悬停动作组', () => {
  it('细指针下默认隐藏、行悬停显示；时间同步淡出', () => {
    const m = [...stripped.matchAll(/@media \(hover: hover\) and \(pointer: fine\)\s*\{([\s\S]*?)\n\}/g)].map((x) => x[1]).join('\n')
    expect(m).toMatch(/\.mi-hover-actions\s*\{[^}]*opacity:\s*0/)
    expect(m).toMatch(/\.mail-item:hover \.mi-hover-actions[^{]*\{[^}]*opacity:\s*1/)
    expect(m).toMatch(/\.mail-item:hover \.mi-time[^{]*\{[^}]*opacity:\s*0/)
  })
  it('粗指针下 display:none（看不见的按钮不能可点）', () => {
    const m = stripped.match(/@media\s*\(pointer:\s*coarse\)\s*\{([\s\S]*?)\n\}/)
    expect(m![1]).toMatch(/\.mi-hover-actions\s*\{[^}]*display:\s*none/)
  })
  it('旧的 .mi-del 规则已移除', () => {
    expect(stripped).not.toMatch(/\.mi-del\b/)
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/components/mail/MailList.test.tsx src/lib/ui-polish-r9.test.ts`
Expected: 新增用例 FAIL（找不到 `.mi-hover-actions`；TS 报 Props 多余属性也算失败）。

- [ ] **Step 3: 实现 `RowHoverActions` 并替换三处行内按钮**

在 `MailList.tsx` 的 `SelectBox` 之后加入：

```tsx
// ─────────────────────────────────────────────────────────────────────────────
// 行悬停动作：归档 / 删除 / 已读切换 / 星标
// ─────────────────────────────────────────────────────────────────────────────

/**
 * 悬停时占据时间位的动作组。三种行共用一份，按钮顺序与阅读区工具条一致。
 *
 * 按钮 tabIndex=-1：整份列表只有一个 Tab 停留点（roving），键盘用户有 e/#/Shift+U/s。
 * 粗指针下整组 display:none（见 index.css）——看不见但可点的按钮是第一轮审查抓过的坑。
 * 已标星时星标另有一枚常显的 .mi-star-pin，动作组里的星标照常存在（悬停时覆盖在同一位置）。
 */
function RowHoverActions({
  onArchive,
  onDelete,
  unread,
  onToggleRead,
  flagged,
  onToggleFlag,
  labels,
}: {
  /** null = 不提供归档（账户没有归档文件夹、或已在归档里） */
  onArchive: (() => void) | null
  onDelete: () => void
  unread: boolean
  onToggleRead: () => void
  flagged: boolean
  onToggleFlag: (e: React.MouseEvent) => void
  labels: { archive: string; delete: string; read: string; unread: string; star: string; unstar: string }
}) {
  const stop = (fn: () => void) => (e: React.MouseEvent) => { e.stopPropagation(); fn() }
  return (
    <span className="mi-hover-actions">
      {onArchive && (
        <button type="button" tabIndex={-1} className="icon-btn" onClick={stop(onArchive)}
          aria-label={labels.archive} title={withShortcut(labels.archive, KEY.archive)}>
          <Icon name="archive" size={15} />
        </button>
      )}
      <button type="button" tabIndex={-1} className="icon-btn mha-danger" onClick={stop(onDelete)}
        aria-label={labels.delete} title={withShortcut(labels.delete, KEY.deleteHash)}>
        <Icon name="trash" size={15} />
      </button>
      <button type="button" tabIndex={-1} className="icon-btn" onClick={stop(onToggleRead)}
        aria-label={unread ? labels.read : labels.unread} title={unread ? labels.read : labels.unread}>
        <Icon name={unread ? 'check' : 'mail'} size={15} />
      </button>
      <button type="button" tabIndex={-1} className={'icon-btn' + (flagged ? ' starred' : '')}
        onClick={(e) => { e.stopPropagation(); onToggleFlag(e) }}
        aria-label={flagged ? labels.unstar : labels.star} title={withShortcut(flagged ? labels.unstar : labels.star, KEY.star)}>
        <Icon name={flagged ? 'star-fill' : 'star'} size={15} />
      </button>
    </span>
  )
}
```

在文件顶部 import：`import { KEY, withShortcut } from '@/lib/shortcuts'`（若已 import 则合并）。

三种行组件的 props 各加：

```ts
  /** 归档；null = 本行不提供 */
  onArchive: (() => void) | null
  onToggleRead: () => void
```

`CardRow`：删掉原 `mi-del` 与 `mi-star` 两个按钮，在原位置放：

```tsx
      {msg.flagged && <span className="mi-star-pin" aria-hidden="true"><Icon name="star-fill" size={13} /></span>}
      <RowHoverActions
        onArchive={onArchive}
        onDelete={onDelete}
        unread={isUnread}
        onToggleRead={onToggleRead}
        flagged={msg.flagged}
        onToggleFlag={onToggleFlag}
        labels={{
          archive: t('list.archive'), delete: t('reader.delete'),
          read: t('ctx.markRead'), unread: t('ctx.markUnread'),
          star: t('ctx.star'), unstar: t('ctx.unstar'),
        }}
      />
```

`CompactRow`：删掉末列 `<span style={{ display: 'inline-flex' … }}>…</span>` 整段，替换为同样的 `mi-star-pin` + `RowHoverActions`（标签同上）。
**注意网格列数**：`.mail-item-row` 是 6 列（`28px sender 1fr auto auto auto`），末列原来是删除+星标。保留末列占位：`<span className="mi-pin-col">{msg.flagged && <span className="mi-star-pin" …/>}</span>`，`RowHoverActions` 是绝对定位，不占网格列。

`ThreadRow`：紧凑与卡片两支各自删掉 `mi-del` + `mi-star`，替换为 `mi-star-pin` + `RowHoverActions`，标签用会话版：

```ts
labels={{
  archive: t('list.thread.archive'), delete: t('list.thread.delete'),
  read: t('list.thread.readAll'), unread: t('list.thread.unreadAll'),
  star: t('list.thread.starAll'), unstar: t('list.thread.unstarAll'),
}}
unread={isUnread}
flagged={item.flagged}
```

行渲染处（约 1850–1905 行）为三种行传新 props：

```tsx
// ThreadRow
onArchive={(() => { const fid = archiveTargetOfThread(row.item); return fid == null ? null : () => onArchiveThread(row.item, fid) })()}
onToggleRead={() => onMarkReadThread(row.item, row.item.unread > 0)}
// CompactRow / CardRow
onArchive={(() => { const fid = archiveTargetOf(row.msg); return fid == null ? null : () => onArchiveMessage(row.msg.id, fid) })()}
onToggleRead={() => onMarkRead(row.msg.id, !row.msg.seen)}
```

Props 接口在 `onDeleteMessage` 之后加四个新字段（带 JSDoc，说明 null 语义与「由 Shell 按账户文件夹判定」）。

- [ ] **Step 4: CSS**

删除 `index.css` 中 `.mi-star { … }` 起到 `.mi-del:hover { … }` 止的整段（约 1083–1112 行，含两段 `@media (hover: hover) and (pointer: fine)`），替换为：

```css
/* 行悬停动作组：占据时间位（卡片行右上角 / 紧凑行右端），绝对定位不占网格列，
   悬停不引起任何重排。细指针下与时间交叉淡入；粗指针下 display:none（见文件末尾总块）。 */
.mi-hover-actions {
  position: absolute; right: 12px; top: 10px;
  display: inline-flex; align-items: center; gap: 2px;
  padding-left: 8px;
  /* 遮住下面的时间/附件标签：用行当前底色而不是透明 */
  background: inherit;
}
.mail-item.mail-item-row .mi-hover-actions { top: 50%; transform: translateY(-50%); right: 14px; }
.mi-hover-actions .icon-btn { width: 26px; height: 26px; color: var(--ink-3); }
.mi-hover-actions .icon-btn.starred { color: #e0b04a; }
.mail-item .mi-time, .mail-item .mi-time-col { transition: opacity 0.1s; }
.mi-star-pin { position: absolute; right: 14px; bottom: 12px; color: #e0b04a; display: inline-flex; }
.mail-item.mail-item-row .mi-star-pin { position: static; }
@media (hover: hover) and (pointer: fine) {
  .mi-hover-actions { opacity: 0; pointer-events: none; transition: opacity 0.1s; }
  .mail-item:hover .mi-hover-actions { opacity: 1; pointer-events: auto; }
  .mail-item:hover .mi-time, .mail-item:hover .mi-time-col { opacity: 0; }
  .mi-hover-actions .icon-btn:hover { color: var(--ink); background: var(--bg-active); }
  .mi-hover-actions .icon-btn.mha-danger:hover { color: var(--destructive); }
}
@media (prefers-reduced-motion: reduce) {
  .mi-hover-actions, .mail-item .mi-time, .mail-item .mi-time-col { transition: none; }
}
```

`.mail-item` 的底色来自 `.selected` / `.batch-selected` / `:hover`，`background: inherit` 取的是父元素——**父元素是行本身**，所以生效。若截图发现悬停时遮罩是透明（行 hover 色画在伪元素上的情况），改为 `background: var(--bg)` 加 `.mail-item:hover .mi-hover-actions { background: var(--bg-hover) }` 的不透明组合色（`--bg-hover` 是半透明，需 `background: linear-gradient(var(--bg-hover), var(--bg-hover)), var(--bg)`）。

在文件末尾 `@media (pointer: coarse)` 总块里追加：

```css
  .mi-hover-actions { display: none; }
```

- [ ] **Step 5: i18n**

`zh.json`：`list.archive: "归档"`，`list.thread.archive: "归档整个会话"`。
`en.json`：`list.archive: "Archive"`，`list.thread.archive: "Archive conversation"`。
运行 Global Constraints 里的 JSON 校验命令。

- [ ] **Step 6: Shell 接线**

```ts
import { archiveTargetForMessage, archiveTargetForThread } from '@/lib/archive-target'
import { useFoldersOfAccounts } from '@/lib/queries'
// …组件内，accounts 之后：
const accountIds = useMemo(() => accounts.filter((a) => a.enabled).map((a) => a.id), [accounts])
const { folders: allFolders } = useFoldersOfAccounts(accountIds)
const foldersByAccount = useMemo(() => {
  const m = new Map<number, Folder[]>()
  for (const f of allFolders) {
    const arr = m.get(f.account_id)
    if (arr) arr.push(f)
    else m.set(f.account_id, [f])
  }
  return m
}, [allFolders])

function onArchiveOne(id: number, folderId: number) {
  runUndoable({
    message: t('reader.archivedToast'),
    ids: [id],
    commit: () => moveOne.mutate({ id, folderId }),
  })
}
function onArchiveThread(item: ThreadListItem, folderId: number) {
  runUndoable({
    message: t('reader.archivedToast'),
    threadIds: [item.thread_id],
    commit: () => threadMove.mutate({ threadIds: [item.thread_id], folderId, inFolderId: threadScope }),
  })
}
```

> ⚠ `useFoldersOfAccounts` 复用 `['folders', id]` 缓存。侧栏只给**展开/激活**账户取文件夹（第〇节坑 8），这里会对所有账户发起请求，所以只取启用的账户：`accounts.filter((a) => a.enabled).map((a) => a.id)`（`Account.enabled: boolean`）。这是本任务唯一新增的网络请求，写进 ledger。

`<MailList>` 传：

```tsx
archiveTargetOf={(m) => archiveTargetForMessage(foldersByAccount, m)}
archiveTargetOfThread={(it) => archiveTargetForThread(foldersByAccount, it)}
onArchiveMessage={onArchiveOne}
onArchiveThread={onArchiveThread}
```

- [ ] **Step 7: 跑测试**

Run: `pnpm exec vitest run src/components/mail/MailList.test.tsx src/lib/ui-polish-r9.test.ts src/lib/touch-targets.test.ts src/lib/ui-polish-r8.test.ts && pnpm build 2>&1 | tail -5`
Expected: 新增用例全过；MailList 既有失败只有基线那一条；build 无错。

---

### Task 4: `select-by`——按条件选择

**Files:**
- Create: `src/lib/select-by.ts`
- Test: `src/lib/select-by.test.ts`

**Interfaces:**
- Produces:
  - `type SelectCondition = 'all' | 'none' | 'read' | 'unread' | 'flagged' | 'unflagged'`
  - `selectMessagesBy(list: MessageListItem[], c: SelectCondition): number[]`
  - `selectThreadsBy(list: ThreadListItem[], c: SelectCondition): string[]`
  - `selectionState(selected: number, visible: number): 'none' | 'some' | 'all'`

- [ ] **Step 1: 写失败测试**

```ts
import { describe, it, expect } from 'vitest'
import { selectMessagesBy, selectThreadsBy, selectionState } from './select-by'
import type { MessageListItem, ThreadListItem } from './types'

const m = (id: number, seen: boolean, flagged: boolean) => ({ id, seen, flagged }) as MessageListItem
const th = (thread_id: string, unread: number, flagged: boolean) => ({ thread_id, unread, flagged }) as ThreadListItem

describe('selectMessagesBy', () => {
  const list = [m(1, true, false), m(2, false, true), m(3, false, false)]
  it.each([
    ['all', [1, 2, 3]],
    ['none', []],
    ['read', [1]],
    ['unread', [2, 3]],
    ['flagged', [2]],
    ['unflagged', [1, 3]],
  ] as const)('%s', (c, ids) => {
    expect(selectMessagesBy(list, c)).toEqual(ids)
  })
})

describe('selectThreadsBy', () => {
  const list = [th('a', 0, false), th('b', 2, true)]
  it('会话未读按 unread > 0 判定，返回字符串 id', () => {
    expect(selectThreadsBy(list, 'unread')).toEqual(['b'])
    expect(selectThreadsBy(list, 'read')).toEqual(['a'])
    expect(selectThreadsBy(list, 'flagged')).toEqual(['b'])
  })
})

describe('selectionState', () => {
  it('三态', () => {
    expect(selectionState(0, 5)).toBe('none')
    expect(selectionState(2, 5)).toBe('some')
    expect(selectionState(5, 5)).toBe('all')
    expect(selectionState(0, 0)).toBe('none')
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/select-by.test.ts`
Expected: FAIL，模块不存在。

- [ ] **Step 3: 实现**

```ts
// 列表「按条件选择」的判定。只作用于已加载的可见行——与「全选当前可见」同口径：
// 未翻到的页不在内存里，选中它们意味着对看不见的邮件做批量操作。
import type { MessageListItem, ThreadListItem } from './types'

export type SelectCondition = 'all' | 'none' | 'read' | 'unread' | 'flagged' | 'unflagged'

function match(c: SelectCondition, unread: boolean, flagged: boolean): boolean {
  switch (c) {
    case 'all': return true
    case 'none': return false
    case 'read': return !unread
    case 'unread': return unread
    case 'flagged': return flagged
    case 'unflagged': return !flagged
  }
}

export function selectMessagesBy(list: MessageListItem[], c: SelectCondition): number[] {
  return list.filter((x) => match(c, !x.seen, x.flagged)).map((x) => x.id)
}

/** 会话：任一成员未读即视为未读（与列表加粗口径一致） */
export function selectThreadsBy(list: ThreadListItem[], c: SelectCondition): string[] {
  return list.filter((x) => match(c, x.unread > 0, x.flagged)).map((x) => x.thread_id)
}

/** 全选框三态 */
export function selectionState(selected: number, visible: number): 'none' | 'some' | 'all' {
  if (selected === 0 || visible === 0) return 'none'
  return selected >= visible ? 'all' : 'some'
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `pnpm exec vitest run src/lib/select-by.test.ts`
Expected: PASS（8 项）

---

### Task 5: 选择栏重做 + 悬停复选框（R9-2）

**Files:**
- Modify: `src/components/mail/MailList.tsx`（工具栏 1386–1530 行；`selecting` 推导；Props 新增 `onReplaceSelection` / `onReplaceSelectionThread`）
- Modify: `src/pages/Shell.tsx`（传两个新 props）
- Modify: `src/index.css`（`.list-toolbar` / `.lt-*` 段约 893–910；`.mi-select` 段约 1008–1016；末尾 coarse 总块）
- Modify: `src/locales/zh.json`、`en.json`
- Test: `MailList.test.tsx`、`ui-polish-r9.test.ts`

**Interfaces:**
- Consumes: Task 4 的 `selectMessagesBy` / `selectThreadsBy` / `selectionState` / `SelectCondition`。
- Produces（MailList 新 props）:
  - `onReplaceSelection: (ids: number[]) => void`
  - `onReplaceSelectionThread: (ids: string[]) => void`

- [ ] **Step 1: 写失败的组件测试**

`base` 补：`onReplaceSelection: noop, onReplaceSelectionThread: noop,`

```ts
describe('选择栏', () => {
  // …mount / beforeEach / afterEach…
  const toolbar = () => container.querySelector('.list-toolbar')!

  it('未选中：最左是全选框，其后是筛选 chips', async () => {
    await mount()
    const first = toolbar().firstElementChild!
    expect(first.classList.contains('lt-select')).toBe(true)
    expect(toolbar().querySelector('.lt-chips')).toBeTruthy()
    expect(toolbar().querySelector('.lt-actions')).toBeNull()
  })

  it('选中后：chips 隐藏，出现计数与操作组；筛选状态不丢', async () => {
    await mount({ selectedIds: new Set([1]), filter: { ...EMPTY_FILTER, unread: true } })
    expect(toolbar().querySelector('.lt-chips')).toBeNull()
    expect(toolbar().querySelector('.lt-count')?.textContent).toBe('list.selectedCount')
    expect(toolbar().querySelector('.lt-actions')).toBeTruthy()
  })

  it('部分选中时全选框是 indeterminate', async () => {
    await mount({ selectedIds: new Set([1]) })
    const cb = toolbar().querySelector<HTMLInputElement>('.lt-select input[type=checkbox]')!
    expect(cb.indeterminate).toBe(true)
    expect(cb.checked).toBe(false)
  })

  it('全选框：非全选时点击 = 全选可见；全选时点击 = 清空', async () => {
    let all = 0, cleared = 0
    await mount({ onSelectAllVisible: () => all++, onClearSelection: () => cleared++ })
    const cb = () => toolbar().querySelector<HTMLInputElement>('.lt-select input')!
    await act(async () => cb().click())
    expect(all).toBe(1)
    await mount({ selectedIds: new Set([1, 2]), onSelectAllVisible: () => all++, onClearSelection: () => cleared++ })
    await act(async () => cb().click())
    expect(cleared).toBe(1)
  })

  it('细指针下不再有单独的「进入选择模式」按钮', async () => {
    await mount()
    const labels = [...toolbar().querySelectorAll('button')].map((b) => b.getAttribute('aria-label'))
    expect(labels).not.toContain('list.selectMode')
    // 粗指针用的那一枚由 CSS 控制显隐，DOM 上带 .lt-select-touch 类
    expect(toolbar().querySelector('.lt-select-touch')).toBeTruthy()
  })
})
```

按条件选择走 radix 下拉，jsdom 下打开菜单需要 pointer 事件，验证放到截图回归；判定逻辑已由 Task 4 覆盖。

`ui-polish-r9.test.ts` 追加：

```ts
describe('R9-2 选择栏对齐', () => {
  it('工具栏左内边距与行复选框列共用 --select-col-x', () => {
    expect(block('.list-toolbar')).toMatch(/padding-left:\s*var\(--select-col-x\)/)
    expect(block('.mail-item')).toMatch(/padding:\s*14px\s+20px/)
    expect(stripped).toMatch(/--select-col-x:\s*20px/)
  })
  it('无选中时复选框仅在悬停行覆盖头像出现', () => {
    const m = [...stripped.matchAll(/@media \(hover: hover\) and \(pointer: fine\)\s*\{([\s\S]*?)\n\}/g)].map((x) => x[1]).join('\n')
    expect(m).toMatch(/\.mail-list:not\(\.selecting\) \.mail-item:hover \.mi-select-hover[^{]*\{[^}]*display:\s*flex/)
  })
  it('粗指针下显示选择开关', () => {
    const m = stripped.match(/@media\s*\(pointer:\s*coarse\)\s*\{([\s\S]*?)\n\}/)
    expect(m![1]).toMatch(/\.lt-select-touch\s*\{[^}]*display:\s*inline-flex/)
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/components/mail/MailList.test.tsx src/lib/ui-polish-r9.test.ts`
Expected: 新增「选择栏」「R9-2」用例 FAIL。

- [ ] **Step 3: 重写工具栏 JSX**

用以下结构替换 `<div className="list-toolbar">…</div>` 整段（批量操作按钮组的内部 JSX——已读/未读/星标/移动 DropMenu/删除——原样保留，只移动位置）：

```tsx
      <div className="list-toolbar">
        {/* 全选框固定在最左，与行复选框同一竖列（--select-col-x）。
            它本身就是「进入选择」的入口：细指针下不再需要单独的开关。 */}
        <span className="lt-select">
          <label className="lt-all" title={t('list.selectAll')}>
            <input
              ref={selectAllRef}
              type="checkbox"
              checked={selState === 'all'}
              onChange={() => (selState === 'all' ? exitSelectMode() : onSelectAllVisible())}
              aria-label={t('list.selectAll')}
            />
          </label>
          <DropMenu
            align="start"
            trigger={
              <button type="button" className="lt-btn lt-select-by" aria-label={t('list.selectBy')} title={t('list.selectBy')}>
                <Icon name="chevron-down" size={12} />
              </button>
            }
            items={SELECT_CONDITIONS.map((c) => ({
              key: c.id,
              label: t(c.labelKey),
              onSelect: () => applyCondition(c.id),
            }))}
          />
        </span>

        {/* 粗指针（触屏）没有悬停，保留一个显式的「选择」开关；细指针下 CSS 隐藏 */}
        {!alwaysShowSelect && (
          <button
            type="button"
            className={'lt-btn lt-select-touch' + (selecting ? ' active' : '')}
            onClick={toggleSelectMode}
            aria-label={selecting ? t('list.exitSelect') : t('list.selectModeTouch')}
            aria-pressed={selecting}
          >
            <Icon name="check" size={16} />
          </button>
        )}

        {selectedCount === 0 ? (
          <div className="lt-chips">{/* …原 chips 与清除按钮，原样… */}</div>
        ) : (
          <>
            <span className="lt-count">{t('list.selectedCount', { count: selectedCount })}</span>
            <span className="lt-sep" />
            <div className="lt-actions">
              {/* …原 已读 / 未读 / 星标 / 移动 / 删除 按钮原样… */}
            </div>
            <span className="spacer" />
            <button
              type="button" className="lt-btn"
              onClick={exitSelectMode}
              title={t('list.clearSelection')} aria-label={t('list.clearSelection')}
            >
              <Icon name="close" size={16} />
            </button>
          </>
        )}
      </div>
```

组件体内（`selecting` 推导附近）新增：

```tsx
  const visibleCount = threadMode ? threads.length : filtered.length
  const selState = selectionState(selectedCount, visibleCount)
  const selectAllRef = useRef<HTMLInputElement>(null)
  // indeterminate 只能经 DOM 属性设置，没有对应的 HTML 特性
  useEffect(() => {
    if (selectAllRef.current) selectAllRef.current.indeterminate = selState === 'some'
  }, [selState])

  function applyCondition(c: SelectCondition) {
    if (c === 'none') { exitSelectMode(); return }
    if (threadMode) onReplaceSelectionThread(selectThreadsBy(threads, c))
    else onReplaceSelection(selectMessagesBy(filtered, c))
  }
```

模块级常量：

```tsx
/** 按条件选择的菜单项。labelKey 写字面量：keys.test.ts 只认得字面量 t('…') */
const SELECT_CONDITIONS: { id: SelectCondition; labelKey: string }[] = [
  { id: 'all', labelKey: 'list.selectByAll' },
  { id: 'none', labelKey: 'list.selectByNone' },
  { id: 'read', labelKey: 'list.selectByRead' },
  { id: 'unread', labelKey: 'list.selectByUnread' },
  { id: 'flagged', labelKey: 'list.selectByFlagged' },
  { id: 'unflagged', labelKey: 'list.selectByUnflagged' },
]
```

> ⚠ `t(c.labelKey)` 是动态键，`keys.test.ts` 扫不到。为了让它扫到，在 `SELECT_CONDITIONS` 上方加一行注释式的字面量引用不可行（注释不被扫描）——改为把 label 在组件内用字面量求值：`const conditionLabel: Record<SelectCondition, string> = { all: t('list.selectByAll'), none: t('list.selectByNone'), … }`，`SELECT_CONDITIONS` 只保留 id 列表。按这个写法实现。

`selecting` 推导不变（`alwaysShowSelect || selectMode || selectedCount > 0`），它仍控制 `.mail-list.selecting`（独立复选框列）。`selectMode` 现在只由粗指针的 `.lt-select-touch` 打开。

- [ ] **Step 4: 行内悬停复选框（覆盖头像）**

三种行组件在 `.mi-avatar-wrap` 内追加一个悬停复选框（与独立列的 `SelectBox` 并存：一个负责「无选择态悬停」，一个负责「选择态独立列」，CSS 只让其中一个可见）：

```tsx
      <span className="mi-avatar-wrap">
        <div className="avatar-sq" …>…</div>
        <label className="mi-select-hover" onClick={(e) => e.stopPropagation()}>
          <input type="checkbox" tabIndex={-1} checked={selected} onChange={onToggleSelect} aria-hidden="true" />
        </label>
      </span>
```

`aria-hidden` + `tabIndex=-1`：读屏与键盘用户已有独立列 `SelectBox`（带完整 aria-label）与 `x` 快捷键，同一行不出现两个同名复选框。

- [ ] **Step 5: CSS**

替换 `.list-toolbar` 规则的 `padding: 0 14px;` 为：

```css
  padding: 0 14px 0 var(--select-col-x);
```

在 `:root` 通用令牌区（非色调块）加 `--select-col-x: 20px;`（= `.mail-item` 的左内边距；改行内边距必须同改这里，测试钉着）。

新增 / 修改：

```css
.lt-select { display: inline-flex; align-items: center; gap: 0; width: 38px; }
.lt-select .lt-all { width: 20px; justify-content: center; padding: 0; }
.lt-select .lt-select-by { width: 18px; height: 24px; color: var(--ink-3); }
.lt-select-touch { display: none; }
.list-toolbar .spacer { flex: 1 1 auto; }

/* 悬停复选框：覆盖在头像上，只在「无选择态」的被指行出现。
   选择态改用独立列（.mi-select），头像不再被遮挡——第一轮的结论针对的是持续选择态。 */
.mi-select-hover {
  display: none; position: absolute; inset: 0;
  align-items: center; justify-content: center;
  background: var(--surface); border-radius: 6px;
  cursor: pointer;
}
.mi-select-hover input { width: 16px; height: 16px; accent-color: var(--accent); cursor: pointer; }
@media (hover: hover) and (pointer: fine) {
  .mail-list:not(.selecting) .mail-item:hover .mi-select-hover { display: flex; }
}
@container list (max-width: 340px) {
  .lt-count { font-variant-numeric: tabular-nums; }
}
```

删除原 `.lt-chips` 前的 `.lt-sep` 用法不需要改 CSS（类仍在用）。

文件末尾 coarse 总块追加：

```css
  .lt-select-touch { display: inline-flex; }
  .mi-select-hover { display: none !important; }
```

- [ ] **Step 6: i18n**

| 键 | zh | en |
|---|---|---|
| `list.selectBy` | 按条件选择 | Select by |
| `list.selectByAll` | 全部可见邮件 | All visible |
| `list.selectByNone` | 不选 | None |
| `list.selectByRead` | 已读 | Read |
| `list.selectByUnread` | 未读 | Unread |
| `list.selectByFlagged` | 已加星标 | Starred |
| `list.selectByUnflagged` | 未加星标 | Not starred |
| `list.selectModeTouch` | 选择邮件 | Select messages |

`list.selectMode` 若不再被任何源码引用，从两个语言文件删除（`keys.test.ts` 若检查未使用键会报）。

- [ ] **Step 7: Shell 接线**

```tsx
onReplaceSelection={(ids) => setSelectedIds(new Set(ids))}
onReplaceSelectionThread={(ids) => setSelectedThreadIds(new Set(ids))}
```

- [ ] **Step 8: 跑测试 + build**

Run: `pnpm exec vitest run src/components/mail/MailList.test.tsx src/lib/ui-polish-r9.test.ts src/lib/touch-targets.test.ts src/locales && pnpm build 2>&1 | tail -5`
Expected: 新增用例全过；既有失败只剩基线；build 无错。

---

### Task 6: 字体栈、字重令牌与等宽收口（R9-3）

**Files:**
- Modify: `src/index.css`（令牌 165–167 行；`body` 433；`h1-h4` 451；列表行 1017–1060；侧栏 `.folder-row`；38 处 mono）
- Modify: `src/lib/mail-frame.ts`（`MAIL_BODY_CSS` 字体栈）
- Test: `src/lib/ui-polish-r9.test.ts`、`src/lib/mail-frame.test.ts`

**Interfaces:**
- Produces: 令牌 `--fw-regular/--fw-medium/--fw-semibold/--fw-bold`；导出常量 `MONO_ALLOWED`（只在测试文件里，供后续加白名单）。

- [ ] **Step 1: 写失败测试**

`ui-polish-r9.test.ts` 追加：

```ts
describe('R9-3 字体与字重', () => {
  it('字重令牌齐全', () => {
    for (const [k, v] of [['regular', 400], ['medium', 500], ['semibold', 600], ['bold', 700]] as const) {
      expect(stripped).toMatch(new RegExp(`--fw-${k}:\\s*${v}`))
    }
  })
  it('正文字体栈：system-ui 打头，YaHei UI 在 YaHei 之前', () => {
    const m = stripped.match(/--font-body:\s*([^;]+);/)
    expect(m![1].trim().startsWith('system-ui')).toBe(true)
    expect(m![1].indexOf('"Microsoft YaHei UI"')).toBeLessThan(m![1].indexOf('"Microsoft YaHei",'))
  })
  it('标题类用 semibold', () => {
    expect(stripped).toMatch(/h1,\s*h2,\s*h3,\s*h4\s*\{[^}]*font-weight:\s*var\(--fw-semibold\)/)
    expect(block('.list-title')).toMatch(/font-weight:\s*var\(--fw-semibold\)/)
    expect(block('.settings-block h3')).toMatch(/font-weight:\s*var\(--fw-semibold\)/)
  })
  it('已读行发件人与主题是正常黑；未读用 bold / semibold', () => {
    expect(block('.mail-item .mi-sender')).toMatch(/color:\s*var\(--ink\)/)
    expect(block('.mail-item .mi-sender')).toMatch(/font-weight:\s*var\(--fw-medium\)/)
    expect(block('.mail-item .mi-subject')).toMatch(/color:\s*var\(--ink\)/)
    expect(stripped).toMatch(/\.mail-item\.unread \.mi-sender\s*\{[^}]*font-weight:\s*var\(--fw-bold\)/)
    expect(stripped).toMatch(/\.mail-item\.unread \.mi-subject\s*\{[^}]*font-weight:\s*var\(--fw-semibold\)/)
  })
  it('等宽字体只出现在白名单选择器', () => {
    const MONO_ALLOWED = new Set([
      '.compose-btn .kbd', '.search-input .kbd', '.search-help .sh-row code',
      '.reader-empty .shortcuts kbd', '.sc-kbd', '.settings-list-row .slr-mono',
      '.list-error-detail', '.attach-card .ac-ic',
    ])
    const hits: string[] = []
    for (const m of stripped.matchAll(/([^{}]+)\{([^{}]*)\}/g)) {
      if (/font-family:\s*var\(--font-mono\)/.test(m[2])) hits.push(m[1].trim().replace(/\s+/g, ' '))
    }
    expect(hits.filter((s) => !MONO_ALLOWED.has(s))).toEqual([])
  })
  it('数字类改为比例字体 + 等宽数字', () => {
    for (const sel of ['.mail-item .mi-time', '.folder-row .f-count', '.slider-row .slider-val']) {
      expect(block(sel)).toMatch(/font-variant-numeric:\s*tabular-nums/)
      expect(block(sel)).not.toMatch(/--font-mono/)
    }
  })
})
```

`mail-frame.test.ts` 追加：

```ts
  it('正文字体栈与界面同一组中文字体序列', () => {
    expect(MAIL_BODY_CSS).toMatch(/system-ui[^;]*"Microsoft YaHei UI"[^;]*"Microsoft YaHei"[^;]*"Noto Sans SC"/)
  })
```

（`MAIL_BODY_CSS` 已导出；若测试文件未 import，补上。）

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/ui-polish-r9.test.ts src/lib/mail-frame.test.ts`
Expected: R9-3 用例全部 FAIL；mono 用例列出约 30 个越界选择器。

- [ ] **Step 3: 令牌与基础排版**

```css
  --font-display: var(--font-body);
  --font-body: system-ui, -apple-system, "Segoe UI Variable Text", "Segoe UI",
               "PingFang SC", "Microsoft YaHei UI", "Microsoft YaHei",
               "Noto Sans SC", "Noto Sans CJK SC", sans-serif;
  --fw-regular: 400; --fw-medium: 500; --fw-semibold: 600; --fw-bold: 700;
```

> `--font-display` 写在 `--font-body` 前也没关系（自定义属性在使用时才解析）。

`h1,h2,h3,h4 { … font-weight: var(--fw-semibold); … }`

逐个把以下选择器的 `font-weight: 500` 改成 `var(--fw-semibold)`：`.list-title`、`.settings-block h3`、`.reader-empty h2`（实际选择器名以 `grep -n "font-display" index.css` 结果中带 `font-weight: 500` 的标题类为准：786、1186、1570、1587、1621、1914、2245 行附近）。非标题的 500（按钮、标签）保留为 `var(--fw-medium)`。

把文件中剩余的 `font-weight: 600` → `var(--fw-semibold)`、`700` → `var(--fw-bold)`、`400` → `var(--fw-regular)`（`sed` 前先 `grep -n` 核对每处上下文；`@font-face` 若存在不改）。

- [ ] **Step 4: 列表行层级**

```css
.mail-item .mi-sender { font-size: 13.5px; color: var(--ink); font-weight: var(--fw-medium); flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mail-item .mi-time { font-size: 11px; color: var(--ink-3); font-variant-numeric: tabular-nums; flex: 0 0 auto; }
.mail-item .mi-subject { font-size: 13.5px; color: var(--ink); margin-top: 2px; line-height: 19px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mail-item.unread .mi-sender { color: var(--ink); font-weight: var(--fw-bold); }
.mail-item.unread .mi-subject { color: var(--ink); font-weight: var(--fw-semibold); }
.mail-item.unread .mi-preview { color: var(--ink-2); }
.mail-item.unread .mi-time, .mail-item.unread .mi-time-col { color: var(--accent-ink); font-weight: var(--fw-semibold); }
```

（原 `.mail-item.unread .mi-sender, .mail-item.unread .mi-subject` 合并规则删除，拆成上面两条。）
侧栏 `.folder-row` 的文件夹名颜色 `--ink-2` → `--ink`（当前项靠底色区分，不变）。

- [ ] **Step 5: 等宽收口**

对 Step 2 输出里每个越界选择器：删掉 `font-family: var(--font-mono);`，若是数字/时间/计数类加 `font-variant-numeric: tabular-nums;`，字号 < 11px 的改为 11px。邮箱地址类（`.sidebar-foot .me-mail`、`.thread-item .ti-addr`、`.sd-body .ac-mail`）只删 mono，不加 tabular-nums。

- [ ] **Step 6: `MAIL_BODY_CSS` 字体栈**

```ts
  font-family: system-ui, -apple-system, "Segoe UI", "PingFang SC", "Microsoft YaHei UI", "Microsoft YaHei", "Noto Sans SC", Roboto, Helvetica, Arial, sans-serif;
```

> 这会改变缺省 srcDoc（字体栈变了，这是有意的）。Task 8 的「缺省逐字节不变」比的是「传 null」与「不传」，不受本步影响。

- [ ] **Step 7: 跑测试 + build**

Run: `pnpm exec vitest run src/lib && pnpm build 2>&1 | tail -5`
Expected: R9-3 全过；`ui-polish-r8.test.ts`、`theme-tokens.test.ts`、`touch-targets.test.ts` 不回归。

---

### Task 7: `appearance-prefs` + 全局落地（uiScale / 对比度 / 动效）

**Files:**
- Create: `src/lib/appearance-prefs.ts`
- Test: `src/lib/appearance-prefs.test.ts`
- Modify: `src/main.tsx`（`initTheme()` 之后调 `initAppearance()`）
- Modify: `src/index.css`（对比度与动效规则）
- Test: `src/lib/theme-tokens.test.ts`（对比度增强的对比度断言）

**Interfaces:**
- Produces:

```ts
export interface Appearance {
  uiScale: 90 | 100 | 110 | 120
  textContrast: 'standard' | 'high'
  motion: 'system' | 'reduce'
  bodyFontSize: 13 | 14 | 15 | 16 | 18
  bodyLineHeight: 1.4 | 1.6 | 1.8
  readingWidth: 'original' | 'comfort' | 'wide'
  bodyZoom: 90 | 100 | 110 | 125 | 150
}
export const APPEARANCE_DEFAULTS: Appearance
export const APPEARANCE_OPTIONS: { [K in keyof Appearance]: readonly Appearance[K][] }
export function getAppearance(): Appearance          // 快照稳定：同一份数据返回同一引用
export function setAppearance(patch: Partial<Appearance>): void
export function resetAppearance(keys: (keyof Appearance)[]): void
export function subscribeAppearance(fn: () => void): () => void
export function applyAppearance(a: Appearance, root?: HTMLElement): void
export function initAppearance(): void
```

（Task 1 判定不可用的键从类型、默认值、选项中删除。）

- [ ] **Step 1: 写失败测试**

```ts
import { describe, it, expect, beforeEach } from 'vitest'
import {
  APPEARANCE_DEFAULTS, getAppearance, setAppearance, resetAppearance,
  subscribeAppearance, applyAppearance,
} from './appearance-prefs'

const KEY = 'flymail_appearance'

describe('appearance-prefs', () => {
  beforeEach(() => { localStorage.clear(); setAppearance({}) /* 刷新快照 */ })

  it('未存过返回默认值', () => {
    expect(getAppearance()).toEqual(APPEARANCE_DEFAULTS)
  })

  it('非法值逐项回落默认，合法项保留', () => {
    localStorage.setItem(KEY, JSON.stringify({ uiScale: 175, textContrast: 'high', bodyZoom: 'x' }))
    setAppearance({})
    const a = getAppearance()
    expect(a.uiScale).toBe(100)
    expect(a.textContrast).toBe('high')
    expect(a.bodyZoom).toBe(100)
  })

  it('损坏的 JSON 回落默认而不抛', () => {
    localStorage.setItem(KEY, '{oops')
    setAppearance({})
    expect(getAppearance()).toEqual(APPEARANCE_DEFAULTS)
  })

  it('快照稳定：未写入时两次读取是同一引用（useSyncExternalStore 契约）', () => {
    expect(getAppearance()).toBe(getAppearance())
  })

  it('写入后引用变化、通知订阅者、持久化', () => {
    const before = getAppearance()
    let n = 0
    const off = subscribeAppearance(() => n++)
    setAppearance({ bodyFontSize: 16 })
    off()
    expect(getAppearance()).not.toBe(before)
    expect(getAppearance().bodyFontSize).toBe(16)
    expect(n).toBe(1)
    expect(JSON.parse(localStorage.getItem(KEY)!).bodyFontSize).toBe(16)
  })

  it('resetAppearance 只重置指定键', () => {
    setAppearance({ bodyFontSize: 16, uiScale: 110 })
    resetAppearance(['bodyFontSize'])
    expect(getAppearance().bodyFontSize).toBe(14)
    expect(getAppearance().uiScale).toBe(110)
  })

  it('applyAppearance 写 data 属性与 zoom；默认值时不留残余', () => {
    const el = document.createElement('div')
    applyAppearance({ ...APPEARANCE_DEFAULTS, uiScale: 110, textContrast: 'high', motion: 'reduce' }, el)
    expect(el.style.zoom).toBe('1.1')
    expect(el.dataset.contrast).toBe('high')
    expect(el.dataset.motion).toBe('reduce')
    applyAppearance(APPEARANCE_DEFAULTS, el)
    expect(el.style.zoom).toBe('')
    expect(el.dataset.contrast).toBeUndefined()
    expect(el.dataset.motion).toBeUndefined()
  })
})
```

`theme-tokens.test.ts` 追加（复用该文件的 `themeBlocks()` / `luminance()`）：

```ts
describe('文字对比增强', () => {
  /** 与 CSS 里的 color-mix(in srgb, ink P%, bg) 同一算法 */
  function mix(a: string, b: string, p: number): string {
    const ch = (h: string, i: number) => parseInt(h.replace('#', '').slice(i, i + 2), 16)
    return '#' + [0, 2, 4].map((i) => Math.round(ch(a, i) * p + ch(b, i) * (1 - p)).toString(16).padStart(2, '0')).join('')
  }
  const ratio = (x: string, y: string) => {
    const [l1, l2] = [luminance(x), luminance(y)].sort((p, q) => q - p)
    return (l1 + 0.05) / (l2 + 0.05)
  }
  it('规则存在，且选择器权重高于色调块', () => {
    expect(css).toMatch(/:root\[data-contrast="high"\]\[data-theme\]\[data-mode\]\s*\{/)
  })
  it('所有色调亮暗下，增强后的 ink-2 ≥ 7:1、ink-3 ≥ 4.5:1', () => {
    const p2 = Number(/--ink-2:\s*color-mix\(in srgb, var\(--ink\) (\d+)%/.exec(css)![1]) / 100
    const p3 = Number(/--ink-3:\s*color-mix\(in srgb, var\(--ink\) (\d+)%/.exec(css)![1]) / 100
    for (const [name, v] of themeBlocks()) {
      if (!v.ink?.startsWith('#') || !v.bg?.startsWith('#')) continue
      expect(ratio(mix(v.ink, v.bg, p2), v.bg), `${name} ink-2`).toBeGreaterThanOrEqual(7)
      expect(ratio(mix(v.ink, v.bg, p3), v.bg), `${name} ink-3`).toBeGreaterThanOrEqual(4.5)
    }
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/appearance-prefs.test.ts src/lib/theme-tokens.test.ts`
Expected: FAIL（模块不存在 / 规则不存在）。

- [ ] **Step 3: 实现 `appearance-prefs.ts`**

```ts
// 外观与正文阅读偏好：只存本设备（与栏宽一致——大屏的设置不该污染手机）。
//
// 一个 JSON 存全部键，而不是每项一个 localStorage 键：这些偏好总是成组读写
// （外观预览要同时读五项），一个键也让「恢复默认」与损坏回落只有一处逻辑。
//
// 快照必须缓存：useSyncExternalStore 用 Object.is 比较 getSnapshot 的返回值，
// 每次 JSON.parse 出新对象会让订阅组件无限重渲染（React error #185）。

const STORAGE_KEY = 'flymail_appearance'

export interface Appearance {
  uiScale: 90 | 100 | 110 | 120
  textContrast: 'standard' | 'high'
  motion: 'system' | 'reduce'
  bodyFontSize: 13 | 14 | 15 | 16 | 18
  bodyLineHeight: 1.4 | 1.6 | 1.8
  readingWidth: 'original' | 'comfort' | 'wide'
  bodyZoom: 90 | 100 | 110 | 125 | 150
}

export const APPEARANCE_DEFAULTS: Appearance = {
  uiScale: 100,
  textContrast: 'standard',
  motion: 'system',
  bodyFontSize: 14,
  bodyLineHeight: 1.6,
  readingWidth: 'original',
  bodyZoom: 100,
}

export const APPEARANCE_OPTIONS: { [K in keyof Appearance]: readonly Appearance[K][] } = {
  uiScale: [90, 100, 110, 120],
  textContrast: ['standard', 'high'],
  motion: ['system', 'reduce'],
  bodyFontSize: [13, 14, 15, 16, 18],
  bodyLineHeight: [1.4, 1.6, 1.8],
  readingWidth: ['original', 'comfort', 'wide'],
  bodyZoom: [90, 100, 110, 125, 150],
}

const listeners = new Set<() => void>()
let snapshot: Appearance | null = null

function read(): Appearance {
  let raw: Record<string, unknown> = {}
  try {
    const s = localStorage.getItem(STORAGE_KEY)
    if (s) {
      const v: unknown = JSON.parse(s)
      if (v && typeof v === 'object') raw = v as Record<string, unknown>
    }
  } catch {
    /* 存储不可用或 JSON 损坏：整体回落默认 */
  }
  const out = { ...APPEARANCE_DEFAULTS }
  for (const k of Object.keys(APPEARANCE_DEFAULTS) as (keyof Appearance)[]) {
    const opts = APPEARANCE_OPTIONS[k] as readonly unknown[]
    if (opts.includes(raw[k])) (out as Record<string, unknown>)[k] = raw[k]
  }
  return out
}

export function getAppearance(): Appearance {
  if (snapshot == null) snapshot = read()
  return snapshot
}

function write(next: Appearance) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(next))
  } catch {
    /* 不持久化，本次会话内仍生效 */
  }
  snapshot = next
  applyAppearance(next)
  for (const fn of Array.from(listeners)) fn()
}

/** 合并写入。空 patch 用于从存储重新载入（测试与跨标签页同步用） */
export function setAppearance(patch: Partial<Appearance>): void {
  snapshot = null
  write({ ...getAppearance(), ...patch })
}

export function resetAppearance(keys: (keyof Appearance)[]): void {
  const patch: Partial<Appearance> = {}
  for (const k of keys) (patch as Record<string, unknown>)[k] = APPEARANCE_DEFAULTS[k]
  setAppearance(patch)
}

export function subscribeAppearance(fn: () => void): () => void {
  listeners.add(fn)
  return () => { listeners.delete(fn) }
}

/**
 * 把界面级偏好落到根元素上。默认值不写属性：没改过设置的用户 DOM 与以前完全一致。
 * 正文级偏好（body*、readingWidth）不在这里——它们注入邮件 iframe，见 mail-frame.ts。
 */
export function applyAppearance(a: Appearance, root: HTMLElement = document.documentElement): void {
  root.style.zoom = a.uiScale === 100 ? '' : String(a.uiScale / 100)
  if (a.textContrast === 'high') root.dataset.contrast = 'high'
  else delete root.dataset.contrast
  if (a.motion === 'reduce') root.dataset.motion = 'reduce'
  else delete root.dataset.motion
}

export function initAppearance(): void {
  applyAppearance(getAppearance())
  // 另一个标签页改了设置：重新载入并通知本页订阅者
  window.addEventListener('storage', (e) => {
    if (e.key === STORAGE_KEY) setAppearance({})
  })
}
```

> 测试 `beforeEach` 里的 `setAppearance({})` 会写一次存储——`localStorage.clear()` 之后它写入的是默认值，不影响「未存过」用例的断言（断言比的是值）。

- [ ] **Step 4: CSS**

在色调块之后加：

```css
/* 文字对比增强：一条规则覆盖全部色调，把次级文字向 --ink 收拢。
   用 srgb 混色而不是 oklab：theme-tokens.test.ts 要按同一算法逐色调算对比度。
   选择器带 [data-theme][data-mode] 是为了权重高过色调块（两个属性选择器）。 */
:root[data-contrast="high"][data-theme][data-mode] {
  --ink-2: color-mix(in srgb, var(--ink) 88%, var(--bg));
  --ink-3: color-mix(in srgb, var(--ink) 72%, var(--bg));
}
```

（百分比先按此值跑测试；不达标则上调，记 ledger。）

动效：找到 `index.css` 中现有的 `@media (prefers-reduced-motion: reduce)` 块（611、2570、3078 行），在每一块的规则选择器前再加一组 `:root[data-motion="reduce"] …` 的同义规则（复制规则体，选择器前缀化），不改原块。

- [ ] **Step 5: `main.tsx`**

```ts
import { initAppearance } from '@/lib/appearance-prefs'
// …
initTheme()
initAppearance()
```

- [ ] **Step 6: 跑测试 + build**

Run: `pnpm exec vitest run src/lib/appearance-prefs.test.ts src/lib/theme-tokens.test.ts && pnpm build 2>&1 | tail -5`
Expected: PASS；build 无错。

---

### Task 8: 正文排版注入 iframe

**Files:**
- Modify: `src/lib/mail-frame.ts`（`FrameDocumentOptions`、`buildFrameDocument`）
- Modify: `src/components/mail/MessageBody.tsx`（订阅 appearance）
- Test: `src/lib/mail-frame.test.ts`、`src/components/mail/MessageBody.test.tsx`

**Interfaces:**
- Consumes: Task 7 的 `getAppearance` / `subscribeAppearance` / `APPEARANCE_DEFAULTS`。
- Produces:

```ts
export interface BodyStyle { fontSize: number; lineHeight: number; maxWidth: number | null; zoom: number }
export function bodyStyleFrom(a: Pick<Appearance, 'bodyFontSize' | 'bodyLineHeight' | 'readingWidth' | 'bodyZoom'>): BodyStyle | null
// FrameDocumentOptions 新增 bodyStyle?: BodyStyle | null
```

- [ ] **Step 1: 写失败测试**

`mail-frame.test.ts`：

```ts
describe('正文排版偏好', () => {
  const opts = { html: '<p>x</p>', allowRemote: false, foldQuote: false, quoteHideCss: '', nonce: null, token: null }

  it('缺省（bodyStyle 为空）与不传完全相同', () => {
    expect(buildFrameDocument({ ...opts, bodyStyle: null })).toBe(buildFrameDocument(opts))
  })

  it('全部默认值时 bodyStyleFrom 返回 null（不注入任何东西）', () => {
    expect(bodyStyleFrom({ bodyFontSize: 14, bodyLineHeight: 1.6, readingWidth: 'original', bodyZoom: 100 })).toBeNull()
  })

  it('注入字号、行高、阅读宽度与缩放', () => {
    const s = bodyStyleFrom({ bodyFontSize: 16, bodyLineHeight: 1.8, readingWidth: 'comfort', bodyZoom: 125 })!
    const doc = buildFrameDocument({ ...opts, bodyStyle: s })
    expect(doc).toMatch(/body\s*\{\s*font-size:\s*16px;\s*line-height:\s*1\.8;\s*max-width:\s*720px;\s*margin:\s*0 auto;\s*\}/)
    expect(doc).toMatch(/html\s*\{\s*zoom:\s*1\.25;\s*\}/)
  })

  it('不改写邮件内联样式：不出现 !important 与通配选择器', () => {
    const s = bodyStyleFrom({ bodyFontSize: 18, bodyLineHeight: 1.4, readingWidth: 'wide', bodyZoom: 150 })!
    const injected = buildFrameDocument({ ...opts, bodyStyle: s }).replace(buildFrameDocument(opts), '')
    expect(buildFrameDocument({ ...opts, bodyStyle: s })).not.toMatch(/\*\s*\{[^}]*font-size/)
    expect(injected).not.toMatch(/!important/)
  })
})
```

（`bodyStyleFrom` 从 `mail-frame.ts` 导出；文件顶部 import 补上。）

`MessageBody.test.tsx` 新增 describe「正文排版偏好」：沿用该文件 `MessageBody 的附件区` 的 mount 写法（QueryClient + Confirm/Toast provider、`MockAdapter`），
用 `detailWith([])` 并补上 `html_body: '<p>hi</p>'`（字段名以 `MessageDetail` 为准）渲染；
断言 `container.querySelector('iframe')!.getAttribute('srcdoc')` 不含 `font-size: 18px`，
然后 `await act(async () => setAppearance({ bodyFontSize: 18 }))`，再断言包含。
`afterEach` 里 `localStorage.clear(); setAppearance({})`，免得污染其它用例。

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/mail-frame.test.ts src/components/mail/MessageBody.test.tsx`
Expected: FAIL（`bodyStyleFrom` 未导出）。

- [ ] **Step 3: 实现**

`mail-frame.ts`：

```ts
import type { Appearance } from './appearance-prefs'

/** 正文排版偏好（设置 → 阅读 → 邮件正文）。null = 全默认，不注入 */
export interface BodyStyle { fontSize: number; lineHeight: number; maxWidth: number | null; zoom: number }

const READING_WIDTH_PX = { original: null, comfort: 720, wide: 960 } as const

export function bodyStyleFrom(
  a: Pick<Appearance, 'bodyFontSize' | 'bodyLineHeight' | 'readingWidth' | 'bodyZoom'>,
): BodyStyle | null {
  if (a.bodyFontSize === 14 && a.bodyLineHeight === 1.6 && a.readingWidth === 'original' && a.bodyZoom === 100) return null
  return { fontSize: a.bodyFontSize, lineHeight: a.bodyLineHeight, maxWidth: READING_WIDTH_PX[a.readingWidth], zoom: a.bodyZoom / 100 }
}

/**
 * 只写 body 基准与 html 缩放：邮件自带的内联字号不覆盖——覆盖会把营销邮件的版式打散。
 * 想整体放大的用户用 zoom。放在 MAIL_BODY_CSS 之后，靠层叠顺序覆盖同名声明。
 */
function bodyStyleCss(s: BodyStyle): string {
  const width = s.maxWidth == null ? '' : ` max-width: ${s.maxWidth}px; margin: 0 auto;`
  const zoom = s.zoom === 1 ? '' : `html { zoom: ${s.zoom}; }`
  return `body { font-size: ${s.fontSize}px; line-height: ${s.lineHeight};${width} }${zoom}`
}
```

`FrameDocumentOptions` 加：

```ts
  /** 正文排版偏好；null/缺省 = 不注入（输出与没有这个参数时逐字节相同） */
  bodyStyle?: BodyStyle | null
```

`buildFrameDocument` 的 style 串改为：

```ts
    `<style>${MAIL_BODY_CSS}${o.darkBody ? MAIL_BODY_DARK_CSS : ''}` +
    `${o.bodyStyle ? bodyStyleCss(o.bodyStyle) : ''}` +
    `${o.foldQuote ? o.quoteHideCss : ''}</style>` +
```

> `bodyStyleCss` 输出的 body 规则 `font-size: 16px; line-height: 1.8; max-width: 720px; margin: 0 auto;`——测试正则依赖这个顺序与空格，实现与测试一致即可。`max-width` 与 MAIL_BODY_CSS 的 `margin: 0; padding: 14px 16px` 叠加：`margin: 0 auto` 覆盖 `margin: 0`，padding 保留。

> `appearance-prefs` → `mail-frame` 的类型依赖只用 `import type`，不引入运行时环。

`MessageBody.tsx`：

```ts
import { getAppearance, subscribeAppearance } from '@/lib/appearance-prefs'
import { bodyStyleFrom } from '@/lib/mail-frame'
// …在 darkBody 推导之后：
  // 正文排版偏好同样随 srcDoc 一次性注入，必须订阅：设置页里拨动滑块，
  // 已打开的那封要立即跟着变。
  const appearance = useSyncExternalStore(subscribeAppearance, getAppearance, () => getAppearance())
  const bodyStyle = useMemo(() => bodyStyleFrom(appearance), [appearance])
// buildFrameDocument({... darkBody, bodyStyle })
```

（`useMemo` 依赖 `appearance` 引用：快照稳定，只在写入后变化。`useMemo` 若未 import 则补上。）

- [ ] **Step 4: 跑测试 + build**

Run: `pnpm exec vitest run src/lib/mail-frame.test.ts src/components/mail/MessageBody.test.tsx src/components/mail/ReaderTranslate.test.tsx && pnpm build 2>&1 | tail -5`
Expected: PASS；build 无错。

---

### Task 9: 外观页重组 + 迷你实时预览（R9-4 §5.2）

**Files:**
- Create: `src/components/settings/AppearancePreview.tsx`
- Modify: `src/components/settings/sections/AppearanceSection.tsx`
- Modify: `src/index.css`（`.appearance-preview` 段，放在 `.theme-card` 相关样式之后）
- Modify: `src/locales/zh.json`、`en.json`
- Test: `src/components/settings/AppearanceSection.test.tsx`（新建）

**Interfaces:**
- Consumes: Task 7 的 `getAppearance` / `setAppearance` / `resetAppearance` / `subscribeAppearance` / `APPEARANCE_OPTIONS`；既有 `getTheme`、`LayoutMode`、`ListStyle`。
- Produces: `AppearancePreview({ tone, mode, listStyle, layoutMode, appearance })`。

- [ ] **Step 1: 写失败测试**

```tsx
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { AppearanceSection } from './sections/AppearanceSection'
import { getAppearance, setAppearance } from '@/lib/appearance-prefs'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k, i18n: { language: 'zh', changeLanguage: () => Promise.resolve() } }),
}))
;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('外观页', () => {
  let container: HTMLDivElement
  let root: Root
  beforeEach(() => {
    localStorage.clear(); setAppearance({})
    container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container)
  })
  afterEach(async () => { await act(async () => root.unmount()); container.remove() })

  async function mount() {
    await act(async () => root.render(
      <AppearanceSection listStyle="card" onChangeListStyle={() => {}} alwaysShowSelect={false}
        onChangeAlwaysShowSelect={() => {}} layoutMode="three" onChangeLayoutMode={() => {}} />,
    ))
  }

  it('顶部有预览，且挂着当前色调与亮暗', async () => {
    await mount()
    const p = container.querySelector('.appearance-preview')!
    expect(p).toBeTruthy()
    expect(p.getAttribute('data-theme')).toBeTruthy()
    expect(p.getAttribute('data-mode')).toMatch(/light|dark/)
  })

  it('切换文字对比：写入偏好，预览同步 data-contrast', async () => {
    await mount()
    const btn = [...container.querySelectorAll('button')].find((b) => b.textContent === 'settings.appearance.contrastHigh')!
    await act(async () => btn.click())
    expect(getAppearance().textContrast).toBe('high')
    expect(container.querySelector('.appearance-preview')!.getAttribute('data-contrast')).toBe('high')
  })

  it('界面字号四档', async () => {
    await mount()
    const group = container.querySelector('[data-pref="uiScale"]')!
    expect([...group.querySelectorAll('button')].map((b) => b.textContent)).toEqual(['90%', '100%', '110%', '120%'])
  })

  it('「恢复默认」只重置本组', async () => {
    setAppearance({ textContrast: 'high', bodyFontSize: 16 })
    await mount()
    const reset = container.querySelector<HTMLButtonElement>('[data-reset="text"]')!
    await act(async () => reset.click())
    expect(getAppearance().textContrast).toBe('standard')
    expect(getAppearance().bodyFontSize).toBe(16)
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/components/settings/AppearanceSection.test.tsx`
Expected: FAIL（无 `.appearance-preview`）。

- [ ] **Step 3: `AppearancePreview.tsx`**

```tsx
// 外观页顶部的迷你三栏预览。
//
// 用真实的类名与令牌渲染（.mail-item / .avatar-sq / .folder-row…），只在外层 scale 缩小：
// 色板与排版只有 index.css 一份，预览不另画一套，改了样式预览自动跟上。
// data-theme / data-mode / data-contrast 挂在预览根上，令牌在子树内重新生效（同 ThemeCard 的做法）。
import { useTranslation } from 'react-i18next'
import type { ThemeMode, ToneId } from '@/lib/theme'
import type { ListStyle } from '@/lib/list-prefs'
import type { LayoutMode } from '@/lib/layout-mode'
import type { Appearance } from '@/lib/appearance-prefs'

export function AppearancePreview({ tone, mode, listStyle, layoutMode, appearance }: {
  tone: ToneId; mode: ThemeMode; listStyle: ListStyle; layoutMode: LayoutMode; appearance: Appearance
}) {
  const { t } = useTranslation()
  const rows = [
    { from: t('settings.appearance.pvFrom1'), subject: t('settings.appearance.pvSubject1'), time: '09:41', unread: true, hue: 3 },
    { from: t('settings.appearance.pvFrom2'), subject: t('settings.appearance.pvSubject2'), time: '昨天', unread: false, hue: 6 },
    { from: t('settings.appearance.pvFrom3'), subject: t('settings.appearance.pvSubject3'), time: '10/12', unread: false, hue: 1 },
  ]
  return (
    <div
      className="appearance-preview"
      data-theme={tone}
      data-mode={mode}
      data-contrast={appearance.textContrast === 'high' ? 'high' : undefined}
      data-layout={layoutMode}
      aria-hidden="true"
    >
      <div className="ap-canvas" style={{ zoom: appearance.uiScale / 100 }}>
        <div className="ap-side">
          {[t('folder.inbox'), t('folder.sent'), t('folder.archive')].map((n, i) => (
            <div key={n} className={'folder-row' + (i === 0 ? ' active' : '')}>{n}</div>
          ))}
        </div>
        <div className="ap-list">
          {rows.map((r) => (
            <div key={r.subject} className={'mail-item' + (listStyle === 'compact' ? ' mail-item-row' : '') + (r.unread ? ' unread' : '')}>
              <span className="mi-unread-dot" />
              <span className="mi-avatar-wrap"><div className="avatar-sq" data-hue={r.hue}>{r.from.slice(0, 1)}</div></span>
              <div style={{ minWidth: 0 }}>
                <div className="mi-top"><span className="mi-sender">{r.from}</span><span className="mi-time">{r.time}</span></div>
                <div className="mi-subject">{r.subject}</div>
              </div>
            </div>
          ))}
        </div>
        <div className="ap-reader">
          <div className="ap-title">{rows[0].subject}</div>
          <div className="ap-line" /><div className="ap-line" style={{ width: '85%' }} /><div className="ap-line" style={{ width: '60%' }} />
        </div>
      </div>
    </div>
  )
}
```

> ⚠ `t('folder.inbox')` 这类字面量会被 R8-4 测试（禁止 `` t(`folder.${…}`) `` 模板串）放过，因为它是字面量；但要确认 `folder.inbox / folder.sent / folder.archive` 键存在（`grep '"folder"' -A 12 src/locales/zh.json`）。时间串「昨天」不能写死中文：改用 `t('settings.appearance.pvYesterday')`。

- [ ] **Step 4: 重组 `AppearanceSection`**

- 订阅偏好：`const appearance = useSyncExternalStore(subscribeAppearance, getAppearance, getAppearance)`。
- 在 return 的最前面渲染 `<AppearancePreview tone={currentTone} mode={currentMode} listStyle={listStyle} layoutMode={layoutMode} appearance={appearance} />`。
- 块顺序：主题（色调卡 + 亮暗切换并进同一块）→ 布局（布局模式 + 三条栏宽 + 列表密度 + 选择框常显）→ 文字与界面（新）→ 语言。
- 「文字与界面」块：

```tsx
      <div className="settings-block">
        <div className="sb-head">
          <h3>{t('settings.appearance.textTitle')}</h3>
          <button type="button" className="link-btn" data-reset="text"
            onClick={() => resetAppearance(['uiScale', 'textContrast', 'motion'])}>
            {t('settings.appearance.reset')}
          </button>
        </div>
        <Row label={t('settings.appearance.uiScale')} help={t('settings.appearance.uiScaleHint')}>
          <div className="mode-toggle" data-pref="uiScale">
            {APPEARANCE_OPTIONS.uiScale.map((v) => (
              <button key={v} type="button" className={appearance.uiScale === v ? 'active' : ''}
                onClick={() => setAppearance({ uiScale: v })}>{v}%</button>
            ))}
          </div>
        </Row>
        <Row label={t('settings.appearance.contrast')} help={t('settings.appearance.contrastHint')}>
          <div className="mode-toggle" data-pref="textContrast">
            <button type="button" className={appearance.textContrast === 'standard' ? 'active' : ''}
              onClick={() => setAppearance({ textContrast: 'standard' })}>{t('settings.appearance.contrastStandard')}</button>
            <button type="button" className={appearance.textContrast === 'high' ? 'active' : ''}
              onClick={() => setAppearance({ textContrast: 'high' })}>{t('settings.appearance.contrastHigh')}</button>
          </div>
        </Row>
        <Row label={t('settings.appearance.reduceMotion')} help={t('settings.appearance.reduceMotionHint')}>
          <Toggle on={appearance.motion === 'reduce'} onChange={(on) => setAppearance({ motion: on ? 'reduce' : 'system' })} />
        </Row>
      </div>
```

（Task 1 判定 uiScale 不可用时整行不渲染，测试「界面字号四档」相应删除并记 ledger。）

- [ ] **Step 5: CSS**

```css
/* 外观页顶部预览条：sticky 停在滚动区顶部。内容区只有 ~640px 宽，放不下左右两栏。 */
.appearance-preview {
  position: sticky; top: 0; z-index: 2;
  height: 150px; margin: 0 0 8px;
  border: 1px solid var(--rule); border-radius: 10px;
  background: var(--bg); overflow: hidden;
  box-shadow: 0 6px 12px -10px rgba(0,0,0,0.25);
}
.appearance-preview .ap-canvas {
  display: grid; grid-template-columns: 110px 220px 1fr;
  height: 100%; font-size: 12px; pointer-events: none;
}
.appearance-preview[data-layout="two-slide"] .ap-canvas { grid-template-columns: 110px 1fr 160px; }
.appearance-preview .ap-side { background: var(--bg-alt); border-right: 1px solid var(--rule); padding: 8px 6px; }
.appearance-preview .ap-side .folder-row { padding: 3px 8px; font-size: 11px; }
.appearance-preview .ap-list { border-right: 1px solid var(--rule); overflow: hidden; }
.appearance-preview .mail-item { height: auto; padding: 7px 10px; grid-template-columns: 22px 1fr; gap: 8px; }
.appearance-preview .avatar-sq { width: 22px; height: 22px; font-size: 10px; }
.appearance-preview .mi-sender, .appearance-preview .mi-subject { font-size: 11.5px; }
.appearance-preview .mi-top { height: auto; }
.appearance-preview .ap-reader { background: var(--surface); padding: 12px; }
.appearance-preview .ap-title { font-weight: var(--fw-semibold); font-size: 13px; color: var(--ink); margin-bottom: 8px; }
.appearance-preview .ap-line { height: 6px; border-radius: 3px; background: var(--ink-4); opacity: .45; margin: 6px 0; width: 100%; }
.settings-block .sb-head { display: flex; align-items: baseline; justify-content: space-between; }
```

`.link-btn` 仓库里没有，一并新增：

```css
.link-btn { color: var(--accent-ink); font-size: 12.5px; }
@media (hover: hover) and (pointer: fine) { .link-btn:hover { text-decoration: underline; } }
```

- [ ] **Step 6: i18n**

| 键（`settings.appearance.*`） | zh | en |
|---|---|---|
| `textTitle` | 文字与界面 | Text & interface |
| `reset` | 恢复默认 | Reset |
| `uiScale` | 界面字号 | Interface size |
| `uiScaleHint` | 整个界面按比例缩放，只影响本设备 | Scales the whole interface on this device |
| `contrast` | 文字对比 | Text contrast |
| `contrastHint` | 增强后次要文字更深，适合觉得字偏细、偏淡的屏幕 | Darkens secondary text for screens where text looks thin |
| `contrastStandard` | 标准 | Standard |
| `contrastHigh` | 增强 | High |
| `reduceMotion` | 减少动效 | Reduce motion |
| `reduceMotionHint` | 关闭过渡与动画；系统已开启时始终生效 | Turns off transitions; always on when the system asks for it |
| `pvFrom1` / `pvFrom2` / `pvFrom3` | 林晓 / 项目周报 / 账单通知 | Lin Xiao / Weekly report / Billing |
| `pvSubject1` / `pvSubject2` / `pvSubject3` | 周五评审改到下午三点 / 第 42 周进度汇总 / 你的 10 月账单已生成 | Friday review moved to 3 pm / Week 42 summary / Your October bill is ready |
| `pvYesterday` | 昨天 | Yesterday |

JSON 校验。

- [ ] **Step 7: 跑测试 + build**

Run: `pnpm exec vitest run src/components/settings src/locales src/lib/ui-polish-r8.test.ts && pnpm build 2>&1 | tail -5`
Expected: PASS；build 无错。

---

### Task 10: 阅读页「邮件正文」分组 + 正文预览（R9-4 §5.3）

**Files:**
- Create: `src/components/settings/BodyPreview.tsx`
- Modify: `src/components/settings/sections/ReadingSection.tsx`
- Modify: `src/index.css`（`.body-preview`）
- Modify: `src/locales/zh.json`、`en.json`
- Test: `src/components/settings/ReadingSection.test.tsx`（新建）

**Interfaces:**
- Consumes: Task 7 的偏好 API；Task 8 的 `bodyStyleFrom` / `buildFrameDocument`；既有 `MAIL_FRAME_SANDBOX`。

- [ ] **Step 1: 写失败测试**

```tsx
// 骨架同 AppearanceSection.test.tsx（mock i18n、act 环境、mount/unmount）
it('正文分组四项控件齐全，位于会话视图之后、阅读隐私之前', async () => {
  await mount()
  const titles = [...container.querySelectorAll('.settings-block h3')].map((h) => h.textContent)
  expect(titles.indexOf('settings.reading.bodyTitle')).toBe(titles.indexOf('settings.reading.viewTitle') + 1)
  for (const k of ['bodyFontSize', 'bodyLineHeight', 'readingWidth', 'bodyZoom']) {
    expect(container.querySelector(`[data-pref="${k}"]`), k).toBeTruthy()
  }
})

it('拨动字号：写入偏好，预览 iframe 的文档随之更新', async () => {
  await mount()
  const btn = container.querySelector<HTMLButtonElement>('[data-pref="bodyFontSize"] button:last-child')!
  await act(async () => btn.click())
  expect(getAppearance().bodyFontSize).toBe(18)
  expect(container.querySelector('.body-preview iframe')!.getAttribute('srcdoc')).toContain('font-size: 18px')
})

it('预览 iframe 带与阅读区相同的 sandbox', async () => {
  await mount()
  expect(container.querySelector('.body-preview iframe')!.getAttribute('sandbox')).toBe(MAIL_FRAME_SANDBOX)
})
```

`ReadingSection` 内含 `TrustedSendersSection`（用 react-query）：mount 时包 `QueryClientProvider`，或 `vi.mock('@/components/settings/TrustedSendersSection', () => ({ TrustedSendersSection: () => null }))`——用后者，本测试不关心它。

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/components/settings/ReadingSection.test.tsx`
Expected: FAIL。

- [ ] **Step 3: `BodyPreview.tsx`**

```tsx
// 设置 → 阅读里的正文预览：用阅读区同一个 buildFrameDocument 与 sandbox 渲染一段示例邮件，
// 偏好改动即时反映。不注入引导脚本（nonce/token 为 null）：示例内容固定，不需要自适应高度。
import { useTranslation } from 'react-i18next'
import { buildFrameDocument, bodyStyleFrom, MAIL_FRAME_SANDBOX } from '@/lib/mail-frame'
import type { Appearance } from '@/lib/appearance-prefs'

export function BodyPreview({ appearance }: { appearance: Appearance }) {
  const { t } = useTranslation()
  const html =
    `<p>${t('settings.reading.pvGreeting')}</p>` +
    `<p>${t('settings.reading.pvBody1')}</p>` +
    `<p>${t('settings.reading.pvBody2')}</p>` +
    `<p style="color:#57606a">— ${t('settings.reading.pvSign')}</p>`
  return (
    <div className="body-preview">
      <iframe
        title={t('settings.reading.previewTitle')}
        sandbox={MAIL_FRAME_SANDBOX}
        srcDoc={buildFrameDocument({
          html, allowRemote: false, foldQuote: false, quoteHideCss: '',
          nonce: null, token: null, bodyStyle: bodyStyleFrom(appearance),
        })}
      />
    </div>
  )
}
```

> 示例文案来自语言文件（受信内容，随构建打包），直接拼进 HTML；仓库没有 `escapeHtml`，不为此新增。
> 预览 iframe 仍走 `MAIL_FRAME_SANDBOX` 与 CSP，即使文案出错也关在沙箱里。

- [ ] **Step 4: `ReadingSection` 新增分组**

在「会话视图」块之后插入：

```tsx
      <div className="settings-block">
        <div className="sb-head">
          <h3>{t('settings.reading.bodyTitle')}</h3>
          <button type="button" className="link-btn" data-reset="body"
            onClick={() => resetAppearance(['bodyFontSize', 'bodyLineHeight', 'readingWidth', 'bodyZoom'])}>
            {t('settings.appearance.reset')}
          </button>
        </div>
        <p className="help">{t('settings.reading.bodyHelp')}</p>
        <BodyPreview appearance={appearance} />
        <Row label={t('settings.reading.fontSize')}>
          <div className="mode-toggle" data-pref="bodyFontSize">
            {APPEARANCE_OPTIONS.bodyFontSize.map((v) => (
              <button key={v} type="button" className={appearance.bodyFontSize === v ? 'active' : ''}
                onClick={() => setAppearance({ bodyFontSize: v })}>{v}</button>
            ))}
          </div>
        </Row>
        <Row label={t('settings.reading.lineHeight')}>
          <div className="mode-toggle" data-pref="bodyLineHeight">
            <button type="button" className={appearance.bodyLineHeight === 1.4 ? 'active' : ''} onClick={() => setAppearance({ bodyLineHeight: 1.4 })}>{t('settings.reading.lhCompact')}</button>
            <button type="button" className={appearance.bodyLineHeight === 1.6 ? 'active' : ''} onClick={() => setAppearance({ bodyLineHeight: 1.6 })}>{t('settings.reading.lhStandard')}</button>
            <button type="button" className={appearance.bodyLineHeight === 1.8 ? 'active' : ''} onClick={() => setAppearance({ bodyLineHeight: 1.8 })}>{t('settings.reading.lhLoose')}</button>
          </div>
        </Row>
        <Row label={t('settings.reading.width')} help={t('settings.reading.widthHint')}>
          <div className="mode-toggle" data-pref="readingWidth">
            <button type="button" className={appearance.readingWidth === 'original' ? 'active' : ''} onClick={() => setAppearance({ readingWidth: 'original' })}>{t('settings.reading.widthOriginal')}</button>
            <button type="button" className={appearance.readingWidth === 'comfort' ? 'active' : ''} onClick={() => setAppearance({ readingWidth: 'comfort' })}>{t('settings.reading.widthComfort')}</button>
            <button type="button" className={appearance.readingWidth === 'wide' ? 'active' : ''} onClick={() => setAppearance({ readingWidth: 'wide' })}>{t('settings.reading.widthWide')}</button>
          </div>
        </Row>
        <Row label={t('settings.reading.zoom')} help={t('settings.reading.zoomHint')}>
          <div className="mode-toggle" data-pref="bodyZoom">
            {APPEARANCE_OPTIONS.bodyZoom.map((v) => (
              <button key={v} type="button" className={appearance.bodyZoom === v ? 'active' : ''}
                onClick={() => setAppearance({ bodyZoom: v })}>{v}%</button>
            ))}
          </div>
        </Row>
      </div>
```

（Task 1 判定 bodyZoom 不可用时去掉最后一行。）`appearance` 由 `useSyncExternalStore(subscribeAppearance, getAppearance, getAppearance)` 取得。

- [ ] **Step 5: CSS**

```css
.body-preview { border: 1px solid var(--rule); border-radius: 8px; overflow: hidden; margin: 0 0 12px; }
.body-preview iframe { display: block; width: 100%; height: 170px; border: 0; background: #fff; }
```

- [ ] **Step 6: i18n**

| 键（`settings.reading.*`） | zh | en |
|---|---|---|
| `bodyTitle` | 邮件正文 | Message body |
| `bodyHelp` | 只影响邮件正文，与界面字号相互独立；邮件自带的字号与版式不会被改写 | Affects message bodies only, independent of interface size; a message's own fonts and layout are kept |
| `previewTitle` | 正文预览 | Body preview |
| `fontSize` | 正文字号 | Font size |
| `lineHeight` | 行高 | Line height |
| `lhCompact` / `lhStandard` / `lhLoose` | 紧凑 / 标准 / 宽松 | Compact / Standard / Relaxed |
| `width` | 阅读宽度 | Reading width |
| `widthHint` | 限制宽屏下一行的长度，长行不易跟读 | Limits line length on wide screens |
| `widthOriginal` / `widthComfort` / `widthWide` | 原始 / 舒适 / 宽 | Original / Comfortable / Wide |
| `zoom` | 正文缩放 | Body zoom |
| `zoomHint` | 连同邮件自带的字号与图片一起放大，只影响本设备 | Scales the whole message including its own fonts and images, on this device |
| `pvGreeting` | 你好， | Hi, |
| `pvBody1` | 周五的评审改到下午三点，会议室不变。附件里是更新后的议程，请提前看一下第二部分。 | Friday's review moves to 3 pm, same room. The updated agenda is attached — please read part two beforehand. |
| `pvBody2` | 如果时间冲突，回复这封邮件告诉我，我们再协调。 | If the time doesn't work, reply and we'll sort it out. |
| `pvSign` | 林晓 | Lin Xiao |

JSON 校验。

- [ ] **Step 7: 跑测试 + build**

Run: `pnpm exec vitest run src/components/settings src/locales && pnpm build 2>&1 | tail -5`
Expected: PASS；build 无错。

---

### Task 11: 截图回归 + 全量验证 + 独立审查 + 验收记录

**Files:**
- 脚本：`$SCRATCH/shot/r9-verify.mjs`
- Modify: `docs/flymail/ui-polish-r9.md`（追加「第八节 验收记录」）

- [ ] **Step 1: 全量测试与 lint**

Run: `pnpm test > $SCRATCH/r9-test.log 2>&1; tail -30 $SCRATCH/r9-test.log; pnpm lint > $SCRATCH/r9-lint.log 2>&1; tail -5 $SCRATCH/r9-lint.log; pnpm build 2>&1 | tail -5`
Expected: 失败项 ⊆ 基线；lint 错误数 ≤ 基线；build 无错。

- [ ] **Step 2: 重建本机 Docker 实例并确认新资源**

仓库根执行 `FLYMAIL_BIND=127.0.0.1 docker compose up -d --build`；`curl -s http://127.0.0.1:8086/ | grep -o 'assets/[^"]*\.css'` 取 CSS 名，`curl -s http://127.0.0.1:8086/<该路径> | grep -c 'mi-hover-actions'` ≥ 1。

- [ ] **Step 3: 截图脚本**

覆盖（1440×900 / 1024×768 / 390×844 × 亮/暗）：
1. 卡片行与紧凑行悬停（含已标星行、未读行、聚合视图下无归档账户的行）；
2. 选择栏：未选中、选中 2 封（列表宽 380 与 300 两档，确认操作组不溢出）、按条件选择菜单展开；
3. 无选中时悬停复选框覆盖头像；
4. 390 宽：工具栏出现「选择」开关、悬停动作组不存在（`page.locator('.mi-hover-actions').first().isVisible()` 为 false）；
5. 字重：已读/未读行对比特写；侧栏；设置分区标题；
6. 外观页：预览条（标准 / 增强对比、紧凑 / 卡片、三栏 / 双栏），滚动后预览仍吸顶；
7. 阅读页正文分组：18px + 1.8 + 舒适宽度 + 125%，以及打开一封真实邮件看效果；
8. uiScale 110%/120%（若 Task 1 保留）：滚动到底、j/k、右键菜单。

每张图逐一看，问题记入 ledger 并修复（每个修复先加能复现的断言）。

- [ ] **Step 4: 独立审查**

派发一个**没参与实现**的审查代理（能指定模型时用最强的；没有子代理能力时自己按「找缺陷而非确认正确」做一遍，并在汇报里注明是自审），输入：本计划路径、spec 路径、Review Focus 原文、`git diff --stat` 与改动文件列表、ledger 的 Ruling 行。Critical / Important 一次修完（每条先写失败测试），Minor 记 ledger。

- [ ] **Step 5: 写验收记录**

在 spec 末尾追加「## 八、验收记录（日期）」：测试/lint 数字与基线对比、截图张数与发现的问题及修法、探针结论、审查结论、Rulings 列表。

- [ ] **Step 6: 请用户实机看字重**

请用户在 Windows（远程 Chrome 或桌面端）打开一次列表与设置页，确认「字偏细」是否改善——这是本轮唯一必须人眼在目标平台确认的项。

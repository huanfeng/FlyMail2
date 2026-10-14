# FlyMail 第八轮 UI/UE 打磨 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修掉 7 条一眼可见的排版硬伤，并把头像、侧栏、断点、细节拉到商业产品的层级感。

**Architecture:** 全部在 `flymail/frontend`。判定逻辑进 `src/lib/` 纯函数并配 vitest；
宽度判据用 CSS 容器查询（`.col.list` / `.col.reader` / `.reader-slide` 设为容器）；
视口断点只保留 1024 / 768 / 480。CSS 结构性约束用「读 `index.css` 原文断言」的既有套路钉住。

**Tech Stack:** React 19 + TypeScript 6 + Tailwind v4（仅令牌桥接，样式主体是 `src/index.css` 手写类）+ radix-ui DropdownMenu + vitest/jsdom + i18next。

**Spec:** `docs/flymail/ui-polish-r8.md`

## Global Constraints

- 包管理器 `pnpm`；**不要自己跑 `pnpm dev`**；浏览器验证走本机 Docker 实例 `http://127.0.0.1:8086`（admin / 见 `docker inspect flymail`）。
- **不提交 git**（前端 `CLAUDE.md`）。每个任务结束只跑验证，不 commit。
- 所有用户可见文案走 i18n：`src/locales/zh.json` 与 `en.json` 同步加键，键不得重复；改完用 `node -e "JSON.parse(require('fs').readFileSync('src/locales/zh.json','utf8'))"` 校验格式。`src/locales/keys.test.ts` 会扫描源码里的 `t('...')` 字面量并比对语言文件。
- 读 CSS 原文做断言只能用 `node:fs`（vitest `css: false`，`?raw` 拿到空串），文件顶部加 `/// <reference types="node" />`；**先剥注释再匹配**（仓库 CSS 注释极密）。
- 判据用 `pnpm build`（`tsc -b`）而不是 `tsc --noEmit`。
- 不改 `.mail-item` 行高与 `estimateSize` 常量。
- 基线：`pnpm test` 有 1 项既有失败（`MailList.test.tsx › 分组打乱顺序后…`），`pnpm lint` 既有 17 错误；本轮判据是**不新增**。
- 测试命令：`pnpm exec vitest run <file>`；全量 `pnpm test`。

## Review Focus

1. **窄容器 + 搜索有输入**：清除按钮 `×` 要始终可见（只隐藏 kbd 提示，不隐藏清除）。→ Task 1 CSS 断言只命中 `.search-input .kbd`。
2. **会话卡短日期跨年/无效日期**：去年 12 月 31 日要显示年份；`'not-a-date'` 原样返回不抛。→ Task 3 单测。
3. **同类型文件夹 ≥ 2 且顺序由 `sort_order` 决定**：哪个拿 i18n 名必须稳定（按入参顺序取第一个，入参已由后端按 sort_order 排好）。→ Task 4 单测钉「第一个」。
4. **折叠的自定义文件夹里有未读**：折叠行要显示折叠部分的未读合计，否则未读消失。→ Task 9 `splitFolders` 返回 `hiddenUnread`，单测钉。
5. **阅读区图标模式下工具条按钮的可访问名**：隐藏 `<span>` 后读屏靠 `aria-label`。→ Task 2 渲染测试断言每颗 `.tb-btn` 都有 `aria-label` 或可见文本。

---

### Task 1: 容器查询基建 + 搜索框快捷键提示

**Files:**
- Modify: `flymail/frontend/src/index.css`（`.col.list` 460 行附近、`.col.reader` 469、`.reader-slide` 1607、`.search-input .kbd` 773、`.compose-btn .kbd` 716）
- Test: `flymail/frontend/src/lib/ui-polish-r8.test.ts`（新建，本轮所有 CSS 结构断言都放这里）

**Interfaces:**
- Produces: 容器名 `list`（`.col.list`）、`reader`（`.col.reader` 与 `.reader-slide`），后续任务的 `@container` 规则都用这两个名字。

- [ ] **Step 1: 写失败测试**

```ts
/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve as resolvePath } from 'node:path'
import { describe, it, expect } from 'vitest'

const css = readFileSync(resolvePath(__dirname, '../index.css'), 'utf8')
/** 剥掉块注释：仓库 CSS 注释里大量出现选择器与属性名，不剥会误匹配 */
export const stripped = css.replace(/\/\*[\s\S]*?\*\//g, '')

/** 取某个选择器块的声明文本（第一处命中） */
export function block(selector: string): string {
  const esc = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const m = stripped.match(new RegExp(`(^|[}\\s])${esc}\\s*\\{([^}]*)\\}`, 'm'))
  return m ? m[2] : ''
}

describe('R8-1 容器查询基建与搜索框 kbd', () => {
  it('列表栏与阅读区声明为容器', () => {
    expect(block('.col.list')).toMatch(/container:\s*list\s*\/\s*inline-size/)
    expect(block('.col.reader')).toMatch(/container:\s*reader\s*\/\s*inline-size/)
    expect(block('.reader-slide')).toMatch(/container:\s*reader\s*\/\s*inline-size/)
  })
  it('搜索框 kbd 不折行', () => {
    expect(block('.search-input .kbd')).toMatch(/white-space:\s*nowrap/)
  })
  it('粗指针下 kbd 提示一律隐藏（搜索框与写邮件按钮）', () => {
    const m = stripped.match(/@media\s*\(pointer:\s*coarse\)\s*\{([\s\S]*?)\n\}/)
    expect(m).not.toBeNull()
    expect(m![1]).toMatch(/\.search-input \.kbd[^{]*\{[^}]*display:\s*none/)
    expect(m![1]).toMatch(/\.compose-btn \.kbd[^{]*\{[^}]*display:\s*none/)
  })
  it('窄列表容器下只隐藏 kbd，不动清除按钮', () => {
    const m = stripped.match(/@container list \(max-width: 400px\)\s*\{([\s\S]*?)\n\}/)
    expect(m).not.toBeNull()
    expect(m![1]).toMatch(/\.search-input \.kbd\s*\{[^}]*display:\s*none/)
    expect(m![1]).not.toMatch(/icon-btn/)
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd flymail/frontend && pnpm exec vitest run src/lib/ui-polish-r8.test.ts`
Expected: 4 个用例 FAIL（容器/nowrap/媒体块都不存在）。

- [ ] **Step 3: 改 CSS**

在 `.col.list`、`.col.reader` 各自的声明末尾追加 `container: list / inline-size;` / `container: reader / inline-size;`；`.reader-slide` 追加 `container: reader / inline-size;`。

`.search-input .kbd` 追加 `white-space: nowrap; flex: 0 0 auto;`。

在 `.search-input .kbd` 规则之后新增：

```css
/* 键盘提示只给键盘用户看：触摸设备上「Ctrl K」是噪音，而且 380px 列宽下它还会折行。 */
@media (pointer: coarse) {
  .search-input .kbd { display: none; }
  .compose-btn .kbd { display: none; }
}
/* 列表栏本身不够宽时也收起提示（容器查询：三栏桌面下列表也可能只有 300px）。
   只收 kbd，不动有输入时的清除按钮——那是功能，不是提示。 */
@container list (max-width: 400px) {
  .search-input .kbd { display: none; }
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `pnpm exec vitest run src/lib/ui-polish-r8.test.ts src/lib/focus-ring.test.ts src/lib/dialog-buttons.test.ts`
Expected: 全部 PASS（后两个是既有 CSS 测试，确认没被新块干扰）。

---

### Task 2: 阅读区工具条窄容器图标模式

**Files:**
- Modify: `flymail/frontend/src/components/mail/ReaderToolbar.tsx`（四颗带文字按钮：回复、转发、翻译、归档、删除）
- Modify: `flymail/frontend/src/index.css`（`.reader-toolbar` 1148 行附近）
- Test: `flymail/frontend/src/components/mail/ReaderToolbar.test.tsx`（新建）
- Test: `flymail/frontend/src/lib/ui-polish-r8.test.ts`

**Interfaces:**
- Consumes: Task 1 的容器名 `reader`。

- [ ] **Step 1: 写失败测试（渲染）**

```tsx
import { describe, it, expect, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { ReaderToolbar } from '@/components/mail/ReaderToolbar'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

describe('ReaderToolbar 可访问名', () => {
  it('每颗按钮在文字隐藏后仍有 aria-label', () => {
    const host = document.createElement('div')
    document.body.appendChild(host)
    const root: Root = createRoot(host)
    act(() => {
      root.render(
        <ReaderToolbar
          onPrev={() => {}}
          onNext={() => {}}
          onReply={() => {}}
          onForward={() => {}}
          onTranslate={() => {}}
          onArchive={() => {}}
          onDelete={() => {}}
          moreItems={[]}
        />,
      )
    })
    const btns = Array.from(host.querySelectorAll<HTMLButtonElement>('.tb-btn'))
    expect(btns.length).toBeGreaterThanOrEqual(7)
    for (const b of btns) {
      expect(b.getAttribute('aria-label'), b.outerHTML).toBeTruthy()
    }
    act(() => root.unmount())
    host.remove()
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/components/mail/ReaderToolbar.test.tsx`
Expected: FAIL，回复/转发/翻译/归档/删除五颗没有 `aria-label`。

- [ ] **Step 3: 给五颗按钮补 aria-label**

每颗带 `<span>` 文字的按钮加 `aria-label={...}`，值与 span 内容同源：
回复 `t('reader.reply')`、转发 `t('reader.forward')`、归档 `t('reader.archive')`、删除 `t('reader.delete')`；
翻译按钮的 aria-label 用与 span 相同的三态表达式。

- [ ] **Step 4: CSS 测试 + 规则**

在 `ui-polish-r8.test.ts` 追加：

```ts
describe('R8-2 阅读区工具条图标模式', () => {
  it('窄阅读容器下隐藏按钮文字', () => {
    const m = stripped.match(/@container reader \(max-width: 560px\)\s*\{([\s\S]*?)\n\}/)
    expect(m).not.toBeNull()
    expect(m![1]).toMatch(/\.tb-btn > span\s*\{[^}]*display:\s*none/)
  })
})
```

在 `.tb-sep` 规则之后加：

```css
/* 阅读区 560px 以下切图标模式：文字由 aria-label 承担。
   1024 平板与把列表拖宽的桌面都会落到这个宽度，原来的整颗换行会把工具条叠成两行。 */
@container reader (max-width: 560px) {
  .tb-btn > span { display: none; }
  .tb-btn { padding: 0 8px; }
  .tb-sep { margin: 0 3px; }
}
```

- [ ] **Step 5: 跑测试**

Run: `pnpm exec vitest run src/components/mail/ReaderToolbar.test.tsx src/lib/ui-polish-r8.test.ts src/components/mail/ReaderTranslate.test.tsx`
Expected: PASS。

---

### Task 3: 短日期 + 会话卡头部不再挤掉发件人

**Files:**
- Modify: `flymail/frontend/src/lib/mail-format.ts`（`formatDate` 之后新增 `formatShortDate`）
- Modify: `flymail/frontend/src/components/mail/ThreadReader.tsx:285`（`.ti-time`）
- Modify: `flymail/frontend/src/components/mail/Reader.tsx:239`（`.th-time`）
- Modify: `flymail/frontend/src/index.css`（`.thread-item .ti-time` 1391、`.ti-folder`）
- Test: `flymail/frontend/src/lib/mail-format.test.ts`

**Interfaces:**
- Produces: `formatShortDate(dateStr: string, now?: Date): string`。

- [ ] **Step 1: 写失败测试**

在 `mail-format.test.ts` 的 import 里加 `formatShortDate`，追加：

```ts
describe('formatShortDate', () => {
  const now = new Date(2026, 9, 14, 15, 0, 0) // 2026-10-14 15:00 本地时间
  it('今天只显示时分', () => {
    expect(formatShortDate(new Date(2026, 9, 14, 9, 5).toISOString(), now)).toBe('09:05')
  })
  it('本年显示月日', () => {
    const s = formatShortDate(new Date(2026, 8, 17, 19, 38).toISOString(), now)
    expect(s).not.toContain('2026')
    expect(s).toMatch(/9/)
    expect(s).toMatch(/17/)
  })
  it('跨年显示年份（去年 12 月 31 日）', () => {
    expect(formatShortDate(new Date(2025, 11, 31, 23, 0).toISOString(), now)).toBe('2025/12/31')
  })
  it('无效日期原样返回', () => {
    expect(formatShortDate('not-a-date', now)).toBe('not-a-date')
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/mail-format.test.ts`
Expected: FAIL `formatShortDate is not a function`。

- [ ] **Step 3: 实现**

```ts
/**
 * 会话卡 / 阅读头用的短日期：今天 → HH:mm；本年 → 月日（跟随 locale）；更早 → YYYY/M/D。
 * 完整时间放在元素的 title 里，这里只负责「一眼知道大概什么时候」。
 */
export function formatShortDate(dateStr: string, now: Date = new Date()): string {
  const d = new Date(dateStr)
  if (Number.isNaN(d.getTime())) return dateStr
  const sameDay =
    d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate()
  if (sameDay) {
    const hh = String(d.getHours()).padStart(2, '0')
    const mm = String(d.getMinutes()).padStart(2, '0')
    return `${hh}:${mm}`
  }
  if (d.getFullYear() === now.getFullYear()) {
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  }
  return `${d.getFullYear()}/${d.getMonth() + 1}/${d.getDate()}`
}
```

- [ ] **Step 4: 接到组件**

`ThreadReader.tsx:285`：
```tsx
<span className="ti-time" title={formatDate(msg.date)}>{formatShortDate(msg.date)}</span>
```
（import 增加 `formatShortDate`。）

`Reader.tsx:239`：
```tsx
<div className="th-time" title={formatDate(detail.date)}>{formatShortDate(detail.date)}</div>
```

- [ ] **Step 5: CSS**

`.thread-item .ti-time` 已有 `white-space: nowrap`；补一条窄容器规则（放在 Task 2 的 `@container reader (max-width: 560px)` 块**之后**、作为独立块）：

```css
/* 480px 以下的阅读容器：文件夹 chip 只在聚合/搜索视图有信息量，窄屏先保发件人 */
@container reader (max-width: 480px) {
  .thread-item .ti-folder { display: none; }
}
```

并在 `ui-polish-r8.test.ts` 追加：
```ts
describe('R8-3 会话卡窄容器', () => {
  it('480px 以下隐藏文件夹 chip', () => {
    const m = stripped.match(/@container reader \(max-width: 480px\)\s*\{([\s\S]*?)\n\}/)
    expect(m).not.toBeNull()
    expect(m![1]).toMatch(/\.ti-folder\s*\{[^}]*display:\s*none/)
  })
})
```

- [ ] **Step 6: 跑测试**

Run: `pnpm exec vitest run src/lib/mail-format.test.ts src/lib/ui-polish-r8.test.ts src/components/mail/ReaderTranslate.test.tsx`
Expected: PASS。

---

### Task 4: 同类型多个文件夹不再同名

**Files:**
- Modify: `flymail/frontend/src/lib/mail-format.ts`（`folderName` 之后新增 `folderLabels`）
- Modify: `flymail/frontend/src/components/mail/AccountSidebar.tsx`（`AccountBlock` 内 622–640 行的 label 计算；472 行右键确认文案）
- Test: `flymail/frontend/src/lib/mail-format.test.ts`

**Interfaces:**
- Produces: `folderLabels(folders: Pick<Folder,'id'|'type'|'display_name'>[], t: (k: string) => string): Map<number, string>`。

- [ ] **Step 1: 写失败测试**

```ts
describe('folderLabels', () => {
  const t = (k: string) => `T(${k})`
  it('同类型第二个起用服务器原名，第一个用 i18n', () => {
    const m = folderLabels(
      [
        { id: 1, type: 'trash', display_name: '[Gmail]/已删除邮件' },
        { id: 2, type: 'trash', display_name: '已删除' },
        { id: 3, type: 'inbox', display_name: 'INBOX' },
      ],
      t,
    )
    expect(m.get(1)).toBe('T(folder.trash)')
    expect(m.get(2)).toBe('已删除')
    expect(m.get(3)).toBe('T(folder.inbox)')
  })
  it('custom 一律用 display_name', () => {
    const m = folderLabels([{ id: 9, type: 'custom', display_name: 'Google/G+' }], t)
    expect(m.get(9)).toBe('Google/G+')
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/mail-format.test.ts`
Expected: FAIL `folderLabels is not a function`。

- [ ] **Step 3: 实现**

```ts
/**
 * 一个账户内全部文件夹的显示名。
 *
 * 同一类型（比如 Gmail 的 `[Gmail]/已删除邮件` 与 `已删除` 都归 trash）只有第一个
 * 拿 i18n 名，其余退回服务器原名——否则侧栏里出现两个一模一样的「已删除」，
 * 用户分不清哪个是哪个。入参顺序即后端的 sort_order 顺序。
 */
export function folderLabels(
  folders: Pick<Folder, 'id' | 'type' | 'display_name'>[],
  t: (k: string) => string,
): Map<number, string> {
  const seen = new Set<string>()
  const out = new Map<number, string>()
  for (const f of folders) {
    const first = !seen.has(f.type)
    seen.add(f.type)
    out.set(f.id, first ? folderName(f.type, f.display_name, t) : f.display_name)
  }
  return out
}
```

- [ ] **Step 4: 接到侧栏**

`AccountBlock` 内、`folders.filter(...).map(...)` 之前：
```ts
const labels = folderLabels(folders, t)
```
`.map` 里把 `const label = f.type === 'custom' ? f.display_name : t(\`folder.${f.type}\`)` 换成
`const label = labels.get(f.id) ?? f.display_name`。
472 行的 `folderName(f.type, f.display_name, t)` 换成 `labels.get(f.id) ?? f.display_name`（`folderCtxItems` 若在 `labels` 作用域之外，把 `labels` 作为参数传入或把 `const labels` 提到函数体顶部——以 `tsc -b` 通过为准）。

- [ ] **Step 5: 跑测试**

Run: `pnpm exec vitest run src/lib/mail-format.test.ts src/components/mail/SidebarFolders.test.tsx src/components/mail/AccountSidebarHealth.test.tsx && pnpm build`
Expected: PASS，build 绿。

---

### Task 5: 账户健康横幅改为列表栏顶部内联

**Files:**
- Modify: `flymail/frontend/src/pages/Shell.tsx:1401`（删除根部的 `<HealthBanner />`）与 1422（`list=` 插槽）
- Modify: `flymail/frontend/src/index.css:2912`（`.health-banner`）
- Test: `flymail/frontend/src/lib/ui-polish-r8.test.ts`；既有 `src/lib/health-banner.test.tsx` 必须保持绿。

**Interfaces:**
- Produces: `list` 插槽的包装结构 `.list-stack > (.health-banner?) + .list-stack-main + (.compose-fab)`，Task 6 复用。

- [ ] **Step 1: 写失败 CSS 测试**

```ts
describe('R8-5 健康横幅内联', () => {
  it('不再固定定位，无 z-index', () => {
    const b = block('.health-banner')
    expect(b).not.toMatch(/position:\s*fixed/)
    expect(b).not.toMatch(/z-index/)
    expect(b).toMatch(/border-bottom/)
  })
  it('列表栈撑满列栏', () => {
    expect(block('.list-stack')).toMatch(/flex-direction:\s*column/)
    expect(block('.list-stack-main')).toMatch(/min-height:\s*0/)
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/ui-polish-r8.test.ts`
Expected: R8-5 两例 FAIL。

- [ ] **Step 3: CSS**

把 `.health-banner` 整块替换为：

```css
/* M17 账户健康横幅。R8 起不再浮在底部中央——它可能持续数天，浮层会一直压在
   列表/阅读区分界上，连设置弹窗都盖。改为列表栏顶部的内联条：占布局、随列表栏
   一起显示/隐藏、被弹窗遮罩自然盖住。断线横幅 .conn-banner 是秒级瞬态，保持浮层。 */
.health-banner {
  display: flex; align-items: center; gap: 10px; flex-wrap: wrap;
  padding: 8px 14px;
  background: var(--warning-wash); color: var(--warning-ink);
  border-bottom: 1px solid color-mix(in srgb, var(--destructive) 35%, transparent);
  font-size: 12.5px;
}
.health-banner-text { flex: 1 1 12rem; min-width: 0; }
.list-stack { display: flex; flex-direction: column; height: 100%; min-height: 0; position: relative; }
.list-stack-main { flex: 1 1 0; min-height: 0; }
```
（`.health-banner-actions` / `.health-banner-btn` 保留。）

- [ ] **Step 4: Shell 接线**

删除 1401 行的 `<HealthBanner />` 及其注释；`list=` 插槽改为：

```tsx
list={
  <div className="list-stack">
    <HealthBanner />
    <div className="list-stack-main">
      {view === 'drafts' && accountId != null ? (
        <DraftsList accountId={accountId} onOpenDraft={openDraft} />
      ) : (
        <MailList
          /* …原有 props 原样保留… */
        />
      )}
    </div>
  </div>
}
```

- [ ] **Step 5: 跑测试 + 构建**

Run: `pnpm exec vitest run src/lib/ui-polish-r8.test.ts src/lib/health-banner.test.tsx src/components/mail/AppLayout.test.tsx && pnpm build`
Expected: PASS，build 绿。

---

### Task 6: 手机端写邮件 FAB

**Files:**
- Modify: `flymail/frontend/src/pages/Shell.tsx`（Task 5 的 `.list-stack` 内、`.list-stack-main` 之后）
- Modify: `flymail/frontend/src/index.css`（768 断点块 476 行附近）
- Test: `flymail/frontend/src/lib/ui-polish-r8.test.ts`

- [ ] **Step 1: 写失败 CSS 测试**

```ts
describe('R8-6 移动端写邮件 FAB', () => {
  it('桌面隐藏、768 以下显示且为可触达尺寸', () => {
    expect(block('.compose-fab')).toMatch(/display:\s*none/)
    const m = stripped.match(/@media \(max-width: 768px\)\s*\{([\s\S]*?)\n\}/g) ?? []
    const hit = m.find((blk) => /\.compose-fab\s*\{[^}]*display:\s*(flex|grid)/.test(blk))
    expect(hit).toBeTruthy()
    expect(hit!).toMatch(/\.compose-fab\s*\{[^}]*(width|height):\s*56px/)
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/ui-polish-r8.test.ts`
Expected: R8-6 FAIL。

- [ ] **Step 3: CSS**

在 `.compose-btn .kbd` 规则之后加：
```css
/* 移动端写邮件入口。侧栏那颗按钮在手机上藏在抽屉里，列表页没有任何写信入口。
   挂在 .list-stack 内：阅读面板激活时列表栏整体隐藏，FAB 跟着消失，不需要额外状态。 */
.compose-fab { display: none; }
```
在第一个 `@media (max-width: 768px) {` 块（476 行那块）内追加：
```css
  .compose-fab {
    display: grid; place-items: center;
    position: absolute; right: 16px; bottom: 20px;
    width: 56px; height: 56px; border-radius: 50%;
    background: var(--primary); color: var(--primary-foreground);
    box-shadow: var(--shadow-lg);
    z-index: 5;
  }
  .compose-fab:active { transform: translateY(1px); }
```

- [ ] **Step 4: Shell 接线**

`.list-stack-main` 之后：
```tsx
<button
  type="button"
  className="compose-fab"
  onClick={onCompose}
  aria-label={t('sidebar.compose')}
>
  <Icon name="compose" size={22} />
</button>
```
（`onCompose` 是 Shell 里已有的、侧栏 `onCompose={() => { onCompose(); setDrawerOpen(false) }}` 用的那个函数；`Icon` 若未 import 则补 `import { Icon } from '@/components/ui/Icon'`。）

- [ ] **Step 5: 跑测试 + 构建**

Run: `pnpm exec vitest run src/lib/ui-polish-r8.test.ts && pnpm build`
Expected: PASS。

---

### Task 7: 通知中心计数文案

**Files:**
- Modify: `flymail/frontend/src/components/notifications/NotificationsPage.tsx:150-155`
- Modify: `flymail/frontend/src/locales/zh.json`、`en.json`（`notif` 段）
- Test: `flymail/frontend/src/components/notifications/NotificationsPage.test.tsx`

- [ ] **Step 1: 写失败测试**

查看该测试文件现有的 mock 方式（`react-i18next` 是否被 mock、如何渲染），在同一套件里追加：

```tsx
it('头部只报未读数，不把已加载条数说成总数', async () => {
  mock.onGet(/\/notifications/).reply(200, {
    notifications: [],
    unread_count: 547,
  })
  // …按本文件既有方式渲染 NotificationsPage…
  const sub = document.querySelector('.nd-sub')!
  expect(sub.textContent).toContain('547')
  expect(sub.textContent).not.toMatch(/共|total/)
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/components/notifications/NotificationsPage.test.tsx`
Expected: 新用例 FAIL（文案含「共」或「total」）。

- [ ] **Step 3: 实现**

zh.json `notif` 段加 `"unreadSummary": "{{count}} 条未读"`；en.json 加 `"unreadSummary": "{{count}} unread"`。

组件：
```tsx
<span className="nd-sub">{t('notif.unreadSummary', { count: unreadCount })}</span>
```
删除 `isZh` 变量（若其它地方不再用）。

- [ ] **Step 4: 校验 JSON 与测试**

Run: `node -e "for (const f of ['zh','en']) JSON.parse(require('fs').readFileSync('src/locales/'+f+'.json','utf8'))" && pnpm exec vitest run src/components/notifications/NotificationsPage.test.tsx src/locales/keys.test.ts`
Expected: PASS。

---

### Task 8: 发件人头像按色相区分

**Files:**
- Create: `flymail/frontend/src/lib/avatar-color.ts`
- Modify: `flymail/frontend/src/index.css`（`:root` 全局令牌 ~145 行附近、`.avatar-sq` 980）
- Modify: `flymail/frontend/src/components/mail/MailList.tsx:372,489,642`
- Modify: `flymail/frontend/src/components/mail/ThreadReader.tsx:219`
- Modify: `flymail/frontend/src/components/mail/Reader.tsx:204`
- Test: `flymail/frontend/src/lib/avatar-color.test.ts`（新建）、`src/lib/ui-polish-r8.test.ts`

**Interfaces:**
- Produces: `avatarHue(key: string): number`（1..8）、`avatarKey(name: string, addr: string): string`。

- [ ] **Step 1: 写失败单测**

```ts
import { describe, it, expect } from 'vitest'
import { avatarHue, avatarKey } from '@/lib/avatar-color'

describe('avatarHue', () => {
  it('结果落在 1..8', () => {
    for (const k of ['a@b.com', 'x', '', '张三', 'chip3@example.com']) {
      const h = avatarHue(k)
      expect(h).toBeGreaterThanOrEqual(1)
      expect(h).toBeLessThanOrEqual(8)
    }
  })
  it('同一键稳定、不同键有区分', () => {
    expect(avatarHue('chip3@example.com')).toBe(avatarHue('chip3@example.com'))
    const set = new Set(['a@x', 'b@x', 'c@x', 'd@x', 'e@x', 'f@x', 'g@x', 'h@x'].map(avatarHue))
    expect(set.size).toBeGreaterThan(2)
  })
  it('键优先用地址并忽略大小写与空白', () => {
    expect(avatarKey('张三', ' Zhang@Example.com ')).toBe('zhang@example.com')
    expect(avatarKey('张三', '')).toBe('张三')
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/avatar-color.test.ts`
Expected: FAIL（模块不存在）。

- [ ] **Step 3: 实现**

```ts
/**
 * 发件人头像底色。
 *
 * 此前所有头像都是同一个 --accent，一列邮件扫过去没有任何区分度。
 * 按发件人地址做稳定哈希取 8 个色相之一，令牌 --avatar-1..8 定义在 index.css
 * （oklch，白字对比 ≥ 4.5，亮暗共用一套——判据是「字对底」不是「底对页」）。
 */
export function avatarKey(name: string, addr: string): string {
  const a = addr.trim().toLowerCase()
  return a || name.trim()
}

/** FNV-1a 32 位哈希，映射到 1..8 */
export function avatarHue(key: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < key.length; i++) {
    h ^= key.charCodeAt(i)
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return (h % 8) + 1
}
```

- [ ] **Step 4: CSS 令牌 + 断言**

`:root` 全局令牌块（`--warning-ink` 之后）加：
```css
  /* 发件人头像 8 色相。L 0.52 / C 0.11：白字对比 ≥ 4.5（lib/ui-polish-r8.test.ts 按公式算） */
  --avatar-1: oklch(0.52 0.11 25);
  --avatar-2: oklch(0.52 0.11 60);
  --avatar-3: oklch(0.52 0.11 110);
  --avatar-4: oklch(0.52 0.11 160);
  --avatar-5: oklch(0.52 0.11 210);
  --avatar-6: oklch(0.52 0.11 250);
  --avatar-7: oklch(0.52 0.11 290);
  --avatar-8: oklch(0.52 0.11 330);
```
`.avatar-sq` 规则之后加：
```css
.avatar-sq[data-hue="1"] { background: var(--avatar-1); }
.avatar-sq[data-hue="2"] { background: var(--avatar-2); }
.avatar-sq[data-hue="3"] { background: var(--avatar-3); }
.avatar-sq[data-hue="4"] { background: var(--avatar-4); }
.avatar-sq[data-hue="5"] { background: var(--avatar-5); }
.avatar-sq[data-hue="6"] { background: var(--avatar-6); }
.avatar-sq[data-hue="7"] { background: var(--avatar-7); }
.avatar-sq[data-hue="8"] { background: var(--avatar-8); }
```

`ui-polish-r8.test.ts` 追加（oklch L 到相对亮度：Y ≈ L³ 是近似；用 oklab 精确换算：`Y = l*l*l` 对中性色成立，带彩度时误差 < 0.01，对 4.5 门槛足够）：
```ts
describe('R8-8 头像色相令牌', () => {
  it('8 个令牌齐全且白字对比 ≥ 4.5', () => {
    for (let i = 1; i <= 8; i++) {
      const m = stripped.match(new RegExp(`--avatar-${i}:\\s*oklch\\(([\\d.]+)\\s+([\\d.]+)\\s+([\\d.]+)\\)`))
      expect(m, `--avatar-${i}`).not.toBeNull()
      const L = Number(m![1])
      const Y = L * L * L
      const contrast = (1.0 + 0.05) / (Y + 0.05)
      expect(contrast).toBeGreaterThanOrEqual(4.5)
      expect(stripped).toMatch(new RegExp(`\\.avatar-sq\\[data-hue="${i}"\\]`))
    }
  })
})
```

- [ ] **Step 5: 接到组件**

MailList 三处把 `style={{ background: 'var(--accent)' }}` 换成 `data-hue={avatarHue(avatarKey(name, addr))}`，其中：
- 372 与 489 行：`avatarKey(msg.from_name, msg.from_addr)`
- 642 行：`avatarKey(avatarOf?.name ?? '', avatarOf?.email ?? '')`
ThreadReader 219：`<div className="avatar-sq" data-hue={avatarHue(avatarKey(msg.from_name, msg.from_addr))} style={{ color: 'white' }}>`
Reader 204：同理用 `detail.from_name, detail.from_addr`。
`.avatar-sq` 基础规则保留 `color: white`，所以 ThreadReader/Reader 的内联 `color` 可一并删掉。import：`import { avatarHue, avatarKey } from '@/lib/avatar-color'`。

- [ ] **Step 6: 跑测试 + 构建**

Run: `pnpm exec vitest run src/lib/avatar-color.test.ts src/lib/ui-polish-r8.test.ts src/components/mail/MailList.test.tsx && pnpm build`
Expected: 除既有那 1 项失败外 PASS。

---

### Task 9: 侧栏自定义文件夹折叠

**Files:**
- Create: `flymail/frontend/src/lib/sidebar-folders.ts`
- Modify: `flymail/frontend/src/components/mail/AccountSidebar.tsx`（`AccountBlock` 文件夹列表 620–645 行）
- Modify: `flymail/frontend/src/index.css`（`.folder-row` 648 附近加 `.folder-more`）
- Modify: `zh.json` / `en.json`（`sidebar.moreFolders` / `sidebar.lessFolders`）
- Test: `flymail/frontend/src/lib/sidebar-folders.test.ts`（新建）

**Interfaces:**
- Produces:
  ```ts
  export interface FolderSplit<T> { visible: T[]; hiddenCount: number; hiddenUnread: number; foldable: boolean }
  export function splitFolders<T extends Pick<Folder,'id'|'type'|'unread_count'>>(folders: T[], activeId: number | null, showMore: boolean, threshold?: number): FolderSplit<T>
  export function loadMoreExpanded(): Record<number, boolean>
  export function saveMoreExpanded(map: Record<number, boolean>): void
  ```

- [ ] **Step 1: 写失败单测**

```ts
import { describe, it, expect } from 'vitest'
import { splitFolders } from '@/lib/sidebar-folders'

const f = (id: number, type: string, unread = 0) => ({ id, type, unread_count: unread })

describe('splitFolders', () => {
  it('custom 少于阈值时不折叠', () => {
    const r = splitFolders([f(1, 'inbox'), f(2, 'custom'), f(3, 'custom')], null, false)
    expect(r.foldable).toBe(false)
    expect(r.visible.map((x) => x.id)).toEqual([1, 2, 3])
  })
  it('custom 达到阈值且未展开时折叠，系统文件夹常显', () => {
    const r = splitFolders([f(1, 'inbox'), f(2, 'trash'), f(3, 'custom', 2), f(4, 'custom'), f(5, 'custom', 5)], null, false)
    expect(r.foldable).toBe(true)
    expect(r.visible.map((x) => x.id)).toEqual([1, 2])
    expect(r.hiddenCount).toBe(3)
    expect(r.hiddenUnread).toBe(7)
  })
  it('激活的 custom 即使折叠也常显，且不计入隐藏数', () => {
    const r = splitFolders([f(1, 'inbox'), f(3, 'custom', 2), f(4, 'custom'), f(5, 'custom')], 4, false)
    expect(r.visible.map((x) => x.id)).toEqual([1, 4])
    expect(r.hiddenCount).toBe(2)
    expect(r.hiddenUnread).toBe(2)
  })
  it('展开时全部显示', () => {
    const r = splitFolders([f(1, 'inbox'), f(3, 'custom'), f(4, 'custom'), f(5, 'custom')], null, true)
    expect(r.visible.length).toBe(4)
    expect(r.foldable).toBe(true)
    expect(r.hiddenCount).toBe(0)
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/sidebar-folders.test.ts`
Expected: FAIL（模块不存在）。

- [ ] **Step 3: 实现**

```ts
import type { Folder } from '@/lib/types'

export interface FolderSplit<T> {
  /** 要渲染的行（顺序保持入参顺序） */
  visible: T[]
  /** 折叠起来的条数（展开时为 0） */
  hiddenCount: number
  /** 折叠部分的未读合计：折叠不能让未读消失 */
  hiddenUnread: number
  /** 是否需要「更多」开关（custom 达到阈值才有） */
  foldable: boolean
}

/**
 * 系统文件夹常显；custom 文件夹 ≥ threshold 个时折进「更多」，
 * 但当前激活的那个即使折叠也常显——选中项不能从侧栏消失。
 */
export function splitFolders<T extends Pick<Folder, 'id' | 'type' | 'unread_count'>>(
  folders: T[],
  activeId: number | null,
  showMore: boolean,
  threshold = 3,
): FolderSplit<T> {
  const customCount = folders.filter((f) => f.type === 'custom').length
  const foldable = customCount >= threshold
  if (!foldable || showMore) {
    return { visible: folders, hiddenCount: 0, hiddenUnread: 0, foldable }
  }
  const visible: T[] = []
  let hiddenCount = 0
  let hiddenUnread = 0
  for (const f of folders) {
    if (f.type !== 'custom' || f.id === activeId) visible.push(f)
    else {
      hiddenCount++
      hiddenUnread += f.unread_count
    }
  }
  return { visible, hiddenCount, hiddenUnread, foldable }
}

const KEY = 'flymail.sidebar.more-folders'

/** 各账户「更多文件夹」展开状态；读不到或格式不对时返回空表 */
export function loadMoreExpanded(): Record<number, boolean> {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (parsed == null || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    const out: Record<number, boolean> = {}
    for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
      const id = Number(k)
      if (Number.isInteger(id) && id > 0 && typeof v === 'boolean') out[id] = v
    }
    return out
  } catch {
    return {}
  }
}

export function saveMoreExpanded(map: Record<number, boolean>): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(map))
  } catch {
    /* 存储不可用时只是不记忆 */
  }
}
```

- [ ] **Step 4: 侧栏接线**

在 `AccountSidebar` 主组件里加状态 `const [moreMap, setMoreMap] = useState<Record<number, boolean>>(loadMoreExpanded)` 与
```ts
function toggleMore(accountId: number) {
  setMoreMap((prev) => {
    const next = { ...prev, [accountId]: !prev[accountId] }
    saveMoreExpanded(next)
    return next
  })
}
```
把 `showMore={!!moreMap[account.id]}` 与 `onToggleMore={() => toggleMore(account.id)}` 传给 `AccountBlock`（在其 props 接口里加这两个字段）。

`AccountBlock` 里：
```ts
const selectable = folders.filter((f) => f.selectable)
const split = splitFolders(selectable, activeFolderId, showMore)
```
`.map` 改为遍历 `split.visible`；在 `.map` 之后、草稿箱行之前加：
```tsx
{split.foldable && (
  <button type="button" className="folder-row folder-more" onClick={onToggleMore} aria-expanded={showMore}>
    <span className="f-icon"><Icon name={showMore ? 'chevron-up' : 'chevron-down'} size={12} /></span>
    <span className="f-label">
      {showMore ? t('sidebar.lessFolders') : t('sidebar.moreFolders', { count: split.hiddenCount })}
    </span>
    {!showMore && split.hiddenUnread > 0 && <span className="f-count">{split.hiddenUnread}</span>}
  </button>
)}
```
（`f-label` / `f-count` 的类名以 `FolderRow` 内部实际使用的为准，保持同一外观。`chevron-up` 若 Icon 集里没有，用 `chevron-down` 加 `style={{ transform: 'rotate(180deg)' }}`。）

i18n：zh `"moreFolders": "更多文件夹 ({{count}})"`、`"lessFolders": "收起文件夹"`；en `"moreFolders": "More folders ({{count}})"`、`"lessFolders": "Fewer folders"`。

CSS（`.folder-row.active` 之后）：
```css
.folder-row.folder-more { color: var(--ink-3); font-size: 12.5px; }
```

- [ ] **Step 5: 跑测试 + 构建**

Run: `node -e "for (const f of ['zh','en']) JSON.parse(require('fs').readFileSync('src/locales/'+f+'.json','utf8'))" && pnpm exec vitest run src/lib/sidebar-folders.test.ts src/components/mail/SidebarFolders.test.tsx src/locales/keys.test.ts && pnpm build`
Expected: PASS。`SidebarFolders.test.tsx` 用的 fixture 每账户只有 INBOX + 1 个 custom，不触发折叠；若有用例构造了 ≥ 3 个 custom 并断言它们可见，把该用例的期望改为先点「更多」再断言。

---

### Task 10: 断点收口 + 1024 紧凑档

**Files:**
- Modify: `flymail/frontend/src/index.css`（`@media (max-width: 720px)` 1673、`@media (max-width: 820px)` 2080、`.col.sidebar`/`.col.list` 460–461）
- Test: `flymail/frontend/src/lib/ui-polish-r8.test.ts`

- [ ] **Step 1: 写失败测试**

```ts
describe('R8-10 断点收口', () => {
  it('只剩 1024 / 768 / 480 三档', () => {
    const widths = Array.from(stripped.matchAll(/@media[^{]*max-width:\s*(\d+)px/g)).map((m) => Number(m[1]))
    expect(new Set(widths)).toEqual(new Set([1024, 768, 480]))
  })
  it('1024 档收窄侧栏与列表给阅读区让位', () => {
    const m = stripped.match(/@media \(max-width: 1024px\)\s*\{([\s\S]*?)\n\}/)
    expect(m).not.toBeNull()
    expect(m![1]).toMatch(/\.col\.sidebar\s*\{[^}]*min\(var\(--sidebar-w\),\s*200px\)/)
    expect(m![1]).toMatch(/\.col\.list\s*\{[^}]*min\(var\(--list-w\),\s*340px\)/)
  })
})
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm exec vitest run src/lib/ui-polish-r8.test.ts`
Expected: R8-10 FAIL（存在 720/820）。

- [ ] **Step 3: 改 CSS**

- 1673 行 `@media (max-width: 720px)` 改为 `@media (max-width: 768px)`（内容不动）。
- 2080 行 `@media (max-width: 820px)` 改为 `@media (max-width: 768px)`（内容不动）。
- 在 `.col.reader` 规则之后加：
```css
/* 1024 紧凑桌面：三栏仍并排，但侧栏与列表各让一点，阅读区从 ~400px 回到 ~480px。
   min() 而不是覆盖变量：用户拖得更窄的偏好仍然生效。 */
@media (max-width: 1024px) {
  .col.sidebar { width: min(var(--sidebar-w), 200px); flex-basis: min(var(--sidebar-w), 200px); }
  .col.list { width: min(var(--list-w), 340px); flex-basis: min(var(--list-w), 340px); }
}
```

- [ ] **Step 4: 跑测试**

Run: `pnpm exec vitest run src/lib/ui-polish-r8.test.ts src/lib/focus-ring.test.ts src/lib/dialog-buttons.test.ts src/lib/centered-animation.test.ts`
Expected: PASS。

---

### Task 11: 监控统计卡不折行

**Files:**
- Modify: `flymail/frontend/src/components/settings/MonitoringSection.tsx:66-78, 233`

- [ ] **Step 1: 改 StatCard 与网格**

`StatCard` 数值行样式改为：
```tsx
<div style={{ fontSize: 20, fontWeight: 600, color: 'var(--ink)', fontFamily: 'var(--font-display)', fontVariantNumeric: 'tabular-nums', overflowWrap: 'anywhere', lineHeight: 1.25 }}>
```
233 行网格 `minmax(120px, 1fr)` 改为 `minmax(150px, 1fr)`。

- [ ] **Step 2: 构建**

Run: `pnpm build`
Expected: 绿。（视觉在 Task 14 截图验证。）

---

### Task 12: 撰写器发件人改为 DropMenu

**Files:**
- Modify: `flymail/frontend/src/components/mail/ComposeDialog.tsx:626-638`
- Modify: `flymail/frontend/src/index.css`（`.compose-select` 规则处加 `.compose-select-trigger`）
- Test: `flymail/frontend/src/components/mail/ComposeDialog.test.tsx`

- [ ] **Step 1: 看既有测试是否触碰 From**

Run: `grep -n "compose-select\|fromOverride\|option" src/components/mail/ComposeDialog.test.tsx`
若无命中，跳到 Step 2；若有，把 `select.value = ...; dispatchEvent(change)` 的用例改为「点触发按钮 → 点 `.ctx-item` 文本」。

- [ ] **Step 2: 写失败测试**

在 `ComposeDialog.test.tsx` 末尾按本文件既有渲染套路追加（多账户 fixture 用文件里已有的 `accounts` 两条即可，若只有一条则补第二条）：

```tsx
it('多账户时发件人是下拉按钮而非原生 select', () => {
  // …按既有方式 render ComposeDialog（两个账户）…
  expect(document.querySelector('select.compose-select')).toBeNull()
  const trigger = document.querySelector<HTMLButtonElement>('.compose-select-trigger')
  expect(trigger).not.toBeNull()
  expect(trigger!.getAttribute('aria-labelledby')).toBeTruthy()
})
```

- [ ] **Step 3: 跑测试确认失败**

Run: `pnpm exec vitest run src/components/mail/ComposeDialog.test.tsx`
Expected: 新用例 FAIL。

- [ ] **Step 4: 实现**

```tsx
<label id={`${fid}-from-label`}>{t('compose.from')}</label>
{multiFrom ? (
  <DropMenu
    align="start"
    items={fromOptions.map((o) => ({
      key: o.key,
      label: o.label,
      onSelect: () => set('fromOverride', o.key),
    }))}
    trigger={
      <button
        type="button"
        className="compose-select compose-select-trigger"
        aria-labelledby={`${fid}-from-label`}
        disabled={isBusy}
      >
        <span className="compose-select-text">{fromOption?.label ?? ''}</span>
        <Icon name="chevron-down" size={12} />
      </button>
    }
  />
) : (
  /* 原只读分支不变 */
)}
```
原 `<label htmlFor=...>` 换成上面带 `id` 的 label（单地址分支也沿用同一个 label）。
import `DropMenu`（`@/components/ui/DropMenu`）与 `Icon`（若未引入）。

CSS（紧跟现有 `.compose-select` 规则之后）：
```css
.compose-select-trigger {
  display: inline-flex; align-items: center; gap: 8px; justify-content: space-between;
  width: 100%; text-align: left; cursor: pointer;
}
.compose-select-trigger .compose-select-text { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
```

- [ ] **Step 5: 跑测试 + 构建**

Run: `pnpm exec vitest run src/components/mail/ComposeDialog.test.tsx src/lib/from-options.test.ts && pnpm build`
Expected: PASS。

---

### Task 13: 登录页品牌感

**Files:**
- Modify: `flymail/frontend/src/index.css:2780-2804`（`.login-page`、`.login-logo`）

- [ ] **Step 1: 改 CSS**

`.login-page` 的 `background: var(--bg);` 改为：
```css
  background:
    radial-gradient(60% 50% at 50% 0%, var(--accent-wash) 0%, transparent 70%),
    var(--bg);
```
`.login-logo` 的 `background: var(--accent-wash); color: var(--accent-ink);` 改为
`background: var(--primary); color: var(--primary-foreground);`（与 `.compose-btn` 同一对令牌，18 组主题对比已由 `dialog-buttons.test.ts` 的 primary 断言覆盖）。

- [ ] **Step 2: 跑既有 CSS 测试**

Run: `pnpm exec vitest run src/lib/dialog-buttons.test.ts src/lib/focus-ring.test.ts src/lib/ui-polish-r8.test.ts`
Expected: PASS。

---

### Task 14: 截图回归 + 全量验证 + 独立审查

**Files:**
- 无代码改动；产出 `docs/flymail/ui-polish-r8.md` 末尾「验收记录」一节。

- [ ] **Step 1: 全量验证**

Run（在 `flymail/frontend`）：
```bash
pnpm build && pnpm test 2>&1 | tail -8 && pnpm lint 2>&1 | tail -3
```
Expected：build 绿；test 仅既有那 1 项失败；lint 错误数 ≤ 17（既有）。若 lint 新增的错误落在本轮改动文件，修掉。

- [ ] **Step 2: 重建本机镜像并截图**

```bash
cd /home/dufeng/develop/workspace/go_dev/MailDev && docker compose up -d --build 2>&1 | tail -3
cd <scratchpad>/shot && node shoot.mjs 2>&1 | grep -v "^\s*$" | tail -20
```
逐张对照 spec 的 13 条「验证」：1440 inbox（kbd 单行）、1024 reader（工具条单行、发件人完整、三栏比例）、390 inbox（有 FAB、无 kbd）、390 reader、settings 打开时横幅被遮、侧栏「更多文件夹」、头像多色、登录页亮暗。

- [ ] **Step 3: 独立代码审查**

派 `oh-my-claudecode:code-reviewer`（或 `feature-dev:code-reviewer`）审 `git diff` 全部改动，给它 spec 与本计划路径；按严重度修复，再跑 Step 1。

- [ ] **Step 4: 写验收记录**

在 `docs/flymail/ui-polish-r8.md` 末尾追加「六、验收记录（日期）」：13 条各一行「✅/❌ + 证据（截图名或测试名）」，以及遗留项。

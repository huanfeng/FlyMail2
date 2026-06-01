# FlyMail 账户管理 UI 实现计划

> **For agentic workers:** 用 superpowers:subagent-driven-development 逐任务执行。前端为主，后端 CRUD+连接测试 API 已存在（M2），无需改后端。

**Goal:** 前端实现账户的添加/编辑/删除 + 连接测试 + 按邮箱域名自动填充服务器预设，让用户能在 UI 里加真实账号并触发 M3 同步。

**Architecture:** 纯前端。复用 M2 后端 API（`POST/PUT/DELETE /accounts`、`POST /accounts/test`）。新增 providers 预设表、扩展类型与 Query hooks、AccountDialog 对话框、Sidebar 的增删改入口。

**Tech Stack:** React 19 + TS + Tailwind 4 + radix-ui（Dialog）+ TanStack Query 5 + react-i18next。

---

## 后端 API 契约（已存在，前端对接用）
- `GET /accounts` → **裸数组** `AccountResponse[]`（注意非包裹）。
- `POST /accounts` → 201，body=AccountResponse。请求体 `CreateAccountRequest`：`name,email,password`(必填) + `username?`,`imap_host,imap_port,imap_security,smtp_host,smtp_port,smtp_security`,`proxy?`。
- `PUT /accounts/:id` → 200。`UpdateAccountRequest` 同上但 `password` 可空（空=保持原密码）。
- `DELETE /accounts/:id` → 200 `{status:"ok"}`。
- `POST /accounts/test` → 200 `ConnectionTestResult`：`{imap:bool, smtp:bool, supports_idle:bool, capabilities?:string[], security_mode?:string, imap_error?:string, smtp_error?:string}`。请求体同 CreateAccountRequest 的连接字段（email/username/password/imap_*/smtp_*/proxy）。
- AccountResponse：`id,name,email,username?,auth_type,imap_host,imap_port,imap_security,smtp_host,smtp_port,smtp_security,proxy?,status,last_sync_at?`（无密码）。

---

## Task A1：Provider 预设表 + 类型扩展

**Files:** Create `src/lib/providers.ts`；Modify `src/lib/types.ts`

- [ ] **Step 1:** 写 `src/lib/providers.ts`：
```ts
export type Security = 'ssl' | 'starttls' | 'none'

export interface ServerPreset {
  host: string
  port: number
  security: Security
}

export interface ProviderPreset {
  id: string
  name: string
  domains: string[]
  imap: ServerPreset
  smtp: ServerPreset
  note?: string
}

// 常见邮箱服务商预设（IMAP+SMTP）。国内邮箱 SMTP 统一 ssl/465；保存前可"测试连接"校验。
export const PROVIDER_PRESETS: ProviderPreset[] = [
  { id: 'gmail', name: 'Gmail', domains: ['gmail.com', 'googlemail.com'],
    imap: { host: 'imap.gmail.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.gmail.com', port: 465, security: 'ssl' },
    note: '需应用专用密码或 OAuth' },
  { id: 'outlook', name: 'Outlook', domains: ['outlook.com', 'hotmail.com', 'live.com', 'office365.com'],
    imap: { host: 'outlook.office365.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.office365.com', port: 587, security: 'starttls' } },
  { id: 'yahoo', name: 'Yahoo', domains: ['yahoo.com', 'ymail.com'],
    imap: { host: 'imap.mail.yahoo.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.mail.yahoo.com', port: 465, security: 'ssl' },
    note: '需应用专用密码' },
  { id: '163', name: '网易 163', domains: ['163.com'],
    imap: { host: 'imap.163.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.163.com', port: 465, security: 'ssl' },
    note: '需使用授权码（非登录密码），并在邮箱设置开启 IMAP/SMTP' },
  { id: '126', name: '网易 126', domains: ['126.com'],
    imap: { host: 'imap.126.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.126.com', port: 465, security: 'ssl' },
    note: '需使用授权码（非登录密码）' },
  { id: 'yeah', name: 'Yeah', domains: ['yeah.net'],
    imap: { host: 'imap.yeah.net', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.yeah.net', port: 465, security: 'ssl' },
    note: '需使用授权码（非登录密码）' },
  { id: 'qq', name: 'QQ 邮箱', domains: ['qq.com', 'foxmail.com'],
    imap: { host: 'imap.qq.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.qq.com', port: 465, security: 'ssl' },
    note: '需使用授权码（非登录密码），并在设置开启 IMAP/SMTP' },
  { id: 'sina', name: '新浪邮箱', domains: ['sina.com', 'sina.com.cn'],
    imap: { host: 'imap.sina.com.cn', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.sina.com.cn', port: 465, security: 'ssl' } },
  { id: 'sohu', name: '搜狐邮箱', domains: ['sohu.com'],
    imap: { host: 'imap.sohu.com', port: 993, security: 'ssl' },
    smtp: { host: 'smtp.sohu.com', port: 465, security: 'ssl' } },
]

/** 按邮箱域名匹配预设，未命中返回 null。 */
export function presetForEmail(email: string): ProviderPreset | null {
  const at = email.lastIndexOf('@')
  if (at < 0) return null
  const domain = email.slice(at + 1).trim().toLowerCase()
  if (!domain) return null
  return PROVIDER_PRESETS.find((p) => p.domains.includes(domain)) ?? null
}
```

- [ ] **Step 2:** 扩展 `src/lib/types.ts`：把 `Account` 扩成完整字段，新增 `AccountInput`、`ConnectionTestResult`、`ProxyInput`：
```ts
export interface Account {
  id: number
  name: string
  email: string
  username?: string
  auth_type: string
  imap_host: string
  imap_port: number
  imap_security: string
  smtp_host: string
  smtp_port: number
  smtp_security: string
  status: string
  last_sync_at?: string
}

export interface ProxyInput {
  type: string
  host: string
  port: number
  username?: string
  password?: string
}

// 添加/编辑/测试连接共用的输入（编辑时 password 留空=不改）。
export interface AccountInput {
  name: string
  email: string
  username?: string
  password?: string
  imap_host: string
  imap_port: number
  imap_security: string
  smtp_host: string
  smtp_port: number
  smtp_security: string
  proxy?: ProxyInput
}

export interface ConnectionTestResult {
  imap: boolean
  smtp: boolean
  supports_idle: boolean
  capabilities?: string[]
  security_mode?: string
  imap_error?: string
  smtp_error?: string
}
```
（保留已有的 Folder/Address/MessageListItem/SyncStatus 不变。）

- [ ] **Step 3:** 验证 `pnpm exec tsc -b --noEmit` 无错误。提交：`feat(flymail-fe): provider 预设表 + 账户类型扩展`

---

## Task A2：账户 CRUD + 测试连接 Query hooks

**Files:** Modify `src/lib/queries.ts`

- [ ] **Step 1:** 追加 hooks（`import type { Account, AccountInput, ConnectionTestResult } from '@/lib/types'`）：
```ts
export function useCreateAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: AccountInput): Promise<Account> => {
      const { data } = await api.post<Account>('/accounts', input)
      return data
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['accounts'] }) },
  })
}

export function useUpdateAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, input }: { id: number; input: AccountInput }): Promise<Account> => {
      const { data } = await api.put<Account>(`/accounts/${id}`, input)
      return data
    },
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['accounts'] }) },
  })
}

export function useDeleteAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: number): Promise<void> => {
      await api.delete(`/accounts/${id}`)
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['accounts'] })
      void qc.invalidateQueries({ queryKey: ['folders'] })
    },
  })
}

export function useTestConnection() {
  return useMutation({
    mutationFn: async (input: AccountInput): Promise<ConnectionTestResult> => {
      const { data } = await api.post<ConnectionTestResult>('/accounts/test', input)
      return data
    },
  })
}
```

- [ ] **Step 2:** 验证 `pnpm exec tsc -b --noEmit`。提交：`feat(flymail-fe): 账户 CRUD + 测试连接 Query hooks`

---

## Task A3：AccountDialog 添加/编辑对话框

**Files:** Create `src/components/mail/AccountDialog.tsx`

- [ ] **Step 1:** 实现对话框组件。要点：
  - props：`open: boolean`、`account: Account | null`（null=添加，非空=编辑）、`onOpenChange(open)`。
  - 本地表单 state 覆盖 AccountInput 全字段；编辑时用 account 预填（密码留空，占位提示"留空不修改"）。
  - 邮箱输入 onBlur：`presetForEmail(email)`，命中则填充 imap/smtp 各字段（仅当对应字段为空或处于"未手改"时填充——简单起见：命中即覆盖 host/port/security，但不动 name/password）。
  - "测试连接"按钮：调 `useTestConnection().mutate(buildInput())`，展示结果：IMAP ✓/✗、SMTP ✓/✗、错误文本（imap_error/smtp_error）。
  - "保存"：添加→`useCreateAccount`，编辑→`useUpdateAccount`；成功后 `onOpenChange(false)`。校验：name/email 必填；添加时 password 必填；端口为数字。
  - 用 radix-ui 的 Dialog（`import * as Dialog from 'radix-ui'` 的 Dialog 命名空间——按项目 radix-ui 1.4.3 实际导出方式，先看现有 `src/components/ui/*` 怎么用 radix；若无现成 Dialog 封装，用 radix 的 Dialog.Root/Portal/Overlay/Content）。复用 `@/components/ui/{button,input,label}`。
  - 文本全部走 i18n（`t('account.*')`）。安全方案中所有标签/按钮文案用 i18n key。

  > 实现者注意：先读 `src/components/ui/button.tsx`、`input.tsx`、`label.tsx` 确认导出与用法；读 `package.json` 确认 radix-ui 版本与导入风格（`radix-ui` 聚合包 1.4.3：`import { Dialog } from 'radix-ui'`）。表单字段分组：基本(名称/邮箱/用户名/密码) | IMAP(主机/端口/加密) | SMTP(主机/端口/加密)。加密用 select（ssl/starttls/none）。

- [ ] **Step 2:** 在 `src/locales/zh.json` 和 `en.json` 增加 `account` 段（无重复 key、合法 JSON）：
  zh 示例：`"account": { "add":"添加账户","edit":"编辑账户","delete":"删除账户","name":"名称","email":"邮箱","username":"用户名(可选)","password":"密码","passwordKeep":"留空则不修改","imap":"收件(IMAP)","smtp":"发件(SMTP)","host":"服务器","port":"端口","security":"加密","test":"测试连接","testing":"测试中…","save":"保存","cancel":"取消","imapOk":"IMAP 连接成功","imapFail":"IMAP 失败","smtpOk":"SMTP 连接成功","smtpFail":"SMTP 失败","deleteConfirm":"确认删除该账户？本地缓存的邮件也会移除。" }`
  en 对应英文。

- [ ] **Step 3:** 验证 `pnpm exec tsc -b --noEmit`。提交：`feat(flymail-fe): AccountDialog 添加/编辑表单（预设填充 + 连接测试）`

---

## Task A4：Sidebar 增删改入口 + Shell 接线

**Files:** Modify `src/components/mail/AccountSidebar.tsx`、`src/pages/Shell.tsx`

- [ ] **Step 1:** AccountSidebar：
  - 顶部品牌行右侧加"+"按钮（lucide `Plus`），点击触发 `onAddAccount()`。
  - 每个账户行：hover 显示编辑（lucide `Pencil`）与删除（lucide `Trash2`）小按钮（`e.stopPropagation()` 防冒泡到选择）；删除点击触发 `onDeleteAccount(account)`。
  - 新增 props：`onAddAccount()`、`onEditAccount(account: Account)`、`onDeleteAccount(account: Account)`。
  - 账户对象类型从 `Account`（已扩展）取。

- [ ] **Step 2:** Shell：
  - 引入 `AccountDialog`、`useDeleteAccount`。
  - state：`dialogOpen`、`editingAccount: Account | null`。
  - `onAddAccount` → 设 editingAccount=null、dialogOpen=true；`onEditAccount(a)` → editingAccount=a、dialogOpen=true。
  - `onDeleteAccount(a)` → `window.confirm(t('account.deleteConfirm'))` 通过则 `deleteAccount.mutate(a.id)`；若删的是当前选中账户，清空 url 的 account/folder/message。
  - 渲染 `<AccountDialog open={dialogOpen} account={editingAccount} onOpenChange={setDialogOpen} />`。
  - 把三个回调透传给 AccountSidebar。

- [ ] **Step 3:** 验证 `pnpm exec tsc -b --noEmit` + `pnpm build` 成功。提交：`feat(flymail-fe): Sidebar 增删改入口 + Shell 接线账户对话框`

---

## Task A5：构建 + 真机验证

- [ ] **Step 1:** `pnpm build`（产物进 backend/web/dist）+ 后端 `go build`。
- [ ] **Step 2:**（主控执行）用 admin/admin 起服务 + 无头浏览器：打开 → 登录 → 点"+"添加账户 → 邮箱填 `x@163.com` 验证自动填充 imap.163.com/smtp.163.com → 测试连接（无效凭证应返回结构化失败，不崩）→ 填名称/密码保存 → 账户出现在侧栏 → 编辑/删除可用。
- [ ] **Step 3:** 提交（如有）。

---

## 自检
- 覆盖：预设(A1)、CRUD+test hooks(A2)、对话框 add/edit+preset+test(A3)、sidebar 增删改+Shell 接线(A4)、验证(A5)。✅
- 后端无改动（API 全复用 M2）。
- i18n：account 段 zh/en 对齐，无重复 key。
- 编辑态密码留空=不改（后端 UpdateAccountRequest.password omitempty 语义）。

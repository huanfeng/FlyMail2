// 规则引擎 / 黑名单的纯逻辑层：选项表、校验、数组重排、黑名单 pattern 归一化。
//
// 为什么单独成文件：这些判断在「保存前拦截」「右键屏蔽发件人」「上下箭头排序」三处都要用，
// 且全都是无 IO 的纯函数——放这里才能被 vitest 直接覆盖，不必渲染整棵组件树。

import type { RuleAction, RuleActionType, RuleCondition, RuleField, RuleInput, RuleOp } from '@/lib/types'

/** 条件字段的可选集合（渲染下拉框的顺序即此顺序） */
export const RULE_FIELDS: readonly RuleField[] = [
  'from',
  'to',
  'cc',
  'subject',
  'body',
  'attachment_name',
  'has_attachment',
]

/** 文本字段可用的运算符 */
export const TEXT_OPS: readonly RuleOp[] = [
  'contains',
  'not_contains',
  'equals',
  'regex',
  'starts_with',
  'ends_with',
]

/** 动作类型的可选集合 */
export const RULE_ACTION_TYPES: readonly RuleActionType[] = ['move', 'mark_read', 'star', 'delete', 'notify']

/** 布尔字段：值域固定为 true/false，运算符只有 equals 有意义 */
export function isBooleanField(field: RuleField): boolean {
  return field === 'has_attachment'
}

/**
 * 该字段允许的运算符。
 * has_attachment 只认 equals——后端同样只实现了这一条，前端提前收窄能避免存下一条永不命中的规则。
 */
export function opsForField(field: RuleField): readonly RuleOp[] {
  return isBooleanField(field) ? (['equals'] as const) : TEXT_OPS
}

/** move 之外的动作类型没有取值，统一写空串，避免后端收到脏数据 */
export function actionNeedsValue(type: RuleActionType): boolean {
  return type === 'move'
}

/** 切换字段后修正条件行：运算符与值都可能对新字段无效 */
export function normalizeCondition(cond: RuleCondition): RuleCondition {
  const ops = opsForField(cond.field)
  const op = ops.includes(cond.op) ? cond.op : ops[0]
  if (isBooleanField(cond.field)) {
    return { field: cond.field, op, value: cond.value === 'false' ? 'false' : 'true' }
  }
  // 从布尔字段切回文本字段时，'true'/'false' 只是残留的哨兵值，清掉更符合直觉
  const value = cond.value === 'true' || cond.value === 'false' ? '' : cond.value
  return { field: cond.field, op, value }
}

/** 切换动作类型后修正动作行 */
export function normalizeAction(action: RuleAction): RuleAction {
  return { type: action.type, value: actionNeedsValue(action.type) ? action.value : '' }
}

/**
 * 数组元素移动（上下箭头调优先级用）。
 * 越界一律原样返回副本：调用方在首行按「上移」是常态，不该抛错也不该产生空洞。
 */
export function moveArrayItem<T>(items: readonly T[], from: number, to: number): T[] {
  const next = items.slice()
  if (from < 0 || from >= next.length || to < 0 || to >= next.length || from === to) return next
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  return next
}

/**
 * 用 JS 引擎粗筛正则语法。这是提示，不是判决——最终以后端的 `regexp.Compile`（RE2）为准。
 *
 * 两个引擎的语法只是大部分重合，直接 `new RegExp` 会误杀三种常见的 Go 写法：
 * 内联标志 `(?i)abc`、带作用域的标志组 `(?i:abc)`（JS 两种都不认，只接受字面量末尾的 /i），
 * 以及命名捕获 `(?P<name>...)`（JS 写作 `(?<name>...)`）。文档里恰好教用户写 `(?i)`，
 * 不翻译就会把正确的规则拦在保存之外，所以先译成 JS 等价写法再编译。
 *
 * 反向也漏：`(\w)\1` 反向引用、`(?=foo)` `(?<=a)b` 断言在 JS 合法而 RE2 拒绝，
 * 这里会放行，由保存时后端的 400 兜底——所以界面上的提示措辞是「可能无法编译」。
 */
export function isCompilableRegex(pattern: string): boolean {
  const translated = pattern
    // 带作用域的标志组 (?i:...) (?is:...) → 普通非捕获组
    .replace(/\(\?[imsU-]+:/g, '(?:')
    // 独立的内联标志 (?i) (?is) (?-s)：去掉后不影响语法合法性判断
    .replace(/\(\?[imsU-]+\)/g, '')
    .replace(/\(\?P</g, '(?<')
  try {
    new RegExp(translated)
    return true
  } catch {
    return false
  }
}

/** 规则校验失败的原因码，直接映射到 i18n key `settings.rules.err.<code>` */
export type RuleErrorCode = 'name' | 'conditions' | 'actions' | 'value' | 'regex' | 'moveTarget'

export interface RuleValidationError {
  code: RuleErrorCode
  /** 出错的条件/动作行下标（用于高亮），regex 时同时带上原始 pattern */
  index?: number
  detail?: string
}

/**
 * 保存前校验。返回 null 表示可以提交。
 * regex 的语法粗筛见 {@link isCompilableRegex}：在前端给出行内提示，比等一个 400 再翻译错误信息体验好得多。
 */
export function validateRuleInput(input: RuleInput): RuleValidationError | null {
  if (!input.name.trim()) return { code: 'name' }
  if (input.conditions.length === 0) return { code: 'conditions' }
  if (input.actions.length === 0) return { code: 'actions' }

  for (let i = 0; i < input.conditions.length; i++) {
    const cond = input.conditions[i]
    if (isBooleanField(cond.field)) continue
    if (!cond.value.trim()) return { code: 'value', index: i }
    if (cond.op === 'regex' && !isCompilableRegex(cond.value)) {
      return { code: 'regex', index: i, detail: cond.value }
    }
  }

  for (let i = 0; i < input.actions.length; i++) {
    const act = input.actions[i]
    if (actionNeedsValue(act.type) && !act.value.trim()) return { code: 'moveTarget', index: i }
  }

  return null
}

/** 执行日志里被拆开的单个动作 */
export interface RunActionToken {
  /** 枚举名（`move:归档` 取 `move`），未知取值原样落在这里 */
  type: string
  /** move 的目标文件夹名 / block 的命中地址；其余动作为空串 */
  value: string
  /** 原样片段，识别不了时按这个显示 */
  raw: string
}

/**
 * 只在「逗号后面紧跟着一个已知动作前缀」处切分。
 *
 * 后端用逗号拼接动作，而 `move:` 的参数是文件夹名——名字里带逗号的文件夹（「发票, 收据」）
 * 按裸逗号切会被拆成两截。要求逗号后必须是下一个动作的开头，这种名字就能整段保住。
 */
const RUN_ACTION_SPLIT = /\s*,\s*(?=(?:mark_read|star|delete|notify|move:|block:))/

/**
 * 拆解 RuleRunDTO.action。
 *
 * 后端给的是枚举拼接串而非可读文案：逗号分隔多个动作，带参数的写成 `move:归档` / `block:spam.io`。
 * 每段只按第一个冒号切分，目标文件夹名里再出现冒号也不会被截断。
 * 剩下的死角：文件夹名恰好以某个动作名开头且前面带逗号（`move:发票,star 相关`）仍会被误拆，
 * 要彻底解决得让后端换成结构化字段。
 */
export function parseRunAction(action: string): RunActionToken[] {
  return action
    .split(RUN_ACTION_SPLIT)
    // 前瞻切分不消费「后面不跟动作名」的逗号，段首尾可能残留分隔符（空串、`,`、`move:x,`）
    .map((s) => s.replace(/^[\s,]+/, '').replace(/[\s,]+$/, ''))
    .filter((s) => s.length > 0)
    .map((seg) => {
      const at = seg.indexOf(':')
      if (at < 0) return { type: seg, value: '', raw: seg }
      return { type: seg.slice(0, at).trim(), value: seg.slice(at + 1).trim(), raw: seg }
    })
}

/** move 动作在执行日志里可能带的结果标记 */
export type MoveNote = 'skipped' | 'missing' | 'same' | ''

/**
 * 把 `归档(missing)` 拆成目标名与结果标记。
 *
 * 后端在移动没有真的发生时会给目标名加个后缀：同一封被多条规则要求移动时落选记 `(skipped)`，
 * 目标文件夹解析不到记 `(missing)`，目标就是当前文件夹记 `(same)`。
 * 不拆开的话界面会原样显示英文括号，读起来像文件夹名的一部分。
 */
export function splitMoveNote(value: string): { target: string; note: MoveNote } {
  const m = /^(.*?)\s*\((skipped|missing|same)\)$/.exec(value.trim())
  if (!m) return { target: value.trim(), note: '' }
  return { target: (m[1] ?? '').trim(), note: m[2] as MoveNote }
}

/** 是否为已知动作类型（决定能不能翻译成文案，未知的原样显示） */
export function isKnownActionType(type: string): type is RuleActionType {
  return (RULE_ACTION_TYPES as readonly string[]).includes(type)
}

/**
 * 黑名单 pattern 归一化：小写、去空白、剥掉显示名与 mailto:、域名去掉前导 @。
 *
 * 右键菜单直接把 from_addr 丢进来，而 from_addr 在某些服务端会带成
 * `Alice <alice@example.com>` 或 `<alice@example.com>`，不剥就会存成一条永不命中的记录。
 */
export function normalizeBlockPattern(raw: string): string {
  let s = raw.trim()
  // 优先取尖括号里的地址（`Alice <a@x.com>` → `a@x.com`）
  const angled = /<([^<>]+)>/.exec(s)
  if (angled?.[1]) s = angled[1]
  s = s.trim().toLowerCase()
  if (s.startsWith('mailto:')) s = s.slice('mailto:'.length)
  // 域名形式允许用户输入 `@example.com`，存库统一不带 @
  while (s.startsWith('@')) s = s.slice(1)
  return s.trim()
}

/**
 * 归一化后的 pattern 是否可用：完整地址（a@b.c）或域名（b.c）。
 * 只做基础形状校验，真正的权威判断在后端——这里的目的是拦住空串和明显的手滑。
 */
export function isValidBlockPattern(pattern: string): boolean {
  if (!pattern || /\s/.test(pattern)) return false
  const at = pattern.indexOf('@')
  if (at >= 0) {
    // 地址形式：有且只有一个 @，两侧都非空，域名部分合法
    if (at !== pattern.lastIndexOf('@')) return false
    const local = pattern.slice(0, at)
    const domain = pattern.slice(at + 1)
    return local.length > 0 && isDomain(domain)
  }
  return isDomain(pattern)
}

/** 域名形状：至少一个点，各段非空且只含字母数字与连字符 */
function isDomain(s: string): boolean {
  if (!s.includes('.')) return false
  const labels = s.split('.')
  return labels.length >= 2 && labels.every((l) => /^[a-z0-9-]+$/.test(l) && !l.startsWith('-') && !l.endsWith('-'))
}

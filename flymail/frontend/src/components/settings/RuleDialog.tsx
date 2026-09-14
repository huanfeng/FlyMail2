// 规则添加/编辑对话框：与 ChannelDialog 同一套 radix Dialog 模式（z-70/80 叠在设置弹框之上）。
//
// 条件行 / 动作行都是「选择器 + 值 + 删除」的定长结构，没有引拖拽库——
// 条件之间无序（match=all/any 决定语义），顺序不影响结果，增删就够用。
//
// 表单状态放在内层的 RuleForm 里并用 key 重挂载（React 官方的 reset-by-key），
// 而不是在外层用 useEffect 把 props 抄进 state：关闭时 radix 本来就会卸载 Portal 内容，
// 那个 effect 既多余又会在每次打开时多跑一轮渲染。

import * as React from 'react'
import { Dialog } from 'radix-ui'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Icon } from '@/components/ui/Icon'
import { apiErrorMessage } from '@/lib/api'
import { useAccounts, useCreateRule, useFoldersOfAccounts, useTestRule, useUpdateRule } from '@/lib/queries'
import {
  RULE_ACTION_TYPES,
  RULE_FIELDS,
  actionNeedsValue,
  isBooleanField,
  normalizeAction,
  normalizeCondition,
  opsForField,
  validateRuleInput,
} from '@/lib/rules'
import type { RuleValidationError } from '@/lib/rules'
import type { Rule, RuleAction, RuleActionType, RuleCondition, RuleField, RuleInput, RuleOp } from '@/lib/types'

/**
 * 试运行扫描多少封最近的邮件。
 * 后端的 limit 说的是扫描规模（默认 200，上限 500），命中列表另有固定的 50 条回传上限，
 * 超过时响应里的 truncated 为 true。这里往上要一点，让「扫描 N 封」这个分母更有说服力。
 */
const TEST_SCAN_LIMIT = 300

/** 与 AccountDialog 的 SecuritySelect 同一套下拉框样式 */
const SELECT_CLASS =
  'h-9 rounded-md border border-input bg-transparent px-2 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50'

export interface RuleDialogProps {
  open: boolean
  /** null = 添加模式，非空 = 编辑模式 */
  rule: Rule | null
  onOpenChange: (open: boolean) => void
}

/**
 * 行 id 生成器。
 *
 * 不用 crypto.randomUUID()：桌面端（Wails）经 WebView2 自定义协议加载时不一定处在安全上下文，
 * 那里它可能是 undefined。行 id 只需在本次会话内唯一，自增计数足够。
 */
let rowSeq = 0
function nextRowId(): string {
  rowSeq += 1
  return `row-${rowSeq}`
}

/** 带稳定 id 的条件行 / 动作行 */
interface ConditionRow {
  id: string
  cond: RuleCondition
}
interface ActionRow {
  id: string
  act: RuleAction
}

/**
 * 表单状态。
 *
 * 条件与动作不直接用 RuleInput 里的裸数组：React 需要稳定的 key 才能把某一行的 DOM
 * 认成同一行。用下标当 key 时删掉中间一行，后面每行的 key 都往前挪一位，输入框被复用给了
 * 相邻的行——正在用输入法打字的话，未上屏的组合态会串到别的行去。
 */
interface FormState {
  name: string
  enabled: boolean
  account_id: number
  match: 'all' | 'any'
  conditions: ConditionRow[]
  actions: ActionRow[]
  stop_processing: boolean
}

function initialForm(rule: Rule | null): FormState {
  if (!rule) {
    return {
      name: '',
      enabled: true,
      account_id: 0,
      match: 'all',
      conditions: [{ id: nextRowId(), cond: { field: 'from', op: 'contains', value: '' } }],
      actions: [{ id: nextRowId(), act: { type: 'mark_read', value: '' } }],
      stop_processing: false,
    }
  }
  return {
    name: rule.name,
    enabled: rule.enabled,
    account_id: rule.account_id,
    match: rule.match,
    // 深拷贝：直接引用缓存里的数组会让编辑中的改动渗回列表
    conditions: rule.conditions.map((c) => ({ id: nextRowId(), cond: { ...c } })),
    actions: rule.actions.map((a) => ({ id: nextRowId(), act: { ...a } })),
    stop_processing: rule.stop_processing,
  }
}

interface FieldProps {
  label: string
  hint?: string
  children: React.ReactNode
}

function Field({ label, hint, children }: FieldProps) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label style={{ color: 'var(--ink-2)', fontSize: '0.8125rem' }}>{label}</Label>
      {children}
      {hint && <div style={{ fontSize: '0.75rem', color: 'var(--ink-3)' }}>{hint}</div>}
    </div>
  )
}

export function RuleDialog({ open, rule, onOpenChange }: RuleDialogProps) {
  const { t } = useTranslation()
  const isEdit = rule !== null

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay
          className="fixed inset-0 z-[70]"
          style={{ background: 'rgba(0,0,0,0.35)', backdropFilter: 'blur(2px)' }}
        />
        <Dialog.Content
          className="fixed left-1/2 top-1/2 z-[80] -translate-x-1/2 -translate-y-1/2 w-[560px] max-w-[calc(100vw-2rem)] max-h-[90vh] overflow-hidden rounded-xl shadow-xl flex flex-col gap-0 outline-none"
          style={{ background: 'var(--surface)', color: 'var(--ink)' }}
          aria-describedby={undefined}
        >
          {/* 标题栏 */}
          <div className="flex items-center justify-between px-6 py-4" style={{ borderBottom: '1px solid var(--rule)' }}>
            <Dialog.Title className="text-base font-semibold" style={{ margin: 0 }}>
              {isEdit ? t('settings.rules.editRule') : t('settings.rules.addRule')}
            </Dialog.Title>
            <Dialog.Close asChild>
              <button
                className="rounded-md p-1 text-sm opacity-60 hover:opacity-100 transition-opacity outline-none focus-visible:ring-2 focus-visible:ring-ring"
                style={{ color: 'var(--ink-3)', lineHeight: 1 }}
                aria-label={t('settings.rules.cancel')}
              >
                ✕
              </button>
            </Dialog.Close>
          </div>

          {/* key 切换编辑目标时丢弃旧表单状态与旧的试运行结果 */}
          <RuleForm key={rule?.id ?? 'new'} rule={rule} onSaved={() => onOpenChange(false)} />
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

interface RuleFormProps {
  rule: Rule | null
  onSaved: () => void
}

function RuleForm({ rule, onSaved }: RuleFormProps) {
  const { t } = useTranslation()
  const isEdit = rule !== null

  const [form, setForm] = React.useState<FormState>(() => initialForm(rule))
  const [error, setError] = React.useState<RuleValidationError | null>(null)
  // 保存失败时后端返回的文案（校验错误走 error，两者显示在同一处）
  const [saveError, setSaveError] = React.useState<string | null>(null)

  const { data: accounts = [] } = useAccounts()
  const createRule = useCreateRule()
  const updateRule = useUpdateRule()
  const testRule = useTestRule()
  const isSaving = createRule.isPending || updateRule.isPending

  // 「移动到」的候选账户：规则限定了账户就只列该账户，否则列全部账户
  const targetAccountIds = React.useMemo(
    () => (form.account_id === 0 ? accounts.map((a) => a.id) : [form.account_id]),
    [accounts, form.account_id],
  )
  const { folders } = useFoldersOfAccounts(targetAccountIds)

  /**
   * 目标文件夹名的候选集：按 display_name 去重。
   * 后端跨账户按名字解析目标，所以这里也只能给名字——两个账户各有一个「归档」时它们是同一个选项。
   */
  const folderNames = React.useMemo(() => {
    const seen = new Set<string>()
    for (const f of folders) {
      if (f.selectable && f.display_name) seen.add(f.display_name)
    }
    return [...seen].sort((a, b) => a.localeCompare(b))
  }, [folders])

  /**
   * 所有表单改动的唯一入口。
   * 顺带丢弃上一次的试运行结果：改完条件后旧的命中列表就不再对应当前规则，
   * 留在那里会让人以为改动已经被验证过。
   */
  function updateForm(patch: (prev: FormState) => FormState) {
    setForm(patch)
    setSaveError(null)
    testRule.reset()
  }

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    updateForm((prev) => ({ ...prev, [key]: value }))
  }

  /**
   * 切换作用账户：把所有「移动到」的目标清空。
   * 目标是按名字解析的，候选集随账户变化，而新账户的文件夹此刻还没加载完，
   * 无从判断旧目标是否仍然存在——留一个可能不存在的名字，规则会保存成功却永远不动。
   */
  function changeAccount(accountId: number) {
    updateForm((prev) => ({
      ...prev,
      account_id: accountId,
      actions: prev.actions.map((r) => (r.act.type === 'move' ? { ...r, act: { ...r.act, value: '' } } : r)),
    }))
  }

  // ── 条件行 ──
  function patchCondition(id: string, patch: Partial<RuleCondition>) {
    updateForm((prev) => ({
      ...prev,
      conditions: prev.conditions.map((r) =>
        r.id === id ? { ...r, cond: normalizeCondition({ ...r.cond, ...patch }) } : r,
      ),
    }))
  }
  function addCondition() {
    updateForm((prev) => ({
      ...prev,
      conditions: [...prev.conditions, { id: nextRowId(), cond: { field: 'from', op: 'contains', value: '' } }],
    }))
  }
  function removeCondition(id: string) {
    updateForm((prev) => ({ ...prev, conditions: prev.conditions.filter((r) => r.id !== id) }))
  }

  // ── 动作行 ──
  function patchAction(id: string, patch: Partial<RuleAction>) {
    updateForm((prev) => ({
      ...prev,
      actions: prev.actions.map((r) => (r.id === id ? { ...r, act: normalizeAction({ ...r.act, ...patch }) } : r)),
    }))
  }
  function addAction() {
    updateForm((prev) => ({
      ...prev,
      actions: [...prev.actions, { id: nextRowId(), act: { type: 'mark_read', value: '' } }],
    }))
  }
  function removeAction(id: string) {
    updateForm((prev) => ({ ...prev, actions: prev.actions.filter((r) => r.id !== id) }))
  }

  /** 剥掉只在编辑期存在的行 id，得到可以直接提交的 RuleInput */
  function buildInput(): RuleInput {
    return {
      name: form.name.trim(),
      enabled: form.enabled,
      account_id: form.account_id,
      match: form.match,
      // 条件值一律 trim：正则末尾一个看不见的空格就是一条不同的规则，
      // 复制粘贴带进来的尾随空白会让人对着「为什么不命中」发呆
      conditions: form.conditions.map((r) => ({ ...r.cond, value: r.cond.value.trim() })),
      actions: form.actions.map((r) => ({ ...r.act, value: r.act.value.trim() })),
      stop_processing: form.stop_processing,
    }
  }

  function handleTest() {
    const input = buildInput()
    const err = validateRuleInput(input)
    if (err) {
      setError(err)
      return
    }
    setError(null)
    setSaveError(null)
    testRule.mutate({ rule: input, limit: TEST_SCAN_LIMIT })
  }

  function handleSave() {
    const input = buildInput()
    const err = validateRuleInput(input)
    if (err) {
      setError(err)
      return
    }
    setError(null)
    setSaveError(null)
    // 后端会做前端做不到的校验（RE2 能否编译、目标文件夹在该账户里是否存在），
    // 它的中文错误文案比任何通用提示都准确，原样展示
    const onError = (e: unknown) => setSaveError(apiErrorMessage(e, t('settings.rules.saveFailed')))
    if (isEdit && rule) {
      updateRule.mutate({ id: rule.id, input }, { onSuccess: onSaved, onError })
    } else {
      createRule.mutate(input, { onSuccess: onSaved, onError })
    }
  }

  const errorText = error
    ? error.code === 'regex'
      ? t('settings.rules.err.regex', { pattern: error.detail ?? '' })
      : t(`settings.rules.err.${error.code}`)
    : null

  return (
    <>
      {/* 表单主体（唯一滚动区：外层 overflow-hidden 保圆角，头尾固定） */}
      <div className="flex-1 min-h-0 overflow-y-auto flex flex-col gap-4 px-6 py-5">
        <Field label={t('settings.rules.name')}>
          <Input value={form.name} onChange={(e) => set('name', e.target.value)} />
        </Field>

        <Field label={t('settings.rules.account')} hint={t('settings.rules.accountHint')}>
          <select
            className={SELECT_CLASS + ' w-full'}
            style={{ color: 'var(--ink)' }}
            value={String(form.account_id)}
            onChange={(e) => changeAccount(Number(e.target.value))}
          >
            <option value="0">{t('settings.rules.allAccounts')}</option>
            {accounts.map((a) => (
              <option key={a.id} value={String(a.id)}>
                {a.name} · {a.email}
              </option>
            ))}
          </select>
        </Field>

        <Field label={t('settings.rules.match')}>
          <div className="mode-toggle" style={{ alignSelf: 'flex-start' }}>
            <button type="button" className={form.match === 'all' ? 'active' : ''} onClick={() => set('match', 'all')}>
              {t('settings.rules.matchAll')}
            </button>
            <button type="button" className={form.match === 'any' ? 'active' : ''} onClick={() => set('match', 'any')}>
              {t('settings.rules.matchAny')}
            </button>
          </div>
        </Field>

        {/* ── 条件 ── */}
        <Field label={t('settings.rules.conditions')}>
          <div className="flex flex-col gap-2">
            {form.conditions.map(({ id, cond }) => (
              <div key={id} className="flex items-center gap-2">
                <select
                  className={SELECT_CLASS}
                  style={{ color: 'var(--ink)', flex: '0 0 120px' }}
                  value={cond.field}
                  onChange={(e) => patchCondition(id, { field: e.target.value as RuleField })}
                  aria-label={t('settings.rules.condField')}
                >
                  {RULE_FIELDS.map((f) => (
                    <option key={f} value={f}>{t(`settings.rules.field.${f}`)}</option>
                  ))}
                </select>
                <select
                  className={SELECT_CLASS}
                  style={{ color: 'var(--ink)', flex: '0 0 108px' }}
                  value={cond.op}
                  onChange={(e) => patchCondition(id, { op: e.target.value as RuleOp })}
                  disabled={isBooleanField(cond.field)}
                  aria-label={t('settings.rules.condOp')}
                >
                  {opsForField(cond.field).map((op) => (
                    <option key={op} value={op}>{t(`settings.rules.op.${op}`)}</option>
                  ))}
                </select>
                {isBooleanField(cond.field) ? (
                  <select
                    className={SELECT_CLASS}
                    style={{ color: 'var(--ink)', flex: 1, minWidth: 0 }}
                    value={cond.value}
                    onChange={(e) => patchCondition(id, { value: e.target.value })}
                    aria-label={t('settings.rules.condValue')}
                  >
                    <option value="true">{t('settings.rules.boolYes')}</option>
                    <option value="false">{t('settings.rules.boolNo')}</option>
                  </select>
                ) : (
                  <Input
                    style={{ flex: 1, minWidth: 0 }}
                    value={cond.value}
                    onChange={(e) => patchCondition(id, { value: e.target.value })}
                    placeholder={cond.op === 'regex' ? t('settings.rules.regexPlaceholder') : ''}
                    aria-label={t('settings.rules.condValue')}
                  />
                )}
                <button
                  type="button"
                  className="icon-btn"
                  onClick={() => removeCondition(id)}
                  title={t('settings.rules.removeRow')}
                  aria-label={t('settings.rules.removeRow')}
                  style={{ flexShrink: 0 }}
                >
                  <Icon name="minus" size={13} />
                </button>
              </div>
            ))}
            <button type="button" className="pill-btn" style={{ alignSelf: 'flex-start' }} onClick={addCondition}>
              <Icon name="plus" size={12} />{t('settings.rules.addCondition')}
            </button>
          </div>
        </Field>

        {/* ── 动作 ── */}
        <Field label={t('settings.rules.actions')}>
          <div className="flex flex-col gap-2">
            {form.actions.map(({ id, act }) => (
              <div key={id} className="flex items-center gap-2">
                <select
                  className={SELECT_CLASS}
                  style={{ color: 'var(--ink)', flex: '0 0 148px' }}
                  value={act.type}
                  onChange={(e) => patchAction(id, { type: e.target.value as RuleActionType })}
                  aria-label={t('settings.rules.actionType')}
                >
                  {RULE_ACTION_TYPES.map((ty) => (
                    <option key={ty} value={ty}>{t(`settings.rules.action.${ty}`)}</option>
                  ))}
                </select>
                {actionNeedsValue(act.type) && (
                  <select
                    className={SELECT_CLASS}
                    style={{ color: 'var(--ink)', flex: 1, minWidth: 0 }}
                    value={act.value}
                    onChange={(e) => patchAction(id, { value: e.target.value })}
                    aria-label={t('settings.rules.moveTarget')}
                  >
                    <option value="">{t('settings.rules.pickFolder')}</option>
                    {folderNames.map((n) => (
                      <option key={n} value={n}>{n}</option>
                    ))}
                  </select>
                )}
                <button
                  type="button"
                  className="icon-btn"
                  onClick={() => removeAction(id)}
                  title={t('settings.rules.removeRow')}
                  aria-label={t('settings.rules.removeRow')}
                  style={{ flexShrink: 0, marginLeft: actionNeedsValue(act.type) ? 0 : 'auto' }}
                >
                  <Icon name="minus" size={13} />
                </button>
              </div>
            ))}
            <button type="button" className="pill-btn" style={{ alignSelf: 'flex-start' }} onClick={addAction}>
              <Icon name="plus" size={12} />{t('settings.rules.addAction')}
            </button>
          </div>
        </Field>

        {/* 命中后停止 */}
        <div className="flex items-center justify-between" style={{ gap: 16 }}>
          <div style={{ minWidth: 0 }}>
            <div style={{ fontSize: 13.5, color: 'var(--ink)', fontWeight: 500 }}>{t('settings.rules.stop')}</div>
            <div style={{ fontSize: 12.5, color: 'var(--ink-3)', marginTop: 2 }}>{t('settings.rules.stopHint')}</div>
          </div>
          <button
            type="button"
            role="switch"
            aria-checked={form.stop_processing}
            className={'toggle' + (form.stop_processing ? ' on' : '')}
            onClick={() => set('stop_processing', !form.stop_processing)}
            aria-label={t('settings.rules.stop')}
            style={{ flexShrink: 0 }}
          />
        </div>

        {(errorText || saveError) && (
          <div style={{ fontSize: '0.8125rem', color: 'var(--destructive)' }}>{errorText ?? saveError}</div>
        )}

        {/* ── 试运行 ── */}
        <div style={{ borderTop: '1px solid var(--rule)', paddingTop: 14 }}>
          <button type="button" className="pill-btn" onClick={handleTest} disabled={testRule.isPending}>
            <Icon name="search" size={12} />
            {testRule.isPending ? t('settings.rules.testing') : t('settings.rules.test')}
          </button>
          <div style={{ fontSize: 12, color: 'var(--ink-3)', marginTop: 6 }}>{t('settings.rules.testHint')}</div>

          {testRule.isError && (
            <div style={{ fontSize: '0.8125rem', color: 'var(--destructive)', marginTop: 8 }}>
              {apiErrorMessage(testRule.error, t('settings.rules.testFailed'))}
            </div>
          )}

          {testRule.data && (
            <div style={{ marginTop: 10 }}>
              <div style={{ fontSize: 12.5, color: 'var(--ink-2)' }}>
                {t('settings.rules.testSummary', {
                  matched: testRule.data.matched.length,
                  scanned: testRule.data.scanned,
                })}
                {testRule.data.without_body > 0 && (
                  <span style={{ color: 'var(--ink-3)' }}>
                    {' · '}
                    {t('settings.rules.testWithoutBody', { n: testRule.data.without_body })}
                  </span>
                )}
                {testRule.data.truncated && (
                  <span style={{ color: 'var(--ink-3)' }}>
                    {' · '}
                    {t('settings.rules.testTruncated', { n: testRule.data.matched.length })}
                  </span>
                )}
              </div>
              {testRule.data.matched.length === 0 ? (
                <div style={{ fontSize: 12.5, color: 'var(--ink-3)', marginTop: 8 }}>
                  {t('settings.rules.testNoMatch')}
                </div>
              ) : (
                <div style={{ marginTop: 8, maxHeight: 220, overflowY: 'auto' }}>
                  {testRule.data.matched.map((m) => (
                    <div
                      key={m.id}
                      style={{
                        display: 'flex',
                        alignItems: 'baseline',
                        gap: 10,
                        padding: '5px 0',
                        fontSize: 12.5,
                        borderBottom: '1px solid var(--rule)',
                      }}
                    >
                      <span style={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {m.subject || t('settings.rules.noSubject')}
                      </span>
                      <span
                        style={{
                          color: 'var(--ink-3)',
                          fontFamily: 'var(--font-mono)',
                          flex: '0 0 auto',
                          maxWidth: 170,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                        }}
                      >
                        {m.from_addr}
                      </span>
                      <span style={{ color: 'var(--ink-3)', fontFamily: 'var(--font-mono)', flexShrink: 0 }}>
                        {new Date(m.date).toLocaleDateString()}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* 底部操作 */}
      <div className="flex items-center justify-end gap-2 px-6 py-4" style={{ borderTop: '1px solid var(--rule)' }}>
        <Dialog.Close asChild>
          <Button variant="outline">{t('settings.rules.cancel')}</Button>
        </Dialog.Close>
        <Button onClick={handleSave} disabled={isSaving}>
          {t('settings.rules.save')}
        </Button>
      </div>
    </>
  )
}

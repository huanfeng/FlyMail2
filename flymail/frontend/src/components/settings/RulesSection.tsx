// 设置 → 规则：规则列表（启用开关 / 上下箭头调优先级 / 编辑 / 删除）+ 执行日志折叠区。
// 添加与编辑都走 RuleDialog（radix，浮于设置弹框之上），列表本身不做内嵌变形。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Icon } from '@/components/ui/Icon'
import { useToast } from '@/components/ui/Toast'
import { RuleDialog } from './RuleDialog'
import { apiErrorMessage } from '@/lib/api'
import { useAccounts, useDeleteRule, useReorderRules, useRuleRuns, useRules, useUpdateRule } from '@/lib/queries'
import { isKnownActionType, moveArrayItem, parseRunAction, splitMoveNote } from '@/lib/rules'
import type { Rule, RuleAction, RuleCondition } from '@/lib/types'

export function RulesSection() {
  const { t } = useTranslation()
  const { toast } = useToast()
  const { data: rules = [] } = useRules()
  const { data: accounts = [] } = useAccounts()
  const updateRule = useUpdateRule()
  const deleteRule = useDeleteRule()
  const reorder = useReorderRules()

  const [dialogOpen, setDialogOpen] = React.useState(false)
  const [editing, setEditing] = React.useState<Rule | null>(null)
  const [showRuns, setShowRuns] = React.useState(false)
  // 日志只在展开时拉取：这是纯诊断信息，不值得为它在每次打开设置页时多发一个请求
  const { data: runs = [] } = useRuleRuns(showRuns)

  function openAdd() {
    setEditing(null)
    setDialogOpen(true)
  }
  function openEdit(r: Rule) {
    setEditing(r)
    setDialogOpen(true)
  }

  /** 三个列表内操作共用的失败提示：后端文案优先，取不到再退回通用句子 */
  function toastError(fallbackKey: string) {
    return (e: unknown) => toast(apiErrorMessage(e, t(fallbackKey)))
  }

  function handleDelete(r: Rule) {
    if (!window.confirm(t('settings.rules.deleteConfirm', { name: r.name }))) return
    deleteRule.mutate(r.id, { onError: toastError('settings.rules.deleteFailed') })
  }

  /** 启用开关：复用 PUT /rules/:id，其余字段原样回传 */
  function handleToggleEnabled(r: Rule) {
    updateRule.mutate(
      {
        id: r.id,
        input: {
          name: r.name,
          enabled: !r.enabled,
          account_id: r.account_id,
          match: r.match,
          conditions: r.conditions,
          actions: r.actions,
          stop_processing: r.stop_processing,
        },
      },
      { onError: toastError('settings.rules.toggleFailed') },
    )
  }

  /** 上下箭头：把整张 id 列表按新顺序重发，后端据此重写 priority */
  function handleMove(index: number, delta: number) {
    const ids = rules.map((r) => r.id)
    const next = moveArrayItem(ids, index, index + delta)
    // 越界时 moveArrayItem 原样返回，别为此发一次无意义的请求
    if (next.every((id, i) => id === ids[i])) return
    reorder.mutate(next, { onError: toastError('settings.rules.reorderFailed') })
  }

  /** 规则作用范围的一行说明：账户 + 匹配模式 + 条件/动作条数 */
  function scopeText(r: Rule): string {
    const acc = r.account_id === 0
      ? t('settings.rules.allAccounts')
      : (accounts.find((a) => a.id === r.account_id)?.name ?? t('settings.rules.unknownAccount'))
    const match = r.match === 'all' ? t('settings.rules.matchAll') : t('settings.rules.matchAny')
    return `${acc} · ${match} · ${t('settings.rules.countSummary', {
      conditions: r.conditions.length,
      actions: r.actions.length,
    })}`
  }

  function conditionText(c: RuleCondition): string {
    const field = t(`settings.rules.field.${c.field}`)
    if (c.field === 'has_attachment') {
      return `${field} = ${c.value === 'false' ? t('settings.rules.boolNo') : t('settings.rules.boolYes')}`
    }
    return `${field} ${t(`settings.rules.op.${c.op}`)} ${c.value}`
  }

  function actionText(a: RuleAction): string {
    const label = t(`settings.rules.action.${a.type}`)
    return a.type === 'move' && a.value ? `${label} → ${a.value}` : label
  }

  /**
   * 执行日志里的动作串 → 可读文案。
   * 后端给的是枚举拼接（`mark_read,move:归档`、黑名单则是 `block:spam.io`），
   * 认不出来的取值原样显示，免得后端将来加了新动作时这里变成空白。
   */
  function runActionText(action: string): string {
    const tokens = parseRunAction(action)
    if (tokens.length === 0) return action
    return tokens
      .map((tk) => {
        if (tk.type === 'block' && tk.value) return t('settings.rules.runAction.block', { value: tk.value })
        if (tk.type === 'move' && tk.value) {
          // 移动没真的发生时后端在目标名后加 (skipped)/(missing)/(same)，拆出来单独翻译
          const { target, note } = splitMoveNote(tk.value)
          const text = t('settings.rules.runAction.move', { value: target })
          return note ? `${text}（${t(`settings.rules.runNote.${note}`)}）` : text
        }
        if (isKnownActionType(tk.type)) return t(`settings.rules.action.${tk.type}`)
        return tk.raw
      })
      .join(' · ')
  }

  return (
    <div className="settings-block">
      <h3>{t('settings.rules.title')}</h3>
      <p className="help">{t('settings.rules.help')}</p>

      {rules.length === 0 && <div className="settings-empty">{t('settings.rules.none')}</div>}

      {rules.map((r, i) => (
        <div key={r.id} className="account-card">
          <div
            className="ac-avatar"
            style={{ background: r.enabled ? 'var(--accent)' : 'var(--ink-3)' }}
            aria-hidden="true"
          >
            {i + 1}
          </div>
          <div style={{ minWidth: 0 }}>
            <div className="ac-name">
              {r.name}
              {r.stop_processing && (
                <span className="chip" style={{ fontSize: 11, padding: '1px 7px', marginLeft: 8 }}>
                  {t('settings.rules.stopBadge')}
                </span>
              )}
            </div>
            <div className="ac-mail" style={{ fontFamily: 'inherit' }}>{scopeText(r)}</div>
            <div style={{ display: 'flex', gap: 4, marginTop: 4, flexWrap: 'wrap' }}>
              {r.conditions.map((c, ci) => (
                <span key={`c${ci}`} className="chip" style={{ fontSize: 11, padding: '1px 7px' }}>
                  {conditionText(c)}
                </span>
              ))}
              {r.actions.map((a, ai) => (
                <span
                  key={`a${ai}`}
                  className="chip"
                  style={{ fontSize: 11, padding: '1px 7px', color: 'var(--accent)' }}
                >
                  {actionText(a)}
                </span>
              ))}
            </div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexShrink: 0 }}>
            <button
              type="button"
              role="switch"
              aria-checked={r.enabled}
              className={'toggle' + (r.enabled ? ' on' : '')}
              onClick={() => handleToggleEnabled(r)}
              aria-label={r.enabled ? t('settings.account.disable') : t('settings.account.enable')}
            />
            <button
              type="button"
              className="icon-btn"
              title={t('settings.rules.moveUp')}
              aria-label={t('settings.rules.moveUp')}
              onClick={() => handleMove(i, -1)}
              disabled={i === 0 || reorder.isPending}
            >
              <Icon name="chevron-up" size={13} />
            </button>
            <button
              type="button"
              className="icon-btn"
              title={t('settings.rules.moveDown')}
              aria-label={t('settings.rules.moveDown')}
              onClick={() => handleMove(i, 1)}
              disabled={i === rules.length - 1 || reorder.isPending}
            >
              <Icon name="chevron-down" size={13} />
            </button>
            <button type="button" className="icon-btn" title={t('settings.account.edit')} onClick={() => openEdit(r)}>
              <Icon name="compose" size={13} />
            </button>
            <button
              type="button"
              className="icon-btn"
              title={t('settings.account.delete')}
              onClick={() => handleDelete(r)}
              style={{ color: 'var(--destructive)' }}
            >
              <Icon name="trash" size={13} />
            </button>
          </div>
        </div>
      ))}

      <button type="button" className="pill-btn" style={{ marginTop: 14 }} onClick={openAdd}>
        <Icon name="plus" size={12} /> {t('settings.rules.addRule')}
      </button>

      <RuleDialog open={dialogOpen} rule={editing} onOpenChange={setDialogOpen} />

      {/* 执行日志（可折叠） */}
      <div style={{ marginTop: 20 }}>
        <button type="button" className="pill-btn" onClick={() => setShowRuns((s) => !s)}>
          {t('settings.rules.runsTitle')}
        </button>
        {showRuns && (
          <div style={{ marginTop: 10 }}>
            {runs.length === 0 ? (
              <div className="settings-empty">{t('settings.rules.noRuns')}</div>
            ) : (
              runs.map((run) => (
                <div key={run.id} className="settings-list-row">
                  <span className="slr-fixed slr-mono slr-dim">{new Date(run.created_at).toLocaleString()}</span>
                  <span className="slr-fixed" style={{ color: 'var(--ink-2)', fontWeight: 500 }}>
                    {/* rule_id = 0 是黑名单命中，后端不给规则名 */}
                    {run.rule_id === 0 ? t('settings.rules.byBlocklist') : run.rule_name}
                  </span>
                  <span className="slr-fixed" style={{ color: 'var(--accent)' }} title={run.action}>
                    {runActionText(run.action)}
                  </span>
                  <span className="slr-grow slr-mono slr-dim" title={run.message_key}>
                    {run.message_key}
                  </span>
                </div>
              ))
            )}
          </div>
        )}
      </div>
    </div>
  )
}

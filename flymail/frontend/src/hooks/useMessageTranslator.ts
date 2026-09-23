// 一封邮件的翻译状态机：查缓存、按需翻译、在原文与译文之间切换。
//
// ── 为什么抽成 hook ────────────────────────────────────────────────────────
//
// 单封视图（Reader）与会话视图（ThreadReader）各有一份工具栏，两边要的行为
// 完全一样：按钮是个开关、没翻过才花钱、失败了能重试、换邮件回到原文。
// 抄两份的下场是其中一份迟早漏掉某个重置——而漏掉重置的表现是
// "上一封的译文出现在这一封上"。
//
// ── 花钱的时机 ─────────────────────────────────────────────────────────────
//
// 只有 toggle（首次打开且没有缓存）和 redo（用户明确要求重译）会真的调用 AI。
// 打开邮件时的那次查询走的是只读接口，命中不到就是 204，不产生任何用量。

import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { apiErrorMessage } from '@/lib/api'
import { useMessageTranslation, useTranslateLanguages, useTranslateMessage } from '@/lib/queries'
import type { Translation } from '@/lib/types'

export interface MessageTranslator {
  /** AI 接口已配置到可用程度；为假时按钮置灰（不是隐藏，见 hint） */
  available: boolean
  /** 当前正在看译文 */
  showing: boolean
  /** 正在翻译这一封 */
  busy: boolean
  /** 翻译失败的原因，成功或未开始时为 null */
  error: string | null
  /** 要显示的译文；没在看译文时为 null */
  translation: Translation | null
  /** 译文 ↔ 原文 */
  toggle: () => void
  /** 切回原文 */
  hide: () => void
  /** 无视缓存重新翻译（会真的再花一次钱） */
  redo: () => void
  /**
   * 按钮的悬浮说明。传入这封信识别出的语言，用来提示"已经是目标语言了"。
   *
   * ⚠ 只提示、不禁用：识别有可能出错，而识别错的代价是"用户想翻译却翻不了"，
   * 比多点一次按钮严重得多。
   */
  hint: (detectLang?: string) => string | undefined
}

export function useMessageTranslator(messageId: number | null): MessageTranslator {
  const { t } = useTranslation()
  const { data: langs } = useTranslateLanguages()
  const target = langs?.default_target ?? ''
  const available = (langs?.enabled ?? false) && target !== ''

  const query = useMessageTranslation(messageId, target, { enabled: available })
  const mutation = useTranslateMessage()

  const [showing, setShowing] = React.useState(false)
  const [error, setError] = React.useState<string | null>(null)

  // 换邮件回到原文。不重置的话，下一封会带着上一封留下的"正在看译文"状态打开，
  // 而那一封多半还没有译文——用户看到的是原文，按钮却写着「显示原文」。
  //
  // 写在渲染期间而不是 useEffect 里，是 React 官方给"props 变了就重置 state"
  // 这一类需求的正式写法：effect 要等这一帧提交完才跑，中间那一帧用户会看到
  // 上一封的翻译状态套在新邮件上闪一下。同组件内的 setState 在渲染期间调用
  // 不会级联——React 会直接丢掉这次渲染的结果重来。
  const [prevMessageId, setPrevMessageId] = React.useState(messageId)
  if (prevMessageId !== messageId) {
    setPrevMessageId(messageId)
    setShowing(false)
    setError(null)
  }

  const run = React.useCallback(
    (force: boolean) => {
      if (messageId == null || target === '') return
      setError(null)
      mutation.mutate(
        { id: messageId, lang: target, force },
        { onError: (e) => setError(apiErrorMessage(e, t('reader.translateFailed'))) },
      )
    },
    [messageId, target, mutation, t],
  )

  const toggle = React.useCallback(() => {
    if (showing) {
      setShowing(false)
      return
    }
    setShowing(true)
    setError(null)
    // 已经有译文就只是切个视图——那是本地动作，不该再走一次网络，
    // 更不该让用户看着转圈等一份手里已经有的东西。
    if (query.data) return
    run(false)
  }, [showing, query.data, run])

  const hint = React.useCallback(
    (detectLang?: string) => {
      if (!available) return t('reader.translateNotConfigured')
      if (detectLang && detectLang === target) {
        const native = (langs?.languages ?? []).find((l) => l.code === target)?.native ?? target
        return t('reader.translateSameLang', { lang: native })
      }
      return undefined
    },
    [available, target, langs, t],
  )

  return {
    available,
    showing,
    // ⚠ 比一次 id：mutation 实例跨邮件复用，翻译途中切走再切回来的话，
    // 不比 id 就会把上一封的进行中状态显示在这一封的按钮上。
    busy: mutation.isPending && mutation.variables?.id === messageId,
    error,
    translation: showing ? (query.data ?? null) : null,
    toggle,
    hide: React.useCallback(() => setShowing(false), []),
    redo: React.useCallback(() => {
      setShowing(true)
      run(true)
    }, [run]),
    hint,
  }
}

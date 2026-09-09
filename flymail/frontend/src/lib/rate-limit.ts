// 登录限流（HTTP 429）的解析与文案换算。
//
// 抽成纯函数的理由：这段逻辑的坑全在边界上——后端给的是秒，用户要看的是分钟；
// 秒数可能来自响应体、也可能只有 Retry-After 头；还可能是个负数或者天文数字。
// 在组件里写就只能靠手动点 11 次登录来验证，抽出来可以直接用测试固定住。

import axios from 'axios'

/** 允许的最长等待：后端窗口是 15 分钟，再长的值一律当作异常数据截断 */
export const MAX_RETRY_AFTER_SEC = 3600

/**
 * 从 429 响应里取出还要等多少秒。
 *
 * 两个来源按优先级取：响应体的 `retry_after`（后端明确给的秒数），
 * 其次是标准的 `Retry-After` 响应头。都取不到时返回 0——
 * 调用方据此退回通用文案，而不是编一个假的倒计时。
 *
 * ⚠ Retry-After 头也允许是 HTTP-date 格式，这里只认秒数：后端（internal/auth）
 * 发的是秒，认日期格式属于给不存在的情况写代码。
 */
export function parseRetryAfter(err: unknown): number {
  if (!axios.isAxiosError(err)) return 0
  const resp = err.response
  if (!resp) return 0

  const body = resp.data as { retry_after?: unknown } | undefined
  const fromBody = typeof body?.retry_after === 'number' ? body.retry_after : NaN
  if (Number.isFinite(fromBody) && fromBody > 0) return clampSeconds(fromBody)

  const header = resp.headers?.['retry-after']
  const fromHeader = typeof header === 'string' || typeof header === 'number' ? Number(header) : NaN
  if (Number.isFinite(fromHeader) && fromHeader > 0) return clampSeconds(fromHeader)

  return 0
}

function clampSeconds(sec: number): number {
  return Math.min(Math.ceil(sec), MAX_RETRY_AFTER_SEC)
}

/** 倒计时文案的两种口径：不到一分钟按秒说，超过就按分钟说 */
export interface RetryAfterText {
  unit: 'minutes' | 'seconds'
  value: number
}

/**
 * 把剩余秒数换算成给用户看的单位。
 *
 * 一律向上取整：「请 1 分钟后再试」在还剩 61 秒时说成 1 分钟，用户按提示回来仍然被拒，
 * 那比多等一会儿更让人恼火。0 及负数归到 1 秒，界面上不会出现「请 0 秒后再试」。
 */
export function retryAfterText(seconds: number): RetryAfterText {
  const s = Number.isFinite(seconds) ? Math.ceil(seconds) : 0
  if (s >= 60) return { unit: 'minutes', value: Math.ceil(s / 60) }
  return { unit: 'seconds', value: Math.max(1, s) }
}

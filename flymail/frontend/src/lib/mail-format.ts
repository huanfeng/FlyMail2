// 阅读区共用的小工具：日期/地址/首字母格式化与一个延迟置位的开关。
//
// 放在 lib 而不是 Reader.tsx：单封 Reader 与会话手风琴都要用，
// 而从一个导出组件的文件里再导出普通函数会破坏 Fast Refresh。

import { useEffect, useState } from 'react'
import type { Address } from '@/lib/types'

/** 格式化日期字符串 */
export function formatDate(dateStr: string): string {
  try {
    return new Date(dateStr).toLocaleString()
  } catch {
    return dateStr
  }
}

/**
 * 把 Address 数组渲染为 "name <email>" 逗号连接字符串。
 *
 * 命中当前账户自己的地址时显示为「我」：收信人里躺着一串自己的邮箱地址，
 * 对正在读这封信的人是零信息量，还把真正需要看的其他收件人挤出可视区。
 * 比较忽略大小写与首尾空白——邮箱本地部分理论上大小写敏感，
 * 但没有服务商真的这么用，按大小写敏感比反而会漏判。
 */
export function formatAddresses(addrs: Address[], selfAddr: string, meLabel: string): string {
  const self = selfAddr.trim().toLowerCase()
  return addrs
    .map((a) => {
      if (self && a.email.trim().toLowerCase() === self) return meLabel
      return a.name ? `${a.name} <${a.email}>` : a.email
    })
    .join(', ')
}

/** 取发件人首字母（用于方形头像） */
export function senderInitial(name: string, addr: string): string {
  const s = (name || addr || '?').trim()
  return s.slice(0, 1).toUpperCase()
}

/**
 * flag 连续为 true 超过 delay 毫秒后才返回 true。
 *
 * 用于抑制瞬时 loading 造成的闪烁：本地接口通常几毫秒返回，
 * 立刻挂骨架屏只会让人看到一两帧的跳变，比不显示更糟。
 */
export function useDelayedFlag(flag: boolean, delay: number): boolean {
  const [on, setOn] = useState(false)
  useEffect(() => {
    if (!flag) {
      setOn(false)
      return
    }
    const timer = setTimeout(() => setOn(true), delay)
    return () => clearTimeout(timer)
  }, [flag, delay])
  return on
}

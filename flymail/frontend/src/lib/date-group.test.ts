import { describe, it, expect } from 'vitest'
import { groupByDate } from '@/lib/date-group'
import type { DateGroup } from '@/lib/date-group'

// date-group.ts 单元测试
// 所有断言使用传入固定 now，禁止依赖真实当前时间

// 固定基准时间：2024年6月5日（周三）12:00 本地时间
const NOW = new Date(2024, 5, 5, 12, 0, 0) // 月份从 0 开始，5=6月

// 辅助：构造简单条目
function item(date: string) {
  return { date }
}

function getDate(t: { date: string }) {
  return t.date
}

describe('groupByDate()', () => {
  it('空数组返回空分组', () => {
    const result = groupByDate([], getDate, NOW)
    expect(result).toHaveLength(0)
  })

  it('今天的邮件归入"今天"', () => {
    const items = [item('2024-06-05T08:00:00')]
    const result = groupByDate(items, getDate, NOW)
    expect(result).toHaveLength(1)
    expect(result[0].label).toBe('今天')
    expect(result[0].items).toHaveLength(1)
  })

  it('昨天的邮件归入"昨天"', () => {
    const items = [item('2024-06-04T23:59:00')]
    const result = groupByDate(items, getDate, NOW)
    expect(result).toHaveLength(1)
    expect(result[0].label).toBe('昨天')
  })

  it('本周内（非今昨）归入"本周"', () => {
    // NOW 是周三(6/5)，周一是 6/3
    const items = [item('2024-06-03T10:00:00')]
    const result = groupByDate(items, getDate, NOW)
    expect(result).toHaveLength(1)
    expect(result[0].label).toBe('本周')
  })

  it('本月内（非本周）归入"本月"', () => {
    // 6/1 是本月但不在本周(周一6/3之前)
    const items = [item('2024-06-01T10:00:00')]
    const result = groupByDate(items, getDate, NOW)
    expect(result).toHaveLength(1)
    expect(result[0].label).toBe('本月')
  })

  it('更早的邮件按"YYYY年M月"分组', () => {
    const items = [item('2024-05-15T10:00:00')]
    const result = groupByDate(items, getDate, NOW)
    expect(result).toHaveLength(1)
    expect(result[0].label).toBe('2024年5月')
  })

  it('不同月份的更早邮件各自一个分组', () => {
    const items = [
      item('2024-04-10T10:00:00'),
      item('2024-03-20T10:00:00'),
    ]
    const result = groupByDate(items, getDate, NOW)
    expect(result).toHaveLength(2)
    expect(result[0].label).toBe('2024年4月')
    expect(result[1].label).toBe('2024年3月')
  })

  it('跨分组顺序：今天 → 昨天 → 本月 → 更早', () => {
    const items = [
      item('2024-06-05T09:00:00'), // 今天
      item('2024-06-04T10:00:00'), // 昨天
      item('2024-06-01T10:00:00'), // 本月（非本周）
      item('2024-05-01T10:00:00'), // 更早
    ]
    const result = groupByDate(items, getDate, NOW)
    const labels = result.map((g: DateGroup<{ date: string }>) => g.label)
    expect(labels).toEqual(['今天', '昨天', '本月', '2024年5月'])
  })

  it('多条今天邮件合并进同一分组', () => {
    const items = [
      item('2024-06-05T08:00:00'),
      item('2024-06-05T10:00:00'),
      item('2024-06-05T11:30:00'),
    ]
    const result = groupByDate(items, getDate, NOW)
    expect(result).toHaveLength(1)
    expect(result[0].label).toBe('今天')
    expect(result[0].items).toHaveLength(3)
  })

  it('非法日期字符串归入独立的"更早"分组', () => {
    const items = [item('not-a-date')]
    const result = groupByDate(items, getDate, NOW)
    expect(result).toHaveLength(1)
    expect(result[0].label).toBe('更早')
    expect(result[0].items).toHaveLength(1)
  })

  it('非法日期与有效更早日期分开归组', () => {
    const items = [
      item('2024-05-10T10:00:00'), // 2024年5月
      item('bad-date'),             // 更早
    ]
    const result = groupByDate(items, getDate, NOW)
    const labels = result.map((g: DateGroup<{ date: string }>) => g.label)
    // 两个分组都应存在
    expect(labels).toContain('2024年5月')
    expect(labels).toContain('更早')
  })

  it('NOW 为周一时，周一本身归入今天，上周日归入更早历史月', () => {
    // 2024-06-03 周一
    const monday = new Date(2024, 5, 3, 12, 0, 0)
    const items = [
      item('2024-06-03T08:00:00'), // 今天（周一）
      item('2024-06-02T10:00:00'), // 昨天（周日）
    ]
    const result = groupByDate(items, getDate, monday)
    const labels = result.map((g: DateGroup<{ date: string }>) => g.label)
    expect(labels[0]).toBe('今天')
    expect(labels[1]).toBe('昨天')
  })
})

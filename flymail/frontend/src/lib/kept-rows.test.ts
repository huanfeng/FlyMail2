import { describe, it, expect } from 'vitest'
import { emptyKept, nextKept, withKept, type KeptRows } from '@/lib/kept-rows'

interface Row { id: string; date: string; seen: boolean }
const key = (r: Row) => r.id
// 与后端一致的两级排序键：date DESC，然后 id DESC
const cmp = (a: Row, b: Row) => b.date.localeCompare(a.date) || b.id.localeCompare(a.id)
const look = { isRead: (r: Row) => r.seen, asRead: (r: Row) => ({ ...r, seen: true }) }

const row = (id: string, d: string, seen = false): Row => ({ id, date: `2026-09-${d}T10:00:00Z`, seen })
const list = [row('c', '17'), row('b', '16'), row('a', '15')]

/**
 * 开着「未读」筛选读邮件时，每读一封它就被标已读、不再满足筛选，下次刷新就被删掉。
 *
 * 第一版只钉住**当前那一封**，解决不了真正烦人的那个问题：切到下一封时上一封
 * 立刻消失，后面的行整体上移、下标全变，「下一封」跳到的不是眼睛看到的下一行。
 * 主流客户端的做法是读过的都留在原地显示成已读，直到换筛选/换文件夹才清掉。
 */
describe('nextKept', () => {
  it('记录当前打开的那一行', () => {
    const got = nextKept(emptyKept<Row>('v1'), 'v1', list, 'b', key)
    expect([...got.rows.keys()]).toEqual(['b'])
  })

  /** 读过好几封时全都要留着——这正是列表不再跳动的原因。 */
  it('读过的逐个累积，不是只留最后一封', () => {
    let kept = emptyKept<Row>('v1')
    kept = nextKept(kept, 'v1', list, 'c', key)
    kept = nextKept(kept, 'v1', list, 'b', key)
    kept = nextKept(kept, 'v1', list, 'a', key)
    expect([...kept.rows.keys()].sort()).toEqual(['a', 'b', 'c'])
  })

  /** ⚠ 只记录打开过的行。把翻页划过的全留下，筛选就形同虚设了。 */
  it('没打开过的行不会被留下', () => {
    const kept = nextKept(emptyKept<Row>('v1'), 'v1', list, null, key)
    expect(kept.rows.size).toBe(0)
  })

  it('换筛选/换文件夹时整批清掉', () => {
    let kept = nextKept(emptyKept<Row>('v1'), 'v1', list, 'b', key)
    kept = nextKept(kept, 'v1', list, 'c', key)
    expect(kept.rows.size).toBe(2)

    const after = nextKept(kept, 'v2', list, null, key)
    expect(after.viewKey).toBe('v2')
    expect(after.rows.size).toBe(0)
  })

  /**
   * ⚠ 无变化时必须返回同一引用。
   *
   * 调用方在**渲染期**拿返回值和旧值比引用来决定要不要 setState，
   * 每次都产出新对象就是无限重渲染。
   */
  it('无变化时返回同一引用', () => {
    const once = nextKept(emptyKept<Row>('v1'), 'v1', list, 'b', key)
    expect(nextKept(once, 'v1', list, 'b', key)).toBe(once)
    // 行已被筛掉（不在 list 里）时也不该反复产出新对象
    expect(nextKept(once, 'v1', [list[0]], 'b', key)).toBe(once)
  })
})

describe('withKept', () => {
  it('没留下任何行时原样返回', () => {
    expect(withKept(list, emptyKept<Row>('v1'), key, cmp, look)).toBe(list)
  })

  it('还在列表里的行不重复插入', () => {
    const kept = nextKept(emptyKept<Row>('v1'), 'v1', list, 'b', key)
    expect(withKept(list, kept, key, cmp, look).map(key)).toEqual(['c', 'b', 'a'])
  })

  /** 被筛掉的行要插回**原来的时间位置**，不是追加到末尾。 */
  it('被筛掉的行按日期归位', () => {
    const kept = nextKept(emptyKept<Row>('v1'), 'v1', list, 'b', key)
    const filtered = [list[0], list[2]] // b 被筛掉了
    expect(withKept(filtered, kept, key, cmp, look).map(key)).toEqual(['c', 'b', 'a'])
  })

  it('插回的行显示成已读', () => {
    const kept = nextKept(emptyKept<Row>('v1'), 'v1', list, 'b', key)
    const got = withKept([list[0], list[2]], kept, key, cmp, look)
    expect(got.find((r) => r.id === 'b')?.seen).toBe(true)
  })

  /**
   * ⚠ 不是未读筛选时不改状态。
   *
   * 星标/附件筛选下行消失的原因不是「被读了」，跟着改 seen 就是瞎猜，
   * 会把一封真正未读的邮件显示成已读。
   */
  it('没给 ReadLook 时保持原状态', () => {
    const kept = nextKept(emptyKept<Row>('v1'), 'v1', list, 'b', key)
    const got = withKept([list[0], list[2]], kept, key, cmp)
    expect(got.find((r) => r.id === 'b')?.seen).toBe(false)
  })

  /**
   * 读了好几封之后，列表的顺序和长度都要保持稳定——这是这套机制的全部目的：
   * 下标不动，「下一封」才会落在眼睛看到的下一行上。
   */
  it('连读多封后列表长度与顺序不变', () => {
    let kept: KeptRows<Row> = emptyKept('v1')
    kept = nextKept(kept, 'v1', list, 'c', key)
    kept = nextKept(kept, 'v1', list, 'b', key)
    // 两封都被标已读筛掉了，只剩 a
    const got = withKept([list[2]], kept, key, cmp, look)
    expect(got.map(key)).toEqual(['c', 'b', 'a'])
    expect(got.filter((r) => r.seen).map(key)).toEqual(['c', 'b'])
  })
})


/**
 * ⚠ 同一秒到达的几封邮件，插回后次序必须和后端一致。
 *
 * 只按 date 比较时它们的相对次序是未定义的——实测四封同秒邮件被排成 4、1、2、3，
 * 而「下一封」按数组下标走，于是跳到的不是眼睛看到的下一行。
 * 批量投递、自动通知都会产生同秒邮件，这不是边角情况。
 */
describe('同秒邮件的次序', () => {
  const sameSecond = [
    { id: 'd', date: '2026-09-17T10:00:00Z', seen: false },
    { id: 'c', date: '2026-09-17T10:00:00Z', seen: false },
    { id: 'b', date: '2026-09-17T10:00:00Z', seen: false },
    { id: 'a', date: '2026-09-17T10:00:00Z', seen: false },
  ]

  it('插回后仍按次级键排列', () => {
    let kept = emptyKept<Row>('v1')
    kept = nextKept(kept, 'v1', sameSecond, 'c', key)
    kept = nextKept(kept, 'v1', sameSecond, 'b', key)
    // c、b 被读掉筛走，只剩 d、a
    const got = withKept([sameSecond[0], sameSecond[3]], kept, key, cmp, look)
    expect(got.map(key)).toEqual(['d', 'c', 'b', 'a'])
  })
})

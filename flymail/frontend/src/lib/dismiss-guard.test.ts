import { describe, it, expect } from 'vitest'
import { isDirty } from '@/lib/dismiss-guard'

/**
 * 「改过没有」的判定。
 *
 * 这个函数决定对话框拦不拦「点框外关闭」，所以它的**两个方向都会出事**：
 *
 *   恒判 dirty  → 对话框永远点不掉外面。防误触变成关不掉的框，
 *                 比它要解决的问题更糟。
 *   恒判干净    → 守卫形同虚设，用户照样丢数据。
 *
 * 最容易写出恒真的两种实现都在下面钉住了：按引用比对象数组、JSON 序列化。
 */
describe('isDirty', () => {
  it('完全相同的表单不算改过', () => {
    const a = { name: '', email: '', port: 993 }
    expect(isDirty({ ...a }, { ...a })).toBe(false)
  })

  it('改了任意一个字段就算改过', () => {
    const base = { name: '', email: '', port: 993 }
    expect(isDirty({ ...base, name: 'x' }, base)).toBe(true)
    expect(isDirty({ ...base, port: 143 }, base)).toBe(true)
  })

  it('内容相同的**对象数组**不算改过——这是「关不掉的框」的根因', () => {
    // 规则表单里是 conditions: ConditionRow[] / actions: ActionRow[]。
    // 按引用比的话，每次渲染新建的行对象永远不等于基线里的，
    // 于是规则对话框恒判 dirty、永远点不掉外面。
    const mk = () => ({
      name: 'r',
      conditions: [
        { id: 'c1', field: 'from', op: 'contains', value: 'a@b.c' },
        { id: 'c2', field: 'subject', op: 'is', value: '账单' },
      ],
      actions: [{ id: 'a1', kind: 'move', target: 'INBOX' }],
    })
    expect(isDirty(mk(), mk()), '内容一样却被判成改过').toBe(false)
  })

  it('数组里任意一项变了就算改过', () => {
    const base = { rows: [{ id: '1', v: 'a' }, { id: '2', v: 'b' }] }
    const changed = { rows: [{ id: '1', v: 'a' }, { id: '2', v: 'B' }] }
    expect(isDirty(changed, base)).toBe(true)
  })

  it('数组增删算改过', () => {
    const base = { rows: [{ id: '1' }] }
    expect(isDirty({ rows: [{ id: '1' }, { id: '2' }] }, base)).toBe(true)
    expect(isDirty({ rows: [] }, base)).toBe(true)
  })

  it('键顺序不同不算改过——这是 JSON.stringify 那种写法的坑', () => {
    // JSON.stringify 按插入顺序输出，两个内容相同但键顺序不同的对象
    // 字符串不等，同样会得到恒真的 dirty。
    const a = { name: 'x', email: 'y' }
    const b = { email: 'y', name: 'x' }
    expect(isDirty(a, b)).toBe(false)
  })

  it('嵌套对象按结构比', () => {
    const mk = () => ({ proxy: { host: 'h', port: 1080, auth: { user: 'u' } } })
    expect(isDirty(mk(), mk())).toBe(false)
    const changed = mk()
    changed.proxy.auth.user = 'v'
    expect(isDirty(changed, mk())).toBe(true)
  })

  it('null / undefined 与空串是不同的值', () => {
    // 「没填过」和「填了又删空」在表单语义上确实不同，不该被抹平
    expect(isDirty({ v: null }, { v: undefined })).toBe(true)
    expect(isDirty({ v: '' }, { v: null })).toBe(true)
  })

  it('布尔与数字字段', () => {
    expect(isDirty({ on: true }, { on: false })).toBe(true)
    expect(isDirty({ n: 0 }, { n: 0 })).toBe(false)
  })
})

/**
 * 抖动动画必须保留居中位移。
 *
 * 对话框靠 `translate(-50%,-50%)` 居中，而 `transform` 是**整条**被关键帧
 * 替换的、不是各分量分别插值。关键帧里漏掉那个位移，对话框会在动画期间
 * 被甩到屏幕右下角再弹回来。
 * （同一个坑此前在 @keyframes popCentered 上踩过一次。）
 */
describe('@keyframes dialogNudge', () => {
  it('每一帧都带着居中位移', async () => {
    const fs = await import('node:fs')
    const path = await import('node:path')
    const css = fs.readFileSync(path.resolve(process.cwd(), 'src/index.css'), 'utf-8')
    const m = /@keyframes dialogNudge\s*\{([\s\S]*?)\n\}/.exec(css)
    expect(m, '找不到 dialogNudge 关键帧').not.toBeNull()

    const frames = [...m![1].matchAll(/transform:\s*([^;]+);/g)].map((x) => x[1])
    expect(frames.length, '关键帧里一条 transform 都没有').toBeGreaterThan(1)
    for (const f of frames) {
      expect(f, `这一帧丢了居中位移，对话框会被甩出屏幕：${f}`).toMatch(/-50%/)
    }
  })
})

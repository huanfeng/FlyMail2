import { describe, it, expect } from 'vitest'
import { getShortcutGroups, KEY, withShortcut, keyLabel } from '@/lib/shortcuts'
import zh from '@/locales/zh.json'
import en from '@/locales/en.json'

// shortcuts.ts 单元测试
// 关注点：① 目录结构稳定 ② 所有 i18n 键在 zh/en 均已补全（单一真相源的描述不漂移）

/** 按 'a.b.c' 路径取嵌套对象值；缺失返回 undefined。 */
function getByPath(obj: unknown, path: string): unknown {
  return path.split('.').reduce<unknown>((acc, key) => {
    if (acc && typeof acc === 'object') return (acc as Record<string, unknown>)[key]
    return undefined
  }, obj)
}

describe('KEY 常量', () => {
  it('包含全部单键 + Esc', () => {
    expect(KEY).toMatchObject({
      composeC: 'c',
      composeN: 'n',
      reply: 'r',
      focusSearch: '/',
      next: 'j',
      prev: 'k',
      help: '?',
      escape: 'Escape',
    })
  })
})

describe('getShortcutGroups()', () => {
  const groups = getShortcutGroups()

  it('返回非空分组，每组至少一条', () => {
    expect(groups.length).toBeGreaterThan(0)
    for (const g of groups) {
      expect(g.items.length).toBeGreaterThan(0)
    }
  })

  it('每条目都有非空 keys 与 descKey', () => {
    for (const g of groups) {
      for (const it of g.items) {
        expect(it.keys.length).toBeGreaterThan(0)
        expect(it.keys.every((k) => k.length > 0)).toBe(true)
        expect(it.descKey).toMatch(/^shortcuts\./)
      }
    }
  })

  it('item id 全局唯一（React key / 断言安全）', () => {
    const ids = groups.flatMap((g) => g.items.map((it) => it.id))
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('所有 titleKey / descKey 在 zh 与 en 中都已定义', () => {
    const keys = groups.flatMap((g) => [g.titleKey, ...g.items.map((it) => it.descKey)])
    for (const k of keys) {
      expect(getByPath(zh, k), `zh 缺少 ${k}`).toBeTypeOf('string')
      expect(getByPath(en, k), `en 缺少 ${k}`).toBeTypeOf('string')
    }
  })
})

describe('按钮上的快捷键提示', () => {
  /**
   * 快捷键目录只出现在 `?` 速查浮层和设置的键位表里——两处都得用户先知道
   * 「有快捷键这回事」才会去看。真正的学习时机是他正拿鼠标点那个按钮的时候。
   */
  it('把键位拼在标签后面', () => {
    expect(withShortcut('下一封', KEY.next)).toBe('下一封 (J)')
    expect(withShortcut('回复', KEY.reply)).toBe('回复 (R)')
  })

  /**
   * ⚠ 单字母必须大写。
   *
   * KEY 里存的是小写（匹配时统一小写比较），但键盘上印的是大写。
   * 直接显示 'j' 会让人以为要配合什么修饰键。
   */
  it('单字母显示成大写', () => {
    expect(keyLabel('j')).toBe('J')
    expect(keyLabel('k')).toBe('K')
  })

  it('特殊键用惯用缩写，不是原始 event.key', () => {
    expect(keyLabel('Delete')).toBe('Del')
    expect(keyLabel('Escape')).toBe('Esc')
  })

  it('符号键原样显示', () => {
    expect(keyLabel('#')).toBe('#')
    expect(keyLabel('/')).toBe('/')
    expect(keyLabel('?')).toBe('?')
  })
})

import { describe, it, expect } from 'vitest'
import zh from '@/locales/zh.json'
import en from '@/locales/en.json'

// zh / en 键集对齐校验。
// 缺键在运行时不报错，只是把原始 key（"list.syntax.from"）直接渲染到界面上——
// 这种 bug 只有切到那门语言、点开那个浮层才看得见，必须靠测试兜住。

/** 展开成 'a.b.c' 形式的叶子键路径 */
function leafKeys(obj: unknown, prefix = ''): string[] {
  if (obj === null || typeof obj !== 'object') return [prefix]
  return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) =>
    leafKeys(v, prefix ? `${prefix}.${k}` : k),
  )
}

describe('locales', () => {
  const zhKeys = leafKeys(zh)
  const enKeys = leafKeys(en)

  it('zh 与 en 的键集完全一致', () => {
    const zhSet = new Set(zhKeys)
    const enSet = new Set(enKeys)
    expect(zhKeys.filter((k) => !enSet.has(k)), 'en 缺少的键').toEqual([])
    expect(enKeys.filter((k) => !zhSet.has(k)), 'zh 缺少的键').toEqual([])
  })

  it('所有文案都是非空字符串', () => {
    for (const [lang, keys, src] of [['zh', zhKeys, zh], ['en', enKeys, en]] as const) {
      for (const k of keys) {
        const v = k.split('.').reduce<unknown>(
          (acc, part) => (acc && typeof acc === 'object' ? (acc as Record<string, unknown>)[part] : undefined),
          src,
        )
        expect(typeof v, `${lang}.${k} 应为字符串`).toBe('string')
        expect((v as string).length, `${lang}.${k} 不应为空`).toBeGreaterThan(0)
      }
    }
  })
})

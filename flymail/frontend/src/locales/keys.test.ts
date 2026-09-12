import { describe, it, expect } from 'vitest'
import zh from './zh.json'
import en from './en.json'

/**
 * 语言文件的两条守卫。
 *
 * 第二条是补课：撤销按钮的文案曾经写成 t('common.undo')，而 common 这个
 * 命名空间根本不存在——i18next 取不到键就原样返回键名，于是按钮上显示的是
 * 字面量 "common.undo"。它躲过了类型检查、lint、全部单测和两轮人工审查，
 * 因为 t() 的签名是 (key: string) => string：任何字符串都合法，
 * 取不到键还"成功"返回了一个字符串，失败是静默的。
 *
 * 键对齐（第一条）防的是两份语言文件漂移，引用存在（第二条）防的是引用悬空，
 * 是两件不同的事。只做第一条挡不住上面那个 bug。
 *
 * 源码用 import.meta.glob 读而不是 node:fs：应用的 tsconfig 不带 node 类型，
 * 而 Vite 的这个入口在 vitest 里同样可用。
 */

type Tree = { [k: string]: string | Tree }

/** 把嵌套对象摊平成 'a.b.c' 形式的键集合（只收叶子） */
function leafKeys(obj: Tree, prefix = ''): Set<string> {
  const out = new Set<string>()
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out.add(key)
    else for (const child of leafKeys(v, key)) out.add(child)
  }
  return out
}

/** src 下的全部源码（构建期内联，键是相对本文件的路径） */
const sources = import.meta.glob('../**/*.{ts,tsx}', {
  query: '?raw',
  import: 'default',
  eager: true,
}) as Record<string, string>

describe('语言文件', () => {
  const zhKeys = leafKeys(zh as Tree)
  const enKeys = leafKeys(en as Tree)

  it('zh 与 en 的键完全一致', () => {
    const onlyZh = [...zhKeys].filter((k) => !enKeys.has(k))
    const onlyEn = [...enKeys].filter((k) => !zhKeys.has(k))
    expect({ onlyZh, onlyEn }).toEqual({ onlyZh: [], onlyEn: [] })
  })

  it('代码里引用的每个键都真实存在', () => {
    // t('x.y') 以及键位目录里的 descKey/titleKey/nameKey/labelKey
    const patterns = [
      /\bt\(\s*'([a-zA-Z0-9_.]+)'/g,
      /(?:descKey|titleKey|nameKey|labelKey):\s*'([a-zA-Z0-9_.]+)'/g,
    ]

    const missing: string[] = []
    for (const [path, src] of Object.entries(sources)) {
      if (path.includes('.test.')) continue
      for (const pattern of patterns) {
        pattern.lastIndex = 0
        let m: RegExpExecArray | null
        while ((m = pattern.exec(src)) != null) {
          // 带变量插值的键（模板字符串）匹配不到，这里只查纯字面量
          if (!zhKeys.has(m[1])) missing.push(`${path}: ${m[1]}`)
        }
      }
    }
    expect(missing).toEqual([])
  })
})

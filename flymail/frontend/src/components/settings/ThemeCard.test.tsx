import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { ThemeCard } from '@/components/settings/SettingsDialog'
import { TONES } from '@/lib/theme'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

;(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true

/**
 * 主题预览卡的**组件那一侧**。
 *
 * `theme-tokens.test.ts` 钉的是 index.css：18 组令牌齐全、源码里没有色板副本。
 * 但那些都过了，预览卡照样可能什么都不显示——只要 `data-theme`/`data-mode`
 * 没挂上去或拼错。那时 9 张卡会**全部渲染成当前主题、看起来一模一样**，
 * 550 项测试全绿，只有肉眼能发现。
 *
 * 旧写法（`THEME_PREVIEW[id]` 查不到就 `return null`）的退化形态是卡片消失，
 * 一眼可见；新写法的退化形态是「安静地都对但都一样」，所以这一条必须补。
 */
describe('ThemeCard', () => {
  let container: HTMLDivElement
  let root: Root

  async function render(node: React.ReactNode) {
    await act(async () => root.render(node))
  }

  beforeEach(() => {
    container = document.createElement('div')
    document.body.appendChild(container)
    root = createRoot(container)
  })

  afterEach(async () => {
    await act(async () => root.unmount())
    container.remove()
  })

  it('把被预览的色调与模式挂在预览区上（令牌靠这两个属性在子树内生效）', async () => {
    await render(<ThemeCard id="aqua" label="Aqua" mode="dark" active={false} onClick={() => {}} />)
    const preview = container.querySelector('.tc-preview')
    expect(preview).toBeTruthy()
    expect(preview?.getAttribute('data-theme')).toBe('aqua')
    expect(preview?.getAttribute('data-mode')).toBe('dark')
  })

  it('属性挂在预览区上，而不是整张卡上', async () => {
    // 挂在卡片上会连外框、名称、亮暗标签一起变成被预览的那套主题，
    // 而它们应当跟随**当前**主题——一排卡片的外观会因此各不相同。
    await render(<ThemeCard id="rose" label="Rose" mode="light" active onClick={() => {}} />)
    const card = container.querySelector('.theme-card')
    expect(card?.hasAttribute('data-theme')).toBe(false)
    expect(card?.hasAttribute('data-mode')).toBe(false)
  })

  it('预览区里不写任何颜色（色板只有 index.css 一份）', async () => {
    await render(<ThemeCard id="mint" label="Mint" mode="light" active={false} onClick={() => {}} />)
    // .tc-line 的宽度仍是内联的（那是版式不是颜色），所以只查颜色相关的属性
    for (const el of container.querySelectorAll<HTMLElement>('.tc-preview, .tc-preview *')) {
      expect(el.style.background, `${el.className} 写了内联底色`).toBe('')
      expect(el.style.backgroundColor, `${el.className} 写了内联底色`).toBe('')
      expect(el.style.borderColor, `${el.className} 写了内联边框色`).toBe('')
    }
  })

  it('每个色调都渲染得出卡片', async () => {
    // 旧写法查 THEME_PREVIEW 表，漏一个色调就 return null（那张卡直接不见）。
    // 现在不查表了，这条防的是将来又引入一层映射。
    for (const tone of TONES) {
      await render(<ThemeCard id={tone.id} label={tone.id} mode="light" active={false} onClick={() => {}} />)
      expect(container.querySelector('.tc-preview')?.getAttribute('data-theme'), tone.id).toBe(tone.id)
    }
  })

  it('点击回调带得出是哪一张', async () => {
    const onClick = vi.fn()
    await render(<ThemeCard id="butter" label="Butter" mode="dark" active={false} onClick={onClick} />)
    await act(async () => container.querySelector<HTMLButtonElement>('.theme-card')?.click())
    expect(onClick).toHaveBeenCalledTimes(1)
  })
})

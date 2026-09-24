import { describe, it, expect } from 'vitest'
import {
  DEFAULT_SETTINGS_PAGE,
  onOpenSettingsRequest,
  requestOpenSettings,
  resolveSettingsPage,
} from '@/lib/settings-nav'
import { SETTINGS_PAGES } from '@/components/settings/registry'

describe('resolveSettingsPage', () => {
  it('认得每一个现有页面', () => {
    for (const p of SETTINGS_PAGES) expect(resolveSettingsPage(p.id)).toBe(p.id)
  })

  // 重组前的 ID 可能写在别处（深链、文档），落到不存在的页上就是一个空白面板
  it('旧版页面 ID 映射到现在所在的页', () => {
    expect(resolveSettingsPage('general')).toBe('appearance')
    expect(resolveSettingsPage('security')).toBe('profile')
    expect(resolveSettingsPage('privacy')).toBe('reading')
    expect(resolveSettingsPage('mail')).toBe('sync')
    expect(resolveSettingsPage('signature')).toBe('compose')
    expect(resolveSettingsPage('aliases')).toBe('compose')
    expect(resolveSettingsPage('rules')).toBe('filters')
    expect(resolveSettingsPage('blocklist')).toBe('filters')
  })

  it('认不出来或没给时回到默认页', () => {
    expect(resolveSettingsPage('nope')).toBe(DEFAULT_SETTINGS_PAGE)
    expect(resolveSettingsPage(undefined)).toBe(DEFAULT_SETTINGS_PAGE)
    expect(resolveSettingsPage(null)).toBe(DEFAULT_SETTINGS_PAGE)
  })
})

describe('打开设置的请求通道', () => {
  it('投递给订阅者，取消订阅后不再收到', () => {
    const got: string[] = []
    const off = onOpenSettingsRequest((p) => got.push(p))
    requestOpenSettings('ai')
    off()
    requestOpenSettings('sync')
    expect(got).toEqual(['ai'])
  })
})

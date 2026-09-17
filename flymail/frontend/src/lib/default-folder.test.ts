import { describe, it, expect } from 'vitest'
import { pickDefaultFolder } from '@/lib/default-folder'
import type { Folder } from '@/lib/types'

function f(over: Partial<Folder>): Folder {
  return {
    id: 1, account_id: 1, path: 'X', display_name: 'X', type: 'custom',
    selectable: true, total_count: 0, unread_count: 0, sort_order: 0,
    ...over,
  }
}

/**
 * 选中了账户就一定要有文件夹。
 *
 * ── 缘起（用户提的） ─────────────────────────────────────────────────────────
 *
 * 「有账户、没文件夹」是个走不出去的空状态，列表区显示「没有邮件 · 此文件夹暂无
 * 内容」——这句话本身就是错的，根本没选文件夹。两条最常走的路都会落到这里：
 * 直接打开应用（URL 无参数），以及点侧栏账号名（切账户会清掉 folder）。
 */
describe('pickDefaultFolder', () => {
  it('优先收件箱', () => {
    const inbox = f({ id: 7, type: 'inbox', display_name: 'INBOX' })
    const got = pickDefaultFolder([f({ id: 3, type: 'sent' }), inbox, f({ id: 9, type: 'trash' })])
    expect(got?.id).toBe(7)
  })

  it('没有收件箱时也要给一个，不能让用户停在空状态', () => {
    const got = pickDefaultFolder([f({ id: 3, type: 'archive' }), f({ id: 5, type: 'custom' })])
    expect(got).not.toBeNull()
    expect(got?.id).toBe(3)
  })

  /**
   * ⚠ 不能挑 \Noselect 的文件夹。
   *
   * 它们只是层级上的节点（Gmail 的「[Gmail]」、某些服务商的分组前缀），
   * 点进去 SELECT 会直接失败。挑中它等于把「空白」换成了「报错」，更糟。
   */
  it('跳过不可选中的层级节点', () => {
    const got = pickDefaultFolder([
      f({ id: 2, type: 'custom', selectable: false, display_name: '[Gmail]' }),
      f({ id: 4, type: 'custom', selectable: true, display_name: '工作' }),
    ])
    expect(got?.id).toBe(4)
  })

  it('一个可选文件夹都没有时返回 null，而不是硬塞一个进去', () => {
    expect(pickDefaultFolder([f({ id: 2, selectable: false })])).toBeNull()
    expect(pickDefaultFolder([])).toBeNull()
  })

  /**
   * 收件箱即使不可选中也优先——这一条是故意的。
   *
   * INBOX 在 IMAP 里是必然存在且必然可选的，如果服务端把它标成 \Noselect，
   * 那是服务端的问题；此时退到别的文件夹反而会让用户每次打开都落在奇怪的地方。
   */
  it('收件箱优先于其它可选文件夹', () => {
    const got = pickDefaultFolder([
      f({ id: 4, type: 'custom', selectable: true }),
      f({ id: 8, type: 'inbox', selectable: true }),
    ])
    expect(got?.id).toBe(8)
  })
})

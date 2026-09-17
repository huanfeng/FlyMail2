import type { Folder } from '@/lib/types'

/**
 * 选中了账户却没有文件夹时，默认落到哪个文件夹。
 *
 * ── 为什么需要这个 ───────────────────────────────────────────────────────────
 *
 * 「有账户、没文件夹」是个走不出去的空状态：列表区显示「没有邮件 · 此文件夹暂无
 * 内容」——而这句话本身就是错的，根本没选任何文件夹。用户能进到这里的路至少有
 * 两条，而且都是最常走的：
 *
 *   1. 直接打开应用（URL 上什么参数都没有）
 *   2. 点侧栏的账号名——那一下会切账户并清掉 folder
 *
 * 移动端更糟：点账号名还会把抽屉关掉，于是人被丢在一个空列表上，还得重新把
 * 导航点开才能选文件夹。
 */
export function pickDefaultFolder(folders: Folder[]): Folder | null {
  if (folders.length === 0) return null
  // 收件箱是绝大多数人打开邮箱想看的第一样东西
  const inbox = folders.find((f) => f.type === 'inbox')
  if (inbox) return inbox
  // ⚠ 没有收件箱时也必须给出一个，否则又回到那个空状态。
  // 但不能随便给：\Noselect 的文件夹（纯粹的层级节点，比如 Gmail 的「[Gmail]」）
  // 点进去 SELECT 会直接失败，那是把空白换成了报错。
  return folders.find((f) => f.selectable) ?? null
}

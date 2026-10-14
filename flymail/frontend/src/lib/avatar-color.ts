/**
 * 发件人头像底色。
 *
 * 此前所有头像都是同一个 --accent，一列邮件扫过去没有任何区分度。
 * 按发件人地址做稳定哈希取 8 个色相之一，令牌 --avatar-1..8 定义在 index.css
 * （oklch，白字对比 ≥ 4.5，亮暗共用一套——判据是「字对底」不是「底对页」）。
 */

/** 哈希键：地址优先（小写、去首尾空白），没有地址才退回名字 */
export function avatarKey(name: string, addr: string): string {
  const a = addr.trim().toLowerCase()
  return a || name.trim()
}

/** FNV-1a 32 位哈希，映射到 1..8 */
export function avatarHue(key: string): number {
  let h = 0x811c9dc5
  for (let i = 0; i < key.length; i++) {
    h ^= key.charCodeAt(i)
    h = Math.imul(h, 0x01000193) >>> 0
  }
  return (h % 8) + 1
}

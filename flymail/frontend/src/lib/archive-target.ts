// 归档可用性的单一出口：阅读区工具条（Shell.canArchiveCurrent）与列表行悬停按钮共用。
//
// 两处各自推导过一次就会漂移——例如一处忘了排除 \Noselect 的归档文件夹，
// 按钮亮着、点下去服务器拒绝。
import type { Folder } from './types'

/** 账户的归档文件夹（必须可选）；没有返回 null */
export function archiveFolderOf(folders: Folder[]): Folder | null {
  return folders.find((f) => f.type === 'archive' && f.selectable) ?? null
}

/**
 * 单封邮件的归档目标。账户文件夹没加载到（map 里没有）时返回 null：
 * 宁可暂时不显示按钮，也不显示一个点了才报错的按钮。
 */
export function archiveTargetForMessage(
  foldersByAccount: Map<number, Folder[]>,
  msg: { account_id: number; folder_id: number },
): number | null {
  const folders = foldersByAccount.get(msg.account_id)
  if (!folders) return null
  const target = archiveFolderOf(folders)
  if (target == null || target.id === msg.folder_id) return null
  return target.id
}

/** 会话的归档目标：会话跨文件夹，「整条已在归档」不成立，只看账户有无归档文件夹 */
export function archiveTargetForThread(
  foldersByAccount: Map<number, Folder[]>,
  item: { account_id: number },
): number | null {
  const folders = foldersByAccount.get(item.account_id)
  if (!folders) return null
  return archiveFolderOf(folders)?.id ?? null
}

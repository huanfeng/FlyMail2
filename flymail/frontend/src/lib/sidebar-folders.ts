import type { Folder } from '@/lib/types'

export interface FolderSplit<T> {
  /** 要渲染的行（顺序保持入参顺序） */
  visible: T[]
  /** 折叠起来的条数（展开时为 0） */
  hiddenCount: number
  /** 折叠部分的未读合计：折叠不能让未读消失 */
  hiddenUnread: number
  /** 是否需要「更多」开关（custom 达到阈值才有） */
  foldable: boolean
}

/**
 * 系统文件夹常显；custom 文件夹 ≥ threshold 个时折进「更多」，
 * 但当前激活的那个即使折叠也常显——选中项不能从侧栏消失。
 *
 * 10 个账户时 `Google/G+`、`[Gmail]/重要` 这类自定义文件夹全部平铺，侧栏成墙；
 * 阈值取 3：只有一两个的账户折起来反而多一次点击。
 */
export function splitFolders<T extends Pick<Folder, 'id' | 'type' | 'unread_count'>>(
  folders: T[],
  activeId: number | null,
  showMore: boolean,
  threshold = 3,
): FolderSplit<T> {
  const customCount = folders.filter((f) => f.type === 'custom').length
  const foldable = customCount >= threshold
  if (!foldable || showMore) {
    return { visible: folders, hiddenCount: 0, hiddenUnread: 0, foldable }
  }
  const visible: T[] = []
  let hiddenCount = 0
  let hiddenUnread = 0
  for (const f of folders) {
    if (f.type !== 'custom' || f.id === activeId) visible.push(f)
    else {
      hiddenCount++
      hiddenUnread += f.unread_count
    }
  }
  return { visible, hiddenCount, hiddenUnread, foldable }
}

const KEY = 'flymail.sidebar.more-folders'

/** 各账户「更多文件夹」展开状态；读不到或格式不对时返回空表（与 sidebar-expand 同一套路） */
export function loadMoreExpanded(): Record<number, boolean> {
  try {
    const raw = localStorage.getItem(KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (parsed == null || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    const out: Record<number, boolean> = {}
    for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
      const id = Number(k)
      if (Number.isInteger(id) && id > 0 && typeof v === 'boolean') out[id] = v
    }
    return out
  } catch {
    return {}
  }
}

/** 写回 localStorage；失败不抛（存储不可用时只是不记忆） */
export function saveMoreExpanded(map: Record<number, boolean>): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(map))
  } catch {
    /* 忽略 */
  }
}

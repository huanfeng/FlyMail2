import { FolderType } from '@/api/types'

// Re-export FolderType for convenience
export { FolderType }

// 文件夹类型到图标的映射
export const FOLDER_TYPE_ICONS: Record<FolderType, string> = {
  [FolderType.UNKNOWN]: 'lucide:folder',
  [FolderType.INBOX]: 'lucide:inbox',
  [FolderType.SENT]: 'lucide:send',
  [FolderType.DRAFTS]: 'lucide:file-text',
  [FolderType.TRASH]: 'lucide:trash-2',
  [FolderType.JUNK]: 'lucide:shield-alert',
  [FolderType.ARCHIVE]: 'lucide:archive',
  [FolderType.CUSTOM]: 'lucide:folder'
}

// 文件夹类型到本地化名称的映射（用于国际化）
export const FOLDER_TYPE_NAMES: Record<FolderType, string> = {
  [FolderType.UNKNOWN]: 'folder.type.unknown',
  [FolderType.INBOX]: 'folder.type.inbox',
  [FolderType.SENT]: 'folder.type.sent',
  [FolderType.DRAFTS]: 'folder.type.drafts',
  [FolderType.TRASH]: 'folder.type.trash',
  [FolderType.JUNK]: 'folder.type.junk',
  [FolderType.ARCHIVE]: 'folder.type.archive',
  [FolderType.CUSTOM]: 'folder.type.custom'
}

// 文件夹类型排序优先级
export const FOLDER_TYPE_SORT_ORDER: Record<FolderType, number> = {
  [FolderType.INBOX]: 1,
  [FolderType.SENT]: 2,
  [FolderType.DRAFTS]: 3,
  [FolderType.JUNK]: 4,
  [FolderType.TRASH]: 5,
  [FolderType.ARCHIVE]: 6,
  [FolderType.CUSTOM]: 7,
  [FolderType.UNKNOWN]: 8
}

// 获取文件夹类型对应的图标
export function getFolderTypeIcon(type: FolderType): string {
  return FOLDER_TYPE_ICONS[type] || FOLDER_TYPE_ICONS[FolderType.UNKNOWN]
}

// 获取文件夹类型对应的本地化名称键
export function getFolderTypeName(type: FolderType): string {
  return FOLDER_TYPE_NAMES[type] || FOLDER_TYPE_NAMES[FolderType.UNKNOWN]
}

// 获取文件夹类型排序优先级
export function getFolderTypeSortOrder(type: FolderType): number {
  return FOLDER_TYPE_SORT_ORDER[type] || FOLDER_TYPE_SORT_ORDER[FolderType.UNKNOWN]
}

// 检查是否为系统文件夹
export function isSystemFolder(type: FolderType): boolean {
  return type !== FolderType.CUSTOM && type !== FolderType.UNKNOWN
}

// 检查是否为可删除的文件夹
export function isDeletableFolder(type: FolderType): boolean {
  return type === FolderType.CUSTOM
}

// 获取特定类型的文件夹
export function findFolderByType(folders: { type: FolderType }[], type: FolderType) {
  return folders.find(folder => folder.type === type)
}

// 过滤特定类型的文件夹
export function filterFoldersByType(folders: { type: FolderType }[], type: FolderType) {
  return folders.filter(folder => folder.type === type)
}
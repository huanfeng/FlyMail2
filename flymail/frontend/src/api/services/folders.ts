import { api } from '../axios'
import { API_ENDPOINTS } from '../config'
import type { Folder } from '../types'
import { ApiError } from '../ApiError'
import { FolderType, getFolderTypeSortOrder } from '@/utils/folderType'
import { i18n } from '@/locales'

class FoldersService {
  /**
   * Get folders list for specific account
   */
  async getFolders(accountId: number) {
    const response = await api.get<{ folders: Folder[] }>(
      API_ENDPOINTS.FOLDERS.LIST(accountId)
    )
    
    if (response.code === 0 && response.data) {
      return response.data.folders
    }
    
    throw new ApiError(response)
  }

  /**
   * Sync folders from IMAP server
   */
  async syncFolders(accountId: number) {
    const response = await api.post<{ 
      folders: Folder[]; 
      count: number; 
      synced_at: string 
    }>(
      API_ENDPOINTS.FOLDERS.SYNC(accountId)
    )
    
    if (response.code === 0 && response.data) {
      return response.data
    }
    
    throw new ApiError(response)
  }

  /**
   * Get folder tree structure for display
   * This is a client-side helper to organize folders hierarchically
   */
  buildFolderTree(folders: Folder[]): FolderTreeNode[] {
    const folderMap = new Map<string, FolderTreeNode>()
    const rootFolders: FolderTreeNode[] = []

    // First pass: create all folder nodes
    folders.forEach(folder => {
      folderMap.set(folder.raw_name, {
        ...folder,
        children: []
      })
    })

    // Second pass: build tree structure
    folders.forEach(folder => {
      const node = folderMap.get(folder.raw_name)!
      
      if (folder.parent_name && folderMap.has(folder.parent_name)) {
        const parent = folderMap.get(folder.parent_name)!
        parent.children.push(node)
      } else {
        rootFolders.push(node)
      }
    })

    return this.sortFolders(rootFolders)
  }

  /**
   * Sort folders by type priority and name
   */
  private sortFolders(folders: FolderTreeNode[]): FolderTreeNode[] {
    return folders.sort((a, b) => {
      const priorityA = getFolderTypeSortOrder(a.type)
      const priorityB = getFolderTypeSortOrder(b.type)
      
      if (priorityA !== priorityB) {
        return priorityA - priorityB
      }
      
      return a.name.localeCompare(b.name)
    }).map(folder => ({
      ...folder,
      children: this.sortFolders(folder.children)
    }))
  }

  /**
   * Get folder icon based on type
   * @deprecated Use getFolderTypeIcon from @/utils/folderType instead
   */
  getFolderIcon(type: FolderType): string {
    const iconMap: Record<FolderType, string> = {
      [FolderType.INBOX]: 'lucide:inbox',
      [FolderType.SENT]: 'lucide:send',
      [FolderType.DRAFTS]: 'lucide:file-text',
      [FolderType.TRASH]: 'lucide:trash-2',
      [FolderType.JUNK]: 'lucide:shield-alert',
      [FolderType.ARCHIVE]: 'lucide:archive',
      [FolderType.CUSTOM]: 'lucide:folder',
      [FolderType.UNKNOWN]: 'lucide:folder'
    }
    
    return iconMap[type] || 'lucide:folder'
  }

  /**
   * Get localized folder name
   */
  getLocalizedFolderName(folder: Folder): string {
    const t = (i18n.global as any).t
    
    // 首先基于类型尝试获取本地化名称
    const typeNameMap: Record<FolderType, string> = {
      [FolderType.INBOX]: t('folders.inbox'),
      [FolderType.SENT]: t('folders.sent'),
      [FolderType.DRAFTS]: t('folders.drafts'),
      [FolderType.TRASH]: t('folders.trash'),
      [FolderType.JUNK]: t('folders.junk'),
      [FolderType.ARCHIVE]: t('folders.archive'),
      [FolderType.CUSTOM]: folder.name,
      [FolderType.UNKNOWN]: folder.name
    }
    
    // 如果类型是已知的系统文件夹类型，使用类型映射
    if (typeNameMap[folder.type] && folder.type !== FolderType.CUSTOM && folder.type !== FolderType.UNKNOWN) {
      return typeNameMap[folder.type]
    }
    
    // 对于自定义文件夹或未知类型，回退到原始名称映射
    const rawNameMap: Record<string, string> = {
      'INBOX': t('folders.inbox'),
      'Sent': t('folders.sent'),
      'Drafts': t('folders.drafts'),
      'Trash': t('folders.trash'),
      'Spam': t('folders.junk'),
      'Junk': t('folders.junk'),
      'Archive': t('folders.archive'),
      'All Mail': t('folders.allInbox'),
      'Important': t('folders.important'),
      'Starred': t('folders.starred')
    }
    
    return rawNameMap[folder.raw_name] || folder.name
  }
}

// Extended Folder type with children for tree structure
export interface FolderTreeNode extends Folder {
  children: FolderTreeNode[]
}

export const foldersService = new FoldersService()
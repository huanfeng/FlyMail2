// 内联图登记表：一次撰写会话里，把 blob URL / data: URI 映射回「cid + 文件」。
//
// 为什么需要一张表而不是把 cid 写进 <img> 属性：属性会跟着正文一起被复制、粘贴、
// 存进草稿，于是同一个 cid 可能在文档里出现两次而文件只有一份；表放在会话里，
// 谁引用了谁一目了然，重复引用也只上传一份（见 prepareInlineForSend 的 seen 表）。
//
// 生命周期由调用方掌控：blob URL 不 revoke 就是内存泄漏，而 revoke 早了图就裂——
// 唯一安全的时机是撰写窗口关闭（或换一封信）时，所以这里只提供 reset，不自作主张。

import { useState } from 'react'
import { dataUriToFile, fileToDataUri, newCid } from '@/lib/inline-images'
import type { InlineAsset } from '@/lib/inline-images'

export interface InlineImageStore {
  /** 登记一张图，返回编辑器内预览用的 blob URL */
  register: (file: File) => string
  /** src → cid + 文件；不是本会话登记的内联图返回 null */
  lookup: (src: string) => InlineAsset | null
  /** blob URL → data: URI（存草稿用） */
  toDataUri: (src: string) => Promise<string | null>
  /** 当前已登记的内联图总字节数（与附件合并计入发送大小上限） */
  totalBytes: () => number
  /** 释放所有 blob URL 并清空登记；撰写窗口关闭时调用 */
  reset: () => void
}

/**
 * 建一张空的登记表。
 *
 * 与 React 无关，纯闭包——所以它可以被 useState 的惰性初始化整个建一次，
 * 全程保持同一个对象引用（RichEditor 把它当回调依赖，引用一变就要白跑一轮 effect）。
 */
export function createInlineImageStore(): InlineImageStore {
  const blobs = new Map<string, InlineAsset>()
  // 草稿里带回来的 data: 图，解码一次就缓存住，别每次发送都重解一遍
  const datas = new Map<string, InlineAsset>()

  return {
    register(file) {
      const url = URL.createObjectURL(file)
      blobs.set(url, { cid: newCid(), file })
      return url
    },

    lookup(src) {
      if (src.startsWith('blob:')) return blobs.get(src) ?? null
      if (src.startsWith('data:')) {
        const cached = datas.get(src)
        if (cached) return cached
        const cid = newCid()
        const file = dataUriToFile(src, cid)
        if (!file) return null
        const asset: InlineAsset = { cid, file }
        datas.set(src, asset)
        return asset
      }
      return null
    },

    async toDataUri(src) {
      const asset = blobs.get(src)
      if (!asset) return null
      try {
        return await fileToDataUri(asset.file)
      } catch {
        return null
      }
    },

    totalBytes() {
      let sum = 0
      for (const a of blobs.values()) sum += a.file.size
      for (const a of datas.values()) sum += a.file.size
      return sum
    },

    reset() {
      for (const url of blobs.keys()) URL.revokeObjectURL(url)
      blobs.clear()
      datas.clear()
    },
  }
}

/** 组件级的内联图登记表，整个组件生命周期内是同一张 */
export function useInlineImages(): InlineImageStore {
  const [store] = useState(createInlineImageStore)
  return store
}

/** 把字节数格式化为 B / KB / MB 字符串。 */
export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

/**
 * 把未知形状的错误取成一行可展示的文本；取不出就返回空串（不显示细节行）。
 *
 * 错误细节值得单独一行：「加载失败」只告诉用户出了事，
 * 而 401 / 网络不可达 / 500 对应的下一步动作完全不同。
 */
export function errorText(err: unknown): string {
  if (err instanceof Error) return err.message
  if (typeof err === 'string') return err
  return ''
}

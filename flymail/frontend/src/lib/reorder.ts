/**
 * 列表重排：把 index 处的元素移动 delta 格。
 *
 * 抽成纯函数而不是写在组件里，是因为「上移/下移」的边界条件比它看起来多：
 * 首尾越界、delta 跨多格、空列表。这些在组件里只能靠点按钮来验，
 * 而点按钮验不到「越界时原数组有没有被就地改动」这类问题。
 *
 * 不修改入参：调用方拿它做乐观更新，而 react-query 缓存里的那个数组
 * 是不能就地改的——改了之后回滚时手里的「旧值」也已经被改掉了。
 *
 * @returns 移动后的新数组；无法移动（越界）时返回**原数组本身**，
 *          调用方可用 `next === list` 判断「什么都没发生」，从而不发请求。
 */
export function moveItem<T>(list: readonly T[], index: number, delta: number): readonly T[] {
  const target = index + delta
  if (index < 0 || index >= list.length) return list
  if (target < 0 || target >= list.length) return list
  if (delta === 0) return list

  const next = [...list]
  const [item] = next.splice(index, 1)
  next.splice(target, 0, item)
  return next
}

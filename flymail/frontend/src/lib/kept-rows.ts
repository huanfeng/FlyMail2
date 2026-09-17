/**
 * 在筛选视图里「留住已经不满足筛选的行」。
 *
 * ── 缘起（用户报的，两轮） ───────────────────────────────────────────────────
 *
 * 开着「未读」筛选读邮件时，每读一封它就被标成已读、不再满足筛选，下一次刷新
 * 就从列表里删掉。造成两个问题：
 *
 *   1. 正在读的那封从列表消失，选中态没了，「上一封 / 下一封」双双变灰
 *      ——它们按当前项的下标算，而那一项已经不在列表里，下标是 -1
 *   2. 更烦的是**每读一封列表就少一行**：后面的行整体上移，下标全变，
 *      于是「下一封」跳到的不是眼睛看到的下一行
 *
 * 第一版只把**当前那一封**钉住，解决了 1 但解决不了 2：切走的瞬间它仍然消失，
 * 列表照样重排。
 *
 * 主流客户端的做法是：**读过的都留在原地、显示成已读的样子**，直到用户切走再
 * 回来（换筛选、换文件夹）才一次性清掉。这样一次浏览期间列表是稳定的，
 * 下标不会动，也看得出哪些刚读过。
 */

/** 一次「浏览」期间被留下的行。viewKey 变了就整批作废。 */
export interface KeptRows<T> {
  /** 视图身份：账户 / 文件夹 / 筛选 等任一变化都意味着重新开始 */
  viewKey: string
  /** key → 该行在被筛掉之前的最后一份快照 */
  rows: Map<string, T>
}

export function emptyKept<T>(viewKey = ''): KeptRows<T> {
  return { viewKey, rows: new Map() }
}

/** 把快照改成「已读」的样子。 */
export interface ReadLook<T> {
  isRead: (item: T) => boolean
  asRead: (item: T) => T
}

/**
 * 在渲染期算出下一份 KeptRows。
 *
 * 返回值与 prev 相同引用时，调用方据此跳过 setState——⚠ 这是在渲染期同步 state，
 * 每次都产出新对象就是无限重渲染。
 *
 * 只记录**用户打开过的**那些行，不是所有见过的行：后者会把翻页划过的几百条全部
 * 留下，筛选就形同虚设了。
 */
export function nextKept<T>(
  prev: KeptRows<T>,
  viewKey: string,
  list: T[],
  activeKey: string | null,
  keyOf: (item: T) => string,
): KeptRows<T> {
  // 换了视图：上一轮留下的行与这里无关，整批丢掉
  if (prev.viewKey !== viewKey) {
    const fresh = emptyKept<T>(viewKey)
    if (activeKey != null) {
      const found = list.find((item) => keyOf(item) === activeKey)
      if (found != null) fresh.rows.set(activeKey, found)
    }
    return fresh
  }
  if (activeKey == null) return prev
  const found = list.find((item) => keyOf(item) === activeKey)
  // 不在列表里说明它已经被筛掉了，此时留着的是之前记下的快照，不必更新
  if (found == null || prev.rows.get(activeKey) === found) return prev
  const rows = new Map(prev.rows)
  rows.set(activeKey, found)
  return { viewKey, rows }
}

/**
 * 把被筛掉的行插回列表，并显示成已读的样子。
 *
 * ⚠ 按「记住的下标」插回是不行的：列表会分页续拉、会被后台刷新重排，当初那个
 * 下标早就不是原来的意思了。所以要按排序键归位。
 *
 * ⚠⚠ compare 必须与**后端的排序完全一致**，包括次级键。
 *
 * 后端排的是 (date DESC, id DESC) / (date DESC, thread_id DESC)。只按 date 比的话，
 * 同一秒到达的几封（批量投递、自动通知）次序是未定义的，插回去就会打乱——
 * 实测四封同秒邮件被排成 4、1、2、3，而「下一封」按数组下标走，于是跳到的不是
 * 眼睛看到的下一行。这个错法很隐蔽：只有同秒邮件才暴露。
 */
export function withKept<T>(
  list: T[],
  kept: KeptRows<T>,
  keyOf: (item: T) => string,
  compare: (a: T, b: T) => number,
  look?: ReadLook<T>,
): T[] {
  if (kept.rows.size === 0) return list
  const present = new Set(list.map(keyOf))
  const missing: T[] = []
  for (const item of kept.rows.values()) {
    if (present.has(keyOf(item))) continue
    missing.push(look != null && !look.isRead(item) ? look.asRead(item) : item)
  }
  if (missing.length === 0) return list
  return [...list, ...missing].sort(compare)
}

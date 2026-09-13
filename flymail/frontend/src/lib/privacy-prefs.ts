// 阅读隐私相关的本地偏好。
//
// 单独成文件而不是留在 MessageBody / SettingsDialog 里：这个开关现在有三个读者——
// 设置面板（写）、useMessageDetail（决定详情请求带不带 remote=1）、正文组件（初始态），
// 之前它以字面量 'flymail_load_remote_images' 散落在两处，改一处漏一处只会是时间问题。
//
// 做成可订阅的外部存储（配 useSyncExternalStore）而不是纯 getter：
// 开关值参与 useMessageDetail 的 query key，改了开关必须让已挂载的详情查询
// **先换 key、再取数**。只写 localStorage 的话组件不会重新渲染，key 停在旧值上，
// 这时若去 invalidate 就会照着旧口径再请求一次——多打一趟网络，结果还是错的。

const LOAD_REMOTE_IMAGES_KEY = 'flymail_load_remote_images'
const DARK_BODY_KEY = 'flymail_dark_body'

/** 开关变化的订阅者集合（useSyncExternalStore 用） */
const listeners = new Set<() => void>()

/**
 * 「默认显示远程图片」是否打开。
 *
 * 打开等于对所有发件人放行：详情请求一律带 remote=1，服务端不再把远程引用换成占位符。
 * 默认关闭——打开邮件即向发件人回报「已读 + IP + 时间」正是 M12 要堵的洞。
 *
 * localStorage 在少数环境（隐私模式、WebView 关闭存储）会直接抛异常，
 * 读不到时按「关闭」处理：隐私开关的失败方向必须是更保守的那一侧。
 *
 * 直接读 localStorage 而不缓存：返回的是布尔基元，useSyncExternalStore 不会因此
 * 反复触发渲染；换成本地缓存反而要额外处理跨标签页写入造成的不一致。
 */
export function getRemoteImageDefault(): boolean {
  try {
    return localStorage.getItem(LOAD_REMOTE_IMAGES_KEY) === 'true'
  } catch {
    return false
  }
}

/** 写入「默认显示远程图片」并通知订阅者。写失败静默忽略：设置项不值得因存储不可用而中断界面。 */
export function setRemoteImageDefault(on: boolean): void {
  try {
    localStorage.setItem(LOAD_REMOTE_IMAGES_KEY, String(on))
  } catch {
    /* 存储不可用：本次会话内的开关仍由组件 state 生效，只是不持久化 */
  }
  for (const fn of Array.from(listeners)) fn()
}

/**
 * 订阅本文件里任一隐私开关的变化；返回取消订阅函数（useSyncExternalStore 的契约）。
 *
 * 一个订阅者集合服务两个开关，所以名字是中性的：订阅方拿到通知后各自重读自己
 * 关心的那个 getter，值没变时 useSyncExternalStore 自己会跳过重渲染。
 */
export function subscribePrivacyPrefs(onChange: () => void): () => void {
  listeners.add(onChange)
  return () => {
    listeners.delete(onChange)
  }
}

// ── 暗化正文 ─────────────────────────────────────────────────────────────────
//
// 与上面那个开关放同一个文件：读者一样是「设置面板写、正文组件读」，
// 而把两个同族偏好拆到两处正是上面那段注释说的那种"改一处漏一处"。

/**
 * 「暗化正文」是否打开。默认关闭。
 *
 * 关闭是有理由的而不是保守：暗化用的是整体反相（见 MAIL_BODY_DARK_CSS），
 * 对用背景图拼版式的营销邮件会显出接缝。默认让邮件按发件人设计的样子显示，
 * 想要护眼的人自己打开并接受这个代价。
 */
export function getDarkBody(): boolean {
  try {
    return localStorage.getItem(DARK_BODY_KEY) === 'true'
  } catch {
    return false
  }
}

/** 写入「暗化正文」并通知订阅者。 */
export function setDarkBody(on: boolean): void {
  try {
    localStorage.setItem(DARK_BODY_KEY, String(on))
  } catch {
    /* 存储不可用：本次会话内仍由组件 state 生效，只是不持久化 */
  }
  for (const fn of Array.from(listeners)) fn()
}

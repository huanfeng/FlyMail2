/**
 * 本界面的客户端标识，随每个请求发出（请求头 X-Client-Id）。
 *
 * 唯一用途是让 mail_state 事件认得出「这是我自己刚才那次操作的回声」：
 * 发起方在 mutation 的 onSettled 里已经失效过一轮缓存，收到回声再失效一次
 * 就是白打一轮请求（一次标已读会带出 folders ×N + 两个计数接口）。
 *
 * 每个标签页一个，刷新即换新——刷新本来就会把所有数据重取一遍，
 * 没有「刷新前后要认作同一个客户端」的场景。
 *
 * crypto.randomUUID 要 HTTPS 或 localhost 才有（非安全上下文里是 undefined），
 * 而 FlyMail 常常部署在局域网 http 上，所以必须有退路；退路只要够区分同时开着的
 * 几个标签页即可，不需要密码学强度。
 */
function newId(): string {
  try {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID()
    }
  } catch {
    // 落到下面的退路
  }
  return `c-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
}

export const CLIENT_ID = newId()

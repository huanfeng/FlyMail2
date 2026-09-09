// 邮件正文 iframe 的文档构建 + 父子通信协议。
//
// 为什么从 MessageBody.tsx 抽出来：M12 去掉了 iframe 的 allow-same-origin，
// 父窗口从此拿不到正文文档，高度测量与链接拦截只能靠注入脚本 + postMessage。
// 这几样东西（sandbox 属性、CSP、注入脚本、消息校验）合起来就是这个应用的安全边界，
// 放在 React 组件里只能靠肉眼 review；抽成纯模块后可以被单元测试直接盯住。

/**
 * iframe 的 sandbox 属性。
 *
 * ⚠⚠ 任何情况下不得同时开启 `allow-same-origin` 与 `allow-scripts`。⚠⚠
 *
 * 两者同时出现时，iframe 里的脚本与父页面同源——它可以拿到 parent.document、
 * 读走 localStorage 里的 access token、把整个应用改写掉，等于沙箱不存在。
 * 邮件正文是彻头彻尾的不可信输入（发件人任意，内容任意），这条线没有例外。
 *
 * 现在的取舍：保留 allow-scripts 只为了跑我们自己注入的那一段带 nonce 的脚本
 * （量高度、拦链接）；邮件自带的脚本已在服务端净化时整块剥掉，CSP 的
 * `script-src 'nonce-…'` 是第二道——没有 nonce 的内联脚本一律不执行。
 * allow-popups + allow-popups-to-escape-sandbox 让 `<base target=_blank>` 这条
 * 兜底路径（注入脚本失效时）仍能把外链开在新标签而不是顶掉正文。
 */
export const MAIL_FRAME_SANDBOX = 'allow-scripts allow-popups allow-popups-to-escape-sandbox'

/** iframe 兜底高度：测量失败时至少给出可读的一屏，而不是缩成一个小格子 */
export const MIN_BODY_HEIGHT = 240

/**
 * 接受的最大高度。
 *
 * 消息来自不可信文档：即使 token 校验通过（token 本来就写在那份文档里，
 * 邮件内容理论上读得到），伪造一个荒谬的高度也只会把页面撑成一片空白。
 * 两万像素已经远超任何真实邮件，再长的内容交给 iframe 自己的滚动条——
 * 那条兜底路径本来就一直开着（见 MailBodyFrame 里不设 overflow:hidden 的说明）。
 */
export const MAX_FRAME_HEIGHT = 20_000

// ── 随机 token / nonce ───────────────────────────────────────────────────────

/**
 * 生成十六进制随机串，用作 CSP nonce 与 postMessage 校验 token；没有密码学随机源时返回 null。
 *
 * ⚠ 不用 crypto.randomUUID：桌面端（Wails / WebView2）通过自定义协议加载页面时
 * 上下文不被视为 secure context，randomUUID 与 subtle 都可能不存在，
 * 而 getRandomValues 在非安全上下文里仍然可用。
 *
 * ⚠ 拿不到 getRandomValues 时**绝不**退回 Math.random。nonce 是 `script-src` 的唯一
 * 放行条件：一个可预测的 nonce 等于让邮件正文自己猜出来、写进一个带 nonce 的 <script>，
 * CSP 这道防线就此失效。此时调用方应当整段不注入脚本、CSP 发 `script-src 'none'`，
 * 高度交给父窗口的兜底计时器——少一个自适应高度，远远好过多一条脚本执行路径。
 */
export function secureRandomHex(bytes = 16): string | null {
  const c: Crypto | undefined = typeof globalThis !== 'undefined' ? globalThis.crypto : undefined
  if (!c || typeof c.getRandomValues !== 'function') return null
  const arr = new Uint8Array(bytes)
  c.getRandomValues(arr)
  let out = ''
  for (const b of arr) out += b.toString(16).padStart(2, '0')
  return out
}

// ── CSP ──────────────────────────────────────────────────────────────────────

/**
 * 应用自身的 origin，用于 CSP 白名单。
 *
 * ⚠ 去掉 allow-same-origin 之后 iframe 文档是不透明源，CSP 里的 `'self'` 指的是
 * 那个不透明源，再也匹配不上 `/api/v1/messages/…` 这类内联附件地址（cid: 改写的结果）。
 * 必须把父页面的 origin 显式写进 img-src，否则内联图片会被 CSP 拦掉——
 * 这是「移除 allow-same-origin」最容易踩的一脚。
 * srcdoc 文档的 base URL 继承自父文档，所以相对地址仍然解析到这个 origin。
 */
function appOrigin(): string {
  if (typeof window === 'undefined') return ''
  const o = window.location?.origin ?? ''
  // 不写死 http/https：桌面端（Wails/WebView2）用的是自定义虚拟主机，
  // 协议名可能不是这两个。只要形如 scheme://host 就是合法的 CSP 源表达式；
  // 取不到（origin 为 "null"、含空白或引号）时返回空串，交由调用方只留 'self'。
  return /^[a-z][a-z0-9+.-]*:\/\/[^\s;'"]+$/i.test(o) ? o : ''
}

/**
 * 内容安全策略：从根上掐断正文向外发起的请求，而不是逐个正则去剥。
 *
 * 服务端净化（internal/htmlsan）已经是第一道，这里是纵深防御的第二道：
 * 净化漏掉的任何一种外链形式（CSS @import、srcset、poster、background=…）
 * 都在浏览器层面被统一拒绝，不依赖前端认得出它是哪种写法。
 *
 * @param allowRemote 服务端已放行远程引用（用户点了显示 / 发件人在信任名单里），
 *   此时才允许外部图片与字体；否则一律只放行本地与 data:。
 * @param nonce 注入脚本的 nonce；script-src 只认这一个值，
 *   邮件里任何没有 nonce 的内联脚本都不会执行。传 null 表示这次不注入脚本，
 *   策略直接收紧成 `script-src 'none'`。
 */
export function cspMeta(allowRemote: boolean, nonce: string | null): string {
  const self = appOrigin()
  const local = self ? `'self' ${self}` : "'self'"
  // 图片与音视频用同一份来源清单：cid: 内联资源改写后都指向我们自己的附件接口，
  // 少了 media-src 时 default-src 'none' 会把内联音视频一并挡掉。
  const mediaSources = allowRemote ? `${local} data: blob: https: http:` : `${local} data: blob:`
  const policy = [
    "default-src 'none'",
    `img-src ${mediaSources}`,
    `media-src ${mediaSources}`,
    // ⚠ style-src 与 allowRemote 无关，恒定只放行内联样式：远程样式表要么是
    // <link>、要么是 CSS @import，两者都已在服务端净化时整块剥掉。放开 https:
    // 换不来任何能渲染出来的东西，只是白白留一条「打开远程内容即向第三方发请求」的路。
    "style-src 'unsafe-inline'",
    allowRemote ? 'font-src data: https:' : 'font-src data:',
    nonce ? `script-src 'nonce-${nonce}'` : "script-src 'none'",
    "connect-src 'none'",
    "frame-src 'none'",
    "object-src 'none'",
    "form-action 'none'",
    "base-uri 'none'",
  ].join('; ')
  return `<meta http-equiv="Content-Security-Policy" content="${policy}">`
}

// ── 注入脚本 ─────────────────────────────────────────────────────────────────

/**
 * 注入到正文文档里的引导脚本（源码字符串，`__FM_TOKEN__` 在构建时替换）。
 *
 * 它承担两件原本由父窗口越过同源边界完成的事：
 *
 * 1. **高度测量**。沿用原来的哨兵法而不是 documentElement.scrollHeight：
 *    scrollHeight 至少等于视口高度，而 iframe 的视口高度正是我们要算的那个值，
 *    内容变矮时它会把上一次的高度原样返回，表现为「正文框永远不缩回去」。
 *    末尾哨兵的位置由布局引擎给出，只取决于内容，从根上绕开这个循环依赖。
 * 2. **链接拦截**。桌面端（Wails/WebView2）里直接导航会把整个应用页面顶成外站，
 *    且没有后退入口，所以外链必须交给父窗口走 BrowserOpenURL。
 *    父窗口现在碰不到这份文档，只能由文档自己把 href 报上去。
 *
 * 写成 ES5 风格的 IIFE：这段代码不经过打包器与 TS，出问题只能靠读，越朴素越好。
 */
const FRAME_SCRIPT = `(function () {
  var TOKEN = __FM_TOKEN__;
  var lastSent = -1;

  function px(v) { var n = parseFloat(v || '0'); return isFinite(n) ? n : 0; }

  // 量出内容真实高度：末尾哨兵为主，子元素底边为辅，两条路的盲区不重叠，取大者。
  function measure() {
    var body = document.body;
    var root = document.documentElement;
    if (!body) return 0;
    // getBoundingClientRect 相对视口，用户若滚动过 iframe 内部需补回滚动量
    var scrollTop = (root && root.scrollTop) || body.scrollTop || 0;
    var cs = getComputedStyle(body);
    // body 自己的下内边距/外边距：任何一种量法都不含它
    var tail = px(cs.paddingBottom) + px(cs.marginBottom);

    // 复用同一个哨兵：反复插入会惊动 ResizeObserver。
    // clear:both 让它落到所有浮动之下，于是「浮动不撑高父容器」这个盲区也一并消失。
    var sentinel = body.querySelector(':scope > [data-fm-measure]');
    if (!sentinel) {
      sentinel = document.createElement('div');
      sentinel.setAttribute('data-fm-measure', '');
      sentinel.style.cssText = 'display:block;height:0;clear:both;font-size:0;line-height:0;border:0;padding:0;margin:0;';
      body.appendChild(sentinel);
    }
    var bySentinel = sentinel.getBoundingClientRect().top + scrollTop + tail;

    // 哨兵挡不住绝对定位/负 margin 造成的溢出，这一路作为补充
    var byChildren = 0;
    var kids = body.children;
    for (var i = 0; i < kids.length; i++) {
      var child = kids[i];
      if (child === sentinel) continue;
      var rect = child.getBoundingClientRect();
      if (rect.height <= 0) continue;
      var bottom = rect.bottom + scrollTop + px(getComputedStyle(child).marginBottom);
      if (bottom > byChildren) byChildren = bottom;
    }
    if (byChildren > 0) byChildren += tail;

    var best = Math.max(bySentinel, byChildren);
    // 两路都没量到（空文档、尚未布局）才退回 scrollHeight：它带着视口下限，只能兜底
    if (best > 0) return best;
    return Math.max(body.scrollHeight, root ? root.scrollHeight : 0);
  }

  // reason='load' 允许父窗口重设基准（可变矮）；'resize' 只允许增高，
  // 迟到的回调可能量到尚未填上内容的文档，放任它缩小会把正确的高度打回最小值。
  function post(reason) {
    var h;
    try { h = measure(); } catch (e) { return; }
    if (!(h > 0)) return;
    h = Math.ceil(h);
    if (reason !== 'load' && Math.abs(h - lastSent) <= 1) return;
    lastSent = h;
    try {
      parent.postMessage({ type: 'fm:height', token: TOKEN, height: h, reason: reason }, '*');
    } catch (e) { /* 父窗口不可达：没有可做的事 */ }
  }

  function ready() {
    post('load');
    if (typeof ResizeObserver === 'function') {
      try {
        var ro = new ResizeObserver(function () { post('resize'); });
        // 两个都观察：body 随内容长高，documentElement 随视口变化（父窗口设完高度后触发）。
        // 后者不会造成回环——measure 只看内容，高度没变就被上面的阈值挡掉了。
        ro.observe(document.documentElement);
        if (document.body) ro.observe(document.body);
      } catch (e) { /* 老环境没有 ResizeObserver：退化成下面几次定时测量 */ }
    }
    // 字体换上后行高会变，load 时量到的是回退字体的高度
    if (document.fonts && document.fonts.ready && document.fonts.ready.then) {
      document.fonts.ready.then(function () { post('resize'); })['catch'](function () {});
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', ready);
  } else {
    ready();
  }
  // window.load 要等所有子资源（允许远程时包括外部图片）结束才来，可能很晚；
  // DOMContentLoaded 先给一个能用的高度，load 再重设一次基准。
  window.addEventListener('load', function () { post('load'); });
  setTimeout(function () { post('resize'); }, 60);
  setTimeout(function () { post('resize'); }, 300);
  setTimeout(function () { post('resize'); }, 1000);

  // 链接一律不在本文档内导航：外链交给父窗口用系统浏览器打开，
  // mailto: 交给父窗口打开应用自己的撰写器。
  //
  // ⚠ 必须同时监听 auxclick：中键点击不触发 click，只触发 auxclick。
  // 少了它，中键点一个外链就会走 <base target=_blank> + allow-popups-to-escape-sandbox
  // 开出一个脱离沙箱的窗口，完全绕过这里的协议白名单与「交给系统浏览器」的路径。
  function onLinkActivate(e) {
    // click 只认左键，auxclick 只认中键；右键交给浏览器的上下文菜单
    if (e.defaultPrevented) return;
    if (e.type === 'click' ? e.button !== 0 : e.button !== 1) return;
    var el = e.target;
    while (el && el.nodeType === 1 && el.tagName !== 'A') el = el.parentNode;
    if (!el || el.nodeType !== 1) return;
    var raw = el.getAttribute('href') || '';
    // 文档内锚点保持默认滚动行为
    if (!raw || raw.charAt(0) === '#') return;
    // 取 .href 而非 getAttribute：由浏览器把相对地址解析成绝对地址
    var href = String(el.href || raw);
    var lower = href.toLowerCase();
    e.preventDefault();
    var type = null;
    if (lower.indexOf('mailto:') === 0) type = 'fm:mailto';
    else if (lower.indexOf('http:') === 0 || lower.indexOf('https:') === 0) type = 'fm:open';
    // 其余协议（file: javascript: 自定义 scheme…）拦下就结束，不上报
    if (!type) return;
    try { parent.postMessage({ type: type, token: TOKEN, href: href }, '*'); } catch (err) {}
  }
  document.addEventListener('click', onLinkActivate, true);
  document.addEventListener('auxclick', onLinkActivate, true);
})();`

/** 把注入脚本包成带 nonce 的 `<script>` 标签 */
function bootstrapScript(nonce: string, token: string): string {
  // ⚠ 用函数式替换而不是字符串替换值：String.replace 的替换串里 `$&` `$'` `$\`` 都是
  // 特殊模式，会把匹配到的内容重新插回去。token 目前是十六进制、不含 `$`，
  // 但这个约束不该靠「调用方记得」来维持——函数式替换从语法上就不解释这些模式。
  const src = FRAME_SCRIPT.replace('__FM_TOKEN__', () => JSON.stringify(token))
  // 闭合标签拆成两段拼接：脚本正文里出现字面量 </script> 会提前终止标签，
  // 这里虽然不含它，但把写法固定下来省得以后有人往 FRAME_SCRIPT 里塞字符串时踩到。
  return `<script nonce="${nonce}">${src}<` + `/script>`
}

// ── 基础样式 ─────────────────────────────────────────────────────────────────

/**
 * 注入到 HTML 正文 iframe 里的基础样式。
 *
 * 邮件 HTML 几乎都是「假设自己在一个白底、有默认字体的文档里」写的：不注入任何样式时，
 * iframe 用的是浏览器缺省样式（Times New Roman、body margin 8px、图片原始尺寸），
 * 于是渲染结果和其它邮件客户端明显不同。
 *
 * 背景固定为白色而不跟随应用主题：邮件里的前景色是写死的（大量深色文字、
 * 甚至写死 color:#000 的签名），在深色背景上会直接变成黑底黑字。
 */
export const MAIL_BODY_CSS = `
html { -webkit-text-size-adjust: 100%; }
body {
  margin: 0; padding: 14px 16px;
  /* ⚠ 有 padding 就必须配 border-box：邮件模板极常见 body{width:100% !important}
     （GitHub、各类营销邮件都这么写），content-box 下 100% 再加上这里的左右内边距
     就会横向溢出正好 32px，正文凭空多出一条横向滚动条。 */
  box-sizing: border-box;
  background: #ffffff; color: #1f2328;
  font-family: -apple-system, "Segoe UI", "Microsoft YaHei", Roboto, Helvetica, Arial, sans-serif;
  font-size: 14px; line-height: 1.6;
  overflow-wrap: break-word; word-break: break-word;
}
/* ⚠ 千万不要在 body 上写 overflow：CSS 规范会把 body 的 overflow 传播到视口，
   body 自身的计算值则变成 visible。这会让 body.scrollHeight / documentElement.scrollHeight
   的语义随之改变，正是高度量不准的经典来源。宽内容溢出交给 iframe 视口默认的滚动行为。 */
img { max-width: 100%; height: auto; border: 0; }
table { max-width: 100%; }
a { color: #0969da; }
pre { white-space: pre-wrap; word-break: break-word; }
pre, code { font-family: ui-monospace, Consolas, "Courier New", monospace; }
blockquote {
  margin: 8px 0 8px 2px; padding-left: 12px;
  border-left: 3px solid #d0d7de; color: #57606a;
}
hr { border: 0; border-top: 1px solid #d8dee4; margin: 16px 0; }
`

// ── 文档构建 ─────────────────────────────────────────────────────────────────

export interface FrameDocumentOptions {
  /** 已由服务端净化、并做过 cid 改写与引用标记的邮件 HTML */
  html: string
  /** 服务端已放行远程引用：放宽 CSP，允许外部图片与字体 */
  allowRemote: boolean
  /** 折叠引用：注入一条把 [data-fm-quote] 藏起来的样式 */
  foldQuote: boolean
  /** 引用折叠用的 CSS（由 quote-fold 提供，避免本模块反向依赖渲染层） */
  quoteHideCss: string
  /**
   * 本次渲染的 CSP nonce 与 postMessage 校验 token（由 secureRandomHex 生成）。
   * 任一为空表示这次没有密码学随机源可用：不注入脚本，CSP 收紧成 script-src 'none'。
   */
  nonce: string | null
  token: string | null
}

/**
 * 把邮件 HTML 包成一个自带基础样式、CSP 与引导脚本的完整文档，供 iframe srcDoc 使用。
 *
 * 引用折叠只是多注入一条隐藏规则：整份文档照常渲染，只把打了标记的引用容器藏起来——
 * 按字符串把 HTML 切成两半几乎必然切出未闭合标签（见 lib/quote-fold.ts）。
 * 这条路径不依赖同源：折叠样式在生成 srcDoc 时就写进去了。
 */
export function buildFrameDocument(o: FrameDocumentOptions): string {
  // 没有可信随机源就整段不注入脚本：宁可没有自适应高度与链接拦截
  // （父窗口有兜底计时器，外链有 base target=_blank），也不能让 nonce 变成可猜的。
  const useScript = Boolean(o.nonce && o.token)
  return (
    `<meta charset="utf-8">${cspMeta(o.allowRemote, useScript ? o.nonce : null)}` +
    // base target=_blank 是兜底：正常路径是注入脚本拦截点击后交给父窗口，
    // 万一脚本没跑起来，也不至于让链接把 iframe 里的邮件内容顶掉。
    `<base target="_blank">` +
    `<style>${MAIL_BODY_CSS}${o.foldQuote ? o.quoteHideCss : ''}</style>` +
    (useScript ? bootstrapScript(o.nonce as string, o.token as string) : '') +
    o.html
  )
}

// ── 父窗口侧的消息校验 ────────────────────────────────────────────────────────

/** 从正文 iframe 收到的、已通过校验的消息 */
export type FrameMessage =
  | { kind: 'height'; height: number; rebase: boolean }
  | { kind: 'mailto'; href: string }
  | { kind: 'open'; href: string }

/**
 * 来源校验：这条 message 事件确实来自 `win` 这个 iframe 的文档。
 *
 * ⚠ 不能用 `event.origin`：iframe 是不透明源，origin 恒为字符串 "null"，
 * 页面上任何一个 sandbox iframe（甚至另一封邮件的正文）发来的消息都长这样。
 * 只有窗口引用的同一性是伪造不了的。
 *
 * win 为 null（iframe 尚未挂载 / 已卸载）时一律拒绝：此时没有任何合法来源。
 */
export function isFrameEvent(event: Pick<MessageEvent, 'source'>, win: Window | null): boolean {
  if (!win) return false
  return event.source === win
}

/**
 * 校验并解析来自正文 iframe 的 postMessage。
 *
 * 校验分两层，缺一不可：
 * - **来源**：调用方必须先过 isFrameEvent。
 * - **token**：本函数负责。同一页面上可能同时挂着多份正文 iframe（会话手风琴），
 *   token 让每份文档只能驱动自己的那个框，而不是把别人的高度改掉。
 *
 * 形状与取值一律按不可信数据对待：类型不对、协议不在白名单、高度越界都返回 null。
 * token 为 null（这次没注入脚本）时不存在任何合法消息，一律返回 null。
 */
export function parseFrameMessage(data: unknown, token: string | null): FrameMessage | null {
  if (!token || typeof data !== 'object' || data === null) return null
  const m = data as Record<string, unknown>
  if (typeof m.token !== 'string' || m.token !== token) return null

  if (m.type === 'fm:height') {
    const h = m.height
    if (typeof h !== 'number' || !Number.isFinite(h) || h <= 0 || h > MAX_FRAME_HEIGHT) return null
    return { kind: 'height', height: h, rebase: m.reason === 'load' }
  }

  if (m.type === 'fm:mailto' || m.type === 'fm:open') {
    const href = m.href
    if (typeof href !== 'string' || href.length === 0) return null
    const scheme = href.slice(0, href.indexOf(':') + 1).toLowerCase()
    if (m.type === 'fm:mailto') return scheme === 'mailto:' ? { kind: 'mailto', href } : null
    return scheme === 'http:' || scheme === 'https:' ? { kind: 'open', href } : null
  }

  return null
}

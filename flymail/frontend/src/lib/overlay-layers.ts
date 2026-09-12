/**
 * 页面上是否有 radix 的模态浮层（Dialog / AlertDialog）正开着。
 *
 * ── 为什么需要这个判断 ──────────────────────────────────────────────────────
 *
 * 仓库里有两类浮层，键盘处理方式完全不同：
 *
 * - **radix 的**：AccountDialog / RuleDialog / ChannelDialog / ComposeDialog /
 *   确认框。它们通过 Portal 挂在 `<body>` 末尾，自带 FocusScope 与 Esc 处理。
 * - **手写的**：设置面板、通知中心、快捷键速查。它们把键盘处理器装在 `document`
 *   上——Esc 关闭、Tab 在自己的子树里环绕。
 *
 * 从手写浮层里弹出一个 radix 浮层时，两者会打架，因为 radix 那个在 DOM 上
 * **永远位于手写浮层之外**（它在 body 末尾，不是设置面板的子节点）：
 *
 * - **按 Esc**：radix 关掉自己那层，但它只调 `preventDefault()` 而**不**调
 *   `stopPropagation()`，事件照常冒泡到 `document`，设置面板的监听器跟着触发
 *   `onClose()`——用户想取消一次删除，代价是整个设置面板一起消失。
 * - **按 Tab**：焦点在 radix 浮层里，而 focus trap 的判据是「焦点不在我的 root
 *   里就拉回来」，于是每一次 Tab 都被 `preventDefault()` 吞掉，radix 浮层里的
 *   按钮**键盘完全不可达**。
 *
 * 这两个毛病不是确认框带来的：AccountDialog / RuleDialog / ChannelDialog 早就是
 * 从设置面板里弹出的 radix 浮层。`window.confirm` 阻塞 JS 线程、按键根本到不了
 * 页面，反而把这件事在删除这条路径上盖住了；换成真正的 DOM 浮层之后它才显形。
 *
 * ── 判据为什么是 data-state ─────────────────────────────────────────────────
 *
 * 手写的那三个浮层根元素也写着 `role="dialog"`，但 `data-state` 是 radix 自己
 * 打上去的属性，它们没有——所以这个选择器不会匹配到调用方自身。
 * 实测（jsdom 探针）确认框打开时恰好匹配 1 个，就是它自己那层。
 */
export function modalLayerOpen(): boolean {
  return (
    document.querySelector(
      '[role="dialog"][data-state="open"],[role="alertdialog"][data-state="open"]',
    ) != null
  )
}

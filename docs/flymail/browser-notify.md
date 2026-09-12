# 浏览器端新邮件提醒

> 完成：2026-09-12
> 范围：**标签页开着时**的提醒。页面完全关闭也能收到的 Web Push（Service Worker + VAPID）
> 仍在 M16 里排队，没有做。

## 为什么不跟着 `new_mail` 走

后端有两条事件路径，闸门只在其中一条上：

| 事件 | 语义 | 闸门 |
|---|---|---|
| `new_mail`（SSE，既有） | 「有变化，去重新拉」 | 无。基线导入、archive / junk 一律发 |
| 通知 emit（站内 + 外发，既有） | 「值得打扰用户的一件事」 | 文件夹类型 + 非基线未读 + 跨文件夹去重 |

拿 `new_mail` 弹浏览器通知，用户**首次添加账户导入几千封历史邮件时就会被弹窗淹没**，
垃圾箱里的邮件也会来打扰。而通知那一侧不仅闸门齐全，标题与正文也已经拼好了
（单封带发件人与主题，多封聚合成「收到 N 封新邮件」）。

所以这次是给通知 emit **加了一个 SSE 观察者**，而不是扩展 `new_mail` 的负载：

```
emit(eventType, accountID, messageID, title, body)
  ├─ baseEmit  → 站内通知落库 + 外发渠道（Telegram / 飞书 / webhook）
  ├─ emitHook  → 桌面端（Wails）弹系统原生通知      ← 既有
  └─ hub.Publish → SSE {"type":"notify",...}        ← 本次新增
```

`app.go` 里 `hub := sse.NewHub()` 因此要提到 `emit` 定义之前。

事件形状（`notifyStreamEvent`，与前端 `NotifyEvent` 一一对应）：

```json
{"type":"notify","event":"mail_new","account_id":1,"message_id":10,
 "title":"新邮件 · 王五","body":"SSE-notify-check"}
```

`message_id` 仅单封时非 0，点击通知据此直达那封信。

## 四层提醒，各自的触发条件

| 层 | 需要权限 | 何时给 | 默认 |
|---|---|---|---|
| 标签页标题 + 站点图标角标 | 否 | 未读数 > 0 就一直在 | **开** |
| 读屏播报（live region） | 否 | 收到 `notify` 就播报 | 始终 |
| 桌面通知（Notification API） | 是 | `notify` **且标签页不可见** | 关 |
| 提示音（Web Audio 合成） | 否 | 同上 | 关 |

几个判断的理由：

- **只在标签页不可见时弹**（`document.visibilityState === 'hidden'`）：页面就在眼前时列表
  已经自己刷新了，再弹一个系统通知只是噪音。Gmail / Slack 也是这个判据。
- **播报不跟随桌面通知开关**：它不弹窗、不出声，没有理由被那个开关关掉。
  视觉用户看得见未读徽标跳变，读屏用户此前对新邮件到达完全无感知（原 ui-audit 第 3 条）。
- **桌面通知默认关**：它要权限。页面一加载就弹权限框，多数浏览器直接拒绝或折叠，
  而且**一旦被拒就再也问不了**（`denied` 是粘住的）。所以设置页里的开关本身
  就是那次用户点击——权限只能由它来求。
- **拿不到权限就不要把开关打开**：显示成「已开启」而实际弹不出来，比明确告诉用户
  被浏览器拦了更糟。被拒 / 不支持时设置页会直接说出来。
- **按账户分通知 tag**：同一账户的连续来信**替换**上一条而不是堆成一摞。
  一次同步带回十几封时，用户要的是「有新邮件」这一个提示，不是十几个弹窗。

## 实现位置

| 文件 | 职责 |
|---|---|
| `internal/app/app.go` | `notifyStreamEvent` + emit 里 `hub.Publish` |
| `lib/types.ts` | `SyncEvent` / `NotifyEvent` 联合类型 |
| `lib/notify-prefs.ts` | 三个开关，存 localStorage（按浏览器成立，不进后端） |
| `lib/browser-notify.ts` | 权限、弹通知、Web Audio 合成提示音 |
| `hooks/useRealtimeSync.ts` | 两类事件分流 |
| `hooks/useUnreadBadge.ts` | 标题 + canvas 画图标角标 |
| `components/settings/BrowserNotifySection.tsx` | 设置 → 通知 → 这台设备上的提醒 |

几个实现上的选择：

- **偏好存 localStorage 而不是后端**：通知权限是浏览器授予当前源的，
  在公司电脑上开了声音不等于手机上也想要。
- **提示音用 Web Audio 合成**（两声上行小三度，音量 0.06）：省掉一个二进制资源
  与它的加载失败分支，也免了「点开邮件才发现音频 404」这种只在生产暴露的问题。
  设置页给了「试听」——提示音只在标签页不可见时才响，没有它用户根本没机会
  知道自己开的是什么声音。
- **图标角标用 canvas 现画**：省掉「准备两套 ico」和「未读数变化时换哪一张」。
  画不出来（隐私模式 / canvas 被禁）就只改标题，不让整页崩掉。
- `index.html` 的 favicon 换成内联 SVG：首屏就不再是 Vite 脚手架的默认图，
  也少一次请求；有未读时再被 canvas 版换掉，两处配色字形一致（原 ui-audit 第 27 条）。

## 验证

- 单测 16 项：`useRealtimeSync.test.tsx`（6）、`useUnreadBadge.test.tsx`（5）、
  `notify-prefs.test.ts`（5），全部做过正向验证（回退修复即失败）
- 其中最要紧的一条是 **「new_mail 只刷新缓存，绝不弹通知」**——它钉的是整个设计的地基
- 真实部署下端到端验证：容器里投递一封信，SSE 流上确实先后收到
  `new_mail` 与带发件人/主题/message_id 的 `notify`

## 代码审查抓出的两条

**1. 点击通知跳转绕过了已有的完整实现（高）。** 第一版把 `onOpenMessage` 接到了
`selectMessage`，而后者只写 `message` 参数。同一文件里早就有 `openNotification` 做完整版：
拉详情取 `account_id`/`folder_id` 写回 URL、切回邮件视图、清搜索、删 `agg`，
**会话视图下写 `thread` 而不是 `message`**，邮件已删除时回退到收件箱。
那段代码里甚至写着一行注释：「会话视图的第三栏只认 thread 参数：只写 message 的话点通知
会跳到一片空白。」——同一条产品路径上解决过一次的问题，在新入口上又犯了一遍。
→ 抽出 `openMailById(messageId)`，站内通知与浏览器通知共用。

**2. `ctx.roundRect` 没有兜底，会把整个应用打进 ErrorBoundary（中）。**
`roundRect` 要 Chrome 99 / Safari 16.4 / Firefox 112 才有，更老的浏览器上它是 `undefined`。
`drawFavicon` 在 effect 里同步调用，抛出来会一路冒到 React，被本轮新加的最外层
ErrorBoundary 接住——**整个应用因为一枚 favicon 变成错误页**。
模块注释里写的「画不出来就只改标题」意图是对的，只是没实现到位；
而 jsdom 的 `getContext` 返回 null，canvas 那段测试一行都没执行过，覆盖不到。
→ 整个绘制包 try/catch 返回 null。

**3. 提示音会按标签页数量叠加响（中）。** 通知本身有 `tag` 管着——同源多标签弹出的
会互相替换，桌面上只看到一条——**但声音没有 tag 这回事**。开着两个 FlyMail 窗口
然后去干别的，同一批新邮件会听到两声叠在一起。
→ `claimChime()` 用 localStorage 里的时间戳抢一次（3 秒窗口）。
这是「减少重复」不是「严格互斥」：读-改-写不是原子的，两个标签恰好同时读到旧值时
仍会各响一声；SSE 推送到各标签有微小时差，实际撞上的概率很低，
而为一声提示音上真正的锁不值得。设置页的「试听」不走这道闸——那是用户主动要的。

## 已知不足

**桌面通知的文案是后端拼的中文常量**（`manager.go` 里的 `"新邮件 · " + from` 与
`fmt.Sprintf("收到 %d 封新邮件", n)`），切到英文界面时桌面通知仍然是中文。

这不是本次引入的：站内通知列表与 IM 外发一直如此。但桌面通知是个新出口，
这个不一致到这里才第一次显眼。**要修的话得动到通知事件的形状**——
把 title/body 换成结构化字段（事件类型 + 发件人 + 主题 + 计数），由各个消费端自己组装，
站内通知、IM 渠道、桌面通知三处一起改。不是一个顺手能做的改动，单独排。

**需人工确认**（jsdom 测不了的部分）：真实浏览器里授权流程的观感；
系统通知的实际样式与点击跳转；提示音音量在不同设备上是否合适；
canvas 角标在高分屏与深色标签栏下是否清晰。

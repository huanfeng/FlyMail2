# M10 会话线程

> 状态：已完成（2026-09-07）
> 路线图条目：[`roadmap.md`](roadmap.md) M10

## 问题

`Message` 模型早就有 `MessageID / InReplyTo / References / ThreadID` 四列（`thread_id` 带索引），但：

- `thread_id` 从未被写入——全库没有任何代码给它赋值；
- `toMessage` 只落了 `MessageID`，`in_reply_to` / `references_hdr` 两列始终为空；
- 更上游的 core `types.ParsedEmail` 根本没有 In-Reply-To / References 字段，IMAP 只抓 ENVELOPE，
  而 ENVELOPE 只带 In-Reply-To，References 必须额外 `BODY.PEEK[HEADER.FIELDS (References)]`。

所以「核实生成逻辑」的结论是：没有生成逻辑，要从协议层补起。

## 决策

### 线程归属（写入时）

线程 id 形如 `"{account_id}:{root message-id}"`，按账户隔离——两个账户收到同一条讨论不合并成一行，
否则会话级移动会撞上 `ErrCrossAccountMove`。

每批邮件 upsert 之后（`fetchRangeBatched` / `fetchSeqRangeBatched` / `StoreFetched`）跑一次
`assignThreads`：

1. **正向**：拿 References（从后往前）+ In-Reply-To + 自己的 Message-ID 去查同账户里已有 `thread_id`
   的邮件，命中即沿用。带上自己的 Message-ID 是为了 Gmail 副本（INBOX 与「所有邮件」各一行）
   落到同一线程。
2. **反向**：查同账户里 `in_reply_to = 自己的 Message-ID` 的邮件——回复比原信先入库（Sent 先于 INBOX
   同步）时，原信要认领已存在的线程。`in_reply_to` 为此加了索引。
3. 正向、反向各自命中且不同 → 把反向那条线程整体并入正向的（一条 UPDATE）。
4. 都没命中：已有 `thread_id` 的保留（重复 upsert 不把已归并的拆开），否则用自己的 Message-ID
   （没有 Message-ID 的用 `u{folder}-{uid}`）开新线程。

命中多条要合并时，目标取「最早成员所在」的那条（IN + Pluck 没有顺序保证，不能拿第一个）；
整库重建也沿用集合里最早成员已有的 id、没有才用它的 key——两条规则一致，重建不会大面积改名，
前端挂在 thread_id 上的展开态 / 选中 / 游标才不会每次重建后全部失效。

不做主题匹配：没有索引可用，每封一次 LIKE 扫表在同步路径上不可接受；而现代客户端的回复几乎
都带 In-Reply-To。

### 整库重建

`RebuildThreads`（`POST /threads/rebuild`，启动时若存在 `thread_id = ''` 的行也会自动跑一次；
与在线归属经进程内读写锁互斥——重建是「读全表快照 → 内存计算 → 事务回写」，快照之后入库并被在线归属
挂到某线程的新邮件，会因兄弟行被改名而脱离线程且不会自愈）
按账户把全部邮件读进内存、按日期升序重放上面的规则，**外加主题兜底**：主题带 Re:/回复:/Fw:/转发:
等前缀、归一化后与某条既有线程最新一封相同、且时间差在 90 天内 → 并入。兜底只在这里做，
因为老库里的行没有头字段，除此之外无法归并；也正因为它只看主题，才要求必须带回复前缀并限时。

### 列表语义

- **出现条件**：会话在某个范围（文件夹 / 聚合视图 / 搜索，叠加未读/星标/附件筛选）里出现，
  当且仅当至少一封成员命中该范围。
- **排序与游标**：按范围内最新一封的日期降序、同日期按 thread_id 降序，游标 `(before_date, before_thread)`。
  每页都在整个范围上分组再用 HAVING 过滤——**不能**用 `date <= before_date` 预筛：一条会话最新一封在游标之上、
  其余成员在游标之下时，预筛砍掉最新那封后重算的 MAX 落到游标下方，会话会在下一页重复出现（审查时用 4 行数据复现）。
- **汇总口径**：封数 / 未读数 / 星标 / 附件 / 参与者按**账户内整条会话**统计（跨文件夹，Gmail 副本去重），
  这样「收件箱 3 封 + 已发送 2 封」显示 5 封，与展开后看到的一致。
- 主题 / 摘要 / 日期取范围内最新一封。

### 会话级操作

| 操作 | 作用范围 |
|---|---|
| 已读 / 未读、加星 / 去星 | 会话全部成员 |
| 删除、移动（文件夹视图，带 `in_folder_id`） | 只作用于该文件夹内的成员 |
| 删除、移动（聚合 / 搜索视图，不带 `in_folder_id`） | 排除 sent / drafts 文件夹里的成员 |

排除已发送的理由：IMAP MOVE 会把自己的回复从 Sent 挪走，Gmail 的「移动会话」也不会动 Sent 副本。
解析出成员 id 后全部复用既有的 `Batch*`（本地即时生效 + 回写队列）。

## 接口

三个列表接口与单封版一一对应，参数相同，只是分页游标从 `before_id` 换成 `before_thread`：

```
GET /folders/:fid/threads?before_date=&before_thread=&limit=&seen=&flagged=&has_attachment=
GET /aggregate/threads?view=inbox|unread|starred&before_date=&before_thread=&limit=&...
GET /search/threads?q=&before_date=&before_thread=&limit=&...
→ { threads: ThreadListItem[], next_cursor: {before_date, before_thread} | null, total?: number }
```

`total` 只在首页返回（会话数不能从 folders 表现成拿到，因此三个接口首页都算）。
`before_date` 非空但解析失败返回 400（静默退回首页会让拿到坏游标的前端无限重复加载第一页）。
最后一页正好填满时不再给 `next_cursor`（分组时多取一行判断）。

`POST /threads/batch/read|flag|delete` 接受混入多个账户的 thread_ids（成员按账户 → 文件夹分组各自执行）；
只有 `move` 要求全部成员与目标文件夹同账户，否则 400。

```ts
interface ThreadListItem {
  thread_id: string
  account_id: number
  count: number            // 账户内整条会话的封数（去重）
  unread: number
  flagged: boolean         // 任一成员星标
  has_attachment: boolean  // 任一成员有附件
  subject: string          // 范围内最新一封
  snippet: string
  date: string             // 范围内最新一封（RFC3339）
  latest_id: number        // 范围内最新一封的 id
  latest_folder_id: number
  participants: { name: string; email: string }[]  // 按首次出现顺序去重（按邮箱），最多 8 个
}
```

```
GET  /threads/messages?thread_id=&limit=   → { messages: MessageListItem[] }   // 日期升序，账户内跨文件夹，去重；limit 默认与上限 500
POST /threads/batch/read   { thread_ids: string[], read: boolean }
POST /threads/batch/flag   { thread_ids: string[], flagged: boolean }
POST /threads/batch/delete { thread_ids: string[], in_folder_id?: number }
POST /threads/batch/move   { thread_ids: string[], folder_id: number, in_folder_id?: number }
POST /threads/rebuild      → { ok: true, threads: number }
```

`MessageListItem` 已含 `folder_id`，前端用 folders 查询映射出「已发送」等文件夹标签。
`GET /messages/:id` 的详情新增 `thread_id`：通知跳转 / 深链只有单封 id 时，前端据此在会话视图下定位所属会话。

## 前端

- 偏好 `conversationView`（localStorage，默认开）在设置 → 邮件里切换；关闭即回到原单封列表，所有既有能力不变。
- 列表会话行：参与者（去重、最多 3 人展示 + 「等 N 人」）、封数徽标（>1 时显示）、未读加粗。
- Reader 手风琴：`GET /threads/messages` 拿成员，默认展开范围内最新一封与所有未读，其余折叠成一行头部；
  展开一封时才请求 `/messages/:id` 并标已读。
- 引用折叠：文本正文里的 `>` 引用块、HTML 里的 `<blockquote>` / Gmail `gmail_quote` 折叠为「显示引用内容」。
  **这一项在单封视图下同样生效**：它是通用的阅读体验改进，与会话视图无关，因此不受 `conversationView` 开关影响。
  实现见 `lib/quote-fold.ts`——HTML 不切字符串，只给引用容器打 `data-fm-quote` 标记，折叠时往 iframe 注入一条隐藏样式；
  引用容器之前没有任何可见内容时（整封转发）不折，否则折完是一片空白。
- 会话级操作走 `/threads/batch/*`；文件夹视图带 `in_folder_id`。

## 实现细节与取舍

### 协议层（core）

- `types.ParsedEmail` 新增 `InReplyTo` / `References`（已去尖括号；References 空格分隔、保持原序）。
- 元数据抓取（`FetchBody=false`）附带 `BODY.PEEK[HEADER.FIELDS (References In-Reply-To)]`，
  响应区段用 `net/textproto` 解析；ENVELOPE 自带的 In-Reply-To 优先。整封抓取时由 parser 从头里填，
  不受 `FallbackHeaders` 控制。

  > **2026-09-16 已改**：元数据抓取不再要 ENVELOPE，改取整个 `BODY.PEEK[HEADER]`，
  > 信封字段与线程头统一由 `parser.ParseHeaders` 解析；`FallbackHeaders` 选项已删除。
  > 起因是 QQ 的 ENVELOPE 少字段会打断整条连接，详见下面「更正」。
- `parser.MessageIDs` 提取 id 列表：有尖括号只认尖括号里的（旧 Outlook 会在 In-Reply-To 里夹说明文字），
  没有尖括号才按空白/逗号切、只留含 `@` 的片段。
- mail2im 同样消费 `ParsedEmail`，新增字段对它是无害的多余数据。

### 兜底：正文落库时补线程头

**GreenMail 2.1.8 对 `HEADER.FIELDS` 一律返回空内容，ENVELOPE 里也不带 In-Reply-To**（实测；
Gmail 真机验证正常，497 封新同步邮件都带上了头，最大线程是一个 60 封的 GitHub PR 讨论）。

> **⚠ 2026-09-16 更正**：上面那句「GreenMail 不支持 HEADER.FIELDS」是错的，真正的原因是
> **go-imap 把字段名写成带引号的字符串**（`HEADER.FIELDS ("References" "In-Reply-To")`），
> 而有的服务器只认不加引号的原子写法。实测同一封邮件：
>
> | 服务器 | 不加引号 | 加引号 | 整个 `HEADER` |
> |---|---|---|---|
> | GreenMail | 157 字节 | **0** | 362 字节 |
> | QQ | 326 字节 | **2** | 1926 字节 |
> | Gmail / 163 | 正常 | 正常 | 正常 |
>
> 引号是 go-imap 编码器加的，调不掉。所以元数据抓取改成取整个 `BODY.PEEK[HEADER]`。
> 顺带的后果：GreenMail 上线程头现在在**元数据阶段**就拿得到，会话同步完即归并，
> `internal/e2e/thread_test.go` 的期望已相应改为「同步后就是 2 条」。
为了对这类服务器也能用，`StoreParsedBody` 在整封解析出了头时按列独立回填行上还缺的那一列
（ENVELOPE 给了 In-Reply-To 但 HEADER.FIELDS 回空的服务器只缺 References）并重新归属——
正文预取或打开邮件后会话就会归并。GreenMail E2E（`internal/e2e/thread_test.go`）覆盖的正是
这条路径。

### 会话列表查询

三步：
1. **分组**：在范围上 `GROUP BY thread_id`，只取 `thread_id` / `MAX(date || '#' || printf('%012d', id))` / 裸列 `id`。
   复合键让「最新一封」按 (date, id) 决定——同一秒到达的多封（自动通知、GreenMail 的 INTERNALDATE 只到秒）
   单看 `MAX(date)` 平局时裸列会落到任意一行，列表就会显示原信而不是最新回复。
   裸列跟随 MAX 命中行是 SQLite 特性（[bare columns in aggregate](https://sqlite.org/lang_select.html#bareagg)），换库要改窗口函数。
2. **回表**：按这 50 个 id 取展示列。分组阶段带上 subject/snippet 会逼着 SQLite 逐行回表，实测慢 3～4 倍。
3. **汇总**：一条查询取回这 50 条会话在账户内的全部成员（去重副本），Go 里聚合封数 / 未读 / 星标 / 附件 / 参与者。

`last_date` 在 SQL 里从复合键 `substr` 截回，ORDER BY 与游标 HAVING 都用 `(last_date, thread_id)`——
两者必须同源，否则同秒平局跨页时会静默漏行（RFC 5322 的 Date 只有秒精度，同秒并不罕见）。

**副本去重**：搜索范围无条件去重（副本命中时 `latest_id` 否则可能落到「所有邮件」那份，点开的文件夹
上下文会变；与单封搜索同口径）；聚合视图里收件箱按构造没有副本，不去重（相关子查询会让它从 5ms 涨到 38ms），
unread / starred 视图与按已读/星标筛选时去重（Gmail 标签副本与收件箱副本的已读状态会短暂不一致，
不去重时「全部未读」会冒出 unread=0 的会话）；单文件夹范围没有副本。

### 索引与统计

| 索引 | 用途 |
|---|---|
| `idx_msg_folder_thread (folder_id, thread_id, date)` | 分组阶段的覆盖索引（rowid 天然在索引里），文件夹会话页 20ms → 4ms |
| `idx_msg_unread` / `idx_msg_flagged`（同列序，`WHERE seen = 0` / `WHERE flagged = 1` 部分索引） | unread / starred 视图与 `is:unread` 搜索，40ms → 1ms |
| `idx_messages_in_reply_to` | 归属时反向查「谁回复了我」 |

部分索引对绑定参数同样生效：SQLite 会按绑定后的值重新规划，`seen = ?` 绑 0 时选中 `idx_msg_unread`、
绑 1 时走全表（真实库 `EXPLAIN QUERY PLAN` 验证，SQLite 3.53）。查询侧照常用参数，不必拼字面量。

`database.Migrate` 末尾跑一次 `PRAGMA analysis_limit = 1000; ANALYZE`（13.9k 封 25ms，采样上限让开销
不随库线性增长）：没有 `sqlite_stat1` 时规划器会把「JOIN folders WHERE type = 'inbox'」走成先扫 messages
再探 folders，聚合收件箱 40ms；有统计后 5ms。

### 性能实测（真机 13,908 封 / 9,065 条线程，SQLite 3.53，5 次取值）

| 查询 | 耗时 |
|---|---|
| 启动整库线程重建（老库首次） | 0.8s（12.8k 封）；再次重建 80ms |
| 文件夹会话首页 / 第 2 页（5.6k 封的文件夹） | 7ms / 7ms |
| 文件夹会话 + 未读筛选 | 1ms |
| 聚合收件箱首页 / 第 2 页 | 7ms / 5ms |
| 聚合未读 / 星标 | 1～2ms |
| 搜索会话：`发票` / `is:unread` / `from:github`（1.8k 条命中，含副本去重） / `报告` | 3ms / 2ms / 20ms / 2ms |
| 展开一条会话（11 封） | < 1ms |

HTTP 层（含鉴权、JSON）：聚合收件箱 15～20ms，搜索 `Re`（1,795 条会话）35～40ms。
5 万封未实测；按 13.9k 线性外推最慢的 `from:github` 类搜索约 50ms，其余 ≤ 25ms。

## 测试

- `core/parser/threadhdr_test.go`、`core/imap/threadhdr_test.go`：id 列表解析、HEADER.FIELDS 区段解析（含无结尾空行、ENVELOPE 优先）。
- `message/thread_test.go`：正向/反向/分叉合并/账户隔离、Gmail 副本与重复 upsert 幂等、无 Message-ID 兜底、
  正文回填线程头、整库重建（老库主题兜底、90 天窗口、无前缀不并、缺失原信的并查集、幂等）、
  文件夹/聚合/搜索会话页（排序、整条会话汇总、参与者去重、Sent 视角、筛选、逐页游标不重不漏）、成员顺序与副本去重、HTTP 路由。
- `message/thread_perf_test.go`：`FLYMAIL_BENCH_DB=<路径>` 时在真实库副本上计时（上表数据来源）。
- `sync/thread_ops_test.go`：会话级已读/星标全员、删除/移动的文件夹视图与聚合视图范围、跨账户拒绝、未知 id 静默。
- `internal/e2e/thread_test.go`（GreenMail）：同步 → 打开正文归并 → 会话列表 / 成员 → 会话级已读回写到服务器 `\Seen`。
- 后端全量 + `sync`/`message` `-race`、GreenMail E2E 全量、core 全量均通过。

## 未做 / 已知限制

- 写入路径不做主题匹配，也不查「References 里引用了自己」的后代（要 LIKE）；两者都交给整库重建（启动时老库自动跑一次，或 `POST /threads/rebuild`）。
- 主题兜底只认 Re:/Fw:/回复:/转发: 等常见前缀，且要求 90 天内；其它语言前缀不识别。
- 163 账户的 HEADER.FIELDS 路径未验证（本次同步没有新邮件进来）；若与 GreenMail 一样回空，靠正文兜底归并。
- 会话汇总（封数/未读）含回收站里的成员：删除只是本地删行 + 服务器移到回收站，回收站那份下次同步进来后会计入。
- 5 万封规模 P95 未实测。
- 列表「最新一封」按 (date, id) 取，date 是邮件自带时区的文本字节序（与既有 ORDER BY date 同口径）。

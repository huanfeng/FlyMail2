# M9 检索重构：FTS5 全文索引 + 搜索语法 + 服务端兜底

> 创建：2026-09-07
> 对应路线图：[`roadmap.md`](roadmap.md) M9
> 状态：后端完成（含测试）；前端语法提示/高亮/服务端搜索入口同批交付

## 1. 问题

改造前 `SearchMessages` 在 `subject / from_name / from_addr / snippet / message_bodies.text_body`
五列上做 `LIKE '%q%'`，前缀通配符让所有索引失效，每次搜索都是全表 + 全正文扫描；
没有任何限定符语法；只能搜到本地已同步的内容。

## 2. 关键决策

### 2.1 分词：应用层二元切分（bigram），不用 trigram

SQLite 自带两个候选分词器，在本机驱动上实测：

| 方案 | 「发票」「张三」（两字词） | 结论 |
|---|---|---|
| `unicode61` | 整段连续汉字当一个 token，只能整句匹配 | 不可用 |
| `trigram` | 查询词必须 ≥ 3 字符，两字词命中 0 | 不可用 |

中文检索的主流恰恰是两字词，因此切分放在应用层（`flymail/backend/internal/fts`）：

- 索引：每段连续 CJK 文字展开成相邻二元组，空格隔开后交给 `unicode61`
  （`张三给李四` → `张三 三给 给李 李四`）；非 CJK 文本原样透传，unicode61 照常切词/折叠大小写。
- 查询：用户输入做同样展开，包成 **短语**（`"李四 四发"`），要求二元组顺序相邻，精度与整词匹配一致；
  英文/数字 token 做前缀匹配（`"inv"*`），输入 `inv` 能命中 `invoice`。
- 索引与查询共用同一份切分代码，两边不可能漂移。
- 已知限制：单个汉字只能命中索引里孤立的单字（正文里「票」这种单字不可搜）。

### 2.2 表形态：contentless + contentless_delete

- 不能用 `content=` 外部内容模式：它直接读主表原文分词，而主表存的不是切分后的文本。
- 不用普通 FTS 表：会把正文再存一份。
- 选 `content='' , contentless_delete=1`：只存倒排索引；按 rowid 删/改需要 SQLite ≥ 3.43。

因此把 `github.com/glebarez/go-sqlite` 从 v1.21.2 升到 v1.23.0（`modernc.org/sqlite` v1.23.1 → v1.55.0，
SQLite 3.41.2 → 3.53.3）。core / mail2im 的 go.mod 也显式升到同一版本（不只靠 go.work 的最小版本选择，
单独 clone 或 `GOWORK=off` 构建时也一致），全部测试通过。

### 2.3 索引维护：触发器 + Go 注册的 SQL 函数

`internal/fts` 在 `init()` 里把 `Tokenize` 注册为 SQLite 标量函数 `fts_tokens(text)`
（`glebarez/go-sqlite.RegisterDeterministicScalarFunction`，对之后新开的连接生效；GORM 惰性建连，
只要包被 import 就必然先注册）。

`modules/email/message/fts.go` 建虚表 `messages_fts(subject, from_name, from_addr, recipients, body)`
（rowid = messages.id）和 6 个触发器：

| 触发器 | 动作 |
|---|---|
| `messages` AFTER INSERT | 先删再插索引行（contentless 表对重复 rowid 不报错而是留双份，重建期间的并发写会撞上） |
| `messages` AFTER UPDATE OF subject/from/to/cc，且 `WHEN` 值确有变化 | 删 + 重插（同步 upsert 对未变化的行不触发） |
| `messages` AFTER DELETE | 删索引行 |
| `message_bodies` AFTER INSERT / UPDATE OF text_body,html_body / DELETE | 删 + 重插对应索引行 |

`snippet` 不入索引：它由正文派生（`MarkBodySynced` 时写入），body 列已覆盖同样内容，单独索引只会让每次正文落库
多重建一次整行。

**运维注意**：触发器引用的 `fts_tokens` / `fts_strip_html` 是进程内注册的 SQL 函数。用 `sqlite3` CLI 或任何没有
import `internal/fts` 的程序对 `messages` / `message_bodies` 做写操作会报 `no such function`。手工修库时先
DROP 六个触发器，改完由应用启动重建并 `/search/reindex`。

`body` 列优先取 `text_body`；纯 HTML 邮件（text_body 为空）退回 `fts_strip_html(html_body)`（同样是 Go 注册的
SQL 函数：去 script/style、剥标签、解实体；块标签换成空格避免 `<td>发票</td><td>报销</td>` 粘成不存在的词）——
改造前这类邮件正文完全搜不到。

任何 Go 侧写入路径（同步 upsert、批量删除、UIDVALIDITY 重建、正文落库）都自动跟上，
不依赖「记得顺手更新 FTS」。`recipients` 列直接切分 `to_json || cc_json`：unicode61 会按标点把 JSON
拆成 name/email token，`to:` 限定符不需要先解析 JSON。

### 2.4 版本与回填

`PRAGMA user_version` 记录 FTS 结构版本（当前 1；本项目只有 FTS 用它）。`database.Migrate` 在 AutoMigrate 之后调
`message.EnsureFTS`：版本落后时**先 DROP 全部触发器与虚表**（`CREATE ... IF NOT EXISTS` 对已存在对象是跳过的，
不 DROP 新定义不会生效），再建表/触发器并 `RebuildFTS`（`delete-all` → 按 id 区间每批 2000 回填 → `optimize`），
进度写日志，完成后才写版本号（中途崩溃下次重来）。改列、改触发器或改切分策略时把 `ftsSchemaVersion` +1 即可。
`RebuildFTS` 有进程内互斥，连点两次重建不会交错。

运维入口：`POST /search/reindex`（设置 → 邮件 → 重建搜索索引）。

## 3. 搜索语法（`internal/fts/query.go`）

Gmail 风格，限定符大小写不敏感，取值可用双引号（含中文弯引号）包裹：

| 语法 | 落到 |
|---|---|
| 自由词、`"短语"` | FTS 全列 MATCH（多词 AND；引号内英文按相邻短语） |
| `from:` `to:` `subject:` | FTS 列过滤 `{from_name from_addr}` / `recipients` / `subject` |
| `has:attachment` | `messages.has_attachment = 1` |
| `is:unread` / `is:read` / `is:starred` / `is:unstarred` | `seen` / `flagged` |
| `before:` / `after:`（YYYY-MM-DD / YYYY-MM / YYYY，`/` 亦可） | `date <` / `date >=`，按**邮件自带时区的日期**比较（date 列是带偏移的文本，字节序比较；与 IMAP SENTBEFORE/SENTSINCE 口径一致，跨时区邮件在日期边界可能差一天）；before 不含当天 |
| `in:` | `folder_id IN (type = ? OR display_name/path LIKE)` |
| `account:` | `account_id IN (email/name LIKE)` |

非法取值退化为普通文本（`has:banana` 当词搜）；未知 `x:y`（如 `Re:`、URL）原样当文本；
空取值丢弃。带引号的限定符取值保留短语语义（`subject:"hello world"` 要求两词相邻）。解析后没有任何有效条件 → 直接返回空结果，不会退化成列出全部。

MATCH 表达式各部分用显式 `AND` 连接（FTS5 只对相邻裸短语做隐式 AND，括号/列过滤之间不写 AND 会报语法错误）。
所有 token 都加双引号，用户输入的 `OR`/`NOT`/括号不会被当成语法。

仓储侧：`messages.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)` + 结构化 WHERE，
与既有 `Filter`（筛选 chips）、`dedupeSameMessage`、(date,id) keyset 分页正交叠加。

## 4. 服务端兜底（`modules/email/sync/search_remote.go`）

`POST /search/remote {q}`：把同一查询翻译成 IMAP SEARCH（自由词 → TEXT，from/to/subject → HEADER，
is: → \Seen/\Flagged 正反条件，before/after → SENTBEFORE/SENTSINCE；`in:` 用来选文件夹，`account:` 用来选账户
（名称/邮箱子串，与本地口径一致）；`has:` 无 IMAP 对应，靠补抓后的本地搜索再筛）。
**翻译后没有任何 IMAP 条件时直接返回空**——go-imap 会把空条件编码成 `SEARCH ALL`，等于把每个文件夹整个搜一遍再各补抓 200 封。
对匹配的启用账户**并行**、账户内逐可选文件夹串行执行，全部走 runner 前台队列（`ForegroundOp`，与打开邮件同级，不新建连接）。
进度计数经互斥锁共享：`ForegroundOp` 超时先行返回时任务仍在 runner 里继续跑，外层只取一次快照。
本地「哪些 UID 搜不到」的查询按 900 个一批分块（SQLite 绑定变量上限 32766）。

命中中「本地搜不到」的（无行，或有行但 `body_synced=false`）连正文一起补抓入库
（每文件夹最多 200 封，取最新），之后前端重跑本地搜索即可看到——不单独维护一套服务端结果列表。
总时限 90s；单账户失败记入 `errors`，不让整次搜索失败。

`Searcher`（UIDSearch）与 `EnabledAccountLister`（ListEnabledIDs）做成 Session / AccountConfigProvider 的
**可选接口**，真实实现都满足，测试假实现不必全部改。

## 5. 接口变更汇总

| 接口 | 变化 |
|---|---|
| `GET /search/messages?q=` | 不变；q 支持语法；`total` 语义不变 |
| `POST /search/reindex` | 新增，重建索引 |
| `POST /search/remote` | 新增，`{q}` → `{fetched, matched, accounts, folders, errors?}` |

## 6. 测试

- `internal/fts`：切分（中/日/韩/混合）、MatchExpr（前缀/短语/语法字符清洗）、解析器（全部限定符、3 类非法输入、空查询、大小写/全角空格/弯引号）。
- `modules/email/message/fts_test.go`：两字词、短语顺序、正文经触发器入索引、主题更新/删除跟随、全部限定符、计数与列表一致、重建与老库回填、空查询。
- `modules/email/sync/search_remote_*_test.go`：条件翻译、文件夹筛选、补抓入库与二次搜索不重复抓、`in:` 只搜匹配文件夹、`account:` 只搜匹配账户、仅本地限定符时不向服务器发 SEARCH ALL、空查询不碰服务器；sync 包 `-race` 通过。
- 独立审查（opus code-reviewer）发现并已修复：空条件 SEARCH ALL、IN 绑定变量上限、超时路径数据竞争、`account:` 远程未生效、版本递增不重建结构、重复 rowid、snippet 双重切分、限定符丢短语语义。

- 本机 GreenMail E2E 套件（`./e2e.ps1`）在驱动升级 + 触发器下全部通过。

## 7. 实测（开发库副本，12,788 封）

启动时自动回填 12,788 封约 0.5s。搜索耗时（仓储层，含去重与计数）：

| 查询 | 命中 | 修复前 | 修复后 |
|---|---|---|---|
| `after:2026-01-01`（纯结构化） | 833 | 计数 4.6s / 列表 446ms | 8ms / 1ms |
| `通知` | 314 | 654ms | 2.7ms |
| `invoice` | 36 | 365ms | 0.5ms |
| `会议` | 6 | 18ms | 0.5ms |

"修复前"的瓶颈不是 FTS（356 条命中 0ms），而是既有的 `dedupeSameMessage` 相关子查询：
SQLite 只有 `account_id` 单列索引可选，每一候选行都把该账户全部邮件扫一遍（~3ms/行）。
M9 顺手在 `Message` 上加了复合索引 `idx_msg_dedupe (account_id, message_id)`（AutoMigrate 在老库自动补建），
聚合列表与计数同样受益。

真机验证服务端兜底（两个真实账户、37 个可选文件夹）：`结算` 命中 33、补抓 26 封、约 18s，第二次 `fetched=0`。
首次运行时两个账户并行落库触发了 `SQLITE_BUSY`——全仓此前从未设置 `busy_timeout`（默认 0，第二个写者立刻失败），
多账户同步 worker / 回写队列本来也有同样风险。已在 `core/database.OpenSQLite` 的 DSN 上统一加 `_pragma=busy_timeout(5000)`
（core 与 mail2im 共用，纯等待语义，无行为变化）。

## 8. 未做 / 后续

- 5 万封库的 P95 基准未实测（当前最大样本 1.3 万封，见上表；按命中数线性外推仍远低于 100ms）。
- 单字中文查询的已知限制（见 2.1）。
- 命中高亮在前端按查询词做子串标记，未用 FTS `highlight()`（contentless 表不支持）。
- 未开 WAL（读写仍互斥，只是不再立刻失败）；运行时重建索引按 2000 封分批提交，批间让出写锁。

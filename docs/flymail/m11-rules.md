# M11 规则引擎 + 黑名单

> 状态：已完成（2026-09-07）
> 路线图条目：[`roadmap.md`](roadmap.md) M11

## 目标

邮件入库后自动执行用户定义的规则；黑名单前置短路。动作全部走既有的「本地先改 + 回写队列」路径，
重复入库不重复执行。

## 执行时序（关键设计点）

规则跑在账户 runner goroutine 里的 `Manager.syncFolder`（`sync/manager.go`），单账户天然串行：

```
IncrementalSync ──► NewMail{AfterID, Baseline, Count}
      │
      ▼
prefetchNewBodies          新邮件正文 + 附件元数据落库（body_sync_mode=new 默认覆盖收件箱）
      │
      ▼
rules.Apply(account, folder, 本轮新邮件)      ← 仅 folder.type == "inbox" 且 !Baseline
      │   1. 黑名单：命中 → 移入 junk（无则 trash）→ 记执行日志 → 不再看规则
      │   2. 规则按 priority 升序逐条 Evaluate；命中累计动作；stop_processing 则跳出
      │   3. 动作按类型合并执行：先 已读/星标（BatchSetRead/BatchSetFlagged），
      │      再 移动/删除（BatchMove/BatchDelete，删除优先于移动）——
      │      移动/删除会删本地行，所以放最后；每 (文件夹, 动作) 合并成一条回写 op
      │   4. 通知动作：同一规则同轮命中 ≤ 3 封逐封发，更多则合并成一条
      ▼
重算 folder total/unread（移动/删除后行数已变）→ UpdateSyncState
      │
      ▼
SSE new_mail ──► 通知闸门（nm.UnseenTotal / Unseen 重算，被黑名单/移走的不再计入）
```

- **为什么在正文预取之后**：正文包含 / 附件名 条件需要正文；默认 `new` 档下新邮件正文在同一轮就到。
  预取失败或超过单轮 50 封上限的部分，正文条件按「未知 = 不命中」处理，界面明示。
- **为什么只在收件箱**：Gmail 一封邮件在 INBOX 与各标签文件夹各一行，逐文件夹跑会重复动作；
  主流客户端的过滤器也只在到达时执行一次。custom / archive 等文件夹的新增不触发规则。
- **为什么跳过基线导入**：首次同步与 UIDVALIDITY 重建会把整个文件夹当新邮件，对历史邮件执行动作不是用户想要的。
  顺带修正了基线判定：原来是「同步前本地为空」，一个同步时还是空的收件箱之后到的第一批邮件也会被当成基线，
  既不提醒也不跑规则（GreenMail E2E 正是这样撞上的）；现在是「本地为空**且**文件夹没有 UIDNEXT 锚点」，
  即该文件夹从未同步过。
- **幂等**：`rule_runs(account_id, message_key, rule_id)` 唯一，`message_key` 取 RFC Message-ID
  （缺则 `u{folder}-{uid}`），黑名单 `rule_id = 0`。UIDVALIDITY 重建后主键全变，但 Message-ID 不变，
  且重建本身是 Baseline 直接跳过。同一封再次入库时，上次在某条 `stop_processing` 规则停下的，这次也在
  同一条停下——否则被它挡掉的后续规则会趁机执行。执行日志保留 180 天，每天清一次。
- **回写队列**：不新增 op 类型，全部复用 `sync.Service.Batch*`（本地先改 → 按账户/文件夹合并入队 → runner 排空）。
  runner 后台队列容量 64 且满时丢弃，合并成少量 op 正是为了避免这一点。

## 数据模型

```go
type Rule struct {
    ID             uint
    Name           string
    Enabled        bool
    Priority       int     // 升序执行；前端上下箭头调整
    AccountID      uint    // 0 = 全部账户
    Match          string  // all | any
    Conditions     string  // JSON []Condition
    Actions        string  // JSON []Action
    StopProcessing bool    // 命中后不再看后续规则
}
type Condition struct{ Field, Op, Value string }
//  Field: from | to | cc | subject | body | attachment_name | has_attachment
//  Op:    contains | not_contains | equals | regex | starts_with | ends_with
//         has_attachment 只认 equals，Value 为 true/false
type Action struct{ Type, Value string }
//  Type: move（Value = 目标文件夹 path 或 display_name，按账户解析）| mark_read | star | delete | notify
type BlockEntry struct{ ID uint; Pattern string; Note string }   // 地址或域名，小写；域名不带 @
type RuleRun struct{ AccountID uint; MessageKey string; RuleID uint; Action string; CreatedAt time.Time }
```

砍掉的范围与理由：
- **任意邮件头**：库里没有完整 header，要么另落 headers 表要么现抓，本里程碑不做。
- **打标签**：core/imap 没有 CREATE，只能「移动到已有文件夹」；前端目标从各账户 folders 并集里选。
- **停止处理作为动作**：与规则级 `stop_processing` 重复，只保留开关。
- **正则**：Go regexp 是 RE2，线性时间无回溯爆炸；编译结果按 pattern 缓存，编译失败的条件视为不命中并在保存时 400。

匹配细节：
- from / to / cc 同时匹配显示名与地址（`Alice <a@x>` 形式拼接后比较），大小写不敏感（regex 除外，用户自己写 `(?i)`）。
- 黑名单：`from_addr == pattern` 或 `from_addr` 以 `@pattern` / `.pattern` 结尾（域名及其子域）。
- 条件全空的规则视为不命中（避免误配所有邮件）。
- 正文未落库时，`body`、`attachment_name`、**`has_attachment`** 三种条件都按「未知 = 不命中」处理
  （`has_attachment` 列由正文落库时回填，正文没到之前恒为 false，既不能判有也不能判无），试运行把这类邮件计入 `without_body`。
- `attachment_name not_contains X` = 没有任何附件名含 X（任一含即为假，无附件时为真）。
- 一条规则只能有一个 `move`（编译期拒绝）；同一封被多条规则要求移动时**优先级最高的那条决定去向**，
  其余记 `move:X(skipped)`；目标解析不到记 `move:X(missing)`、就是当前文件夹记 `(same)`，都不报错。
- 限定了账户的规则保存时校验目标文件夹存在（按 display_name 或 path，大小写不敏感）；全账户规则按账户各自解析，保存时不校验。
- 黑名单处置失败只记日志，不影响同批规则动作（这批邮件下一轮不会再进引擎）。
- 本轮新邮件按 500 封分页跑完，不截断。
- Gmail 标签文件夹（custom）同步时，已被规则/黑名单处理过的邮件从本轮未读集合里剔除，不再重复提醒。

## 接口

```
GET    /rules                      → { rules: RuleDTO[] }           按 priority 升序
POST   /rules                      RuleInput → RuleDTO             校验：名称非空、条件/动作至少各一、regex 可编译
PUT    /rules/:id                  RuleInput → RuleDTO
DELETE /rules/:id
POST   /rules/reorder              { ids: number[] }               按数组顺序重写 priority
POST   /rules/test                 { rule: RuleInput, limit?: number }
                                   → { matched: MessageListItem[], scanned: number, without_body: number, truncated: boolean }
                                   只读：对规则作用账户的收件箱最近 limit（默认 200、上限 500）封求值，不产生任何副作用；
                                   matched 最多 50 条，超出置 truncated
GET    /blocklist                  → { entries: BlockEntry[] }
POST   /blocklist                  { pattern, note? } → BlockEntry   pattern 归一化为小写、去空白、域名去 @；
                                   本地账户自己的邮箱返回 400（右键点在自己发的邮件上不能把自己拉黑）；重复 409
POST   /rules/reorder              ids 含不存在的规则返回 400（多半是前端缓存过期）
DELETE /blocklist/:id
GET    /rules/runs?limit=          → { runs: RuleRunDTO[] }          最近执行日志（诊断用）
```

通知动作产生新事件类型 `mail_rule`（标题「规则命中 · 规则名」，正文为主题），需同步加进
`notify.ValidEvent` 与前端渠道订阅列表。

## 前端

- 设置 → 新分区「规则」：规则列表（启用开关、上下箭头调优先级、编辑、删除）、规则编辑框
  （名称 / 作用账户 / 全部满足·任一满足 / 条件行增删 / 动作行增删 / 命中后停止），编辑框内「试运行」按钮
  展示命中列表与「N 封无正文未参与正文条件」提示；执行日志折叠区。
- 设置 → 新分区「黑名单」：列表 + 添加（地址或域名）+ 删除。
- 邮件右键菜单 / 会话右键菜单加「屏蔽此发件人」：POST /blocklist，成功后 toast。发件人是本地账户自己时不显示该项
  （后端也会 400 兜底）；会话行取第一个非本人参与者（会话行不带最新一封的发件人，要精确需后端补 `latest_from_addr`）。
- 所有 mutation 失败都透出后端 `error` 文案；正则前端只粗筛（翻译 `(?i)` / `(?i:` / `(?P<`），以保存结果为准。
- 无拖拽库，优先级用上下箭头 + `POST /rules/reorder`。

## 测试

- `rule/engine_test.go`：Compile 校验（12 种非法输入、两个 move 拒绝）、六种运算与 all/any、地址字段纯地址匹配、
  正文未知时 body / attachment_name / has_attachment 一律未知、`attachment_name not_contains` 的真值表、黑名单归一化与子域匹配。
- `rule/service_test.go`（真实 SQLite + 记录动作的假 Actor）：黑名单入 junk、移动 + 已读、stop_processing、同一封再入库幂等、
  删除优先于移动、账户范围隔离、目标文件夹缺失记 missing、限定账户规则保存时校验目标、两条规则争夺 move 时优先级高者胜出且结果确定、
  通知逐封 / 合并、试运行只读且统计无正文、黑名单重复 409、禁止拉黑自己、陌生 id 重排 400、增删改与排序。
- `sync/rules_test.go`：基线导入不跑规则、增量同步只把收件箱新邮件交给引擎、执行后重算计数与未读集合、
  Gmail 标签文件夹里已处理过的邮件不再提醒。
- `message/incremental_test.go`：基线判定改为「本地为空且无 UIDNEXT 锚点」。
- `internal/e2e/rules_test.go`（GreenMail）：基线同步 → 建规则与黑名单 → 投递三封 → 增量同步 → 收件箱只剩普通那封、
  服务器侧归档文件夹 1 封已读 / 回收站 1 封、通知只提醒普通那封、执行日志两条、再同步不重复、试运行只读。
- 后端全量 + `rule`/`sync`/`message` `-race`、GreenMail E2E 全量、core、mail2im 构建均通过。

## 未做 / 已知限制

- 「任意邮件头」条件、「打标签」动作（需 IMAP CREATE）未做，见上文。
- 规则只在收件箱新到邮件上执行；对已有邮件只能试运行预览，不提供「立即应用到历史邮件」。
- 全账户规则的移动目标按名字在各账户内解析，某账户没有同名文件夹时该账户上跳过并记 `move:X(missing)`。
- 正文条件依赖正文预取：`new` 档单轮预取上限 50 封，超出部分的正文条件按未知处理。
- 执行日志的 action 以逗号拼接，文件夹名含逗号时前端按「逗号 + 已知动作前缀」切分。

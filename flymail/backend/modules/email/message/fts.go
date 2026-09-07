package message

import (
	"fmt"
	gosync "sync"
	"time"

	"flymail-core/logger"
	"flymail/internal/fts"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// ── 全文索引（SQLite FTS5）────────────────────────────────────────────────────
//
// messages_fts 是 messages + message_bodies 的检索索引，rowid = messages.id。
//
// 形态选择：contentless（content=''）+ contentless_delete=1。
//   - contentless：虚表只存倒排索引，不存原文——原文本来就在 messages / message_bodies 里，
//     再存一份等于正文双倍占用。
//   - 不能用 content= 外部内容模式：那种模式会直接从主表读原文做分词，
//     而中文必须先经 fts_tokens() 做二元切分（见 internal/fts），主表里的原文不是切分后的样子。
//   - contentless_delete 让 DELETE / UPDATE 按 rowid 生效（SQLite ≥ 3.43），
//     否则 contentless 表是只增不删的。
//
// 维护方式：触发器。任何写 messages / message_bodies 的路径（同步 upsert、批量删除、
// UIDVALIDITY 重建、正文落库……）都自动同步索引，Go 侧不需要记得「顺手更新 FTS」。
// 索引与主表若因 bug 漂移，运维入口 RebuildFTS 可整体重建。
//
// 版本：PRAGMA user_version 记录 FTS 结构版本（本项目只有 FTS 用它）。版本落后时
// 先 DROP 触发器与虚表再重建并回填——CREATE ... IF NOT EXISTS 对已存在的对象是跳过的，
// 不先 DROP，改列/改触发器的新定义根本不会生效。以后改列、改触发器或改切分策略，
// 把 ftsSchemaVersion +1 即可。
//
// ⚠ 触发器引用的 fts_tokens / fts_strip_html 是本进程注册的 SQL 函数：用 sqlite3 CLI
// 或其它未 import internal/fts 的程序对 messages / message_bodies 做写操作会报 no such function。
// 手工修库时先 DROP 触发器，改完启动应用让它重建并 /search/reindex。

// ftsSchemaVersion 当前 FTS 结构版本。改动虚表列、触发器或切分策略时递增，触发启动重建。
const ftsSchemaVersion = 1

// ftsColumns 与虚表定义、触发器、回填 SQL 三处共用的列清单。
// 不含 snippet：它由正文派生（MarkBodySynced 时写入），body 列已经索引了同样的内容，
// 单独索引它只会让每次正文落库多重建一次整行（snippet 的 UPDATE 又触发一遍）。
const ftsColumns = "subject, from_name, from_addr, recipients, body"

// ftsRowSelect 从主表构造一行索引内容（不含 INSERT 前缀与 WHERE）。
// recipients 把 to/cc 的 JSON 串一起切分：unicode61 会按标点把 JSON 拆成 name/email token，
// 正好满足 to: 限定符按名字或地址片段匹配的需要，不必先解析 JSON。
// body 优先用 text_body；纯 HTML 邮件没有文本部分，退回剥掉标签的 html_body，
// 否则新闻邮件这类只有 HTML 的正文永远搜不到。
// from_addr 有意不过切分函数：邮箱地址是 ASCII，unicode61 按 @ 和 . 拆开即可；
// 查询侧对非 CJK 输入也不做切分，两边一致。
const ftsRowSelect = `SELECT m.id, ` + fts.FuncName + `(m.subject), ` + fts.FuncName + `(m.from_name), m.from_addr,
	` + fts.FuncName + `(coalesce(m.to_json, '') || ' ' || coalesce(m.cc_json, '')),
	` + fts.FuncName + `(CASE WHEN coalesce(b.text_body, '') <> '' THEN b.text_body
	                          ELSE ` + fts.StripHTMLFuncName + `(coalesce(b.html_body, '')) END)
	FROM messages m LEFT JOIN message_bodies b ON b.message_id = m.id`

var ftsDDL = []string{
	`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
		` + ftsColumns + `,
		content='', contentless_delete=1, tokenize='unicode61'
	)`,

	// 新邮件入库 → 建索引行（此时正文通常还没到，body 为空；正文落库时由下面的触发器补）。
	// 先 DELETE 再 INSERT：contentless 表对重复 rowid 不报错，会把新旧两份词条都留在索引里；
	// 重建期间（delete-all 与分批回填之间）若有同步写入，就会撞上这种情况。
	`CREATE TRIGGER IF NOT EXISTS messages_fts_ai AFTER INSERT ON messages BEGIN
		DELETE FROM messages_fts WHERE rowid = new.id;
		INSERT INTO messages_fts(rowid, ` + ftsColumns + `)
		` + ftsRowSelect + ` WHERE m.id = new.id;
	END`,

	// 元数据变更 → 重建索引行。只盯与索引有关的列，seen/flagged 之类的状态回写不触发；
	// 并且用 WHEN 跳过「值没变的 UPDATE」——同步 upsert 对已存在的行也会走 ON CONFLICT UPDATE，
	// 不加这一层，每轮全量同步都会把整个文件夹的索引删了重建一遍。
	`CREATE TRIGGER IF NOT EXISTS messages_fts_au AFTER UPDATE OF subject, from_name, from_addr, to_json, cc_json ON messages
	WHEN old.subject IS NOT new.subject OR old.from_name IS NOT new.from_name OR old.from_addr IS NOT new.from_addr
		OR old.to_json IS NOT new.to_json OR old.cc_json IS NOT new.cc_json
	BEGIN
		DELETE FROM messages_fts WHERE rowid = old.id;
		INSERT INTO messages_fts(rowid, ` + ftsColumns + `)
		` + ftsRowSelect + ` WHERE m.id = new.id;
	END`,

	`CREATE TRIGGER IF NOT EXISTS messages_fts_ad AFTER DELETE ON messages BEGIN
		DELETE FROM messages_fts WHERE rowid = old.id;
	END`,

	// 正文落库 / 更新 / 删除 → 重建对应索引行（正文只影响 body 列，但 contentless 表
	// 不支持只改一列，整行删了重插）
	`CREATE TRIGGER IF NOT EXISTS message_bodies_fts_ai AFTER INSERT ON message_bodies BEGIN
		DELETE FROM messages_fts WHERE rowid = new.message_id;
		INSERT INTO messages_fts(rowid, ` + ftsColumns + `)
		` + ftsRowSelect + ` WHERE m.id = new.message_id;
	END`,

	`CREATE TRIGGER IF NOT EXISTS message_bodies_fts_au AFTER UPDATE OF text_body, html_body ON message_bodies
	WHEN old.text_body IS NOT new.text_body OR old.html_body IS NOT new.html_body
	BEGIN
		DELETE FROM messages_fts WHERE rowid = new.message_id;
		INSERT INTO messages_fts(rowid, ` + ftsColumns + `)
		` + ftsRowSelect + ` WHERE m.id = new.message_id;
	END`,

	`CREATE TRIGGER IF NOT EXISTS message_bodies_fts_ad AFTER DELETE ON message_bodies BEGIN
		DELETE FROM messages_fts WHERE rowid = old.message_id;
		INSERT INTO messages_fts(rowid, ` + ftsColumns + `)
		` + ftsRowSelect + ` WHERE m.id = old.message_id;
	END`,
}

// ftsDropDDL 按依赖顺序删掉全部 FTS 对象（触发器先于虚表）。
var ftsDropDDL = []string{
	"DROP TRIGGER IF EXISTS messages_fts_ai",
	"DROP TRIGGER IF EXISTS messages_fts_au",
	"DROP TRIGGER IF EXISTS messages_fts_ad",
	"DROP TRIGGER IF EXISTS message_bodies_fts_ai",
	"DROP TRIGGER IF EXISTS message_bodies_fts_au",
	"DROP TRIGGER IF EXISTS message_bodies_fts_ad",
	"DROP TABLE IF EXISTS messages_fts",
}

// EnsureFTS 建虚表与触发器（幂等），结构版本落后时先 DROP 再重建并回填。
// 由 database.Migrate 在 AutoMigrate 之后调用——触发器引用的主表必须先存在。
func EnsureFTS(db *gorm.DB) error {
	var ver int
	if err := db.Raw("PRAGMA user_version").Scan(&ver).Error; err != nil {
		return fmt.Errorf("fts read user_version: %w", err)
	}
	if ver < ftsSchemaVersion {
		logger.Info("fts: 索引结构版本落后，重建", zap.Int("from", ver), zap.Int("to", ftsSchemaVersion))
		for _, stmt := range ftsDropDDL {
			if err := db.Exec(stmt).Error; err != nil {
				return fmt.Errorf("fts drop: %w", err)
			}
		}
	}
	for _, stmt := range ftsDDL {
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("fts ddl: %w", err)
		}
	}
	if ver >= ftsSchemaVersion {
		return nil
	}
	if err := RebuildFTS(db); err != nil {
		return err
	}
	// 回填完成才写版本号：中途崩溃则下次启动重来，不会留下半截索引冒充完整
	return db.Exec(fmt.Sprintf("PRAGMA user_version = %d", ftsSchemaVersion)).Error
}

// rebuildMu 让同一进程内的重建串行：两次重建交错会各自 delete-all 又各自回填，
// 索引里留下重复词条。/search/reindex 被连点两下就是这种场景。
var rebuildMu gosync.Mutex

// ftsRebuildBatch 每批回填的邮件数。按「上一批的最大 id」做游标而不是 OFFSET 或固定 id 跨度：
// OFFSET 越翻越慢；固定跨度在长期运行、大量删除后 id 稀疏时会跑出成千上万个空批。
const ftsRebuildBatch = 2000

// RebuildFTS 清空并从主表整体重建全文索引。
//
// 分批提交而不是一个大事务：几万封邮件的正文切分要跑一阵，单事务会把 WAL 撑大、
// 并在整个期间挡住其它写入；分批之间穿插进度日志，让运维看得见它在动。
// 重建期间搜索结果不完整，属可接受的短暂状态。
func RebuildFTS(db *gorm.DB) error {
	rebuildMu.Lock()
	defer rebuildMu.Unlock()
	start := time.Now()
	if err := db.Exec("INSERT INTO messages_fts(messages_fts) VALUES('delete-all')").Error; err != nil {
		return fmt.Errorf("fts delete-all: %w", err)
	}
	var total int64
	if err := db.Raw("SELECT COUNT(*) FROM messages").Scan(&total).Error; err != nil {
		return fmt.Errorf("fts count: %w", err)
	}
	var done, last int64
	for {
		// 先定这一批的上界（第 N 个 id），再按 (last, upper] 回填；两条语句都走主键索引
		var upper *int64
		err := db.Raw(
			"SELECT MAX(id) FROM (SELECT id FROM messages WHERE id > ? ORDER BY id LIMIT ?)",
			last, ftsRebuildBatch,
		).Scan(&upper).Error
		if err != nil {
			return fmt.Errorf("fts batch bound: %w", err)
		}
		if upper == nil {
			break
		}
		res := db.Exec(
			"INSERT INTO messages_fts(rowid, "+ftsColumns+") "+ftsRowSelect+" WHERE m.id > ? AND m.id <= ?",
			last, *upper,
		)
		if res.Error != nil {
			return fmt.Errorf("fts backfill (%d,%d]: %w", last, *upper, res.Error)
		}
		done += res.RowsAffected
		last = *upper
		logger.Info("fts: 回填进度", zap.Int64("done", done), zap.Int64("total", total))
	}
	// 合并 FTS 内部的增量段，让重建后的首批查询不必边查边合并
	if err := db.Exec("INSERT INTO messages_fts(messages_fts) VALUES('optimize')").Error; err != nil {
		return fmt.Errorf("fts optimize: %w", err)
	}
	logger.Info("fts: 重建完成", zap.Int64("messages", done), zap.Duration("elapsed", time.Since(start)))
	return nil
}

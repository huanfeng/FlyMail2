package message

import "time"

// Message 是一封邮件的元数据（M3 不含正文；正文在 M4 按需抓取另存）。
type Message struct {
	ID        uint `gorm:"primaryKey" json:"id"`
	AccountID uint `gorm:"index;index:idx_msg_dedupe,priority:1;not null" json:"account_id"`
	// idx_msg_folder_thread (folder_id, thread_id, date) 是会话列表的覆盖索引：按线程分组 + 取每组最新日期
	// 全在索引内完成（rowid 天然带着），不必回表读 subject/snippet；实测 12.8k 封上分组从 30ms 降到 3ms。
	//
	// idx_msg_unread / idx_msg_flagged 是同样列序的**部分索引**（WHERE seen = 0 / flagged = 1）：
	// 「全部未读」「星标」聚合视图与 is:unread / is:starred 搜索否则只能全表扫描（12.8k 封 ~40ms，5 万封就破百）。
	// 部分索引对绑定参数同样生效：SQLite 会按绑定值重新规划，`seen = ?` 绑 0 时选中 idx_msg_unread、绑 1 时
	// 走全表（真实库 EXPLAIN QUERY PLAN 验证，SQLite 3.53），查询侧照常用参数即可。
	FolderID uint   `gorm:"uniqueIndex:idx_msg_folder_uid;index:idx_msg_folder_thread,priority:1;index:idx_msg_unread,priority:1,where:seen = 0;index:idx_msg_flagged,priority:1,where:flagged = 1;not null" json:"folder_id"`
	UID      uint32 `gorm:"uniqueIndex:idx_msg_folder_uid;not null" json:"uid"`

	// idx_msg_dedupe (account_id, message_id) 专供 dedupeSameMessage 的相关子查询：
	// 没有它，SQLite 会挑 account_id 单列索引，每一候选行都把该账户全部邮件扫一遍
	// （1.2 万封的库上实测每行 ~3ms，命中 800 封的搜索计数要 4 秒）。
	MessageID string `gorm:"index;index:idx_msg_dedupe,priority:2" json:"message_id"`
	// in_reply_to 带索引：线程归属要反向查「谁回复了我」（回复比原信先入库的场景）。
	InReplyTo  string `gorm:"index" json:"in_reply_to"`
	References string `gorm:"column:references_hdr" json:"references"`
	// thread_id 形如 "{account_id}:{根邮件 Message-ID}"，入库后由 AssignThreads 赋值，老库由 RebuildThreads 回填。
	ThreadID string `gorm:"index;index:idx_msg_folder_thread,priority:2;index:idx_msg_unread,priority:2,where:seen = 0;index:idx_msg_flagged,priority:2,where:flagged = 1" json:"thread_id"`

	Subject  string `json:"subject"`
	FromName string `json:"from_name"`
	FromAddr string `json:"from_addr"`
	ToJSON   string `json:"-"`
	CcJSON   string `json:"-"`

	Date time.Time `gorm:"index;index:idx_msg_folder_thread,priority:3;index:idx_msg_unread,priority:3,where:seen = 0;index:idx_msg_flagged,priority:3,where:flagged = 1" json:"date"`
	Size int64     `json:"size"`

	Seen     bool `gorm:"not null" json:"seen"`
	Flagged  bool `gorm:"not null" json:"flagged"`
	Answered bool `gorm:"not null" json:"answered"`
	Deleted  bool `gorm:"not null" json:"deleted"`

	HasAttachment bool   `gorm:"not null" json:"has_attachment"`
	Snippet       string `json:"snippet"`
	BodySynced    bool   `gorm:"not null" json:"body_synced"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Message) TableName() string { return "messages" }

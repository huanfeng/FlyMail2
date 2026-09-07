package message_test

import (
	"testing"
	"time"

	"flymail/internal/fts"
	"flymail/modules/email/account"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"

	"gorm.io/gorm"
)

// search 是测试用的简写：解析 + 查询首页，返回主题列表。
func search(t *testing.T, repo *message.Repository, q string) []string {
	t.Helper()
	list, err := repo.SearchMessages(fts.Parse(q), nil, 0, 50, message.Filter{})
	if err != nil {
		t.Fatalf("SearchMessages(%q): %v", q, err)
	}
	return subjectsOf(list)
}

func seedFTS(t *testing.T, repo *message.Repository, db *gorm.DB) *message.BodyRepository {
	t.Helper()
	accts := []account.Account{
		{ID: 1, Name: "工作邮箱", Email: "me@work.com"},
		{ID: 2, Name: "personal", Email: "me@home.org"},
	}
	if err := db.Create(&accts).Error; err != nil {
		t.Fatalf("seed accounts: %v", err)
	}
	folders := []folder.Folder{
		{ID: 1, AccountID: 1, Path: "INBOX", DisplayName: "收件箱", Type: "inbox", Selectable: true},
		{ID: 2, AccountID: 1, Path: "Sent", DisplayName: "已发送", Type: "sent", Selectable: true},
		{ID: 3, AccountID: 2, Path: "INBOX", DisplayName: "Inbox", Type: "inbox", Selectable: true},
	}
	if err := db.Create(&folders).Error; err != nil {
		t.Fatalf("seed folders: %v", err)
	}
	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.Local)
	day := func(d int) time.Time { return base.AddDate(0, 0, d) }
	msgs := []*message.Message{
		{AccountID: 1, FolderID: 1, UID: 1, Subject: "三月发票报销", FromName: "张三", FromAddr: "zhangsan@corp.cn",
			ToJSON: `[{"name":"李四","email":"lisi@corp.cn"}]`, Snippet: "请查收", Date: day(0), Seen: false, HasAttachment: true},
		{AccountID: 1, FolderID: 1, UID: 2, Subject: "Weekly report", FromName: "Alice Wang", FromAddr: "alice@corp.cn",
			Snippet: "progress update", Date: day(1), Seen: true},
		{AccountID: 1, FolderID: 2, UID: 1, Subject: "Re: 三月发票报销", FromName: "我", FromAddr: "me@work.com",
			ToJSON: `[{"name":"张三","email":"zhangsan@corp.cn"}]`, Date: day(2), Seen: true},
		{AccountID: 2, FolderID: 3, UID: 1, Subject: "Newsletter", FromName: "News", FromAddr: "news@site.io",
			Date: day(3), Seen: false, Flagged: true},
	}
	for _, m := range msgs {
		if err := repo.Upsert(m); err != nil {
			t.Fatalf("seed msg: %v", err)
		}
	}
	bodies := message.NewBodyRepository(db)
	// Newsletter 带正文（snippet 由正文派生，索引只看正文）；Weekly report 故意不带，留给触发器测试
	nl, err := repo.GetByFolderUID(3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := bodies.Upsert(&message.MessageBody{MessageID: nl.ID, TextBody: "invoice inside"}); err != nil {
		t.Fatal(err)
	}
	return bodies
}

func TestFTSChineseBigram(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedFTS(t, repo, db)

	// 两字词必须能命中（trigram 做不到，这是选 bigram 的根本原因）
	if got := search(t, repo, "发票"); len(got) != 2 {
		t.Errorf("发票 -> %v, want 2", got)
	}
	// 连续多字按短语匹配：「发票报销」命中，「报销发票」（顺序反了）不命中
	if got := search(t, repo, "发票报销"); len(got) != 2 {
		t.Errorf("发票报销 -> %v", got)
	}
	if got := search(t, repo, "报销发票"); len(got) != 0 {
		t.Errorf("报销发票 应无命中, got %v", got)
	}
	// 自由词不限列：「张三」既命中发件人也命中收件人（回复那封的收件人是张三）
	if got := search(t, repo, "张三"); len(got) != 2 {
		t.Errorf("张三 -> %v, want 2", got)
	}
	// 英文前缀 + 大小写不敏感
	if got := search(t, repo, "week"); len(got) != 1 || got[0] != "Weekly report" {
		t.Errorf("week -> %v", got)
	}
	// 多词 AND
	if got := search(t, repo, "invoice news"); len(got) != 1 {
		t.Errorf("invoice news -> %v", got)
	}
	if got := search(t, repo, "invoice 发票"); len(got) != 0 {
		t.Errorf("invoice 发票 应无交集, got %v", got)
	}
}

func TestFTSBodyIndexedByTrigger(t *testing.T) {
	repo, db := newRepoWithDB(t)
	bodies := seedFTS(t, repo, db)

	if got := search(t, repo, "合同编号"); len(got) != 0 {
		t.Fatalf("正文未入库前不应命中: %v", got)
	}
	var m message.Message
	if err := db.Where("subject = ?", "Weekly report").First(&m).Error; err != nil {
		t.Fatal(err)
	}
	if err := bodies.Upsert(&message.MessageBody{MessageID: m.ID, TextBody: "附件是合同编号 A-2026 的扫描件"}); err != nil {
		t.Fatal(err)
	}
	if got := search(t, repo, "合同编号"); len(got) != 1 || got[0] != "Weekly report" {
		t.Errorf("正文落库后应命中: %v", got)
	}
	if got := search(t, repo, "a-2026"); len(got) != 1 {
		t.Errorf("正文英文数字应命中: %v", got)
	}
	// 正文更新后旧词失效、新词生效
	if err := bodies.Upsert(&message.MessageBody{MessageID: m.ID, TextBody: "改成了采购清单"}); err != nil {
		t.Fatal(err)
	}
	if got := search(t, repo, "合同编号"); len(got) != 0 {
		t.Errorf("旧正文词应失效: %v", got)
	}
	if got := search(t, repo, "采购"); len(got) != 1 {
		t.Errorf("新正文词应生效: %v", got)
	}
	// 主题更新后同样跟随
	if err := db.Model(&m).Update("subject", "Monthly summary").Error; err != nil {
		t.Fatal(err)
	}
	if got := search(t, repo, "weekly"); len(got) != 0 {
		t.Errorf("旧主题应失效: %v", got)
	}
	if got := search(t, repo, "monthly 采购"); len(got) != 1 {
		t.Errorf("新主题 + 保留的正文应同时命中: %v", got)
	}
	// 删除邮件 → 索引行一并消失
	if err := repo.DeleteByID(m.ID); err != nil {
		t.Fatal(err)
	}
	if got := search(t, repo, "采购"); len(got) != 0 {
		t.Errorf("删除后不应命中: %v", got)
	}
}

func TestFTSHTMLOnlyBodyIndexed(t *testing.T) {
	repo, db := newRepoWithDB(t)
	bodies := seedFTS(t, repo, db)
	var m message.Message
	if err := db.Where("subject = ?", "Newsletter").First(&m).Error; err != nil {
		t.Fatal(err)
	}
	// 纯 HTML 邮件：text_body 为空，正文只在 html_body 里
	if err := bodies.Upsert(&message.MessageBody{MessageID: m.ID, HTMLBody: "<table><tr><td>本期</td><td>促销活动</td></tr></table><style>x{}</style>"}); err != nil {
		t.Fatal(err)
	}
	if got := search(t, repo, "促销"); len(got) != 1 || got[0] != "Newsletter" {
		t.Errorf("HTML 正文应可搜到: %v", got)
	}
	// 标签边界不应粘成不存在的词：「期促」跨 <td> 边界
	if got := search(t, repo, "期促"); len(got) != 0 {
		t.Errorf("跨标签的字不应连成词: %v", got)
	}
	// 有 text_body 时以它为准
	if err := bodies.Upsert(&message.MessageBody{MessageID: m.ID, TextBody: "文本版本", HTMLBody: "<p>促销活动</p>"}); err != nil {
		t.Fatal(err)
	}
	if got := search(t, repo, "促销"); len(got) != 0 {
		t.Errorf("有 text_body 时不再索引 html: %v", got)
	}
	if got := search(t, repo, "文本"); len(got) != 1 {
		t.Errorf("text_body 应生效: %v", got)
	}
}

func TestFTSQualifiers(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedFTS(t, repo, db)

	cases := map[string][]string{
		"from:zhang":                         {"三月发票报销"},
		"from:张三":                            {"三月发票报销"},
		"to:lisi":                            {"三月发票报销"},
		"to:张三":                              {"Re: 三月发票报销"},
		"subject:发票 is:unread":               {"三月发票报销"},
		"发票 is:read":                         {"Re: 三月发票报销"},
		"has:attachment":                     {"三月发票报销"},
		"is:starred":                         {"Newsletter"},
		"in:sent":                            {"Re: 三月发票报销"},
		"in:已发送":                             {"Re: 三月发票报销"},
		"account:home":                       {"Newsletter"},
		"account:工作 发票":                      {"Re: 三月发票报销", "三月发票报销"},
		"before:2026-03-02":                  {"三月发票报销"},
		"after:2026-03-03":                   {"Newsletter", "Re: 三月发票报销"},
		"after:2026-03-02 before:2026-03-03": {"Weekly report"},
		// 非法限定符取值退化为文本：没有邮件含 "banana" 一词
		"has:banana": {},
		// in:/account: 走 LIKE，% 与 _ 必须按字面量转义：没有文件夹/账户名含这些字符 → 无命中
		// （不转义的话 % 会匹配全部文件夹）
		"in:% 发票":      {},
		"account:_ 发票": {},
	}
	for q, want := range cases {
		got := search(t, repo, q)
		if len(got) != len(want) {
			t.Errorf("%q -> %v, want %v", q, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%q -> %v, want %v", q, got, want)
				break
			}
		}
	}
}

// ftsHits 数索引里命中某词的行数：重复 rowid 会被计成两行，用来守住「不产生重复索引行」。
func ftsHits(t *testing.T, db *gorm.DB, term string) int64 {
	t.Helper()
	var n int64
	if err := db.Raw("SELECT count(*) FROM messages_fts WHERE messages_fts MATCH ?", fts.Parse(term).Match()).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFTSUpsertUnchangedKeepsSingleIndexRow(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedFTS(t, repo, db)
	m, err := repo.GetByFolderUID(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	// 加 from: 限定只看原件（回复那封的主题也含同一短语）
	if n := ftsHits(t, db, "from:zhangsan subject:三月发票报销"); n != 1 {
		t.Fatalf("before: hits=%d", n)
	}
	// 同步会对已存在的行反复 upsert（值不变）——索引不该因此重建，更不该出现重复行
	for i := 0; i < 3; i++ {
		if err := repo.Upsert(&message.Message{AccountID: m.AccountID, FolderID: m.FolderID, UID: m.UID, Subject: m.Subject,
			FromName: m.FromName, FromAddr: m.FromAddr, ToJSON: m.ToJSON, Date: m.Date, HasAttachment: true}); err != nil {
			t.Fatal(err)
		}
	}
	if n := ftsHits(t, db, "from:zhangsan subject:三月发票报销"); n != 1 {
		t.Errorf("after unchanged upserts: hits=%d, want 1", n)
	}
	// 值变了才重建，且仍只有一行
	if err := db.Model(&message.Message{}).Where("id = ?", m.ID).Update("subject", "四月发票报销").Error; err != nil {
		t.Fatal(err)
	}
	if n := ftsHits(t, db, "from:zhangsan subject:四月"); n != 1 {
		t.Errorf("after change: hits=%d, want 1", n)
	}
	if n := ftsHits(t, db, "from:zhangsan subject:三月"); n != 0 {
		t.Errorf("old subject should be gone: hits=%d", n)
	}
}

func TestFTSBodyDeleteTrigger(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedFTS(t, repo, db)
	// Newsletter 的正文 "invoice inside" 在种子里已入索引
	if got := search(t, repo, "inside"); len(got) != 1 {
		t.Fatalf("before: %v", got)
	}
	nl, _ := repo.GetByFolderUID(3, 1)
	if err := db.Where("message_id = ?", nl.ID).Delete(&message.MessageBody{}).Error; err != nil {
		t.Fatal(err)
	}
	if got := search(t, repo, "inside"); len(got) != 0 {
		t.Errorf("body deleted, term should be gone: %v", got)
	}
	if got := search(t, repo, "newsletter"); len(got) != 1 {
		t.Errorf("subject should still match: %v", got)
	}
}

func TestUIDsNotSearchableBeyondBindLimit(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedFTS(t, repo, db)
	// 40000 个 UID 超过 SQLite 单条语句绑定变量上限（32766），必须分块
	uids := make([]uint32, 0, 40000)
	for i := uint32(1); i <= 40000; i++ {
		uids = append(uids, i)
	}
	need, err := repo.UIDsNotSearchable(1, uids)
	if err != nil {
		t.Fatalf("UIDsNotSearchable: %v", err)
	}
	// 文件夹 1 里 uid 1、2 都没有正文 → 全部 40000 个都算「本地搜不到」
	if len(need) != 40000 {
		t.Errorf("need=%d, want 40000", len(need))
	}
	_ = db
}

func TestFTSCountMatchesList(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedFTS(t, repo, db)
	for _, q := range []string{"发票", "is:unread", "from:zhang 发票", "nothing-matches"} {
		list, _ := repo.SearchMessages(fts.Parse(q), nil, 0, 50, message.Filter{})
		n, err := repo.CountSearchMessages(fts.Parse(q), message.Filter{})
		if err != nil {
			t.Fatal(err)
		}
		if int(n) != len(list) {
			t.Errorf("%q: count %d != list %d", q, n, len(list))
		}
	}
}

func TestFTSRebuildAndBackfill(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedFTS(t, repo, db)

	// 模拟索引漂移：清空索引后搜索为空
	if err := db.Exec("INSERT INTO messages_fts(messages_fts) VALUES('delete-all')").Error; err != nil {
		t.Fatal(err)
	}
	if got := search(t, repo, "发票"); len(got) != 0 {
		t.Fatalf("清空后应无命中: %v", got)
	}
	if err := message.RebuildFTS(db); err != nil {
		t.Fatalf("RebuildFTS: %v", err)
	}
	if got := search(t, repo, "发票"); len(got) != 2 {
		t.Errorf("重建后应恢复: %v", got)
	}

	// 模拟老库升级：版本号归零 + 索引清空，再走一遍 EnsureFTS 应自动回填并写版本号
	if err := db.Exec("INSERT INTO messages_fts(messages_fts) VALUES('delete-all')").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("PRAGMA user_version = 0").Error; err != nil {
		t.Fatal(err)
	}
	if err := message.EnsureFTS(db); err != nil {
		t.Fatalf("EnsureFTS: %v", err)
	}
	if got := search(t, repo, "发票"); len(got) != 2 {
		t.Errorf("升级回填后应恢复: %v", got)
	}
	var ver int
	if err := db.Raw("PRAGMA user_version").Scan(&ver).Error; err != nil || ver < 1 {
		t.Errorf("user_version = %d, err %v", ver, err)
	}
	// 版本落后时应真的重建结构：手工把虚表改成缺列的旧定义，EnsureFTS 后触发器/回填仍能工作
	for _, stmt := range []string{
		"DROP TRIGGER IF EXISTS messages_fts_ai", "DROP TRIGGER IF EXISTS messages_fts_au",
		"DROP TRIGGER IF EXISTS messages_fts_ad", "DROP TRIGGER IF EXISTS message_bodies_fts_ai",
		"DROP TRIGGER IF EXISTS message_bodies_fts_au", "DROP TRIGGER IF EXISTS message_bodies_fts_ad",
		"DROP TABLE messages_fts",
		"CREATE VIRTUAL TABLE messages_fts USING fts5(subject, content='', contentless_delete=1)",
		"PRAGMA user_version = 0",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err := message.EnsureFTS(db); err != nil {
		t.Fatalf("EnsureFTS after schema drift: %v", err)
	}
	if got := search(t, repo, "from:zhang"); len(got) != 1 {
		t.Errorf("结构重建后列过滤应可用: %v", got)
	}

	// 版本已是最新时再调 EnsureFTS 不应重建（清空索引后仍为空即证明没重建）
	if err := db.Exec("INSERT INTO messages_fts(messages_fts) VALUES('delete-all')").Error; err != nil {
		t.Fatal(err)
	}
	if err := message.EnsureFTS(db); err != nil {
		t.Fatal(err)
	}
	if got := search(t, repo, "发票"); len(got) != 0 {
		t.Errorf("版本最新时 EnsureFTS 不应重建，但命中了 %v", got)
	}
}

func TestSearchEmptyQueryReturnsNothing(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedFTS(t, repo, db)
	svc := message.NewService(repo, message.NewBodyRepository(db))
	for _, q := range []string{"...", "\"\"", "from:"} {
		list, cursor, err := svc.ListSearch(q, nil, 0, 50, message.Filter{})
		if err != nil || len(list) != 0 || cursor != nil {
			t.Errorf("ListSearch(%q) = %v, %v, %v; want empty", q, list, cursor, err)
		}
		if n, _ := svc.CountSearchMessages(q, message.Filter{}); n != 0 {
			t.Errorf("CountSearchMessages(%q) = %d", q, n)
		}
	}
}

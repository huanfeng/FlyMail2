package message_test

import (
	"flymail/internal/fts"
	"testing"

	"flymail/modules/email/message"
)

// ptr 取字面量地址，构造 Filter 的三态字段。
func ptr(b bool) *bool { return &b }

// subjectsOf 提取主题列表，便于断言。
func subjectsOf(list []message.Message) []string {
	out := make([]string, 0, len(list))
	for _, m := range list {
		out = append(out, m.Subject)
	}
	return out
}

func TestFilterActive(t *testing.T) {
	if (message.Filter{}).Active() {
		t.Error("零值 Filter 不该是 active")
	}
	// false 是有意义的取值（「只看未读」），不能被当成「不筛选」
	if !(message.Filter{Seen: ptr(false)}).Active() {
		t.Error("Seen=false 应当是 active")
	}
	if !(message.Filter{HasAttachment: ptr(true)}).Active() {
		t.Error("HasAttachment=true 应当是 active")
	}
}

func TestListByFolderFilterSeen(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)

	// folder 1（acct1 inbox）= i1-unread(未读) + i1-read-star(已读星标)
	list, err := repo.ListByFolder(1, 0, 50, message.Filter{Seen: ptr(false)})
	if err != nil {
		t.Fatalf("ListByFolder: %v", err)
	}
	if got := subjectsOf(list); len(got) != 1 || got[0] != "i1-unread" {
		t.Errorf("seen=false 得到 %v, want [i1-unread]", got)
	}

	list, _ = repo.ListByFolder(1, 0, 50, message.Filter{Seen: ptr(true)})
	if got := subjectsOf(list); len(got) != 1 || got[0] != "i1-read-star" {
		t.Errorf("seen=true 得到 %v, want [i1-read-star]", got)
	}
}

// TestListByFolderFilterCombined 验证多维度是 AND 叠加，而非互斥择一。
func TestListByFolderFilterCombined(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)

	// folder 1 内没有「既未读又星标」的邮件
	list, _ := repo.ListByFolder(1, 0, 50, message.Filter{Seen: ptr(false), Flagged: ptr(true)})
	if len(list) != 0 {
		t.Errorf("folder1 未读+星标 得到 %v, want 空", subjectsOf(list))
	}

	// folder 2（trash）里的 trash-unread-star 同时满足两者
	list, _ = repo.ListByFolder(2, 0, 50, message.Filter{Seen: ptr(false), Flagged: ptr(true)})
	if got := subjectsOf(list); len(got) != 1 || got[0] != "trash-unread-star" {
		t.Errorf("folder2 未读+星标 得到 %v, want [trash-unread-star]", got)
	}
}

// TestCountByFolderMatchesFilteredList 是这次改动的核心保证：
// 标题「共 N 封」与列表实际能翻到的条数必须同口径，否则又回到
// 「显示 320 封、列表只有 5 条」的状态。
func TestCountByFolderMatchesFilteredList(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)

	f := message.Filter{Seen: ptr(false)}
	list, _ := repo.ListByFolder(1, 0, 200, f)
	n, err := repo.CountByFolder(1, f)
	if err != nil {
		t.Fatalf("CountByFolder: %v", err)
	}
	if int(n) != len(list) {
		t.Errorf("计数 %d 与列表条数 %d 不一致", n, len(list))
	}

	// 零值 Filter 退化为全量计数
	if all, _ := repo.CountByFolder(1, message.Filter{}); all != 2 {
		t.Errorf("全量计数 = %d, want 2", all)
	}
}

// TestFilterHasAttachment 覆盖 has_attachment 维度。
// 该列由 MarkBodySynced 在正文落库时回填，这里直接置位模拟已预取正文的邮件。
func TestFilterHasAttachment(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)

	if err := db.Model(&message.Message{}).
		Where("subject = ?", "i1-read-star").
		Update("has_attachment", true).Error; err != nil {
		t.Fatalf("set has_attachment: %v", err)
	}

	list, _ := repo.ListByFolder(1, 0, 50, message.Filter{HasAttachment: ptr(true)})
	if got := subjectsOf(list); len(got) != 1 || got[0] != "i1-read-star" {
		t.Errorf("has_attachment=true 得到 %v, want [i1-read-star]", got)
	}
}

// TestFilterOnAggregate 验证筛选在 JOIN folders 的聚合查询上不产生歧义列名，
// 且与聚合自身的 view 条件正确叠加。
func TestFilterOnAggregate(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)

	// starred 视图（已排除 trash）再叠加「已读」= i1-read-star
	list, err := repo.ListAggregate("starred", nil, 0, 50, message.Filter{Seen: ptr(true)})
	if err != nil {
		t.Fatalf("ListAggregate: %v", err)
	}
	if got := subjectsOf(list); len(got) != 1 || got[0] != "i1-read-star" {
		t.Errorf("starred+已读 得到 %v, want [i1-read-star]", got)
	}

	n, err := repo.CountAggregateTotal("starred", message.Filter{Seen: ptr(true)})
	if err != nil {
		t.Fatalf("CountAggregateTotal: %v", err)
	}
	if int(n) != len(list) {
		t.Errorf("聚合计数 %d 与列表条数 %d 不一致", n, len(list))
	}
}

// TestFilterOnSearch 验证筛选在 JOIN message_bodies 的搜索查询上不产生歧义列名，
// 且「命中 N 条」与筛选后的列表同口径。
func TestFilterOnSearch(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)

	// 主题含 "unread" 的共 5 封，其中带星标的只有 trash-unread-star
	f := message.Filter{Flagged: ptr(true)}
	list, err := repo.SearchMessages(fts.Parse("unread"), nil, 0, 50, f)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if got := subjectsOf(list); len(got) != 1 || got[0] != "trash-unread-star" {
		t.Errorf("搜索 unread+星标 得到 %v, want [trash-unread-star]", got)
	}

	n, err := repo.CountSearchMessages(fts.Parse("unread"), f)
	if err != nil {
		t.Fatalf("CountSearchMessages: %v", err)
	}
	if int(n) != len(list) {
		t.Errorf("搜索计数 %d 与列表条数 %d 不一致", n, len(list))
	}

	// 不筛选时应拿到全部 5 封，确认上面的 1 条确实是筛出来的
	if all, _ := repo.CountSearchMessages(fts.Parse("unread"), message.Filter{}); all != 5 {
		t.Errorf("搜索全量计数 = %d, want 5", all)
	}
}

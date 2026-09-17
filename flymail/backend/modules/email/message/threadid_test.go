package message_test

import (
	"strings"
	"testing"

	"flymail/modules/email/message"

	"gorm.io/gorm"
)

// TestThreadKeyGolden 把 thread_id 的格式钉死。
//
// 其余测试用 wantTID 重算公式，那挡不住实现和助手一起漂移；这里写死期望值，
// 摘要算法、截断长度、拼接方式任一改动都会在这里红掉。
//
// 会话划分是**持久化**的：thread_id 一旦变了，老库里所有行的 id 就和新入库的邮件对不上，
// 回复会另开一条会话。所以这个格式不是实现细节，改它必须是一次有意的、带迁移的决定。
func TestThreadKeyGolden(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedThreadFixture(t, db)

	m := put(t, repo, &message.Message{AccountID: 1, FolderID: 1, UID: 1, MessageID: "a@x", Subject: "hi", Date: at(0)})
	const want = "1:bd39e6feafd837011c64" // sha256("a@x")[:20]
	if m.ThreadID != want {
		t.Fatalf("thread_id 格式变了：want %q got %q", want, m.ThreadID)
	}
}

// TestThreadKeyLeaksNoAddress 是这次改动的初衷：thread_id 会原样出现在前端 URL、
// 浏览器历史和访问日志里，里头不能带发件方域名。
func TestThreadKeyLeaksNoAddress(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedThreadFixture(t, db)

	m := put(t, repo, &message.Message{
		AccountID: 1, FolderID: 1, UID: 1,
		MessageID: "CAF=abc123@mail.github.com", Subject: "PR merged", Date: at(0),
	})
	if strings.Contains(m.ThreadID, "@") || strings.Contains(m.ThreadID, "github") {
		t.Fatalf("thread_id 里还带着 Message-ID：%q", m.ThreadID)
	}
}

// tidsByUID 读回若干封的 thread_id，按 (folder, uid) 取。
func tidsByUID(t *testing.T, repo *message.Repository, keys [][2]uint) []string {
	t.Helper()
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, threadOf(t, repo, k[0], uint32(k[1])))
	}
	return out
}

// seedLegacyThreadIDs 直接写入老格式的 thread_id，绕开 AssignThreads。
//
// ⚠ 必须在 database.Migrate 之后写：Migrate 里就跑了 EnsureOpaqueThreadIDs，
// 先插再迁移的话，测试验的是「迁移在同一次运行里顺手处理了它」，而真实升级场景是
// 「老库里早就有这些行」。日期规范化那次就是栽在这个时序上——反转实现测试照样绿。
func seedLegacyThreadIDs(t *testing.T, db *gorm.DB, rows map[[2]uint]string) {
	t.Helper()
	for k, tid := range rows {
		if err := db.Model(&message.Message{}).
			Where("folder_id = ? AND uid = ?", k[0], k[1]).
			Update("thread_id", tid).Error; err != nil {
			t.Fatalf("seed legacy %v: %v", k, err)
		}
	}
}

func TestEnsureOpaqueThreadIDs(t *testing.T) {
	repo, db := newRepoWithDB(t)
	seedThreadFixture(t, db)

	// 四封邮件，写成老库的样子：前两封同一会话，第三封另一个会话，第四封是无 Message-ID 的兜底形态。
	for _, m := range []*message.Message{
		{AccountID: 1, FolderID: 1, UID: 1, MessageID: "a@x.com", Subject: "hi", Date: at(0)},
		{AccountID: 1, FolderID: 2, UID: 1, MessageID: "b@x.com", Subject: "Re: hi", Date: at(1)},
		{AccountID: 1, FolderID: 1, UID: 2, MessageID: "c@y.org", Subject: "other", Date: at(2)},
		{AccountID: 1, FolderID: 1, UID: 3, Subject: "no msgid", Date: at(3)},
	} {
		if err := repo.Upsert(m); err != nil {
			t.Fatal(err)
		}
	}
	seedLegacyThreadIDs(t, db, map[[2]uint]string{
		{1, 1}: "1:a@x.com",
		{2, 1}: "1:a@x.com", // 与上一封同一会话
		{1, 2}: "1:c@y.org",
		{1, 3}: "1:u1-3", // 兜底形态，本来就不带域名
	})

	if err := message.EnsureOpaqueThreadIDs(db); err != nil {
		t.Fatalf("EnsureOpaqueThreadIDs: %v", err)
	}
	got := tidsByUID(t, repo, [][2]uint{{1, 1}, {2, 1}, {1, 2}, {1, 3}})

	// 1. 分组一个都不能动：原来同一会话的还在一起，不同会话的仍然分开
	if got[0] != got[1] {
		t.Errorf("同一会话被拆开了：%q vs %q", got[0], got[1])
	}
	if got[0] == got[2] {
		t.Errorf("两个会话被并到一起了：%q", got[0])
	}
	// 2. 域名没了
	for i, tid := range got {
		if strings.Contains(tid, "@") {
			t.Errorf("第 %d 行仍带 Message-ID：%q", i, tid)
		}
	}
	// 3. 兜底形态原样不动——它本来就不带域名，改它只会白白让老链接失效
	if got[3] != "1:u1-3" {
		t.Errorf("兜底形态不该被改：%q", got[3])
	}
	// 4. 迁移产物必须与新铸产物一致，否则老会话收到新回复时会另开一条
	if want := wantTID(1, "a@x.com"); got[0] != want {
		t.Errorf("迁移结果与 threadKey 不一致：want %q got %q", want, got[0])
	}

	// 5. 幂等：再跑一次不能把摘要再哈希一遍（真实升级里每次启动都会跑）
	if err := message.EnsureOpaqueThreadIDs(db); err != nil {
		t.Fatalf("第二次 EnsureOpaqueThreadIDs: %v", err)
	}
	if again := tidsByUID(t, repo, [][2]uint{{1, 1}, {2, 1}, {1, 2}, {1, 3}}); !equalStrings(again, got) {
		t.Errorf("不幂等：%v -> %v", got, again)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

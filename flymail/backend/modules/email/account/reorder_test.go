package account_test

import (
	"errors"
	"testing"

	"flymail/modules/email/account"
)

// mk 建若干账户并返回它们的 ID（按建立顺序）。
func mk(t *testing.T, r *account.Repository, names ...string) []uint {
	t.Helper()
	ids := make([]uint, 0, len(names))
	for _, n := range names {
		a := &account.Account{Name: n, Email: n + "@example.com", AuthType: "password"}
		if err := r.Create(a); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
		ids = append(ids, a.ID)
	}
	return ids
}

func listIDs(t *testing.T, r *account.Repository) []uint {
	t.Helper()
	list, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	ids := make([]uint, len(list))
	for i, a := range list {
		ids[i] = a.ID
	}
	return ids
}

func eq(a, b []uint) bool {
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

func TestNewAccountGoesToTheEnd(t *testing.T) {
	r := newTestRepo(t)
	ids := mk(t, r, "a", "b")

	// 先把顺序倒过来，再新建一个：它该落在末尾，而不是按 id 插回中间。
	if err := r.Reorder([]uint{ids[1], ids[0]}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	newIDs := mk(t, r, "c")

	want := []uint{ids[1], ids[0], newIDs[0]}
	if got := listIDs(t, r); !eq(got, want) {
		t.Errorf("新账户未排到末尾：got %v, want %v", got, want)
	}
}

func TestReorderIsIdempotentAndComplete(t *testing.T) {
	r := newTestRepo(t)
	ids := mk(t, r, "a", "b", "c")

	want := []uint{ids[2], ids[0], ids[1]}
	for i := 0; i < 2; i++ { // 重复提交同一顺序不应改变结果
		if err := r.Reorder(want); err != nil {
			t.Fatalf("Reorder 第 %d 次: %v", i+1, err)
		}
		if got := listIDs(t, r); !eq(got, want) {
			t.Fatalf("第 %d 次后顺序不对：got %v, want %v", i+1, got, want)
		}
	}
}

// 列表对不上时必须拒绝，而不是替用户猜没提到的账户该放哪儿。
// 真实触发场景：开了两个标签页，A 刚加了账户，B 还照着旧列表点上移。
func TestReorderRejectsMismatchedSet(t *testing.T) {
	r := newTestRepo(t)
	ids := mk(t, r, "a", "b", "c")
	before := listIDs(t, r)

	for name, bad := range map[string][]uint{
		"少一个":    {ids[0], ids[1]},
		"多一个":    {ids[0], ids[1], ids[2], 9999},
		"有重复":    {ids[0], ids[0], ids[1]},
		"换了个陌生的": {ids[0], ids[1], 9999},
		"空列表":    {},
	} {
		t.Run(name, func(t *testing.T) {
			err := r.Reorder(bad)
			if !errors.Is(err, account.ErrOrderMismatch) {
				t.Fatalf("应返回 ErrOrderMismatch，实际 %v", err)
			}
			// 事务必须整体回滚：拒绝了却改了一半，比直接接受更糟
			if got := listIDs(t, r); !eq(got, before) {
				t.Errorf("拒绝后顺序被改动了：got %v, want %v", got, before)
			}
		})
	}
}

// 老库经 AutoMigrate 加列后 sort_order 全为 0。此时只按 sort_order 排在 SQLite
// 下顺序不确定，界面会每次刷新换个排法——所以 List 必须带 id 兜底。
func TestLegacyRowsKeepStableOrder(t *testing.T) {
	r := newTestRepo(t)
	ids := mk(t, r, "a", "b", "c")
	for _, id := range ids {
		if err := r.UpdateFields(id, map[string]any{"sort_order": 0}); err != nil {
			t.Fatalf("模拟老库: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if got := listIDs(t, r); !eq(got, ids) {
			t.Fatalf("第 %d 次读出的顺序与 id 序不符：got %v, want %v", i+1, got, ids)
		}
	}
}

// 编辑账户不能把位次弄丢。
//
// 现在的 Update 是「先 GetByID 再 Save」，位次跟着整行被读出来又写回去，
// 所以是对的。但这是**巧合式正确**：哪天有人把它改成照请求体现场构造一个
// Account 再 Save，sort_order 就会被 Save 写成零值，用户的排序在改一次
// 端口号之后集体跑到最前面——而且不会有任何报错。
func TestUpdateKeepsSortOrder(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ids := createAccounts(t, svc, "a", "b", "c")

	want := []uint{ids[2], ids[1], ids[0]}
	if err := repo.Reorder(want); err != nil {
		t.Fatalf("Reorder: %v", err)
	}

	// 改中间那个账户的一个无关字段
	if _, err := svc.Update(ids[1], account.UpdateAccountRequest{
		Name: "b-renamed", Email: "b@example.com",
		IMAPHost: "imap.example.com", IMAPPort: 143,
		SMTPHost: "smtp.example.com", SMTPPort: 587,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	if got := listIDs(t, repo); !eq(got, want) {
		t.Errorf("编辑后顺序变了：got %v, want %v", got, want)
	}
}

// 删号后位次会留空洞（如 1,3）。这不该影响显示顺序——List 按值排，
// 空洞无所谓；下一次 Reorder 会把全表重写回 1..n。
func TestDeleteLeavesRemainingOrderIntact(t *testing.T) {
	svc, repo, _ := newSvc(t)
	ids := createAccounts(t, svc, "a", "b", "c")
	if err := repo.Reorder([]uint{ids[2], ids[0], ids[1]}); err != nil {
		t.Fatalf("Reorder: %v", err)
	}
	if err := svc.Delete(ids[0]); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := listIDs(t, repo); !eq(got, []uint{ids[2], ids[1]}) {
		t.Errorf("删号后剩余顺序不对：got %v, want %v", got, []uint{ids[2], ids[1]})
	}
}

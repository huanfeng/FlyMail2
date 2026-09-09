package account_test

import (
	"errors"
	"strings"
	"testing"

	"flymail/modules/email/account"
)

// newAcct 建一个账户供别名/签名测试挂载。
func newAcct(t *testing.T, svc *account.Service, email string) uint {
	t.Helper()
	resp, err := svc.Create(account.CreateAccountRequest{
		Name: "Work", Email: email, Password: "p",
		IMAPHost: "h", IMAPPort: 993, SMTPHost: "h", SMTPPort: 465,
	})
	if err != nil {
		t.Fatalf("建账户失败: %v", err)
	}
	return resp.ID
}

func TestAliasCRUD(t *testing.T) {
	svc, _, _ := newSvc(t)
	id := newAcct(t, svc, "main@example.com")

	a, err := svc.CreateAlias(id, account.AliasRequest{Email: "Sales@Example.com", DisplayName: "销售部"})
	if err != nil {
		t.Fatalf("新增别名失败: %v", err)
	}
	// 地址统一小写落库，否则大小写变体能绕过重复检查
	if a.Email != "sales@example.com" {
		t.Errorf("别名应规范化为小写，实际 %q", a.Email)
	}

	list, err := svc.ListAliases(id)
	if err != nil || len(list) != 1 {
		t.Fatalf("列表应有 1 条，实际 %d, err=%v", len(list), err)
	}

	updated, err := svc.UpdateAlias(id, a.ID, account.AliasRequest{
		Email: "sales@example.com", DisplayName: "售前", IsDefault: true,
	})
	if err != nil {
		t.Fatalf("修改别名失败: %v", err)
	}
	if updated.DisplayName != "售前" || !updated.IsDefault {
		t.Errorf("修改未生效: %+v", updated)
	}

	if err := svc.DeleteAlias(id, a.ID); err != nil {
		t.Fatalf("删除别名失败: %v", err)
	}
	if list, _ := svc.ListAliases(id); len(list) != 0 {
		t.Errorf("删除后应为空，实际 %d 条", len(list))
	}
	if err := svc.DeleteAlias(id, a.ID); !errors.Is(err, account.ErrAliasNotFound) {
		t.Errorf("重复删除应返回 ErrAliasNotFound，实际 %v", err)
	}
}

func TestAliasRejectsDuplicateAndPrimary(t *testing.T) {
	svc, _, _ := newSvc(t)
	id := newAcct(t, svc, "main@example.com")

	if _, err := svc.CreateAlias(id, account.AliasRequest{Email: "sales@example.com"}); err != nil {
		t.Fatalf("首次新增应成功: %v", err)
	}
	// 大小写变体也算重复
	if _, err := svc.CreateAlias(id, account.AliasRequest{Email: "SALES@example.com"}); !errors.Is(err, account.ErrAliasDuplicate) {
		t.Errorf("重复别名应被拒，实际 %v", err)
	}
	// 主地址本来就能发信，收进别名只会让发件人下拉出现两个相同条目
	if _, err := svc.CreateAlias(id, account.AliasRequest{Email: "Main@example.com"}); !errors.Is(err, account.ErrAliasIsPrimary) {
		t.Errorf("主地址应被拒，实际 %v", err)
	}
	if _, err := svc.CreateAlias(id, account.AliasRequest{Email: "not-an-email"}); !errors.Is(err, account.ErrInvalidEmail) {
		t.Errorf("非法地址应被拒，实际 %v", err)
	}
}

// TestAliasDefaultIsExclusive 同账户至多一个默认别名。
func TestAliasDefaultIsExclusive(t *testing.T) {
	svc, _, _ := newSvc(t)
	id := newAcct(t, svc, "main@example.com")

	a1, _ := svc.CreateAlias(id, account.AliasRequest{Email: "a@example.com", IsDefault: true})
	a2, _ := svc.CreateAlias(id, account.AliasRequest{Email: "b@example.com", IsDefault: true})

	list, _ := svc.ListAliases(id)
	defaults := 0
	for _, a := range list {
		if a.IsDefault {
			defaults++
			if a.ID != a2.ID {
				t.Errorf("默认别名应是最后置位的 %d，实际 %d", a2.ID, a.ID)
			}
		}
	}
	if defaults != 1 {
		t.Errorf("默认别名应恰好 1 个，实际 %d（a1=%d）", defaults, a1.ID)
	}
}

// TestResolveFromRejectsForeignAlias 这是防伪造的唯一关卡：
// 前端传什么地址都不可信，服务端必须自己查一遍归属。
func TestResolveFromRejectsForeignAlias(t *testing.T) {
	svc, _, _ := newSvc(t)
	mine := newAcct(t, svc, "mine@example.com")
	other := newAcct(t, svc, "other@example.com")
	if _, err := svc.CreateAlias(other, account.AliasRequest{Email: "victim@example.com"}); err != nil {
		t.Fatalf("给另一账户建别名失败: %v", err)
	}

	// 空别名 → 账户主地址
	addr, name, err := svc.ResolveFrom(mine, "")
	if err != nil || addr != "mine@example.com" || name != "Work" {
		t.Errorf("空别名应回落主地址，实际 %q/%q err=%v", addr, name, err)
	}

	// 别的账户的别名 → 拒绝
	if _, _, err := svc.ResolveFrom(mine, "victim@example.com"); !errors.Is(err, account.ErrAliasNotFound) {
		t.Errorf("跨账户别名必须被拒，实际 %v", err)
	}
	// 完全没配过的地址 → 拒绝
	if _, _, err := svc.ResolveFrom(mine, "anyone@evil.com"); !errors.Is(err, account.ErrAliasNotFound) {
		t.Errorf("未配置的地址必须被拒，实际 %v", err)
	}

	// 自己的别名 → 放行，且带显示名
	if _, err := svc.CreateAlias(mine, account.AliasRequest{Email: "sales@example.com", DisplayName: "销售部"}); err != nil {
		t.Fatalf("建别名失败: %v", err)
	}
	addr, name, err = svc.ResolveFrom(mine, "SALES@example.com") // 大小写不敏感
	if err != nil || addr != "sales@example.com" || name != "销售部" {
		t.Errorf("自有别名应放行，实际 %q/%q err=%v", addr, name, err)
	}
}

// TestSignatureSanitized 签名虽由本人编辑，但会被注入撰写器 DOM 并随每封信发出，
// 落库前必须净化。
func TestSignatureSanitized(t *testing.T) {
	svc, _, _ := newSvc(t)
	id := newAcct(t, svc, "main@example.com")

	// 未配置时返回空签名而不是错误
	sig, err := svc.GetSignature(id)
	if err != nil || sig.BodyHTML != "" {
		t.Fatalf("未配置时应返回空签名，实际 %+v err=%v", sig, err)
	}

	saved, err := svc.SaveSignature(id, account.SignatureRequest{
		BodyHTML:   `<p>张三<script>alert(1)</script><img src=x onerror="alert(2)"></p>`,
		UseOnNew:   true,
		UseOnReply: false,
	})
	if err != nil {
		t.Fatalf("保存签名失败: %v", err)
	}
	if strings.Contains(saved.BodyHTML, "script") || strings.Contains(saved.BodyHTML, "onerror") {
		t.Errorf("签名未净化: %q", saved.BodyHTML)
	}
	if !strings.Contains(saved.BodyHTML, "张三") {
		t.Errorf("净化不应吃掉正常内容: %q", saved.BodyHTML)
	}

	got, err := svc.GetSignature(id)
	if err != nil || got.BodyHTML != saved.BodyHTML || !got.UseOnNew || got.UseOnReply {
		t.Errorf("读回不符: %+v err=%v", got, err)
	}
}

// TestDeleteAccountCascadesIdentity 删账户要连带清理别名与签名，
// 否则重建同 id 的账户会捡到上一个账户的发信身份。
func TestDeleteAccountCascadesIdentity(t *testing.T) {
	svc, repo, _ := newSvc(t)
	id := newAcct(t, svc, "main@example.com")
	if _, err := svc.CreateAlias(id, account.AliasRequest{Email: "sales@example.com"}); err != nil {
		t.Fatalf("建别名失败: %v", err)
	}
	if _, err := svc.SaveSignature(id, account.SignatureRequest{BodyHTML: "<p>签名</p>"}); err != nil {
		t.Fatalf("存签名失败: %v", err)
	}

	if err := svc.Delete(id); err != nil {
		t.Fatalf("删账户失败: %v", err)
	}
	if list, err := repo.ListAliases(id); err != nil || len(list) != 0 {
		t.Errorf("别名应被连带清理，实际 %d 条 err=%v", len(list), err)
	}
	sig, err := repo.GetSignature(id)
	if err != nil || sig.BodyHTML != "" {
		t.Errorf("签名应被连带清理，实际 %+v err=%v", sig, err)
	}
}

// TestAliasScopedToAccount 别名操作必须限定在所属账户下。
func TestAliasScopedToAccount(t *testing.T) {
	svc, _, _ := newSvc(t)
	mine := newAcct(t, svc, "mine@example.com")
	other := newAcct(t, svc, "other@example.com")
	a, _ := svc.CreateAlias(other, account.AliasRequest{Email: "x@example.com"})

	if _, err := svc.UpdateAlias(mine, a.ID, account.AliasRequest{Email: "y@example.com"}); !errors.Is(err, account.ErrAliasNotFound) {
		t.Errorf("不能改别的账户的别名，实际 %v", err)
	}
	if err := svc.DeleteAlias(mine, a.ID); !errors.Is(err, account.ErrAliasNotFound) {
		t.Errorf("不能删别的账户的别名，实际 %v", err)
	}
	if list, _ := svc.ListAliases(other); len(list) != 1 {
		t.Errorf("原别名应仍在，实际 %d 条", len(list))
	}
}

// TestAliasUpdateKeepsCreatedAt gorm.Save 对带主键的记录做全字段 UPDATE，
// 拿一个新结构体去存会把 CreatedAt 刷成零值——没有断言就发现不了。
func TestAliasUpdateKeepsCreatedAt(t *testing.T) {
	svc, _, _ := newSvc(t)
	id := newAcct(t, svc, "main@example.com")
	created, err := svc.CreateAlias(id, account.AliasRequest{Email: "a@example.com"})
	if err != nil {
		t.Fatalf("建别名失败: %v", err)
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("新建时 CreatedAt 不应为零值")
	}

	updated, err := svc.UpdateAlias(id, created.ID, account.AliasRequest{
		Email: "a@example.com", DisplayName: "改过",
	})
	if err != nil {
		t.Fatalf("改别名失败: %v", err)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt 应保持不变：建 %v，改后 %v", created.CreatedAt, updated.CreatedAt)
	}

	list, _ := svc.ListAliases(id)
	if len(list) != 1 || list[0].CreatedAt.IsZero() {
		t.Errorf("落库的 CreatedAt 被清空了: %+v", list)
	}
}

// TestAliasRejectsNonAddrSpec 别名表单旁边就有显示名栏，用户极容易把两者写在一起。
// "Bob <bob@e.com>" 这种 name-addr 必须被拒——存下去会让 From 尖括号不配对、Message-ID 多出 >，
// 严格的 MTA 直接 501 拒信，而用户在保存那一刻看到的却是"保存成功"。
func TestAliasRejectsNonAddrSpec(t *testing.T) {
	svc, _, _ := newSvc(t)
	id := newAcct(t, svc, "main@example.com")

	cases := []struct {
		name  string
		input string
	}{
		{"name-addr", "Bob <bob@e.com>"},
		{"name-addr 引号", `"a b"@evil.com`},
		{"尾注（裸 UTF-8）", "a@e.com（备注）"},
		{"域名字面量", "a@[127.0.0.1]"},
		{"双 @", "a@b@c.com"},
		{"引号", "a\"b@c.com"},
		{"尖括号", "a<b@c.com"},
		{"逗号", "a,b@c.com"},
		{"分号", "a;b@c.com"},
		{"控制字符", "a	b@c.com"},
		{">254 字符", strings.Repeat("a", 200) + "@" + strings.Repeat("b", 60) + ".com"},
		{"空", ""},
	}
	for _, tc := range cases {
		if _, err := svc.CreateAlias(id, account.AliasRequest{Email: tc.input}); !errors.Is(err, account.ErrInvalidEmail) {
			t.Errorf("%s: 输入 %q 应被拒，实际 err=%v", tc.name, tc.input, err)
		}
	}

	// 合法的 addr-spec 仍应放行
	if _, err := svc.CreateAlias(id, account.AliasRequest{Email: "sales@example.com"}); err != nil {
		t.Errorf("合法别名被拒: %v", err)
	}
}

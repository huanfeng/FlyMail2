package account_test

import (
	"testing"

	"flymail/modules/email/account"
)

// rawPassword 直接从库里读密文（绕过 Service，因为它刻意不对外暴露密文）。
func rawPassword(t *testing.T, repo *account.Repository, id uint) string {
	t.Helper()
	a, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("取账户 %d: %v", id, err)
	}
	return a.PasswordEnc
}

// seedAcct 建一个可用的密码账户，返回其 ID。
func seedAcct(t *testing.T, svc *account.Service, name, email, pw string) uint {
	t.Helper()
	resp, err := svc.Create(account.CreateAccountRequest{
		Name: name, Email: email, Password: pw,
		IMAPHost: "imap." + name + ".com", IMAPPort: 993, IMAPSecurity: "ssl",
		SMTPHost: "smtp." + name + ".com", SMTPPort: 465, SMTPSecurity: "ssl",
	})
	if err != nil {
		t.Fatalf("建账户 %s: %v", email, err)
	}
	return resp.ID
}

// ── 导出 ────────────────────────────────────────────────────────────────────

func TestExportOmitsPasswordsByDefault(t *testing.T) {
	svc, _, _ := newSvc(t)
	seedAcct(t, svc, "alice", "alice@example.com", "s3cret")

	b, err := svc.Export(nil, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(b.Accounts) != 1 {
		t.Fatalf("导出了 %d 个账户，want 1", len(b.Accounts))
	}
	pa := b.Accounts[0]

	// 判据是字段**不存在**（nil），不是"等于空串"：
	// 空串在导入侧会把已有密码清空，与"这份导出不含密码"是完全不同的两件事。
	if pa.Password != nil {
		t.Errorf("没勾选含密码，导出物里却有密码字段: %q", *pa.Password)
	}
	// 服务器配置照常带上——不导密码不等于不导配置，那才是搬家时真正麻烦的部分
	if pa.IMAPHost != "imap.alice.com" || pa.IMAPPort != 993 || pa.SMTPSecurity != "ssl" {
		t.Errorf("服务器配置没导全: %+v", pa)
	}
}

func TestExportIncludesPasswordsWhenAsked(t *testing.T) {
	svc, _, _ := newSvc(t)
	seedAcct(t, svc, "alice", "alice@example.com", "s3cret")

	b, err := svc.Export(nil, true)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	pa := b.Accounts[0]
	if pa.Password == nil {
		t.Fatal("勾了含密码却没导出密码")
	}
	// 必须是**明文**：库里是 AES 密文，而密钥随部署走，
	// 原样搬过去解不开等于没导（见 portable.go 顶部说明）。
	if *pa.Password != "s3cret" {
		t.Errorf("导出的密码 = %q，want 明文 s3cret", *pa.Password)
	}
	// 文件要自己说明白自己是什么，导入侧才有判据
	if b.Encrypted {
		t.Error("Encrypted 标成了 true，但这份导出确实是明文")
	}
	if b.Version != account.PortableVersion {
		t.Errorf("Version = %d, want %d", b.Version, account.PortableVersion)
	}
}

func TestExportSelectsByID(t *testing.T) {
	svc, _, _ := newSvc(t)
	seedAcct(t, svc, "alice", "alice@example.com", "p1")
	bobID := seedAcct(t, svc, "bob", "bob@example.com", "p2")
	seedAcct(t, svc, "carol", "carol@example.com", "p3")

	b, err := svc.Export([]uint{bobID}, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(b.Accounts) != 1 || b.Accounts[0].Email != "bob@example.com" {
		t.Fatalf("按 id 选择失败: %+v", b.Accounts)
	}
}

func TestExportEmptyIDsMeansAll(t *testing.T) {
	svc, _, _ := newSvc(t)
	seedAcct(t, svc, "alice", "alice@example.com", "p1")
	seedAcct(t, svc, "bob", "bob@example.com", "p2")

	b, err := svc.Export(nil, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(b.Accounts) != 2 {
		t.Errorf("空 id 列表应导出全部，实际 %d 个", len(b.Accounts))
	}
}

// ── 导入 ────────────────────────────────────────────────────────────────────

// bundleOf 造一份最小可用的导入文件。
func bundleOf(accs ...account.PortableAccount) *account.PortableBundle {
	return &account.PortableBundle{Version: account.PortableVersion, Accounts: accs}
}

func portable(email string, pw *string) account.PortableAccount {
	return account.PortableAccount{
		Name: email, Email: email, Password: pw,
		AuthType: account.AuthTypePassword,
		IMAPHost: "imap.new.com", IMAPPort: 993, IMAPSecurity: "ssl",
		SMTPHost: "smtp.new.com", SMTPPort: 465, SMTPSecurity: "ssl",
		Enabled: true,
	}
}

func TestImportCreatesNewAccounts(t *testing.T) {
	svc, repo, enc := newSvc(t)
	pw := "brought-over"

	res, err := svc.Import(bundleOf(portable("new@example.com", &pw)), account.ImportSkip, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Created != 1 || res.Failed != 0 {
		t.Fatalf("结果不对: %+v", res)
	}

	list, _ := svc.List()
	if len(list) != 1 || list[0].Email != "new@example.com" {
		t.Fatalf("账户没建起来: %+v", list)
	}
	// 密码要落成密文，不能原样存明文
	raw := rawPassword(t, repo, list[0].ID)
	if raw == "" || raw == pw {
		t.Errorf("密码没有加密落库: %q", raw)
	}
	if got, _ := enc.Decrypt(raw); got != pw {
		t.Errorf("解密回来 = %q, want %q", got, pw)
	}
}

func TestImportSkipsExistingByDefault(t *testing.T) {
	svc, _, _ := newSvc(t)
	seedAcct(t, svc, "alice", "alice@example.com", "original")
	pw := "from-file"

	res, err := svc.Import(bundleOf(portable("alice@example.com", &pw)), account.ImportSkip, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Skipped != 1 || res.Updated != 0 || res.Created != 0 {
		t.Fatalf("默认应跳过已存在的: %+v", res)
	}
	// 原账户分毫未动
	list, _ := svc.List()
	if list[0].IMAPHost != "imap.alice.com" {
		t.Errorf("跳过模式却改了现有账户: %s", list[0].IMAPHost)
	}
}

func TestImportOverwriteUpdatesExisting(t *testing.T) {
	svc, _, _ := newSvc(t)
	seedAcct(t, svc, "alice", "alice@example.com", "original")
	pw := "from-file"

	res, err := svc.Import(bundleOf(portable("alice@example.com", &pw)), account.ImportOverwrite, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Updated != 1 || res.Created != 0 {
		t.Fatalf("覆盖模式结果不对: %+v", res)
	}
	list, _ := svc.List()
	if list[0].IMAPHost != "imap.new.com" {
		t.Errorf("覆盖没生效: %s", list[0].IMAPHost)
	}
	if len(list) != 1 {
		t.Errorf("覆盖模式建出了重复账户，共 %d 个", len(list))
	}
}

// TestImportWithoutPasswordKeepsExistingOne 是最危险的一条。
//
// 用户拿一份**不含密码**的导出（默认就是这样）去覆盖现有账户，本意是同步
// 服务器地址和端口。若把 nil 当成"空密码"写进去，一个能用的账户会瞬间变成
// 连不上，而且原密码已经没了——用户当场就得去邮箱服务商那里重新找密码。
func TestImportWithoutPasswordKeepsExistingOne(t *testing.T) {
	svc, repo, enc := newSvc(t)
	id := seedAcct(t, svc, "alice", "alice@example.com", "original")

	// Password 为 nil = 这份导出没勾"含密码"
	res, err := svc.Import(bundleOf(portable("alice@example.com", nil)), account.ImportOverwrite, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("结果不对: %+v", res)
	}

	raw := rawPassword(t, repo, id)
	got, err := enc.Decrypt(raw)
	if err != nil {
		t.Fatalf("解密失败——密码很可能被清空了: %v", err)
	}
	if got != "original" {
		t.Errorf("密码被导入覆盖成了 %q，原密码已丢失", got)
	}
	// 而服务器配置**应该**被更新——这才是用户导入的目的
	list, _ := svc.List()
	if list[0].IMAPHost != "imap.new.com" {
		t.Errorf("该更新的配置没更新: %s", list[0].IMAPHost)
	}
}

func TestImportOnlySelectedEmails(t *testing.T) {
	svc, _, _ := newSvc(t)
	pw := "x"
	b := bundleOf(
		portable("a@example.com", &pw),
		portable("b@example.com", &pw),
		portable("c@example.com", &pw),
	)

	res, err := svc.Import(b, account.ImportSkip, []string{"b@example.com"})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("只该导入一个: %+v", res)
	}
	list, _ := svc.List()
	if len(list) != 1 || list[0].Email != "b@example.com" {
		t.Fatalf("挑选导入失败: %+v", list)
	}
}

func TestImportEmailMatchIsCaseInsensitive(t *testing.T) {
	// 邮箱地址的域名部分大小写不敏感，而用户手输时大小写很随意。
	// 按字节比对会让 Alice@Example.com 建出第二个账户，
	// 两个账户同步同一个邮箱、未读数互相打架。
	svc, _, _ := newSvc(t)
	seedAcct(t, svc, "alice", "alice@example.com", "original")

	res, err := svc.Import(bundleOf(portable("Alice@Example.com", nil)), account.ImportSkip, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Skipped != 1 {
		t.Fatalf("大小写不同的同一地址被当成了新账户: %+v", res)
	}
}

func TestImportRejectsNewerVersion(t *testing.T) {
	svc, _, _ := newSvc(t)
	b := bundleOf(portable("a@example.com", nil))
	b.Version = account.PortableVersion + 1

	_, err := svc.Import(b, account.ImportSkip, nil)
	if err == nil {
		t.Fatal("未来版本的文件被照单全收了——字段语义可能已经变了")
	}
}

func TestImportRejectsEncryptedBundle(t *testing.T) {
	// 当前版本不会解密。必须明确拒绝并说清楚，而不是把密文当明文存进去——
	// 那会得到一个"密码是一串乱码"的账户，用户完全看不出哪里错了。
	svc, _, _ := newSvc(t)
	b := bundleOf(portable("a@example.com", nil))
	b.Encrypted = true

	_, err := svc.Import(b, account.ImportSkip, nil)
	if err == nil {
		t.Fatal("加密文件被当成明文处理了")
	}
}

func TestImportReportsPerAccountFailures(t *testing.T) {
	// 一个账户字段残缺不该拖垮其余的：用户手改过 JSON 是常态。
	svc, _, _ := newSvc(t)
	pw := "x"
	bad := portable("broken@example.com", &pw)
	bad.IMAPHost = ""

	res, err := svc.Import(bundleOf(bad, portable("good@example.com", &pw)), account.ImportSkip, nil)
	if err != nil {
		t.Fatalf("整体不该失败: %v", err)
	}
	if res.Created != 1 || res.Failed != 1 {
		t.Fatalf("应当一成一败: %+v", res)
	}
	var sawErr bool
	for _, o := range res.Outcomes {
		if o.Email == "broken@example.com" {
			if o.Action != "failed" || o.Error == "" {
				t.Errorf("失败项没有说明原因: %+v", o)
			}
			sawErr = true
		}
	}
	if !sawErr {
		t.Error("结果里没有那条失败记录")
	}
	list, _ := svc.List()
	if len(list) != 1 || list[0].Email != "good@example.com" {
		t.Errorf("好的那个没导进来: %+v", list)
	}
}

// ── 往返 ────────────────────────────────────────────────────────────────────

func TestExportImportRoundTrip(t *testing.T) {
	// 这条模拟真实用途：A 机导出 → B 机导入 → 配置一致。
	src, _, _ := newSvc(t)
	seedAcct(t, src, "alice", "alice@example.com", "s3cret")

	b, err := src.Export(nil, true)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	dst, dstRepo, enc := newSvc(t) // 另一台机器（另一套库）
	res, err := dst.Import(b, account.ImportSkip, nil)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Created != 1 {
		t.Fatalf("往返失败: %+v", res)
	}

	list, _ := dst.List()
	got := list[0]
	if got.Email != "alice@example.com" || got.IMAPHost != "imap.alice.com" || got.IMAPPort != 993 {
		t.Errorf("配置没搬全: %+v", got)
	}
	raw := rawPassword(t, dstRepo, got.ID)
	if pw, _ := enc.Decrypt(raw); pw != "s3cret" {
		t.Errorf("密码没搬过来: %q", pw)
	}
}

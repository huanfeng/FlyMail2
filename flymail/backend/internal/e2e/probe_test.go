package e2e

import "testing"

// TestProbe_GreenMailCapabilities 第一个真正连 GreenMail 的测试：
// 摸清默认文件夹集合、IDLE 能力，输出诊断日志，供 verbs/realtime 测试自适应。
func TestProbe_GreenMailCapabilities(t *testing.T) {
	requireE2E(t)
	mb := uniqueMailbox(t)
	sendSeed(t, "probe@localhost", mb, "probe-subject", "probe-body")
	sess := imapConnect(t, mb)

	folders, err := sess.ListFolders()
	if err != nil {
		t.Fatalf("ListFolders: %v", err)
	}
	t.Logf("GreenMail 默认文件夹(%d 个):", len(folders))
	for _, f := range folders {
		t.Logf("  path=%q name=%q attrs=%v", f.Path, f.Name, f.Attributes)
	}
	t.Logf("SupportsIDLE=%v", sess.CanIDLE())

	sel, err := sess.SelectFolder("INBOX")
	if err != nil {
		t.Fatalf("SelectFolder INBOX: %v", err)
	}
	t.Logf("INBOX: NumMessages=%d UIDValidity=%d UIDNext=%d", sel.NumMessages, sel.UIDValidity, sel.UIDNext)
	if sel.NumMessages != 1 {
		t.Fatalf("种子邮件未投递: NumMessages=%d 期望 1", sel.NumMessages)
	}
	emails, err := sess.FetchBySeqRange(1, 1, coreimapFetchHeaders())
	if err != nil {
		t.Fatalf("FetchBySeqRange: %v", err)
	}
	if len(emails) != 1 || emails[0].Subject != "probe-subject" {
		t.Fatalf("种子邮件主题不符: %+v", emails)
	}
	t.Logf("种子邮件: uid=%d subject=%q seen=%v flags=%v", emails[0].UID, emails[0].Subject, emails[0].IsRead, emails[0].Flags)
}

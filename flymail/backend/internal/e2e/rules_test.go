package e2e

import (
	"net/http"
	"testing"
	"time"
)

// TestRules_InboundChain 规则链路（GreenMail）：账户基线同步后建规则与黑名单 → 投递三封 →
// 增量同步 → 命中规则的落到目标文件夹并已读、黑名单的进回收站、普通的留在收件箱；
// 服务器侧状态一致；通知只提醒普通那封；执行日志两条；再同步一次不重复执行。
func TestRules_InboundChain(t *testing.T) {
	requireE2E(t)
	ta := newTestApp(t, false)
	c := newClient(t, ta)
	mb := uniqueMailbox(t)
	acctID := c.createAccount(mb)

	const archive = "E2E-Archive"
	sess := imapConnect(t, mb)
	for _, name := range []string{archive, "Trash"} {
		if err := sess.Client.Create(name, nil).Wait(); err != nil {
			t.Fatalf("CREATE %s: %v", name, err)
		}
	}
	// 基线同步：把文件夹同步进来（规则对基线导入不生效，这里收件箱也还是空的）
	c.triggerSyncAndWait(acctID, 60*time.Second)
	folders := c.listFolders(acctID)
	inbox := findFolder(folders, "inbox")
	trash := findFolder(folders, "trash")
	if inbox == nil || trash == nil {
		t.Fatalf("folders after baseline: %+v", folders)
	}

	var created struct {
		ID uint `json:"id"`
	}
	c.mustJSON(http.MethodPost, "/api/v1/rules", map[string]any{
		"name": "归档 seeder", "match": "all",
		"conditions": []map[string]string{{"field": "from", "op": "contains", "value": "seeder@localhost"}},
		"actions":    []map[string]string{{"type": "move", "value": archive}, {"type": "mark_read"}},
	}, http.StatusCreated, &created)
	c.mustJSON(http.MethodPost, "/api/v1/blocklist", map[string]string{"pattern": "spammer@localhost"}, http.StatusCreated, nil)

	sendSeed(t, "seeder@localhost", mb, "rule-hit", "should be archived")
	sendSeed(t, "spammer@localhost", mb, "blocked", "should go to trash")
	sendSeed(t, "friend@localhost", mb, "plain", "stays in inbox")
	c.triggerSyncAndWait(acctID, 60*time.Second)

	msgs := c.listMessages(inbox.ID)
	if len(msgs) != 1 || msgs[0].Subject != "plain" || msgs[0].Seen {
		t.Fatalf("inbox after rules: %+v", msgs)
	}
	if f := findFolder(c.listFolders(acctID), "inbox"); f == nil || f.UnreadCount != 1 || f.TotalCount != 1 {
		t.Errorf("inbox counts should reflect post-rule state: %+v", f)
	}

	// 服务器侧：归档文件夹 1 封且已读，回收站 1 封，收件箱 1 封
	eventually(t, 30*time.Second, 500*time.Millisecond, "rule actions written back", func() bool {
		sel, err := sess.SelectFolder(archive)
		if err != nil || sel.NumMessages != 1 {
			return false
		}
		emails, err := sess.FetchByUIDRange(1, 0, coreimapFetchHeaders())
		if err != nil || len(emails) != 1 || !emails[0].IsRead || emails[0].Subject != "rule-hit" {
			return false
		}
		if sel, err := sess.SelectFolder("Trash"); err != nil || sel.NumMessages != 1 {
			return false
		}
		sel, err = sess.SelectFolder("INBOX")
		return err == nil && sel.NumMessages == 1
	})

	// 通知：只有普通那封触发「新邮件」，标题带发件人
	var notes struct {
		Notifications []struct {
			Type  string `json:"type"`
			Title string `json:"title"`
			Body  string `json:"body"`
		} `json:"notifications"`
	}
	c.mustJSON(http.MethodGet, "/api/v1/notifications", nil, http.StatusOK, &notes)
	mailNew := 0
	for _, n := range notes.Notifications {
		if n.Type == "mail_new" {
			mailNew++
			if n.Body != "plain" {
				t.Errorf("mail_new should be about the plain message only: %+v", n)
			}
		}
	}
	if mailNew != 1 {
		t.Errorf("expected exactly one mail_new notification, got %d: %+v", mailNew, notes.Notifications)
	}

	// 执行日志：规则 1 条 + 黑名单 1 条
	var runs struct {
		Runs []struct {
			RuleID uint   `json:"rule_id"`
			Action string `json:"action"`
		} `json:"runs"`
	}
	c.mustJSON(http.MethodGet, "/api/v1/rules/runs", nil, http.StatusOK, &runs)
	if len(runs.Runs) != 2 {
		t.Fatalf("runs: %+v", runs.Runs)
	}
	seen := map[uint]string{}
	for _, r := range runs.Runs {
		seen[r.RuleID] = r.Action
	}
	if seen[created.ID] != "move:"+archive+",mark_read" || seen[0] != "block:spammer@localhost" {
		t.Errorf("run actions: %v", seen)
	}

	// 幂等：再同步一次，什么都不该变（没有新邮件，也不会重复执行）
	c.triggerSyncAndWait(acctID, 60*time.Second)
	c.mustJSON(http.MethodGet, "/api/v1/rules/runs", nil, http.StatusOK, &runs)
	if len(runs.Runs) != 2 {
		t.Errorf("re-sync must not add runs: %+v", runs.Runs)
	}

	// 试运行只读：对现有收件箱求值，命中 0（seeder 那封已经不在收件箱）
	var test struct {
		Matched []any `json:"matched"`
		Scanned int   `json:"scanned"`
	}
	c.mustJSON(http.MethodPost, "/api/v1/rules/test", map[string]any{"rule": map[string]any{
		"name": "t", "match": "all",
		"conditions": []map[string]string{{"field": "subject", "op": "equals", "value": "plain"}},
		"actions":    []map[string]string{{"type": "delete"}},
	}}, http.StatusOK, &test)
	if test.Scanned != 1 || len(test.Matched) != 1 {
		t.Errorf("dry run: %+v", test)
	}
	if got := c.listMessages(inbox.ID); len(got) != 1 {
		t.Errorf("dry run must not delete: %+v", got)
	}
}

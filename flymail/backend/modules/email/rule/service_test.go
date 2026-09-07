package rule_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"flymail-core/types"

	"flymail/internal/database"
	"flymail/modules/email/folder"
	"flymail/modules/email/message"
	"flymail/modules/email/rule"
)

// fakeActor 记录动作调用，并像真实 sync.Service 一样把移动/删除的行从本地删掉
// （规则引擎依赖这一点决定动作顺序）。
type fakeActor struct {
	mrepo   *message.Repository
	calls   []string
	moved   map[uint][]uint // 目标文件夹 → ids
	deleted []uint
	read    []uint
	starred []uint
}

func (f *fakeActor) BatchMove(ids []uint, target uint) error {
	f.calls = append(f.calls, fmt.Sprintf("move:%d:%d", target, len(ids)))
	f.moved[target] = append(f.moved[target], ids...)
	return f.mrepo.DeleteByIDs(ids)
}
func (f *fakeActor) BatchDelete(ids []uint) error {
	f.calls = append(f.calls, fmt.Sprintf("delete:%d", len(ids)))
	f.deleted = append(f.deleted, ids...)
	return f.mrepo.DeleteByIDs(ids)
}
func (f *fakeActor) BatchSetRead(ids []uint, read bool) error {
	f.calls = append(f.calls, fmt.Sprintf("read:%d", len(ids)))
	f.read = append(f.read, ids...)
	return f.mrepo.SetSeenByIDs(ids, read)
}
func (f *fakeActor) BatchSetFlagged(ids []uint, flagged bool) error {
	f.calls = append(f.calls, fmt.Sprintf("star:%d", len(ids)))
	f.starred = append(f.starred, ids...)
	return f.mrepo.SetFlaggedByIDs(ids, flagged)
}

type env struct {
	svc     *rule.Service
	actor   *fakeActor
	mrepo   *message.Repository
	msvc    *message.Service
	inbox   *folder.Folder
	archive *folder.Folder
	junk    *folder.Folder
	emitted []string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	folders := []folder.Folder{
		{ID: 1, AccountID: 1, Path: "INBOX", DisplayName: "收件箱", Type: "inbox", Selectable: true},
		{ID: 2, AccountID: 1, Path: "Archive", DisplayName: "归档", Type: "custom", Selectable: true},
		{ID: 3, AccountID: 1, Path: "Junk", DisplayName: "垃圾邮件", Type: "junk", Selectable: true},
		{ID: 4, AccountID: 2, Path: "INBOX", DisplayName: "Inbox", Type: "inbox", Selectable: true},
	}
	if err := db.Create(&folders).Error; err != nil {
		t.Fatal(err)
	}
	mrepo := message.NewRepository(db)
	msvc := message.NewService(mrepo, message.NewBodyRepository(db))
	fsvc := folder.NewService(folder.NewRepository(db))
	e := &env{mrepo: mrepo, msvc: msvc, inbox: &folders[0], archive: &folders[1], junk: &folders[2]}
	e.actor = &fakeActor{mrepo: mrepo, moved: map[uint][]uint{}}
	e.svc = rule.NewService(rule.NewRepository(db), fsvc, msvc)
	e.svc.SetActor(e.actor)
	e.svc.SetEmitter(func(eventType string, accountID uint, messageID uint, title, body string) {
		e.emitted = append(e.emitted, eventType+"|"+title+"|"+body)
	})
	return e
}

func (e *env) seed(t *testing.T, uid uint32, from, subject string) message.Message {
	t.Helper()
	m := &message.Message{AccountID: 1, FolderID: 1, UID: uid, MessageID: fmt.Sprintf("m%d@x", uid),
		FromName: "", FromAddr: from, Subject: subject, Date: time.Now()}
	if err := e.mrepo.Upsert(m); err != nil {
		t.Fatal(err)
	}
	got, _ := e.mrepo.GetByFolderUID(1, uid)
	return *got
}

func (e *env) apply(t *testing.T, msgs ...message.Message) bool {
	t.Helper()
	changed, err := e.svc.Apply(1, e.inbox, msgs)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return changed
}

func mustCreate(t *testing.T, e *env, in rule.RuleInput) rule.RuleDTO {
	t.Helper()
	d, err := e.svc.Create(in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return *d
}

func TestApplyBlocklistAndRules(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.AddBlock("Spam.io", "广告"); err != nil {
		t.Fatal(err)
	}
	mustCreate(t, e, rule.RuleInput{Name: "发票归档", Match: rule.MatchAll,
		Conditions:     []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "发票"}},
		Actions:        []rule.Action{{Type: rule.ActionMove, Value: "归档"}, {Type: rule.ActionMarkRead}},
		StopProcessing: true})
	mustCreate(t, e, rule.RuleInput{Name: "全部加星", Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldFrom, Op: rule.OpContains, Value: "@"}},
		Actions:    []rule.Action{{Type: rule.ActionStar}}})

	spam := e.seed(t, 1, "ads@mail.spam.io", "促销")
	invoice := e.seed(t, 2, "acct@corp.cn", "三月发票")
	plain := e.seed(t, 3, "bob@x.io", "hello")

	if !e.apply(t, spam, invoice, plain) {
		t.Fatal("should report changed")
	}
	// 黑名单 → 垃圾邮件文件夹；发票 → 已读 + 归档，且 stop 后不再加星；plain 只加星
	if ids := e.actor.moved[e.junk.ID]; len(ids) != 1 || ids[0] != spam.ID {
		t.Errorf("blocked should move to junk: %v", e.actor.moved)
	}
	if ids := e.actor.moved[e.archive.ID]; len(ids) != 1 || ids[0] != invoice.ID {
		t.Errorf("invoice should move to archive: %v", e.actor.moved)
	}
	if len(e.actor.read) != 1 || e.actor.read[0] != invoice.ID {
		t.Errorf("read: %v", e.actor.read)
	}
	if len(e.actor.starred) != 1 || e.actor.starred[0] != plain.ID {
		t.Errorf("starred: %v (stop_processing should skip the star rule for invoice)", e.actor.starred)
	}
	// 动作顺序：规则的标志位动作在移动之前（移动会删本地行）；黑名单移动独立在前
	idx := func(s string) int {
		for i, c := range e.actor.calls {
			if c == s {
				return i
			}
		}
		return -1
	}
	if idx("read:1") < 0 || idx("move:2:1") < 0 || idx("read:1") > idx("move:2:1") || idx("star:1") > idx("move:2:1") {
		t.Errorf("flag ops should run before rule moves: %v", e.actor.calls)
	}
	// 执行日志：黑名单 rule_id=0
	runs, _ := e.svc.ListRuns(10)
	if len(runs) != 3 {
		t.Fatalf("runs: %+v", runs)
	}
	var blockRun *rule.RuleRun
	for i := range runs {
		if runs[i].RuleID == 0 {
			blockRun = &runs[i]
		}
	}
	if blockRun == nil || blockRun.Action != "block:spam.io" || blockRun.MessageKey != spam.MessageID {
		t.Errorf("block run: %+v", blockRun)
	}

	// 幂等：同一批再来一次（UIDVALIDITY 重建后主键会变，这里模拟同 Message-ID 新行）
	e.actor.calls = nil
	invoice2 := e.seed(t, 12, "acct@corp.cn", "三月发票")
	invoice2.MessageID = invoice.MessageID
	if err := e.mrepo.Upsert(&invoice2); err != nil {
		t.Fatal(err)
	}
	if e.apply(t, invoice2) {
		t.Errorf("already-processed message must not trigger actions again: %v", e.actor.calls)
	}
}

func TestApplyDeleteWinsOverMoveAndScopes(t *testing.T) {
	e := newEnv(t)
	mustCreate(t, e, rule.RuleInput{Name: "归档", Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "x"}},
		Actions:    []rule.Action{{Type: rule.ActionMove, Value: "归档"}}})
	mustCreate(t, e, rule.RuleInput{Name: "删除", Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "x"}},
		Actions:    []rule.Action{{Type: rule.ActionDelete}}})
	// 只对账户 2 生效的规则不该碰账户 1
	mustCreate(t, e, rule.RuleInput{Name: "他人", AccountID: 2, Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "x"}},
		Actions:    []rule.Action{{Type: rule.ActionStar}}})
	m := e.seed(t, 1, "a@b.c", "x")
	e.apply(t, m)
	if len(e.actor.deleted) != 1 || len(e.actor.moved) != 0 || len(e.actor.starred) != 0 {
		t.Errorf("delete should win and account-2 rule must not apply: deleted=%v moved=%v starred=%v",
			e.actor.deleted, e.actor.moved, e.actor.starred)
	}
	// 全账户规则的目标文件夹保存时不校验；运行期解析不到就跳过移动并在日志摘要里标 missing
	mustCreate(t, e, rule.RuleInput{Name: "无此文件夹", Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "y"}},
		Actions:    []rule.Action{{Type: rule.ActionMove, Value: "不存在"}}})
	e.actor.calls = nil
	e.apply(t, e.seed(t, 2, "a@b.c", "y"))
	if len(e.actor.calls) != 0 {
		t.Errorf("missing target folder must be a no-op: %v", e.actor.calls)
	}
	runs, _ := e.svc.ListRuns(1)
	if len(runs) != 1 || runs[0].Action != "move:不存在(missing)" {
		t.Errorf("run should record the skipped move: %+v", runs)
	}
	// 限定账户的规则：目标文件夹必须存在，保存直接拒绝
	if _, err := e.svc.Create(rule.RuleInput{Name: "bad", AccountID: 1, Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "z"}},
		Actions:    []rule.Action{{Type: rule.ActionMove, Value: "不存在"}}}); !errors.Is(err, rule.ErrInvalid) {
		t.Errorf("account-scoped rule with unknown folder must fail: %v", err)
	}
	if _, err := e.svc.Create(rule.RuleInput{Name: "ok", AccountID: 1, Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "z"}},
		Actions:    []rule.Action{{Type: rule.ActionMove, Value: "archive"}}}); err != nil { // 按 path 大小写不敏感
		t.Errorf("existing folder by path should pass: %v", err)
	}
}

// TestApplyFirstMoveWins：两条规则都要移动同一封时，优先级高的那条决定去向，另一条记 skipped。
func TestApplyFirstMoveWins(t *testing.T) {
	e := newEnv(t)
	mustCreate(t, e, rule.RuleInput{Name: "去归档", Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "x"}},
		Actions:    []rule.Action{{Type: rule.ActionMove, Value: "归档"}}})
	mustCreate(t, e, rule.RuleInput{Name: "去垃圾", Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "x"}},
		Actions:    []rule.Action{{Type: rule.ActionMove, Value: "Junk"}}})
	m := e.seed(t, 1, "a@b.c", "x")
	for i := 0; i < 5; i++ { // 多跑几次：结果不能依赖 map 迭代顺序
		e2 := e
		if i > 0 {
			e2 = newEnv(t)
			mustCreate(t, e2, rule.RuleInput{Name: "去归档", Match: rule.MatchAll,
				Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "x"}},
				Actions:    []rule.Action{{Type: rule.ActionMove, Value: "归档"}}})
			mustCreate(t, e2, rule.RuleInput{Name: "去垃圾", Match: rule.MatchAll,
				Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "x"}},
				Actions:    []rule.Action{{Type: rule.ActionMove, Value: "Junk"}}})
			m = e2.seed(t, 1, "a@b.c", "x")
		}
		e2.apply(t, m)
		if len(e2.actor.moved[e2.archive.ID]) != 1 || len(e2.actor.moved[e2.junk.ID]) != 0 {
			t.Fatalf("run %d: first rule's move must win: %v", i, e2.actor.moved)
		}
		runs, _ := e2.svc.ListRuns(10)
		actions := map[string]string{}
		for _, r := range runs {
			actions[r.RuleName] = r.Action
		}
		if actions["去归档"] != "move:归档" || actions["去垃圾"] != "move:Junk(skipped)" {
			t.Errorf("run %d: actions %v", i, actions)
		}
	}
}

func TestApplyNotifyAggregation(t *testing.T) {
	e := newEnv(t)
	mustCreate(t, e, rule.RuleInput{Name: "老板来信", Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldFrom, Op: rule.OpEquals, Value: "boss@corp.cn"}},
		Actions:    []rule.Action{{Type: rule.ActionNotify}}})
	var batch []message.Message
	for i := uint32(1); i <= 2; i++ {
		batch = append(batch, e.seed(t, i, "boss@corp.cn", fmt.Sprintf("任务 %d", i)))
	}
	if e.apply(t, batch...) {
		t.Errorf("notify-only rule does not change messages")
	}
	if len(e.emitted) != 2 || e.emitted[0] != "mail_rule|规则命中 · 老板来信|任务 1" {
		t.Errorf("per-message notify: %v", e.emitted)
	}
	e.emitted = nil
	batch = nil
	for i := uint32(10); i < 15; i++ {
		batch = append(batch, e.seed(t, i, "boss@corp.cn", "many"))
	}
	e.apply(t, batch...)
	if len(e.emitted) != 1 || e.emitted[0] != "mail_rule|规则命中 · 老板来信|命中 5 封新邮件" {
		t.Errorf("aggregated notify: %v", e.emitted)
	}
}

func TestTestIsReadOnlyAndCountsMissingBodies(t *testing.T) {
	e := newEnv(t)
	e.seed(t, 1, "a@b.c", "发票 A")
	withBody := e.seed(t, 2, "a@b.c", "发票 B")
	if err := e.msvc.StoreParsedBody(withBody.ID, &types.ParsedEmail{TextBody: "请查收"}); err != nil {
		t.Fatal(err)
	}
	e.seed(t, 3, "z@b.c", "其它")
	res, err := e.svc.Test(rule.RuleInput{Name: "t", Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: rule.FieldBody, Op: rule.OpContains, Value: "查收"}},
		Actions:    []rule.Action{{Type: rule.ActionDelete}}}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 3 || res.WithoutBody != 2 || len(res.Matched) != 1 || res.Matched[0].Subject != "发票 B" {
		t.Errorf("test result: %+v", res)
	}
	if len(e.actor.calls) != 0 {
		t.Errorf("dry run must not act: %v", e.actor.calls)
	}
	if runs, _ := e.svc.ListRuns(10); len(runs) != 0 {
		t.Errorf("dry run must not record runs: %+v", runs)
	}
	// 非法规则直接 400 级错误
	if _, err := e.svc.Test(rule.RuleInput{Name: "bad", Match: rule.MatchAll,
		Conditions: []rule.Condition{{Field: "header", Op: rule.OpContains, Value: "x"}},
		Actions:    []rule.Action{{Type: rule.ActionStar}}}, 10); !errors.Is(err, rule.ErrInvalid) {
		t.Errorf("want ErrInvalid, got %v", err)
	}
}

func TestBlocklistAndReorder(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.AddBlock("bad address", ""); !errors.Is(err, rule.ErrInvalid) {
		t.Errorf("want ErrInvalid, got %v", err)
	}
	if _, err := e.svc.AddBlock("Spam@X.com", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddBlock("spam@x.com", ""); !errors.Is(err, rule.ErrDuplicate) {
		t.Errorf("want ErrDuplicate, got %v", err)
	}
	// 本地账户自己的地址不能拉黑（右键点在自己发的邮件上）
	e.svc.SetSelfAddresses(func() []string { return []string{"Me@Work.com"} })
	if _, err := e.svc.AddBlock("me@work.com", ""); !errors.Is(err, rule.ErrInvalid) {
		t.Errorf("blocking own address must fail: %v", err)
	}
	// 陌生 id 的重排返回 ErrInvalid
	if err := e.svc.Reorder([]uint{9999}); !errors.Is(err, rule.ErrInvalid) {
		t.Errorf("reorder with unknown id: %v", err)
	}
	a := mustCreate(t, e, rule.RuleInput{Name: "a", Match: rule.MatchAll, Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "a"}}, Actions: []rule.Action{{Type: rule.ActionStar}}})
	b := mustCreate(t, e, rule.RuleInput{Name: "b", Match: rule.MatchAll, Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "b"}}, Actions: []rule.Action{{Type: rule.ActionStar}}})
	c := mustCreate(t, e, rule.RuleInput{Name: "c", Match: rule.MatchAll, Conditions: []rule.Condition{{Field: rule.FieldSubject, Op: rule.OpContains, Value: "c"}}, Actions: []rule.Action{{Type: rule.ActionStar}}})
	if a.Priority >= b.Priority || b.Priority >= c.Priority {
		t.Errorf("new rules append: %d %d %d", a.Priority, b.Priority, c.Priority)
	}
	if err := e.svc.Reorder([]uint{c.ID, a.ID}); err != nil {
		t.Fatal(err)
	}
	list, _ := e.svc.List()
	if len(list) != 3 || list[0].Name != "c" || list[1].Name != "a" || list[2].Name != "b" {
		t.Errorf("reorder: %+v", list)
	}
	if _, err := e.svc.Update(a.ID, rule.RuleInput{Name: "a2", Match: rule.MatchAny, Conditions: a.Conditions, Actions: a.Actions}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Delete(b.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Delete(b.ID); !errors.Is(err, rule.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	list, _ = e.svc.List()
	if len(list) != 2 || list[1].Name != "a2" || list[1].Match != rule.MatchAny {
		t.Errorf("after update/delete: %+v", list)
	}
}

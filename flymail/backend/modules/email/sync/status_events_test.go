package sync

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"

	"flymail/modules/email/folder"
)

// statusMutationCase 是「某个变更方法 → 它应当推出去的快照」。
type statusMutationCase struct {
	name string // 必须等于 statusStore 上的方法名（下面那条守卫据此比对）
	call func(s *statusStore)
	want func(st Status) bool
}

// statusMutationCases 列出所有会改状态的方法。
// ⚠ 这张表是手写的，它本身证明不了「没有遗漏」——那条保证由
// TestStatusStoreMutatorsAllCovered 去解析源码给出。
func statusMutationCases() []statusMutationCase {
	return []statusMutationCase{
		{"begin", func(s *statusStore) { s.begin(1, PhaseQueued) },
			func(st Status) bool { return st.Phase == PhaseQueued }},
		{"markPhase", func(s *statusStore) { s.markPhase(1, PhaseFolders) },
			func(st Status) bool { return st.Phase == PhaseFolders }},
		{"beginFolders", func(s *statusStore) { s.beginFolders(1, 7) },
			func(st Status) bool { return st.FoldersTotal == 7 && st.FoldersDone == 0 }},
		{"enterFolder", func(s *statusStore) { s.enterFolder(1, "收件箱", "inbox") },
			func(st Status) bool { return st.CurrentFolder == "收件箱" && st.CurrentFolderType == "inbox" }},
		{"finishFolder", func(s *statusStore) { s.finishFolder(1) },
			func(st Status) bool { return st.FoldersDone == 1 }},
		{"update", func(s *statusStore) { s.update(1, func(st *Status) { st.FoldersTotal = 42 }) },
			func(st Status) bool { return st.FoldersTotal == 42 }},
		{"markDone", func(s *statusStore) { s.markDone(1) },
			func(st Status) bool { return st.Phase == PhaseDone }},
		{"fail", func(s *statusStore) { s.fail(1, "boom") },
			func(st Status) bool { return st.Phase == PhaseError && st.Error == "boom" }},
		{"beginBodies", func(s *statusStore) { s.beginBodies(1, 200) },
			func(st Status) bool { return st.BodiesTotal == 200 && st.BodiesDone == 0 }},
		{"advanceBodies", func(s *statusStore) { s.advanceBodies(1, 40) },
			func(st Status) bool { return st.BodiesDone == 40 }},
		{"endBodies", func(s *statusStore) { s.endBodies(1) },
			func(st Status) bool { return st.BodiesTotal == 0 && st.BodiesDone == 0 }},
	}
}

// TestStatusStoreNotifiesEveryMutation 逐个验证：表里的每个变更方法都会推一次快照。
//
// ⚠ 它**证明不了**「没有哪条写路径绕过 onChange」——表是手写的，新增一个绕过
// mutate 的方法时它不会红也不会提醒。若在这里那样宣称，就是"证明比宣称弱一档"，
// 比不写注释更糟：下一个人会信它。那条更强的保证见 TestStatusStoreMutatorsAllCovered。
func TestStatusStoreNotifiesEveryMutation(t *testing.T) {
	for _, c := range statusMutationCases() {
		t.Run(c.name, func(t *testing.T) {
			s := newStatusStore()
			var got []Status
			s.setOnChange(func(st Status) { got = append(got, st) })
			// begin 之外的方法都要求已有状态，先建一个（这一次的通知不计入）
			if c.name != "begin" {
				s.begin(1, PhaseQueued)
				got = nil
			}
			c.call(s)
			if len(got) != 1 {
				t.Fatalf("%s 通知了 %d 次，want 1", c.name, len(got))
			}
			if !c.want(got[0]) {
				t.Errorf("%s 推出去的快照不对：%+v", c.name, got[0])
			}
			if got[0].AccountID != 1 {
				t.Errorf("快照缺 AccountID：%+v", got[0])
			}
		})
	}
}

// statusStoreReadOnly 是 statusStore 上**不改状态**的方法，不需要通知。
// 新增只读方法时加进来；新增会改状态的方法则必须进 statusMutationCases。
var statusStoreReadOnly = map[string]bool{
	"get":         true, // 只读快照
	"setOnChange": true, // 装配期注册回调
	"mutate":      true, // 所有变更方法的公共实现，它就是通知本身的出口
}

// TestStatusStoreMutatorsAllCovered 才是那条真正的保证：
// **statusStore 上每一个会改状态的方法都被通知守卫覆盖到**。
//
// 做法是解析同目录的 status.go，枚举全部 `func (s *statusStore) X`，
// 减去只读白名单，剩下的必须逐个出现在 statusMutationCases 里。
//
// 没有它的话，将来谁加一个直接操作 s.statuses[id] 的方法，症状是
// 「某一种同步不显示进度」——而 CI 全绿。
func TestStatusStoreMutatorsAllCovered(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "status.go", nil, 0)
	if err != nil {
		t.Fatalf("解析 status.go: %v", err)
	}

	var mutators []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		if id, ok := star.X.(*ast.Ident); !ok || id.Name != "statusStore" {
			continue
		}
		if statusStoreReadOnly[fn.Name.Name] {
			continue
		}
		mutators = append(mutators, fn.Name.Name)
	}
	// 解析失败时集合为空，下面的循环一条断言都不跑——那是最坏的假绿灯
	if len(mutators) == 0 {
		t.Fatal("一个变更方法都没解析到：解析若失败，这条守卫会静默失效")
	}

	covered := map[string]bool{}
	for _, c := range statusMutationCases() {
		covered[c.name] = true
	}
	for _, m := range mutators {
		if !covered[m] {
			t.Errorf("statusStore.%s 会改状态但没进 statusMutationCases——"+
				"它写的状态推不到前端，症状是某一种同步不显示进度，而 CI 全绿", m)
		}
	}
	for name := range covered {
		if !slices.Contains(mutators, name) {
			t.Errorf("statusMutationCases 里的 %q 在 status.go 里已不存在，表过期了", name)
		}
	}
}

// TestStatusStoreSkipsMissingAccount 无既有状态时不通知（也不 panic）。
func TestStatusStoreSkipsMissingAccount(t *testing.T) {
	s := newStatusStore()
	n := 0
	s.setOnChange(func(Status) { n++ })
	s.markPhase(99, PhaseMessages)
	s.finishFolder(99)
	s.markDone(99)
	if n != 0 {
		t.Errorf("对不存在的账户通知了 %d 次，want 0", n)
	}
}

// TestFullSyncPublishesFolderProgress 验证后台同步把进度推到 SSE 上。
//
// 这条覆盖的是「后台自动同步进度不可见」那个缺口的后端一半：Manager 定时跑的
// 那一路此前只写内存状态，而前端根本不会去问；现在每一次状态变更都发事件。
func TestFullSyncPublishesFolderProgress(t *testing.T) {
	fsvc, msvc, frepo, _ := newTestServices(t)

	// 三个可选文件夹 + 一个 \Noselect 容器（不计入分母）
	paths := []string{"INBOX", "Sent", "Work"}
	for _, p := range paths {
		if err := frepo.UpsertByPath(&folder.Folder{
			AccountID: 1, Path: p, DisplayName: p, Type: "custom", Selectable: true, UIDNext: 1,
		}); err != nil {
			t.Fatalf("预置 %s: %v", p, err)
		}
	}

	sess := &mgrFakeSession{
		listFolders: func() ([]types.FolderInfo, error) {
			return []types.FolderInfo{
				{Name: "INBOX", Path: "INBOX", Attributes: []string{"\\Inbox"}},
				{Name: "Sent", Path: "Sent"},
				{Name: "Work", Path: "Work"},
				{Name: "容器", Path: "Box", Attributes: []string{"\\Noselect"}},
			}, nil
		},
		selectFn: func(path string) (*coreimap.SelectedFolder, error) {
			return &coreimap.SelectedFolder{Path: path, NumMessages: 0, UIDNext: 1}, nil
		},
	}

	pub := &fakePublisher{}
	m := NewManager(&fakeAccountLister{ids: []uint{1}}, fsvc, msvc, pub)
	// 与装配期同一条路径：Service.SetManager 里就是这一句
	m.setStatusStore(newStatusStore())

	if err := m.FullSync(1, sess, nil); err != nil {
		t.Fatalf("FullSync: %v", err)
	}

	var events []StatusEvent
	for i := 0; i < pub.count(); i++ {
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(pub.at(i), &probe); err != nil || probe.Type != "sync_status" {
			continue
		}
		var ev StatusEvent
		if err := json.Unmarshal(pub.at(i), &ev); err != nil {
			t.Fatalf("解析事件 %d: %v", i, err)
		}
		events = append(events, ev)
	}
	if len(events) == 0 {
		t.Fatal("一个 sync_status 事件都没发出来——前端将完全看不到后台同步")
	}

	// 阶段顺序：folders 必须先于 messages，done 收尾
	phases := make([]Phase, 0, len(events))
	for _, e := range events {
		phases = append(phases, e.Phase)
	}
	if phases[0] != PhaseFolders {
		t.Errorf("首个事件的阶段是 %q，want %q", phases[0], PhaseFolders)
	}
	if last := phases[len(phases)-1]; last != PhaseDone {
		t.Errorf("末个事件的阶段是 %q，want %q", last, PhaseDone)
	}

	// 分母只数可选文件夹：\Noselect 的容器不该占一格，否则进度条永远走不满
	var sawTotal int
	maxDone := 0
	seenFolders := map[string]bool{}
	for _, e := range events {
		if e.FoldersTotal > sawTotal {
			sawTotal = e.FoldersTotal
		}
		if e.FoldersDone > maxDone {
			maxDone = e.FoldersDone
		}
		if e.CurrentFolder != "" {
			seenFolders[e.CurrentFolder] = true
		}
	}
	if sawTotal != len(paths) {
		t.Errorf("FoldersTotal = %d, want %d（\\Noselect 容器不计入）", sawTotal, len(paths))
	}
	if maxDone != len(paths) {
		t.Errorf("FoldersDone 最高只到 %d, want %d", maxDone, len(paths))
	}
	for _, p := range paths {
		if !seenFolders[p] {
			t.Errorf("没有推送过正在同步 %q——用户看不到当前在哪个文件夹", p)
		}
	}

	// 类型必须跟着名字一起给：系统文件夹的名字在前端走 i18n，
	// 只给 DisplayName 的话进度行显示 "INBOX" 而侧栏列表显示「收件箱」。
	sawInboxType := false
	for _, e := range events {
		if e.CurrentFolder == "INBOX" {
			if e.CurrentFolderType == "" {
				t.Error("推送了 CurrentFolder 却没给 CurrentFolderType")
			}
			sawInboxType = true
		}
	}
	if !sawInboxType {
		t.Error("没有一条事件带上 INBOX")
	}

	// 终态不该还挂着「正在同步某文件夹」
	if fin := events[len(events)-1]; fin.CurrentFolder != "" {
		t.Errorf("done 事件仍带 CurrentFolder=%q", fin.CurrentFolder)
	}

	// 事件里不该出现 total / processed：那两个字段删掉了。
	// 一个在进度阶段恒为 0 的字段挂在进度事件上，下一个人就会拿它画进度条。
	for i := 0; i < pub.count(); i++ {
		var raw map[string]any
		if json.Unmarshal(pub.at(i), &raw) != nil || raw["type"] != "sync_status" {
			continue
		}
		if _, ok := raw["total"]; ok {
			t.Error("sync_status 事件仍带 total 字段")
		}
		if _, ok := raw["processed"]; ok {
			t.Error("sync_status 事件仍带 processed 字段")
		}
	}
}

// TestTerminalStatusGoesCritical 验证**终态走不可丢通道，中间帧走可丢通道**。
//
// 判据是「会不会被后续快照取代」：中间帧丢了无所谓——每条事件都带完整快照，
// 后到的天然覆盖先到的；而丢掉 done / error 是致命的，前端会永远停在"同步中"，
// 因为再没有后续事件来纠正它（账户行只观察缓存，手动触发那一路也已经收手）。
// 这正是「粘住」与「短暂倒退」的分界。
func TestTerminalStatusGoesCritical(t *testing.T) {
	fsvc, msvc, frepo, _ := newTestServices(t)
	if err := frepo.UpsertByPath(&folder.Folder{
		AccountID: 1, Path: "INBOX", DisplayName: "INBOX", Type: "inbox", Selectable: true, UIDNext: 1,
	}); err != nil {
		t.Fatalf("预置: %v", err)
	}
	sess := &mgrFakeSession{
		listFolders: func() ([]types.FolderInfo, error) {
			return []types.FolderInfo{{Name: "INBOX", Path: "INBOX", Attributes: []string{"\\Inbox"}}}, nil
		},
		selectFn: func(path string) (*coreimap.SelectedFolder, error) {
			return &coreimap.SelectedFolder{Path: path, NumMessages: 0, UIDNext: 1}, nil
		},
	}
	pub := &fakePublisher{}
	m := NewManager(&fakeAccountLister{ids: []uint{1}}, fsvc, msvc, pub)
	m.setStatusStore(newStatusStore())
	if err := m.FullSync(1, sess, nil); err != nil {
		t.Fatalf("FullSync: %v", err)
	}

	sawTerminal, sawIntermediate := false, false
	for i := 0; i < pub.count(); i++ {
		var ev StatusEvent
		if json.Unmarshal(pub.at(i), &ev) != nil || ev.Type != "sync_status" {
			continue
		}
		terminal := ev.Phase == PhaseDone || ev.Phase == PhaseError
		if terminal {
			sawTerminal = true
			if pub.droppable(i) {
				t.Errorf("终态 %q 走了可丢通道——它被丢掉的话前端永远停在同步中", ev.Phase)
			}
		} else {
			sawIntermediate = true
			if !pub.droppable(i) {
				t.Errorf("中间帧 %q 走了不可丢通道——进度会把 new_mail 挤出缓冲", ev.Phase)
			}
		}
	}
	if !sawTerminal || !sawIntermediate {
		t.Fatalf("样本不足：terminal=%v intermediate=%v", sawTerminal, sawIntermediate)
	}
}

// TestPublishStatusNoPublisher 没有 Publisher 时静默（单测里的 Service 就是这种装配）。
func TestPublishStatusNoPublisher(t *testing.T) {
	m := &Manager{}
	m.publishStatus(Status{AccountID: 1, Phase: PhaseDone, UpdatedAt: time.Now()})
}

package message_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"flymail/modules/email/message"
)

// HTTP 层验证筛选参数：仓储层的行为已由 filter_test.go 覆盖，
// 这里盯的是 handler 特有的两件事——查询串能否解析成 Filter，
// 以及 total 字段「只在首页 + 有筛选时出现」的分支是否如实。

func newFilterRouter(t *testing.T) *gin.Engine {
	t.Helper()
	repo, db := newRepoWithDB(t)
	seedAggregate(t, repo, db)
	svc := message.NewService(repo, message.NewBodyRepository(db))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	message.RegisterRoutes(r.Group(""), svc)
	return r
}

type listResp struct {
	Messages []struct {
		Subject string `json:"subject"`
		Seen    bool   `json:"seen"`
		Flagged bool   `json:"flagged"`
	} `json:"messages"`
	Total *int64 `json:"total"`
}

func getJSON(t *testing.T, r *gin.Engine, path string) listResp {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, strings.NewReader(""))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s → %d: %s", path, w.Code, w.Body.String())
	}
	var out listResp
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("解析响应失败: %v (%s)", err, w.Body.String())
	}
	return out
}

func subjects(r listResp) []string {
	out := make([]string, 0, len(r.Messages))
	for _, m := range r.Messages {
		out = append(out, m.Subject)
	}
	return out
}

func TestHandlerFolderFilterAndTotal(t *testing.T) {
	r := newFilterRouter(t)

	// 不筛选：folder 1 两封，且不返回 total（前端此时用 folders 表的现成计数）
	all := getJSON(t, r, "/folders/1/messages")
	if len(all.Messages) != 2 {
		t.Errorf("不筛选得到 %v, want 2 封", subjects(all))
	}
	if all.Total != nil {
		t.Errorf("不筛选时不该返回 total，得到 %d", *all.Total)
	}

	// seen=false：只剩未读那封，且 total 与之一致
	unread := getJSON(t, r, "/folders/1/messages?seen=false")
	if got := subjects(unread); len(got) != 1 || got[0] != "i1-unread" {
		t.Errorf("seen=false 得到 %v, want [i1-unread]", got)
	}
	if unread.Total == nil {
		t.Fatal("筛选生效时应返回 total")
	}
	if *unread.Total != int64(len(unread.Messages)) {
		t.Errorf("total=%d 与列表条数 %d 不一致", *unread.Total, len(unread.Messages))
	}

	// 翻页请求（带游标）不重复计算 total
	page2 := getJSON(t, r, "/folders/1/messages?seen=false&before_uid=1")
	if page2.Total != nil {
		t.Errorf("翻页时不该返回 total，得到 %d", *page2.Total)
	}
}

// TestHandlerFilterCombined 验证多个参数经查询串仍是 AND 叠加。
func TestHandlerFilterCombined(t *testing.T) {
	r := newFilterRouter(t)

	// folder 2（trash）里 trash-unread-star 同时满足未读 + 星标
	both := getJSON(t, r, "/folders/2/messages?seen=false&flagged=true")
	if got := subjects(both); len(got) != 1 || got[0] != "trash-unread-star" {
		t.Errorf("未读+星标 得到 %v, want [trash-unread-star]", got)
	}

	// folder 1 里没有同时满足的
	none := getJSON(t, r, "/folders/1/messages?seen=false&flagged=true")
	if len(none.Messages) != 0 {
		t.Errorf("folder1 未读+星标 得到 %v, want 空", subjects(none))
	}
	if none.Total == nil || *none.Total != 0 {
		t.Errorf("空结果的 total 应为 0，得到 %v", none.Total)
	}
}

// TestHandlerFilterIgnoresGarbage 验证非法取值被当作「该维度不筛选」而非报错：
// 筛选是渐进增强，一个拼错的参数不该让整个列表 500/400。
func TestHandlerFilterIgnoresGarbage(t *testing.T) {
	r := newFilterRouter(t)

	resp := getJSON(t, r, "/folders/1/messages?seen=maybe&flagged=")
	if len(resp.Messages) != 2 {
		t.Errorf("非法筛选值应退化为不筛选，得到 %v", subjects(resp))
	}
	if resp.Total != nil {
		t.Errorf("退化为不筛选后不该返回 total，得到 %d", *resp.Total)
	}
}

// TestHandlerAggregateFilter 验证聚合链路（JOIN folders）同样接受筛选。
func TestHandlerAggregateFilter(t *testing.T) {
	r := newFilterRouter(t)

	// starred 视图叠加「已读」→ i1-read-star
	resp := getJSON(t, r, "/aggregate/messages?view=starred&seen=true")
	if got := subjects(resp); len(got) != 1 || got[0] != "i1-read-star" {
		t.Errorf("starred+已读 得到 %v, want [i1-read-star]", got)
	}
	if resp.Total == nil || *resp.Total != 1 {
		t.Errorf("聚合 total 应为 1，得到 %v", resp.Total)
	}
}

// TestHandlerSearchFilter 验证搜索链路（JOIN message_bodies）同样接受筛选，
// 且「命中总数」与筛选后的列表同口径。
func TestHandlerSearchFilter(t *testing.T) {
	r := newFilterRouter(t)

	all := getJSON(t, r, "/search/messages?q=unread")
	if all.Total == nil || *all.Total != 5 {
		t.Fatalf("搜索命中总数应为 5，得到 %v", all.Total)
	}

	starred := getJSON(t, r, "/search/messages?q=unread&flagged=true")
	if got := subjects(starred); len(got) != 1 || got[0] != "trash-unread-star" {
		t.Errorf("搜索+星标 得到 %v, want [trash-unread-star]", got)
	}
	if starred.Total == nil || *starred.Total != 1 {
		t.Errorf("筛选后命中总数应为 1，得到 %v", starred.Total)
	}
}

func TestHandlerReindex(t *testing.T) {
	r := newFilterRouter(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/search/reindex", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("reindex status %d: %s", w.Code, w.Body.String())
	}
	// 重建后搜索仍可用
	if all := getJSON(t, r, "/search/messages?q=unread"); all.Total == nil || *all.Total != 5 {
		t.Fatalf("重建后命中总数应为 5，得到 %v", all.Total)
	}
}

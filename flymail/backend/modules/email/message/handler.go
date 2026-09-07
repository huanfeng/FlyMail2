package message

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 挂载邮件列表路由：
//   - GET /folders/:fid/messages?before_uid=&limit=      单文件夹列表
//   - GET /aggregate/messages?view=&before_date=&before_id=&limit=  跨账户聚合列表
//   - GET /aggregate/counts                              聚合入口徽标计数
//   - GET /aggregate/account-unread                      各账户未读数（侧栏角标）
//   - GET /search/messages?q=&before_date=&before_id=&limit=        跨账户搜索
//
// 三个列表接口（folder / aggregate / search）都额外接受可叠加的筛选参数
// ?seen=&flagged=&has_attachment=（见 Filter），彼此为 AND 关系；
// 筛选生效时首页响应附带 total = 筛选后的条目总数。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/folders/:fid/messages", h.list)
	rg.GET("/aggregate/messages", h.listAggregate)
	rg.GET("/aggregate/counts", h.aggregateCounts)
	rg.GET("/aggregate/account-unread", h.accountUnread)
	rg.GET("/search/messages", h.search)
	rg.GET("/contacts", h.contacts)
}

func (h *handler) contacts(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	list, err := h.svc.SearchContacts(q, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"contacts": list})
}

func (h *handler) search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusOK, gin.H{"messages": []MessageListItem{}, "next_cursor": nil})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	var beforeDate *time.Time
	if s := c.Query("before_date"); s != "" {
		if tm, err := time.Parse(time.RFC3339Nano, s); err == nil {
			beforeDate = &tm
		} else if tm, err := time.Parse(time.RFC3339, s); err == nil {
			beforeDate = &tm
		}
	}
	beforeID, _ := strconv.ParseUint(c.DefaultQuery("before_id", "0"), 10, 64)
	f := parseFilter(c)

	items, cursor, err := h.svc.ListSearch(q, beforeDate, uint(beforeID), limit, f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{"messages": items, "next_cursor": cursor}
	// 命中总数只在第一页算：这是一次全表 LIKE，翻页时重复计算会让开销翻倍，
	// 而结果对同一次搜索是不变的，前端记住首页那个数即可。
	// 与列表同筛选条件，否则「命中 N 条」会大于筛选后实际能翻到的条数。
	if beforeDate == nil && beforeID == 0 {
		total, err := h.svc.CountSearchMessages(q, f)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}

// validAggregateView 限定聚合视图取值。
func validAggregateView(v string) bool {
	return v == "inbox" || v == "unread" || v == "starred"
}

func (h *handler) listAggregate(c *gin.Context) {
	view := c.Query("view")
	if !validAggregateView(view) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid view"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	var beforeDate *time.Time
	if s := c.Query("before_date"); s != "" {
		// 游标日期为全精度 RFC3339Nano；兼容退化的 RFC3339。
		if tm, err := time.Parse(time.RFC3339Nano, s); err == nil {
			beforeDate = &tm
		} else if tm, err := time.Parse(time.RFC3339, s); err == nil {
			beforeDate = &tm
		}
	}
	beforeID, _ := strconv.ParseUint(c.DefaultQuery("before_id", "0"), 10, 64)
	f := parseFilter(c)

	items, cursor, err := h.svc.ListAggregate(view, beforeDate, uint(beforeID), limit, f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{"messages": items, "next_cursor": cursor}
	// 同 list：仅首页且有筛选时补总数。不筛选时前端用 /aggregate/counts 的现成计数。
	if beforeDate == nil && beforeID == 0 && f.Active() {
		total, cerr := h.svc.CountAggregateView(view, f)
		if cerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": cerr.Error()})
			return
		}
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}

func (h *handler) aggregateCounts(c *gin.Context) {
	counts, err := h.svc.AggregateCounts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"counts": counts})
}

// accountUnread 返回各账户未读数：{"counts": {"1": 3}}（键为账户 id 的字符串形式）。
func (h *handler) accountUnread(c *gin.Context) {
	counts, err := h.svc.AccountUnreadCounts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"counts": counts})
}

type handler struct{ svc *Service }

func (h *handler) list(c *gin.Context) {
	folderID, err := strconv.ParseUint(c.Param("fid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder id"})
		return
	}
	beforeUID, _ := strconv.ParseUint(c.DefaultQuery("before_uid", "0"), 10, 32)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	f := parseFilter(c)

	items, err := h.svc.List(uint(folderID), uint32(beforeUID), limit, f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := gin.H{"messages": items}
	// 筛选生效时补一个筛选后的总数，否则前端标题只能显示 folders 表里的全量计数——
	// 「共 320 封 · 32 未读」配着一屏 5 条未读，两个数字互相打脸。
	// 只在首页算：翻页时结果不变，重复扫表纯属浪费（与 search 同款处理）。
	// 不筛选时不算：folders 表已有现成的 total_count/unread_count。
	if beforeUID == 0 && f.Active() {
		total, cerr := h.svc.CountByFolder(uint(folderID), f)
		if cerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": cerr.Error()})
			return
		}
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}

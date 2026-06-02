package message

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 挂载邮件列表路由：
//   - GET /folders/:fid/messages?before_uid=&limit=      单文件夹列表
//   - GET /aggregate/messages?view=&before_date=&before_id=&limit=  跨账户聚合列表
//   - GET /aggregate/counts                              聚合入口徽标计数
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/folders/:fid/messages", h.list)
	rg.GET("/aggregate/messages", h.listAggregate)
	rg.GET("/aggregate/counts", h.aggregateCounts)
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

	items, cursor, err := h.svc.ListAggregate(view, beforeDate, uint(beforeID), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": items, "next_cursor": cursor})
}

func (h *handler) aggregateCounts(c *gin.Context) {
	counts, err := h.svc.AggregateCounts()
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

	items, err := h.svc.List(uint(folderID), uint32(beforeUID), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"messages": items})
}

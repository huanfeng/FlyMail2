package rule

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 挂载规则与黑名单路由：
//   - GET/POST /rules，PUT/DELETE /rules/:id，POST /rules/reorder，POST /rules/test，GET /rules/runs
//   - GET/POST /blocklist，DELETE /blocklist/:id
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/rules", h.list)
	rg.POST("/rules", h.create)
	rg.POST("/rules/reorder", h.reorder)
	rg.POST("/rules/test", h.test)
	rg.GET("/rules/runs", h.runs)
	rg.PUT("/rules/:id", h.update)
	rg.DELETE("/rules/:id", h.remove)
	rg.GET("/blocklist", h.listBlocks)
	rg.POST("/blocklist", h.addBlock)
	rg.DELETE("/blocklist/:id", h.deleteBlock)
}

type handler struct{ svc *Service }

func writeErr(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	case errors.Is(err, ErrDuplicate):
		c.JSON(http.StatusConflict, gin.H{"error": "already exists"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return uint(id), true
}

func (h *handler) list(c *gin.Context) {
	rules, err := h.svc.List()
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"rules": rules})
}

func (h *handler) create(c *gin.Context) {
	var in RuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	d, err := h.svc.Create(in)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, d)
}

func (h *handler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in RuleInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	d, err := h.svc.Update(id, in)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

func (h *handler) remove(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) reorder(c *gin.Context) {
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if err := h.svc.Reorder(body.IDs); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) test(c *gin.Context) {
	var body struct {
		Rule  RuleInput `json:"rule"`
		Limit int       `json:"limit"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	res, err := h.svc.Test(body.Rule, body.Limit)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *handler) runs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	runs, err := h.svc.ListRuns(limit)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs})
}

func (h *handler) listBlocks(c *gin.Context) {
	entries, err := h.svc.ListBlocks()
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

func (h *handler) addBlock(c *gin.Context) {
	var body struct {
		Pattern string `json:"pattern"`
		Note    string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	e, err := h.svc.AddBlock(body.Pattern, body.Note)
	if err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusCreated, e)
}

func (h *handler) deleteBlock(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteBlock(id); err != nil {
		writeErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

package account

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册账户管理路由到给定分组（调用方负责套用鉴权中间件）。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	g := rg.Group("/accounts")
	g.GET("", h.list)
	g.POST("", h.create)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
	g.POST("/test", h.testConnection)
	// 配置导出/导入。用 POST 导出的理由见 exportAccounts 的注释。
	g.POST("/export", h.exportAccounts)
	g.POST("/import", h.importAccounts)
	g.POST("/:id/enabled", h.setEnabled)
	// 静态段要与 "/:id" 并存。gin 支持这种同层兄弟，但顺序敏感的实现历来是
	// 这类路由的翻车点，所以 handler_order_test.go 里钉了一条「PUT /accounts/order
	// 不会被 /:id 吃掉」的用例。
	g.PUT("/order", h.reorder)
	registerIdentityRoutes(g, h)
	registerOAuthRoutes(g, h)
}

type handler struct{ svc *Service }

func parseID(c *gin.Context) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return 0, false
	}
	return uint(id64), true
}

func (h *handler) list(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *handler) create(c *gin.Context) {
	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.Create(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *handler) get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	resp, err := h.svc.Get(id)
	if errors.Is(err, ErrAccountNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *handler) update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.Update(id, req)
	if errors.Is(err, ErrAccountNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *handler) delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(id); errors.Is(err, ErrAccountNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// reorder 接收完整的账户 ID 顺序并落库。
//
// 用完整列表而不是「上移一位」：见 Repository.Reorder 的注释。
func (h *handler) reorder(c *gin.Context) {
	var body struct {
		IDs []uint `json:"ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的请求体"})
		return
	}
	if err := h.svc.Reorder(body.IDs); err != nil {
		if errors.Is(err, ErrOrderMismatch) {
			// 409 而不是 400：请求本身没毛病，是客户端手里的账户列表过时了。
			// 前端据此重取列表并提示用户重来，而不是把这当成一个 bug 报错。
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) setEnabled(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := h.svc.SetEnabled(id, body.Enabled)
	if errors.Is(err, ErrAccountNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "操作失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) testConnection(c *gin.Context) {
	var req TestConnectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.svc.TestConnection(req))
}

// ── 配置导出 / 导入 ─────────────────────────────────────────────────────────

// exportAccounts 导出账户配置。
//
// ⚠ 用 POST 而不是 GET，尽管它不改变任何状态。理由是 include_passwords：
// GET 的查询串会落进反向代理与浏览器的访问日志、也留在历史记录里，
// 那就等于到处留下一行「这个请求的响应里有明文密码」的路标。
// 同理，导出的 id 列表也放在请求体里。
func (h *handler) exportAccounts(c *gin.Context) {
	var body struct {
		IDs              []uint `json:"ids"`
		IncludePasswords bool   `json:"include_passwords"`
	}
	// 允许空body：整体导出、不含密码
	_ = c.ShouldBindJSON(&body)

	bundle, err := h.svc.Export(body.IDs, body.IncludePasswords)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// 含密码的导出物绝不能被任何中间层缓存下来
	if body.IncludePasswords {
		c.Header("Cache-Control", "no-store, max-age=0")
	}
	c.JSON(http.StatusOK, bundle)
}

// importAccounts 按上传的配置建立/更新账户。
func (h *handler) importAccounts(c *gin.Context) {
	var body struct {
		Bundle *PortableBundle `json:"bundle"`
		Mode   string          `json:"mode"`
		Only   []string        `json:"only"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Bundle == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	res, err := h.svc.Import(body.Bundle, ImportMode(body.Mode), body.Only)
	if err != nil {
		// 版本不认与"加密的还读不了"都是用户能看懂并据此行动的，给 400 而不是 500
		if errors.Is(err, ErrPortableVersion) || errors.Is(err, ErrPortableEncrypted) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

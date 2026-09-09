package account

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// registerIdentityRoutes 注册别名与签名路由（由 RegisterRoutes 调用）。
func registerIdentityRoutes(g *gin.RouterGroup, h *handler) {
	g.GET("/:id/aliases", h.listAliases)
	g.POST("/:id/aliases", h.createAlias)
	g.PUT("/:id/aliases/:aliasId", h.updateAlias)
	g.DELETE("/:id/aliases/:aliasId", h.deleteAlias)
	g.GET("/:id/signature", h.getSignature)
	g.PUT("/:id/signature", h.saveSignature)
}

func parseAliasID(c *gin.Context) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param("aliasId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的别名 ID"})
		return 0, false
	}
	return uint(id64), true
}

// respondIdentityErr 把服务层错误映射为状态码；返回 true 表示已响应。
func respondIdentityErr(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrAccountNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "账户不存在"})
	case errors.Is(err, ErrAliasNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "别名不存在"})
	case errors.Is(err, ErrAliasDuplicate):
		c.JSON(http.StatusConflict, gin.H{"error": "该别名已存在"})
	case errors.Is(err, ErrAliasIsPrimary):
		c.JSON(http.StatusBadRequest, gin.H{"error": "该地址是账户主地址，无需添加为别名"})
	case errors.Is(err, ErrInvalidEmail):
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱地址格式无效"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "操作失败"})
	}
	return true
}

func (h *handler) listAliases(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	list, err := h.svc.ListAliases(id)
	if respondIdentityErr(c, err) {
		return
	}
	c.JSON(http.StatusOK, list)
}

func (h *handler) createAlias(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req AliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	a, err := h.svc.CreateAlias(id, req)
	if respondIdentityErr(c, err) {
		return
	}
	c.JSON(http.StatusCreated, a)
}

func (h *handler) updateAlias(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	aliasID, ok := parseAliasID(c)
	if !ok {
		return
	}
	var req AliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	a, err := h.svc.UpdateAlias(id, aliasID, req)
	if respondIdentityErr(c, err) {
		return
	}
	c.JSON(http.StatusOK, a)
}

func (h *handler) deleteAlias(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	aliasID, ok := parseAliasID(c)
	if !ok {
		return
	}
	if respondIdentityErr(c, h.svc.DeleteAlias(id, aliasID)) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *handler) getSignature(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	sig, err := h.svc.GetSignature(id)
	if respondIdentityErr(c, err) {
		return
	}
	c.JSON(http.StatusOK, sig)
}

func (h *handler) saveSignature(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req SignatureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sig, err := h.svc.SaveSignature(id, req)
	if respondIdentityErr(c, err) {
		return
	}
	c.JSON(http.StatusOK, sig)
}

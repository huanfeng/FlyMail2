package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 在给定路由组下注册 auth 相关端点。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	g := rg.Group("/auth")
	g.POST("/login", h.login)
	g.POST("/refresh", h.refresh)
	g.POST("/logout", h.logout)
}

type handler struct{ svc *Service }

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *handler) login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	pair, err := h.svc.Login(req.Username, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败"})
		return
	}
	c.JSON(http.StatusOK, pair)
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *handler) refresh(c *gin.Context) {
	var req refreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	pair, err := h.svc.Refresh(req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token 无效"})
		return
	}
	c.JSON(http.StatusOK, pair)
}

func (h *handler) logout(c *gin.Context) {
	// 无状态 JWT：登出由前端丢弃 token 实现；此端点保留用于审计/未来黑名单。
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// RegisterProtectedRoutes 在已受 Middleware 保护的路由组下注册需登录态的 auth 端点。
func RegisterProtectedRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	g := rg.Group("/auth")
	g.POST("/change-password", h.changePassword)
}

type changePasswordReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *handler) changePassword(c *gin.Context) {
	username := c.GetString(ContextUsernameKey)
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if req.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "新密码不能为空"})
		return
	}
	if err := h.svc.ChangePassword(username, req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "旧密码错误"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "修改密码失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

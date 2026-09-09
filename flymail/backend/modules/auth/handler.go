package auth

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"flymail-core/logger"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RegisterRoutes 在给定路由组下注册 auth 相关端点。limiter 可为 nil（不限流，单测用）。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service, limiter *Limiter) {
	h := &handler{svc: svc, limiter: limiter}
	g := rg.Group("/auth")
	g.POST("/login", h.login)
	g.POST("/refresh", h.refresh)
	g.POST("/logout", h.logout)
}

type handler struct {
	svc     *Service
	limiter *Limiter
}

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
	ip := c.ClientIP()
	if h.limiter != nil {
		var tooMany *ErrTooManyAttempts
		if err := h.limiter.Check(ip); errors.As(err, &tooMany) {
			retry := int(tooMany.RetryAfter.Round(time.Second).Seconds())
			if retry < 1 {
				retry = 1
			}
			// 不记用户名：用户常把密码敲进用户名框，日志里会留存明文
			logger.Warn("auth: 登录限流", zap.String("client_ip", ip), zap.Int("username_len", len(req.Username)), zap.Int("retry_after", retry))
			c.Header("Retry-After", strconv.Itoa(retry))
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "尝试次数过多，请稍后再试", "retry_after": retry})
			return
		} else if err != nil {
			logger.Warn("auth: 限流检查失败", zap.Error(err))
		}
	}
	pair, err := h.svc.Login(req.Username, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		if h.limiter != nil {
			if err := h.limiter.Fail(ip); err != nil {
				logger.Warn("auth: 记录登录失败次数失败", zap.Error(err))
			}
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "登录失败"})
		return
	}
	if h.limiter != nil {
		_ = h.limiter.Reset(ip)
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
	g.GET("/me", h.me)
	g.PUT("/profile", h.updateProfile)
}

// profileResp 资料响应（不含密码哈希）。
type profileResp struct {
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Email       string     `json:"email"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

func toProfileResp(u *AdminUser) profileResp {
	return profileResp{
		Username:    u.Username,
		DisplayName: u.DisplayName,
		Email:       u.Email,
		CreatedAt:   u.CreatedAt,
		LastLoginAt: u.LastLoginAt,
	}
}

func (h *handler) me(c *gin.Context) {
	username := c.GetString(ContextUsernameKey)
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	u, err := h.svc.Profile(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取资料失败"})
		return
	}
	c.JSON(http.StatusOK, toProfileResp(u))
}

type updateProfileReq struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func (h *handler) updateProfile(c *gin.Context) {
	username := c.GetString(ContextUsernameKey)
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}
	var req updateProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	u, err := h.svc.UpdateProfile(username, req.DisplayName, req.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新资料失败"})
		return
	}
	c.JSON(http.StatusOK, toProfileResp(u))
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

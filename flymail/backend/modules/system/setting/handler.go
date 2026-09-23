package setting

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"flymail/internal/ai"
	"flymail/internal/lang"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册设置路由到给定分组（调用方负责套用鉴权中间件）。
func RegisterRoutes(rg *gin.RouterGroup, svc *Service) {
	h := &handler{svc: svc}
	rg.GET("/settings", h.getAll)
	rg.PUT("/settings", h.setAll)
}

type handler struct{ svc *Service }

func (h *handler) getAll(c *gin.Context) {
	m := h.svc.All()
	// 补充缺省值
	if _, ok := m[KeySyncDepth]; !ok {
		m[KeySyncDepth] = DefaultSyncDepth
	}
	if _, ok := m[KeySyncMaxConcurrent]; !ok {
		m[KeySyncMaxConcurrent] = DefaultSyncMaxConcurrent
	}
	if _, ok := m[KeySyncMaxIdleConns]; !ok {
		m[KeySyncMaxIdleConns] = DefaultSyncMaxIdleConns
	}
	if _, ok := m[KeyNotifyBodyRunes]; !ok {
		m[KeyNotifyBodyRunes] = DefaultNotifyBodyRunes
	}
	if m[KeyTranslateTargetLang] == "" {
		m[KeyTranslateTargetLang] = DefaultTranslateTargetLang
	}
	c.JSON(http.StatusOK, gin.H{"settings": m})
}

func (h *handler) setAll(c *gin.Context) {
	var body struct {
		Settings map[string]string `json:"settings"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	// 校验 sync_depth
	if v, ok := body.Settings[KeySyncDepth]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 100 || n > 5000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sync_depth 必须是 100..5000 的整数"})
			return
		}
	}
	// 校验 sync_max_concurrent
	if v, ok := body.Settings[KeySyncMaxConcurrent]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 64 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sync_max_concurrent 必须是 1..64 的整数"})
			return
		}
	}
	// 校验 sync_max_idle_conns
	if v, ok := body.Settings[KeySyncMaxIdleConns]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 1000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sync_max_idle_conns 必须是 0..1000 的整数"})
			return
		}
	}

	// 校验 notify_body_runes。
	//
	// 上界 20000 不是飞书的限制（那是按字节算的，另有兜底裁剪），
	// 而是「再多也没意义」：两万字的邮件推进聊天群，谁也不会在那里读完。
	// 允许 0，表示用内置默认。
	if v, ok := body.Settings[KeyNotifyBodyRunes]; ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 20000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "notify_body_runes 必须是 0..20000 的整数"})
			return
		}
	}

	// 校验 app_base_url：允许留空（表示不带链接），否则必须是带主机名的 http/https 绝对地址。
	// 存进去之前统一去掉结尾斜杠，拼链接时就不用两边都判断。
	if v, ok := body.Settings[KeyAppBaseURL]; ok {
		normalized, err := NormalizeBaseURL(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		body.Settings[KeyAppBaseURL] = normalized
	}

	// 校验 body_sync_mode
	if v, ok := body.Settings[KeyBodySyncMode]; ok && !ValidBodySyncMode(v) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body_sync_mode 必须是 new/recent/all 之一"})
		return
	}
	// 校验 body_sync_recent_days
	if v, ok := body.Settings[KeyBodySyncRecentDays]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "body_sync_recent_days 必须是 1..3650 的整数"})
			return
		}
	}

	// 校验 ai_base_url：允许留空（= 关掉翻译功能），否则必须是能拼出
	// chat/completions 的 http(s) 地址。这里就把它归一化后落库，
	// 让"用户填了哪种形状"这件事只在进门这一处处理——服务那侧拿到的
	// 永远是可以直接发请求的完整地址。
	if v, ok := body.Settings[KeyAIBaseURL]; ok {
		if trimmed := strings.TrimSpace(v); trimmed == "" {
			body.Settings[KeyAIBaseURL] = ""
		} else {
			endpoint, err := ai.Endpoint(trimmed)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			body.Settings[KeyAIBaseURL] = endpoint
		}
	}

	// 模型名只裁空白，不做格式校验：模型名是服务商定的，什么形状都有
	// （gpt-4o-mini、deepseek-chat、qwen2.5:7b、accounts/fireworks/...），
	// 拿正则卡住只会在人家出新模型那天变成假故障。裁空白则是必须的——
	// 从文档里复制模型名极容易带上尾随空格，而带空格的模型名会让接口
	// 报一个与"填错了"毫无关系的 404。
	if v, ok := body.Settings[KeyAIModel]; ok {
		body.Settings[KeyAIModel] = strings.TrimSpace(v)
	}
	if v, ok := body.Settings[KeyAIAPIKey]; ok {
		body.Settings[KeyAIAPIKey] = strings.TrimSpace(v)
	}

	// 校验 translate_target_lang：必须在清单内。
	// 不接受清单外的代码，是因为它会一路传进提示词——一个手误的 "zzz"
	// 会让模型自由发挥，而用户只会看到一封被翻成随机语言的邮件。
	if v, ok := body.Settings[KeyTranslateTargetLang]; ok && !lang.IsSupported(v) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "translate_target_lang 不是受支持的语言代码"})
		return
	}

	// 校验 oauth_google_client_id：允许留空（= 撤掉入口），否则去掉首尾空白后存。
	// 只做空白裁剪不做格式校验：Google 的客户端 ID 形如 <数字>-<串>.apps.googleusercontent.com，
	// 但这个格式由 Google 定、随时可能变，拿正则卡住只会在人家改格式那天变成假故障。
	// 真正的判据是走一次授权——填错了那边会明确报 invalid_client。
	// 裁空白则是必须的：从后台复制粘贴极容易带上换行或空格，而带空格的 client_id
	// 会让授权在浏览器里报一个与「填错了」毫无关系的错。
	for _, k := range []string{KeyOAuthGoogleClientID, KeyOAuthGoogleClientSecret} {
		if v, ok := body.Settings[k]; ok {
			body.Settings[k] = strings.TrimSpace(v)
		}
	}

	if err := h.svc.SetMany(body.Settings); err != nil {
		if errors.Is(err, ErrNoEncryptor) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

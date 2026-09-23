package sync

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
)

// MailStateEvent 通知「本地邮件状态被用户改过」——已读/未读、星标、删除、移动。
//
// 它存在的理由是**同时开着多个界面**（多标签页、桌面端与浏览器并存）。
// 此前 SSE 只推 new_mail / notify / sync_status，三者都只描述「服务器那边有新东西」；
// 用户自己点掉一封未读，别的界面完全无从得知：
//   - 标已读不发任何事件；
//   - 计数类查询（aggregate-counts / account-unread / folders）只靠 30 秒轮询兜底，
//     而标签页隐藏时浏览器会暂停定时器；
//   - 前端全局 refetchOnWindowFocus 是关的，切回去也不重取。
//
// 三者叠加的结果是未读角标能在错误值上停留到用户按 F5 为止。
//
// 不带账户/文件夹 id：一次批量或会话级操作本就可能横跨多个账户与文件夹，
// 而接收方要做的只是「把计数与列表重新拉一遍」，给了也用不上。
type MailStateEvent struct {
	Type string `json:"type"` // 恒为 "mail_state"
	// Origin 是发起这次操作的客户端标识（请求头 X-Client-Id）。
	// 发起方自己在 mutation 的 onSettled 里已经失效过一轮缓存，
	// 收到自己的回声再失效一次就是白打一轮请求——它据此忽略。
	Origin string `json:"origin,omitempty"`
}

// SetPublisher 注入 SSE 发布能力（sse.Hub 满足）。未注入时广播静默跳过。
func (s *Service) SetPublisher(p Publisher) { s.pub = p }

// PublishMailState 广播一条 mail_state。origin 为发起方客户端标识，可为空。
//
// 走 Publish 而不是 PublishProgress，因为它不该和一轮同步的 2n+4 条进度抢同一格缓冲。
// ⚠ 但 Publish **也是尽力而为**：hub 的 16 槽满了照样 default 丢弃（见 internal/sse/hub.go）。
// 这条事件没有投递保证，所以它只是让计数「更快」对上，而不是唯一的正确性来源——
// 真正的兜底是前端的 focus 重取与 30 秒轮询（见 frontend/src/lib/queries.ts 的 COUNT_REFETCH）。
// 谁要是把兜底撤了、只靠这条事件，冻结的后台标签页塞满缓冲那一刻就会退回老 bug。
func (s *Service) PublishMailState(origin string) {
	if s.pub == nil {
		return
	}
	payload, err := json.Marshal(MailStateEvent{Type: "mail_state", Origin: origin})
	if err != nil {
		return
	}
	s.pub.Publish(payload)
}

// mailStateNotify 是挂在「会改动本地邮件状态」那一组路由上的中间件：
// 处理成功（HTTP < 400）就广播一条 mail_state。
//
// 做成中间件而不是在十几个 handler 结尾各写一行：写操作以后还会增加，
// 逐个补调用总有一天会漏掉一个，而漏掉的表现（别的标签页角标不动）
// 隔着一层 SSE 极难联想到是某个 handler 少了一行。
func (h *handler) mailStateNotify() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Writer.Status() < 400 {
			h.svc.PublishMailState(clientIDOf(c))
		}
	}
}

// maxOriginLen 是 origin 的截断长度。
//
// 这个值只参与一次相等比较，长一个字节都没有用处；而不截断的话，一个畸形或被改过的
// 客户端能拿 Go 默认允许的 1MB 请求头，让每条 mail_state 都是 1MB，再乘以订阅者数量扇出。
const maxOriginLen = 64

// clientIDOf 取发起方标识并截断。
func clientIDOf(c *gin.Context) string {
	id := c.GetHeader("X-Client-Id")
	if len(id) > maxOriginLen {
		return id[:maxOriginLen]
	}
	return id
}

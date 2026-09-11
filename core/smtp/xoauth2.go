package smtp

import (
	"errors"
	"fmt"
	"net/smtp"
)

// xoauth2Auth 实现 SMTP 的 XOAUTH2 认证（Gmail / Outlook 在停用基本认证后的唯一通道）。
//
// 与 net/smtp 内置的 PlainAuth 不同，标准库没有提供 XOAUTH2，且 PlainAuth 的 TLS 自检
// 在我们自建隐式 TLS 连接时会误判——这里同样由调用方通过 secured 保证链路已加密，
// 未加密时直接拒绝发送 Bearer 令牌（令牌等价于长期凭证，明文外泄后果重于密码）。
type xoauth2Auth struct {
	username, token, host string
}

func (a *xoauth2Auth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if server.Name != a.host {
		return "", nil, errors.New("wrong host name")
	}
	return "XOAUTH2", []byte(fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", a.username, a.token)), nil
}

// Next 在服务端返回错误质询（一段 JSON）时被调用。协议要求客户端回一个空响应，
// 让服务端把交互推进到明确的失败应答，否则连接会挂在质询状态。
func (a *xoauth2Auth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		return []byte{}, nil
	}
	return nil, nil
}

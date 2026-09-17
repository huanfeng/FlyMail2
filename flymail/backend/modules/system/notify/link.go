package notify

import (
	"net/url"
	"strconv"
	"strings"
)

// MailLink 拼出「打开这封邮件」的绝对地址。
//
// base 为空时返回空串——没配对外访问地址就不带链接，绝不用相对路径或请求里的
// Host 凑一个：服务端看到的 Host 可能是反代内网名、容器名或 127.0.0.1，
// 凑出来的是一条点了打不开的死链，而用户要到点击那一刻才发现。
//
// ⚠ folderID 也要带上。前端的邮件列表是按文件夹取的，只给 message 的话
// 右边能打开、左边那列却是空的（会话视图下则是两边都空，见前端的补定位逻辑）。
func MailLink(base string, accountID, folderID, messageID uint) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	q := url.Values{}
	if accountID != 0 {
		q.Set("account", strconv.FormatUint(uint64(accountID), 10))
	}
	if folderID != 0 {
		q.Set("folder", strconv.FormatUint(uint64(folderID), 10))
	}
	if messageID != 0 {
		q.Set("message", strconv.FormatUint(uint64(messageID), 10))
	}
	if len(q) == 0 {
		return base + "/"
	}
	return base + "/?" + q.Encode()
}

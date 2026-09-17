package setting

import (
	"errors"
	"net/url"
	"strings"
)

// NormalizeBaseURL 校验并规整「对外访问地址」。
//
// 空串是合法取值，表示「没配」——通知里就不带链接，其余功能照常。
//
// ⚠ 必须是带主机名的绝对地址。只写 `mail.example.com` 这种没有 scheme 的串，
// url.Parse 会把它当成相对路径解析成功（Host 为空），拼出来的链接是
// `mail.example.com/?message=1`，点开是死链——这类错误在配置界面上看不出来，
// 只有等通知发出去、用户点了才发现。
//
// 结尾斜杠在这里统一去掉，拼链接的地方就不必两边都判断。
func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errors.New("对外访问地址不是合法的 URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("对外访问地址必须以 http:// 或 https:// 开头")
	}
	if u.Host == "" {
		return "", errors.New("对外访问地址缺少主机名")
	}
	// 只保留 scheme://host[:port] 与路径前缀，丢掉查询串与锚点：
	// 它们拼到链接里只会和我们自己的参数打架。
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

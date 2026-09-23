package oauth

import "testing"

// PasswordAuth 钉住的是两家服务商**当前的协议事实**，不是我们的偏好：
//   - Gmail 2024-09 停掉了明文口令，但应用专用密码（开两步验证后）仍可用于 IMAP/SMTP。
//   - Microsoft 2024-09-16 对个人 Outlook.com/Hotmail/Live 彻底关闭基本认证，
//     没有应用专用密码这种东西，OAuth 是唯一的路。
//
// 值反了不会有任何报错，只会让界面把 Outlook 用户往一条死路上指——
// 他们会去翻一个根本不存在的设置，翻半天然后断定是 FlyMail 坏了。
func TestPasswordAuthReflectsProviderReality(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want bool
	}{
		{ProviderGoogle, true},
		{ProviderMicrosoft, false},
	} {
		p, ok := Lookup(tc.id, "")
		if !ok {
			t.Fatalf("%s: 查不到提供方", tc.id)
		}
		if p.PasswordAuth != tc.want {
			t.Errorf("%s: PasswordAuth = %v, 期望 %v", tc.id, p.PasswordAuth, tc.want)
		}
	}
}

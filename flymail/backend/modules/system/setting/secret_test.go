package setting_test

import (
	"errors"
	"strings"
	"testing"

	"flymail/internal/crypto"
	"flymail/modules/system/setting"
)

// newSecretSvc 构建一个注入了真实加密器的 Service。
func newSecretSvc(t *testing.T) *setting.Service {
	t.Helper()
	enc, err := crypto.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("crypto.New: %v", err)
	}
	svc := newSvc(t)
	svc.SetEncryptor(enc)
	return svc
}

// TestSecretNotReturnedByAll 是这套东西存在的理由：密文设置绝不能出现在
// GET /settings 的响应里。All 是全仓唯一把 settings 表整个交出去的地方，
// 密文若随之出网，加密存储就退化成「多编码了一层的明文」。
func TestSecretNotReturnedByAll(t *testing.T) {
	svc := newSecretSvc(t)
	const plain = "GOCSPX-super-secret-value"
	if err := svc.SetMany(map[string]string{setting.KeyOAuthGoogleClientSecret: plain}); err != nil {
		t.Fatalf("SetMany: %v", err)
	}

	all := svc.All()
	if _, ok := all[setting.KeyOAuthGoogleClientSecret]; ok {
		t.Fatal("All() 里出现了密文设置本身")
	}
	for k, v := range all {
		if strings.Contains(v, plain) {
			t.Fatalf("All() 的 %s 里泄露了明文", k)
		}
	}
	if got := all[setting.KeyOAuthGoogleClientSecret+setting.SecretSetSuffix]; got != "true" {
		t.Fatalf("_set 标记 = %q, want \"true\"", got)
	}
}

// TestSecretRoundTrip：存明文、库里是密文、GetSecret 能原样取回。
func TestSecretRoundTrip(t *testing.T) {
	svc := newSecretSvc(t)
	const plain = "GOCSPX-abc123"
	if err := svc.SetMany(map[string]string{setting.KeyOAuthGoogleClientSecret: plain}); err != nil {
		t.Fatalf("SetMany: %v", err)
	}
	if got := svc.GetSecret(setting.KeyOAuthGoogleClientSecret); got != plain {
		t.Fatalf("GetSecret = %q, want %q", got, plain)
	}
	// 未经解密的裸读必须拿不到明文
	if raw := svc.GetString(setting.KeyOAuthGoogleClientSecret, ""); raw == plain {
		t.Fatal("库里存的是明文")
	}
}

// TestSecretEmptyClears：空串是「清除」，不是「加密一个空串」。
// 管理员要撤掉 client_secret 时，界面上能做的就是把输入框清空。
func TestSecretEmptyClears(t *testing.T) {
	svc := newSecretSvc(t)
	if err := svc.SetMany(map[string]string{setting.KeyOAuthGoogleClientSecret: "x"}); err != nil {
		t.Fatalf("SetMany: %v", err)
	}
	if err := svc.SetMany(map[string]string{setting.KeyOAuthGoogleClientSecret: ""}); err != nil {
		t.Fatalf("SetMany 清除: %v", err)
	}
	if got := svc.GetSecret(setting.KeyOAuthGoogleClientSecret); got != "" {
		t.Fatalf("清除后 GetSecret = %q, want 空", got)
	}
	if got := svc.All()[setting.KeyOAuthGoogleClientSecret+setting.SecretSetSuffix]; got != "false" {
		t.Fatalf("清除后 _set = %q, want \"false\"", got)
	}
}

// TestSecretWithoutEncryptorRejected：没有加密器时拒绝保存，而不是静默存明文。
// 静默降级会让一个本该加密的凭据以为自己安全地躺在库里。
func TestSecretWithoutEncryptorRejected(t *testing.T) {
	svc := newSvc(t) // 不注入加密器
	err := svc.SetMany(map[string]string{setting.KeyOAuthGoogleClientSecret: "x"})
	if !errors.Is(err, setting.ErrNoEncryptor) {
		t.Fatalf("err = %v, want ErrNoEncryptor", err)
	}
	if got := svc.GetString(setting.KeyOAuthGoogleClientSecret, ""); got != "" {
		t.Fatalf("被拒绝后库里仍写入了 %q", got)
	}
}

// TestNonSecretUnaffected：普通设置照旧明文存取，不受这套机制影响。
func TestNonSecretUnaffected(t *testing.T) {
	svc := newSecretSvc(t)
	if err := svc.SetMany(map[string]string{setting.KeyOAuthGoogleClientID: "123.apps.googleusercontent.com"}); err != nil {
		t.Fatalf("SetMany: %v", err)
	}
	if got := svc.All()[setting.KeyOAuthGoogleClientID]; got != "123.apps.googleusercontent.com" {
		t.Fatalf("client_id = %q，普通设置不该被改动", got)
	}
}

package aiprovider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	coredb "flymail-core/database"

	"flymail/internal/ai"
	"flymail/internal/crypto"
	"flymail/modules/system/setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 不用 internal/database.Migrate：那个包 import 了本包，包内测试反向 import 会成环。
func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := coredb.OpenSQLite(coredb.Options{Path: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, e := db.DB(); e == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := db.AutoMigrate(&Provider{}, &setting.Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newEnc(t *testing.T) *crypto.Encryptor {
	t.Helper()
	enc, err := crypto.New("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func newTestSvc(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db := newDB(t)
	return NewService(NewRepository(db), newEnc(t), ai.NewHealth()), db
}

func ptr[T any](v T) *T { return &v }

func mustCreate(t *testing.T, s *Service, name, base, model, key string) *View {
	t.Helper()
	v, err := s.Create(Input{Name: ptr(name), BaseURL: ptr(base), Model: ptr(model), APIKey: ptr(key)})
	if err != nil {
		t.Fatalf("Create %s: %v", name, err)
	}
	return v
}

func TestCreateNormalizesAndAppends(t *testing.T) {
	s, _ := newTestSvc(t)
	a := mustCreate(t, s, " A ", "https://api.deepseek.com/v1", "  deepseek-chat\n", "sk-a")
	b := mustCreate(t, s, "", "http://127.0.0.1:11434/v1/", "qwen2.5:7b", "")

	if a.BaseURL != "https://api.deepseek.com/v1/chat/completions" {
		t.Errorf("地址没有归一：%s", a.BaseURL)
	}
	if a.Name != "A" || a.Model != "deepseek-chat" {
		t.Errorf("名字/模型没裁空白：%q %q", a.Name, a.Model)
	}
	if !a.KeySet || b.KeySet {
		t.Errorf("key_set 不对：a=%v b=%v", a.KeySet, b.KeySet)
	}
	// 名字留空用主机名顶上
	if b.Name != "127.0.0.1:11434" {
		t.Errorf("空名字应取主机名：%q", b.Name)
	}
	list, _ := s.List()
	if len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Errorf("新建的应排到末尾：%+v", list)
	}
}

func TestCreateValidates(t *testing.T) {
	s, _ := newTestSvc(t)
	cases := []Input{
		{BaseURL: ptr("api.openai.com/v1"), Model: ptr("m")}, // 少写 scheme
		{BaseURL: ptr(""), Model: ptr("m")},
		{BaseURL: ptr("https://x/v1"), Model: ptr("  ")},
	}
	for _, in := range cases {
		_, err := s.Create(in)
		var invalid *InvalidError
		if !errors.As(err, &invalid) {
			t.Errorf("%+v 应被判为输入不合法，得到 %v", in, err)
		}
	}
}

// 停用状态必须如实落库：gorm 对带 default 的零值字段会改用默认值，
// 一个「新建即停用」的配置会悄悄变成启用。
func TestCreateDisabledPersists(t *testing.T) {
	s, _ := newTestSvc(t)
	v, err := s.Create(Input{BaseURL: ptr("https://x/v1"), Model: ptr("m"), Enabled: ptr(false)})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := s.repo.Get(v.ID)
	if p.Enabled {
		t.Error("停用的配置被存成了启用")
	}
}

// 密钥框留空 = 不动；只有 clear_key 才清除。否则只改个模型名就会把密钥抹掉。
func TestUpdateKeySemantics(t *testing.T) {
	s, _ := newTestSvc(t)
	v := mustCreate(t, s, "A", "https://x/v1", "m", "sk-1")

	if _, err := s.Update(v.ID, Input{Model: ptr("m2"), APIKey: ptr("")}); err != nil {
		t.Fatal(err)
	}
	if got := activeKey(t, s, v.ID); got != "sk-1" {
		t.Errorf("留空密钥框不该动密钥：%q", got)
	}
	if _, err := s.Update(v.ID, Input{APIKey: ptr(" sk-2 ")}); err != nil {
		t.Fatal(err)
	}
	if got := activeKey(t, s, v.ID); got != "sk-2" {
		t.Errorf("新密钥没生效（或没裁空白）：%q", got)
	}
	if _, err := s.Update(v.ID, Input{ClearKey: true}); err != nil {
		t.Fatal(err)
	}
	if got := activeKey(t, s, v.ID); got != "" {
		t.Errorf("clear_key 没清掉密钥：%q", got)
	}
}

func activeKey(t *testing.T, s *Service, id uint) string {
	t.Helper()
	list, err := s.Active()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range list {
		if a.ID == id {
			return a.Config.APIKey
		}
	}
	t.Fatalf("配置 %d 不在使用列表里", id)
	return ""
}

func TestUpdateKeepsCreatedAt(t *testing.T) {
	s, _ := newTestSvc(t)
	v := mustCreate(t, s, "A", "https://x/v1", "m", "")
	if _, err := s.Update(v.ID, Input{Name: ptr("B")}); err != nil {
		t.Fatal(err)
	}
	p, _ := s.repo.Get(v.ID)
	if p.CreatedAt.IsZero() || p.Name != "B" {
		t.Errorf("CreatedAt=%v Name=%q", p.CreatedAt, p.Name)
	}
}

func TestActiveSkipsDisabledInOrder(t *testing.T) {
	s, _ := newTestSvc(t)
	a := mustCreate(t, s, "A", "https://a/v1", "m", "")
	b := mustCreate(t, s, "B", "https://b/v1", "m", "")
	c := mustCreate(t, s, "C", "https://c/v1", "m", "")
	if _, err := s.Update(b.ID, Input{Enabled: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Reorder([]uint{c.ID, b.ID, a.ID}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.Active()
	if len(list) != 2 || list[0].Name != "C" || list[1].Name != "A" {
		t.Errorf("使用列表应为 C,A：%+v", list)
	}
}

func TestReorderRejectsStaleList(t *testing.T) {
	s, _ := newTestSvc(t)
	a := mustCreate(t, s, "A", "https://a/v1", "m", "")
	mustCreate(t, s, "B", "https://b/v1", "m", "")
	if err := s.Reorder([]uint{a.ID}); !errors.Is(err, ErrOrderMismatch) {
		t.Errorf("少一条应拒绝：%v", err)
	}
}

func TestDeleteForgetsHealth(t *testing.T) {
	s, _ := newTestSvc(t)
	v := mustCreate(t, s, "A", "https://a/v1", "m", "")
	s.health.RecordFail(v.ID, &ai.APIError{Status: 402})
	if err := s.Delete(v.ID); err != nil {
		t.Fatal(err)
	}
	if st := s.health.Get(v.ID); st.Failures != 0 {
		t.Error("删除后应清掉健康状态，免得 ID 复用时继承冷却")
	}
	if err := s.Delete(v.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("再删一次应是 ErrNotFound：%v", err)
	}
}

// ── 迁移 ──────────────────────────────────────────────────────────────────

func TestMigrateLegacy(t *testing.T) {
	db := newDB(t)
	enc := newEnc(t)
	// 按旧版的方式存：地址已归一，密钥是密文
	sRepo := setting.NewRepository(db)
	ct, _ := enc.Encrypt("sk-old")
	_ = sRepo.Set(legacyKeyBaseURL, "https://api.openai.com/v1/chat/completions")
	_ = sRepo.Set(legacyKeyModel, "gpt-4o-mini")
	_ = sRepo.Set(legacyKeyAPIKey, ct)

	repo := NewRepository(db)
	migrated, err := MigrateLegacy(repo, sRepo)
	if err != nil || !migrated {
		t.Fatalf("migrated=%v err=%v", migrated, err)
	}
	svc := NewService(repo, enc, nil)
	list, _ := svc.Active()
	if len(list) != 1 {
		t.Fatalf("应迁出一条：%+v", list)
	}
	got := list[0]
	if got.Name != "api.openai.com" || got.Config.Model != "gpt-4o-mini" || got.Config.APIKey != "sk-old" {
		t.Errorf("迁移结果不对：%+v", got)
	}
	for _, k := range []string{legacyKeyBaseURL, legacyKeyModel, legacyKeyAPIKey} {
		if _, found, _ := sRepo.Get(k); found {
			t.Errorf("旧键 %s 没删", k)
		}
	}
	// 再跑一遍：不重复插入
	if migrated, err := MigrateLegacy(repo, sRepo); err != nil || migrated {
		t.Errorf("第二次不该再迁：migrated=%v err=%v", migrated, err)
	}
	if n, _ := repo.Count(); n != 1 {
		t.Errorf("重复迁移插了多余的行：%d", n)
	}
}

// 新表已有数据时，残留的旧键不能复活成一条配置。
func TestMigrateLegacySkipsWhenTableHasRows(t *testing.T) {
	db := newDB(t)
	repo := NewRepository(db)
	sRepo := setting.NewRepository(db)
	_ = repo.Create(&Provider{Name: "new", BaseURL: "https://n/v1/chat/completions", Model: "m", Enabled: true})
	_ = sRepo.Set(legacyKeyBaseURL, "https://old/v1/chat/completions")
	_ = sRepo.Set(legacyKeyModel, "old")

	if migrated, err := MigrateLegacy(repo, sRepo); err != nil || migrated {
		t.Fatalf("migrated=%v err=%v", migrated, err)
	}
	if n, _ := repo.Count(); n != 1 {
		t.Errorf("行数 = %d", n)
	}
	if _, found, _ := sRepo.Get(legacyKeyBaseURL); found {
		t.Error("旧键应被清掉")
	}
}

// 旧版没配全（= 没开翻译）时不造出一条残缺配置。
func TestMigrateLegacyIncomplete(t *testing.T) {
	db := newDB(t)
	repo := NewRepository(db)
	sRepo := setting.NewRepository(db)
	_ = sRepo.Set(legacyKeyBaseURL, "")
	_ = sRepo.Set(legacyKeyModel, "m")
	if migrated, err := MigrateLegacy(repo, sRepo); err != nil || migrated {
		t.Fatalf("migrated=%v err=%v", migrated, err)
	}
	if n, _ := repo.Count(); n != 0 {
		t.Errorf("行数 = %d", n)
	}
}

// ── 测试连接 ──────────────────────────────────────────────────────────────

type fakeChat struct{ err error }

func (f fakeChat) Chat(context.Context, []ai.Message) (string, error) { return "OK", f.err }

func TestTestRecordsHealth(t *testing.T) {
	s, _ := newTestSvc(t)
	v := mustCreate(t, s, "A", "https://a/v1", "m", "")

	s.newChat = func(ai.Config) (chatClient, error) { return fakeChat{err: &ai.APIError{Status: 402}}, nil }
	res, err := s.Test(context.Background(), v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.Kind != ai.KindQuota || !res.Provider.Cooling {
		t.Errorf("失败的测试应记冷却：%+v", res)
	}

	// 充值后点一下测试，冷却立即解除
	s.newChat = func(ai.Config) (chatClient, error) { return fakeChat{}, nil }
	res, err = s.Test(context.Background(), v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Provider.Cooling {
		t.Errorf("成功的测试应解除冷却：%+v", res)
	}
}

// ── HTTP ──────────────────────────────────────────────────────────────────

func do(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	r.ServeHTTP(res, req)
	return res
}

func TestHTTPRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, _ := newTestSvc(t)
	r := gin.New()
	RegisterRoutes(r.Group("/"), s)

	res := do(t, r, http.MethodPost, "/ai/providers",
		`{"name":"A","base_url":"https://a/v1","model":"m","api_key":"sk-secret-value"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("create: %d %s", res.Code, res.Body.String())
	}
	// 密钥（明文或密文）都不能出现在任何响应里
	if strings.Contains(res.Body.String(), "sk-secret") || strings.Contains(res.Body.String(), "api_key\"") {
		t.Fatalf("响应里带出了密钥：%s", res.Body.String())
	}
	var created View
	_ = json.Unmarshal(res.Body.Bytes(), &created)

	if res := do(t, r, http.MethodGet, "/ai/providers", ""); !strings.Contains(res.Body.String(), `"key_set":true`) ||
		strings.Contains(res.Body.String(), "api_key\"") {
		t.Errorf("list: %s", res.Body.String())
	}
	if res := do(t, r, http.MethodPost, "/ai/providers", `{"base_url":"nope","model":"m"}`); res.Code != http.StatusBadRequest {
		t.Errorf("非法地址应 400：%d", res.Code)
	}
	if res := do(t, r, http.MethodPut, "/ai/providers/order", `{"ids":[999]}`); res.Code != http.StatusConflict {
		t.Errorf("过时的顺序应 409：%d", res.Code)
	}
	path := "/ai/providers/" + itoa(created.ID)
	if res := do(t, r, http.MethodPut, path, `{"enabled":false}`); res.Code != http.StatusOK ||
		!strings.Contains(res.Body.String(), `"enabled":false`) {
		t.Errorf("update: %d %s", res.Code, res.Body.String())
	}
	if res := do(t, r, http.MethodPost, path+"/reset", ""); res.Code != http.StatusOK {
		t.Errorf("reset: %d", res.Code)
	}
	if res := do(t, r, http.MethodDelete, path, ""); res.Code != http.StatusOK {
		t.Errorf("delete: %d", res.Code)
	}
	if res := do(t, r, http.MethodPut, path, `{"name":"x"}`); res.Code != http.StatusNotFound {
		t.Errorf("改不存在的应 404：%d", res.Code)
	}
}

func itoa(n uint) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

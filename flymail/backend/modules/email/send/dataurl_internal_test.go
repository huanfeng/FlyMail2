package send

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"flymail-core/types"

	"github.com/gin-gonic/gin"

	"flymail/modules/email/account"
	"flymail/modules/email/folder"
)

// 这几个用例注入小阈值来验证内存闸门：真按生产阈值构造输入要几十 MB，
// 而闸门本身与阈值大小无关。放在包内测试是为了能改这些不导出的旋钮。

// setLimits 临时替换大小阈值，用例结束后还原。
func setLimits(t *testing.T, htmlMax int, attTotal int64) {
	t.Helper()
	oldHTML, oldTotal := maxInlineHTML, maxAttachmentTotal
	maxInlineHTML, maxAttachmentTotal = htmlMax, attTotal
	t.Cleanup(func() { maxInlineHTML, maxAttachmentTotal = oldHTML, oldTotal })
}

// dataImg 拼一张 n 字节的 data: 图。
func dataImg(n int) string {
	return `<img src="data:image/png;base64,` +
		base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'x'}, n)) + `">`
}

// TestInlineHTMLSizeGate 正文长度是解码前唯一 O(1) 可判的量。
// 没有它，"单张 10 MiB × 张数无上限" 能让一个请求把堆吃干（实测 2.4 GiB / 209 秒）。
func TestInlineHTMLSizeGate(t *testing.T) {
	setLimits(t, 64, 25<<20)

	// 每张图都远小于任何单张/总量限制，能触发的只可能是长度闸门
	html := strings.Repeat(dataImg(4), 4)
	out, atts, err := InlineDataURIImages(html)
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("超长正文应返回 ErrPayloadTooLarge，实际 %v", err)
	}
	if len(atts) != 0 || out != html {
		t.Errorf("拒收时应原样返回且不产出附件，实际 %d 个附件", len(atts))
	}
}

// TestInlineTotalBudgetStopsEarly 总量超限必须在解码前判掉：先把所有图解进内存
// 再判总量，等于把攻击者的输入完整放大一遍才拒绝（实测 40 张 9 MiB 图吃掉 2.4 GiB）。
//
// 这里用一段"字符集合法但解不开"的 base64 来区分两种实现：
// 先判长度的会返回超限错误；先解码的会解码失败、按"这张图保持原样"放过去，一声不吭。
func TestInlineTotalBudgetStopsEarly(t *testing.T) {
	setLimits(t, 40<<20, 1<<10)

	// 4097 个字符不是 4 的倍数，必然解不开；但声明的解码长度 3072 已超出 1 KiB 预算
	html := `<img src="data:image/png;base64,` + strings.Repeat("A", 4097) + `">`
	out, atts, err := InlineDataURIImages(html)
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("超预算应在解码前返回 ErrPayloadTooLarge，实际 %v", err)
	}
	if len(atts) != 0 || out != html {
		t.Errorf("拒收时应原样返回且不产出附件，实际 %d 个附件", len(atts))
	}
}

// TestInlineTotalBudgetAccumulates 张数无上限时，只有累计才拦得住：
// 每张都合规，加起来超总量同样要拒。
func TestInlineTotalBudgetAccumulates(t *testing.T) {
	setLimits(t, 40<<20, 1<<10)

	html := strings.Repeat(dataImg(400), 4) // 单张 400 字节合规，四张 1600 字节超 1 KiB
	if _, _, err := InlineDataURIImages(html); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("累计超总量应返回 ErrPayloadTooLarge，实际 %v", err)
	}
}

// TestInlineSingleImageBudget 单张超限同样是 400 级错误，不是服务端故障。
func TestInlineSingleImageBudget(t *testing.T) {
	setLimits(t, 40<<20, 25<<20)
	old := maxInlineDataURI
	maxInlineDataURI = 16
	t.Cleanup(func() { maxInlineDataURI = old })

	if _, _, err := InlineDataURIImages(dataImg(64)); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("单张超限应返回 ErrPayloadTooLarge，实际 %v", err)
	}
}

// TestSendRouteLimitsBodySize 路由层没有请求体上限时，正文闸门也只是在
// "已经把整个请求读进内存之后"才生效。MaxBytesReader 才是第一道闸。
func TestSendRouteLimitsBodySize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	old := maxRequestBody
	maxRequestBody = 64
	t.Cleanup(func() { maxRequestBody = old })

	r := gin.New()
	RegisterRoutes(r.Group(""), &Service{})

	body := `{"account_id":1,"to":["to@example.com"],"body_html":"` +
		strings.Repeat("x", 1024) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("超长请求体应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
}

// stubAccounts / stubFolders 是包内测试用的最小实现（外部测试包里的 fake 在这里用不到）。
type stubAccounts struct{}

func (stubAccounts) SMTPConfig(uint) (types.SMTPConfig, error) { return types.SMTPConfig{}, nil }
func (stubAccounts) IMAPConfig(uint) (types.IMAPConfig, error) { return types.IMAPConfig{}, nil }
func (stubAccounts) Get(uint) (*account.AccountResponse, error) {
	return &account.AccountResponse{ID: 1, Email: "sender@example.com"}, nil
}
func (stubAccounts) ResolveFrom(uint, string) (string, string, error) {
	return "sender@example.com", "", nil
}

type stubFolders struct{}

func (stubFolders) FindByType(uint, string) (*folder.Folder, error) { return nil, nil }

// TestSendRouteInlineTooLargeIsBadRequest 正文里的 data: 图超限是客户端输入问题：
// 回 500 会让前端提示"服务器错误"并鼓励重试，而重试只会再吃一遍同样的内存。
func TestSendRouteInlineTooLargeIsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setLimits(t, 40<<20, 1<<10)

	sent := false
	svc := NewService(stubAccounts{}, stubFolders{})
	svc.SetSenders(
		func(types.SMTPConfig, string, []string, []byte) error { sent = true; return nil },
		func(types.IMAPConfig, string, []byte) error { return nil },
	)
	r := gin.New()
	RegisterRoutes(r.Group(""), svc)

	payload, err := json.Marshal(map[string]any{
		"account_id": 1,
		"to":         []string{"to@example.com"},
		"body_html":  dataImg(4 << 10), // 4 KiB，超出注入的 1 KiB 预算
	})
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("内联图超限应 400，实际 %d: %s", rec.Code, rec.Body.String())
	}
	if sent {
		t.Error("拒收的请求不应触发 SMTP 发送")
	}
}

// allocOf 返回执行 fn 期间的累计分配字节数。
func allocOf(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestStripWSMatchesBase64Len 预算闸门用 base64Len 估大小、去空白用 stripWS 定容量，
// 两者对"什么算空白"的定义必须一模一样，否则估出来的大小与实际处理的字符对不上。
func TestStripWSMatchesBase64Len(t *testing.T) {
	for _, payload := range []string{
		"AAAA", "", "   ", "AA\tBB", "AA\nBB\r\nCC", "A\vB\fC", " A B C ", "AAAA====",
	} {
		got := stripWS(payload)
		if len(got) != base64Len(payload) {
			t.Errorf("payload %q: stripWS 长度 %d 与 base64Len %d 不一致", payload, len(got), base64Len(payload))
		}
		if strings.ContainsAny(got, " \t\n\v\f\r") {
			t.Errorf("payload %q: 结果仍含空白 %q", payload, got)
		}
	}
}

// TestStripWSAllocationTracksValidChars 去空白的分配量只能与有效字符数挂钩。
// strings.Fields 会为每段非空白 run 各留一个 16 字节的 string header——
// "A A A …" 这种 payload 上就是有效字符数的 16 倍，与预算闸门判的解码大小完全脱钩。
func TestStripWSAllocationTracksValidChars(t *testing.T) {
	payload := strings.Repeat("A ", 1<<20) // 2 MiB 输入，1 Mi 个有效字符
	var got string
	grew := allocOf(func() { got = stripWS(payload) })

	if len(got) != 1<<20 {
		t.Fatalf("有效字符数应为 %d，实际 %d", 1<<20, len(got))
	}
	// 缓冲区 + string(b) 两份即 2×，留一倍余量
	if limit := uint64(4 * len(got)); grew > limit {
		t.Errorf("去空白分配了 %d 字节（上限 %d），说明分配量跟着空白密度走", grew, limit)
	}
}

// TestInlineWhitespaceDensePayloadDoesNotAmplify 空白密集的 payload 不能绕过预算闸门。
//
// 闸门只约束"解码后的字节数"，而去空白那一步若按「每段非空白 run 一个 string header」
// 计费，代价就跟着空白密度走：一封卡在 maxInlineHTML 之下的信照样能把堆吃掉
// （实测 8 MiB 输入吃 119 MiB）。断言方式是同长度的密集/稀疏两份输入对比——
// 密集那份有效字符只有一半，分配量绝不该反而更多。用比值而不是绝对阈值，
// 是因为绝对值里混着与本问题无关的常数倍拷贝（tokenizer、属性、输出缓冲）。
func TestInlineWhitespaceDensePayloadDoesNotAmplify(t *testing.T) {
	const half = 1 << 20
	dense := strings.Repeat("A ", half)   // 2 MiB，1 Mi 个有效字符，每个都是独立一段
	sparse := strings.Repeat("A", 2*half) // 同样 2 MiB，但只有一段

	for _, tc := range []struct{ name, prefix, suffix string }{
		{"src", `<img src="data:image/png;base64,`, `">`},
		{"srcset", `<img srcset="data:image/png;base64,`, ` 2x">`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := func(payload string) uint64 {
				html := tc.prefix + payload + tc.suffix
				return allocOf(func() {
					if _, _, err := InlineDataURIImages(html); err != nil {
						t.Fatalf("不该报错: %v", err)
					}
				})
			}
			denseAlloc, sparseAlloc := run(dense), run(sparse)
			if limit := sparseAlloc * 13 / 10; denseAlloc > limit {
				t.Errorf("空白密集输入分配 %d 字节，同长度稀疏输入只要 %d（上限 %d）——分配量跟着空白密度走了",
					denseAlloc, sparseAlloc, limit)
			}
		})
	}
}

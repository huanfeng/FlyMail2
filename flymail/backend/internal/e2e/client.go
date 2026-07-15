package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// apiClient 调 flymail 真实 HTTP API 的测试客户端。
// DTO 字段名均已核对源码（modules/auth、modules/email/{account,folder,message,sync,send}）。
type apiClient struct {
	t       *testing.T
	baseURL string
	token   string
}

func newClient(t *testing.T, ta *testApp) *apiClient {
	t.Helper()
	c := &apiClient{t: t, baseURL: ta.baseURL}
	c.login(adminUser, adminPass)
	return c
}

func (c *apiClient) do(method, path string, body any) (*http.Response, []byte) {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("marshal %s %s: %v", method, path, err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, rdr)
	if err != nil {
		c.t.Fatalf("new request %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, data
}

// mustJSON 断言状态码并把响应解码到 out（out 可为 nil）。
func (c *apiClient) mustJSON(method, path string, body any, wantStatus int, out any) {
	c.t.Helper()
	resp, data := c.do(method, path, body)
	if resp.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: 期望 %d 实际 %d: %s", method, path, wantStatus, resp.StatusCode, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: 解码失败 %v: %s", method, path, err, data)
		}
	}
}

// login 调 /auth/login 并保存 access_token（响应无外层包装）。
func (c *apiClient) login(user, pass string) {
	c.t.Helper()
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	c.mustJSON(http.MethodPost, "/api/v1/auth/login",
		map[string]string{"username": user, "password": pass}, http.StatusOK, &out)
	if out.AccessToken == "" {
		c.t.Fatal("login 响应缺 access_token")
	}
	c.token = out.AccessToken
}

// createAccount 建一个指向 GreenMail 的账户（security=none，auth.disabled 密码随意），返回账户 id。
// 注意：账户创建即 enabled=true，后台 Manager（若已启动）最迟 30s reconcile 拾取。
func (c *apiClient) createAccount(mailbox string) uint {
	c.t.Helper()
	var out struct {
		ID uint `json:"id"`
	}
	c.mustJSON(http.MethodPost, "/api/v1/accounts", map[string]any{
		"name":          "e2e-" + mailbox,
		"email":         mailbox,
		"username":      mailbox,
		"password":      "e2e",
		"imap_host":     greenmailHost(),
		"imap_port":     greenmailIMAPPort(),
		"imap_security": "none",
		"smtp_host":     greenmailHost(),
		"smtp_port":     greenmailSMTPPort(),
		"smtp_security": "none",
	}, http.StatusCreated, &out)
	if out.ID == 0 {
		c.t.Fatal("createAccount 响应缺 id")
	}
	return out.ID
}

// triggerSyncAndWait 触发同步并轮询状态至 done（error 则失败）。
// 触发返回 202；409（已在跑，如 Manager 自动同步）视为已触发，继续等。
func (c *apiClient) triggerSyncAndWait(accountID uint, timeout time.Duration) {
	c.t.Helper()
	resp, data := c.do(http.MethodPost, "/api/v1/accounts/"+utoa(accountID)+"/sync", nil)
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusConflict {
		c.t.Fatalf("trigger sync: 期望 202/409 实际 %d: %s", resp.StatusCode, data)
	}
	var st struct {
		Phase string `json:"phase"`
		Error string `json:"error"`
	}
	eventually(c.t, timeout, 300*time.Millisecond, "同步完成 account "+utoa(accountID), func() bool {
		c.mustJSON(http.MethodGet, "/api/v1/accounts/"+utoa(accountID)+"/sync/status", nil, http.StatusOK, &st)
		if st.Phase == "error" {
			c.t.Fatalf("同步失败: %s", st.Error)
		}
		return st.Phase == "done"
	})
}

type folderDTO struct {
	ID          uint   `json:"id"`
	AccountID   uint   `json:"account_id"`
	Path        string `json:"path"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	Selectable  bool   `json:"selectable"`
	TotalCount  int    `json:"total_count"`
	UnreadCount int    `json:"unread_count"`
}

// listFolders 列账户文件夹（响应外层包 {"folders":[...]}）。
func (c *apiClient) listFolders(accountID uint) []folderDTO {
	c.t.Helper()
	var out struct {
		Folders []folderDTO `json:"folders"`
	}
	c.mustJSON(http.MethodGet, "/api/v1/accounts/"+utoa(accountID)+"/folders", nil, http.StatusOK, &out)
	return out.Folders
}

// findFolder 按 type 找文件夹（inbox/sent/trash/...），找不到返回 nil。
func findFolder(folders []folderDTO, typ string) *folderDTO {
	for i := range folders {
		if folders[i].Type == typ {
			return &folders[i]
		}
	}
	return nil
}

type addressDTO struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type messageItem struct {
	ID       uint         `json:"id"`
	FolderID uint         `json:"folder_id"`
	UID      uint32       `json:"uid"`
	Subject  string       `json:"subject"`
	FromName string       `json:"from_name"`
	FromAddr string       `json:"from_addr"`
	To       []addressDTO `json:"to"`
	Seen     bool         `json:"seen"`
	Flagged  bool         `json:"flagged"`
	Snippet  string       `json:"snippet"`
}

// listMessages 列文件夹邮件（响应外层包 {"messages":[...]}，UID 降序）。
func (c *apiClient) listMessages(folderID uint) []messageItem {
	c.t.Helper()
	var out struct {
		Messages []messageItem `json:"messages"`
	}
	c.mustJSON(http.MethodGet, "/api/v1/folders/"+utoa(folderID)+"/messages", nil, http.StatusOK, &out)
	return out.Messages
}

type messageDetail struct {
	messageItem
	Cc         []addressDTO `json:"cc"`
	TextBody   string       `json:"text_body"`
	HTMLBody   string       `json:"html_body"`
	BodySynced bool         `json:"body_synced"`
}

// messageDetail 取邮件详情（首访会按需连 IMAP 抓正文）。
func (c *apiClient) messageDetail(id uint) messageDetail {
	c.t.Helper()
	var out messageDetail
	c.mustJSON(http.MethodGet, "/api/v1/messages/"+utoa(id), nil, http.StatusOK, &out)
	return out
}

// markRead / markFlagged / deleteMessage / moveMessage：回写动作（本地写库即返回 200，
// IMAP 回写走异步单 worker 队列，服务器端断言必须 eventually 轮询）。
func (c *apiClient) markRead(id uint, read bool) {
	c.t.Helper()
	c.mustJSON(http.MethodPost, "/api/v1/messages/"+utoa(id)+"/read",
		map[string]bool{"read": read}, http.StatusOK, nil)
}

func (c *apiClient) markFlagged(id uint, flagged bool) {
	c.t.Helper()
	c.mustJSON(http.MethodPost, "/api/v1/messages/"+utoa(id)+"/flag",
		map[string]bool{"flagged": flagged}, http.StatusOK, nil)
}

func (c *apiClient) deleteMessage(id uint) {
	c.t.Helper()
	c.mustJSON(http.MethodPost, "/api/v1/messages/"+utoa(id)+"/delete", nil, http.StatusOK, nil)
}

func (c *apiClient) moveMessage(id, folderID uint) {
	c.t.Helper()
	c.mustJSON(http.MethodPost, "/api/v1/messages/"+utoa(id)+"/move",
		map[string]uint{"folder_id": folderID}, http.StatusOK, nil)
}

// send 发送邮件（to 为字符串数组，正文字段 body_html）。
func (c *apiClient) send(accountID uint, to []string, subject, bodyHTML string) {
	c.t.Helper()
	c.mustJSON(http.MethodPost, "/api/v1/send", map[string]any{
		"account_id": accountID,
		"to":         to,
		"subject":    subject,
		"body_html":  bodyHTML,
	}, http.StatusOK, nil)
}

func utoa(n uint) string { return strconv.FormatUint(uint64(n), 10) }

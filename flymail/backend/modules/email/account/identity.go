package account

import (
	"errors"
	"net/mail"
	"strings"
	"time"

	"gorm.io/gorm"

	"flymail/internal/htmlsan"
)

// ── 发件身份：别名 + 签名 ──────────────────────────────────────────────────────
// 两者回答的是同一个问题：这封信以谁的身份发出、末尾署谁的名。

var (
	ErrAliasNotFound  = errors.New("alias not found")
	ErrAliasDuplicate = errors.New("alias already exists")
	ErrAliasIsPrimary = errors.New("alias equals account primary address")
	ErrInvalidEmail   = errors.New("invalid email address")
)

// Alias 是账户下的一个可选发信地址。
type Alias struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	AccountID   uint   `gorm:"not null;uniqueIndex:idx_alias_account_email" json:"account_id"`
	Email       string `gorm:"not null;uniqueIndex:idx_alias_account_email" json:"email"`
	DisplayName string `json:"display_name"`
	// IsDefault 同账户至多一个：置位时其余在同一事务内清零。
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Alias) TableName() string { return "account_aliases" }

// Signature 与账户 1:1——主键即外键。
type Signature struct {
	AccountID  uint      `gorm:"primaryKey" json:"account_id"`
	BodyHTML   string    `json:"body_html"`
	UseOnNew   bool      `json:"use_on_new"`
	UseOnReply bool      `json:"use_on_reply"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Signature) TableName() string { return "account_signatures" }

// AliasRequest 别名的新增/修改入参。
type AliasRequest struct {
	Email       string `json:"email" binding:"required,email"`
	DisplayName string `json:"display_name"`
	IsDefault   bool   `json:"is_default"`
}

// SignatureRequest 签名保存入参。
type SignatureRequest struct {
	BodyHTML   string `json:"body_html"`
	UseOnNew   bool   `json:"use_on_new"`
	UseOnReply bool   `json:"use_on_reply"`
}

// normalizeEmail 统一小写去空格：地址比较（去重、别名归属校验）必须走它，
// 否则 Sales@x.com 能绕过与 sales@x.com 的重复检查。
func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// parseAddrSpec 校验并规范化别名地址，只接受朴素的 addr-spec。
//
// 不能只调 mail.ParseAddress 就把原始输入落库：它接受的是整个 name-addr，
// 所以 "Bob <bob@e.com>"（别名表单旁边就有显示名栏，这是极自然的误输入）会被判合法。
// 那个串原样存下去，发信时写出的是尖括号不配对的 From 头和多一个 > 的 Message-ID——
// 严格的 MTA 直接 501 拒信，宽松的投进去但线程断掉。而保存那一刻用户看到的是"成功"。
func parseAddrSpec(input string) (string, error) {
	parsed, err := mail.ParseAddress(strings.TrimSpace(input))
	if err != nil || parsed.Name != "" {
		return "", ErrInvalidEmail
	}
	addr := normalizeEmail(parsed.Address)
	if !plainAddrSpec(addr) {
		return "", ErrInvalidEmail
	}
	return addr, nil
}

// addrSpecForbidden 是地址里不允许出现的字符：它们在邮件头里都有语法含义。
const addrSpecForbidden = "\"'()<>[],;:\\"

// plainAddrSpec 判断是否为可直接写进邮件头的朴素地址：
// 纯 ASCII、单个 @、无引号/空格/括号，域名部分不是 [127.0.0.1] 这种字面量。
func plainAddrSpec(s string) bool {
	if s == "" || len(s) > 254 {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at != strings.LastIndexByte(s, '@') || at == len(s)-1 {
		return false
	}
	if s[at+1] == '[' { // 域名字面量：合法 RFC 但没有服务器会认这种别名
		return false
	}
	for _, r := range s {
		if r <= 0x20 || r > 0x7e {
			return false // 控制字符、空格，以及裸 UTF-8（没协商 SMTPUTF8 会被拒）
		}
	}
	// 这些字符在邮件头里有语法含义，出现在地址里就会把 From/Message-ID 写坏
	return !strings.ContainsAny(s, addrSpecForbidden)
}

// ── Repository ────────────────────────────────────────────────────────────────

func (r *Repository) ListAliases(accountID uint) ([]Alias, error) {
	var list []Alias
	err := r.db.Where("account_id = ?", accountID).Order("id asc").Find(&list).Error
	return list, err
}

func (r *Repository) GetAlias(accountID, aliasID uint) (*Alias, error) {
	var a Alias
	err := r.db.Where("id = ? AND account_id = ?", aliasID, accountID).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAliasNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// isUniqueViolation 判断错误是否来自唯一索引冲突。
// core 侧建连没开 gorm 的 TranslateError，所以拿不到 gorm.ErrDuplicatedKey，
// 只能认驱动原文；两条都判，日后 core 打开翻译这里不用跟着改。
func isUniqueViolation(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// SaveAlias 新增或更新别名；IsDefault 置位时在同一事务里清零同账户其余项。
//
// 服务层的查重是第一道防线，但并发下两个请求可能都查完再都写——最终挡住的是唯一索引。
// 那条错误必须仍然是 ErrAliasDuplicate（→ 409），落到 handler 的默认分支就成了 500，
// 前端只能显示"操作失败"，用户看不出重试没有意义。
func (r *Repository) SaveAlias(a *Alias) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(a).Error; err != nil {
			if isUniqueViolation(err) {
				return ErrAliasDuplicate
			}
			return err
		}
		if !a.IsDefault {
			return nil
		}
		return tx.Model(&Alias{}).
			Where("account_id = ? AND id <> ?", a.AccountID, a.ID).
			Update("is_default", false).Error
	})
}

func (r *Repository) DeleteAlias(accountID, aliasID uint) error {
	res := r.db.Where("account_id = ?", accountID).Delete(&Alias{}, aliasID)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAliasNotFound
	}
	return nil
}

// GetSignature 取签名；未配置返回空签名而非错误。
// 用 Find 而不是 First：没配签名是绝大多数账户的常态，First 会让 GORM
// 把 ErrRecordNotFound 当错误打进日志，每次读都刷一条噪音。
func (r *Repository) GetSignature(accountID uint) (*Signature, error) {
	var list []Signature
	if err := r.db.Where("account_id = ?", accountID).Limit(1).Find(&list).Error; err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return &Signature{AccountID: accountID}, nil
	}
	return &list[0], nil
}

func (r *Repository) SaveSignature(s *Signature) error { return r.db.Save(s).Error }

// ── Service ───────────────────────────────────────────────────────────────────

func (s *Service) ListAliases(accountID uint) ([]Alias, error) {
	if _, err := s.repo.GetByID(accountID); err != nil {
		return nil, err
	}
	list, err := s.repo.ListAliases(accountID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []Alias{}
	}
	return list, nil
}

// CreateAlias 新增别名。
func (s *Service) CreateAlias(accountID uint, req AliasRequest) (*Alias, error) {
	return s.saveAlias(accountID, nil, req)
}

// UpdateAlias 修改别名。先读出原记录再改字段：gorm.Save 对带主键的记录做全字段 UPDATE，
// 拿一个新结构体去存会把 CreatedAt 刷成零值。
func (s *Service) UpdateAlias(accountID, aliasID uint, req AliasRequest) (*Alias, error) {
	existing, err := s.repo.GetAlias(accountID, aliasID)
	if err != nil {
		return nil, err
	}
	return s.saveAlias(accountID, existing, req)
}

// saveAlias 校验并保存；existing 为 nil 表示新增。
func (s *Service) saveAlias(accountID uint, existing *Alias, req AliasRequest) (*Alias, error) {
	acct, err := s.repo.GetByID(accountID)
	if err != nil {
		return nil, err
	}
	email, err := parseAddrSpec(req.Email)
	if err != nil {
		return nil, err
	}
	// 主地址本来就能发信，收进别名只会让发件人下拉出现两个一样的条目。
	if email == normalizeEmail(acct.Email) {
		return nil, ErrAliasIsPrimary
	}
	var aliasID uint
	if existing != nil {
		aliasID = existing.ID
	}
	siblings, err := s.repo.ListAliases(accountID)
	if err != nil {
		return nil, err
	}
	for i := range siblings {
		if siblings[i].ID != aliasID && normalizeEmail(siblings[i].Email) == email {
			return nil, ErrAliasDuplicate
		}
	}
	a := &Alias{AccountID: accountID}
	if existing != nil {
		a = existing // 保留 ID 与 CreatedAt
	}
	a.Email = email
	a.DisplayName = strings.TrimSpace(req.DisplayName)
	a.IsDefault = req.IsDefault
	if err := s.repo.SaveAlias(a); err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Service) DeleteAlias(accountID, aliasID uint) error {
	return s.repo.DeleteAlias(accountID, aliasID)
}

// ResolveFrom 解析发信身份：alias 为空用账户主地址，否则必须是该账户已配置的别名。
// 这是防伪造的唯一关卡——前端传什么地址都不可信，服务端必须自己查一遍归属。
func (s *Service) ResolveFrom(accountID uint, alias string) (addr string, display string, err error) {
	acct, err := s.repo.GetByID(accountID)
	if err != nil {
		return "", "", err
	}
	alias = normalizeEmail(alias)
	if alias == "" || alias == normalizeEmail(acct.Email) {
		return acct.Email, acct.Name, nil
	}
	list, err := s.repo.ListAliases(accountID)
	if err != nil {
		return "", "", err
	}
	for i := range list {
		if normalizeEmail(list[i].Email) == alias {
			return list[i].Email, list[i].DisplayName, nil
		}
	}
	return "", "", ErrAliasNotFound
}

func (s *Service) GetSignature(accountID uint) (*Signature, error) {
	if _, err := s.repo.GetByID(accountID); err != nil {
		return nil, err
	}
	return s.repo.GetSignature(accountID)
}

// SaveSignature 保存签名。HTML 先净化再落库：签名虽由本人编辑，
// 但它会被注入撰写器 DOM 并随每封信发出，净化一次比日后排查便宜得多。
func (s *Service) SaveSignature(accountID uint, req SignatureRequest) (*Signature, error) {
	if _, err := s.repo.GetByID(accountID); err != nil {
		return nil, err
	}
	sig := &Signature{
		AccountID:  accountID,
		BodyHTML:   htmlsan.Sanitize(req.BodyHTML, true).HTML,
		UseOnNew:   req.UseOnNew,
		UseOnReply: req.UseOnReply,
	}
	if err := s.repo.SaveSignature(sig); err != nil {
		return nil, err
	}
	return sig, nil
}

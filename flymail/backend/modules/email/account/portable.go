package account

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ── 账户配置的导出 / 导入 ────────────────────────────────────────────────────
//
// 用途是「换一台机器继续用」和「重装前留一份」，不是备份工具——
// 邮件、附件、规则、签名都不在这里，整库备份走 `flymail db backup`。
//
// ⚠ 关于密码：当前版本**只能导出明文**。
//
// 库里存的是 AES 密文，而密钥来自部署的 FLYMAIL_CRYPTO_ENCRYPTION_KEY——
// 换一台机器就换了一把钥匙，原样搬过去解不开，等于没导。所以导出必须先解密，
// 落到文件里就是明文。
//
// 这不是一个可以靠"注意保管"糊过去的事实，所以：
//   1. 密码是**独立开关**，默认不导（IncludePasswords）
//   2. 导出物里带 `encrypted: false`，让文件自己说明白自己是什么
//   3. 前端在勾选时必须二次确认并明示风险
//
// 以后要加口令加密（PBKDF2 + AES-GCM）时，`Encrypted` 字段就是切换点：
// 导入侧按它决定要不要问口令，老文件照旧能读。

// PortableVersion 是导出格式的版本号。
//
// 独立于应用版本：应用升级得比格式频繁得多，拿应用版本做判据会让导入侧
// 无谓地拒绝一堆其实兼容的文件。只有字段语义真的变了才 +1。
const PortableVersion = 1

// PortableBundle 是导出文件的顶层结构。
type PortableBundle struct {
	Version int `json:"version"`
	// Encrypted 标记密码字段是不是密文。当前恒为 false（见本文件顶部说明）；
	// 留着它是为了让导入侧从第一天起就有判据，而不是以后去猜老文件的格式。
	Encrypted  bool              `json:"encrypted"`
	ExportedAt time.Time         `json:"exported_at"`
	Accounts   []PortableAccount `json:"accounts"`
}

// PortableAccount 是一个账户的可搬运配置。
//
// 刻意**不复用** AccountResponse：那个是给界面看的，字段随 UI 需要增删，
// 拿它当交换格式会让每次 UI 改动都变成一次格式破坏。
type PortableAccount struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Username string `json:"username,omitempty"`
	// Password 仅在导出时勾选了「含密码」才有值。omitempty 不够——
	// 空字符串和"没有这个字段"在导入侧是两件事（前者会把密码清空），
	// 所以用指针：nil = 这份导出不含密码。
	Password     *string `json:"password,omitempty"`
	AuthType     string  `json:"auth_type"`
	IMAPHost     string  `json:"imap_host"`
	IMAPPort     int     `json:"imap_port"`
	IMAPSecurity string  `json:"imap_security"`
	SMTPHost     string  `json:"smtp_host"`
	SMTPPort     int     `json:"smtp_port"`
	SMTPSecurity string  `json:"smtp_security"`

	ProxyType     string  `json:"proxy_type,omitempty"`
	ProxyHost     string  `json:"proxy_host,omitempty"`
	ProxyPort     int     `json:"proxy_port,omitempty"`
	ProxyUsername string  `json:"proxy_username,omitempty"`
	ProxyPassword *string `json:"proxy_password,omitempty"`

	Enabled bool `json:"enabled"`
}

// ErrPortableVersion 表示导入文件的格式版本本程序读不了。
var ErrPortableVersion = errors.New("unsupported export version")

// ErrPortableEncrypted 表示文件是加密的，而当前版本还不会解。
var ErrPortableEncrypted = errors.New("encrypted export is not supported yet")

// Export 导出指定账户的配置。ids 为空表示全部。
//
// includePasswords 为真时把库里的密文解密成明文写进去——调用方必须已经
// 向用户明示过这件事（见本文件顶部）。
func (s *Service) Export(ids []uint, includePasswords bool) (*PortableBundle, error) {
	all, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	want := map[uint]bool{}
	for _, id := range ids {
		want[id] = true
	}

	out := make([]PortableAccount, 0, len(all))
	for i := range all {
		a := &all[i]
		if len(want) > 0 && !want[a.ID] {
			continue
		}
		pa := PortableAccount{
			Name: a.Name, Email: a.Email, Username: a.Username,
			AuthType:      a.AuthType,
			IMAPHost:      a.IMAPHost,
			IMAPPort:      a.IMAPPort,
			IMAPSecurity:  a.IMAPSecurity,
			SMTPHost:      a.SMTPHost,
			SMTPPort:      a.SMTPPort,
			SMTPSecurity:  a.SMTPSecurity,
			ProxyType:     a.ProxyType,
			ProxyHost:     a.ProxyHost,
			ProxyPort:     a.ProxyPort,
			ProxyUsername: a.ProxyUsername,
			Enabled:       a.Enabled,
		}
		if includePasswords {
			// ⚠ OAuth 账户不导令牌：refresh_token 的作用域通常覆盖整个邮箱，
			// 比密码更危险，而且多数服务商会把它绑定到签发时的 client_id——
			// 搬到别处也用不了。导入侧重新走一次授权即可。
			if a.AuthType == AuthTypePassword && a.PasswordEnc != "" {
				pw, derr := s.enc.Decrypt(a.PasswordEnc)
				if derr != nil {
					return nil, fmt.Errorf("解密账户 %s 的密码失败: %w", a.Email, derr)
				}
				pa.Password = &pw
			}
			if a.ProxyPasswordEnc != "" {
				pp, derr := s.enc.Decrypt(a.ProxyPasswordEnc)
				if derr != nil {
					return nil, fmt.Errorf("解密账户 %s 的代理密码失败: %w", a.Email, derr)
				}
				pa.ProxyPassword = &pp
			}
		}
		out = append(out, pa)
	}

	return &PortableBundle{
		Version:    PortableVersion,
		Encrypted:  false,
		ExportedAt: time.Now(),
		Accounts:   out,
	}, nil
}

// ImportMode 决定遇到"这个邮箱地址已经有账户了"时怎么办。
type ImportMode string

const (
	// ImportSkip 跳过已存在的（默认，最安全：不会动到正在用的账户）
	ImportSkip ImportMode = "skip"
	// ImportOverwrite 用导入的配置覆盖已存在的同邮箱账户
	ImportOverwrite ImportMode = "overwrite"
)

// ImportOutcome 是单个账户的导入结果。
type ImportOutcome struct {
	Email  string `json:"email"`
	Action string `json:"action"` // created / updated / skipped / failed
	Error  string `json:"error,omitempty"`
}

// ImportResult 汇总一次导入。
type ImportResult struct {
	Created  int             `json:"created"`
	Updated  int             `json:"updated"`
	Skipped  int             `json:"skipped"`
	Failed   int             `json:"failed"`
	Outcomes []ImportOutcome `json:"outcomes"`
}

// Import 按 bundle 建立/更新账户。
//
// only 非空时只导入其中列出的邮箱地址——这是"精细化操作"的落点：
// 用户可以拿一份包含五个账户的文件，只挑其中两个导进来。
//
// **逐个账户独立处理，不是一个事务**：一个账户的字段有问题不该让另外四个
// 也导不进来。结果里逐条报告，由用户决定要不要修了再来一次。
func (s *Service) Import(b *PortableBundle, mode ImportMode, only []string) (*ImportResult, error) {
	if b == nil {
		return nil, errors.New("empty bundle")
	}
	if b.Version > PortableVersion {
		return nil, fmt.Errorf("%w: 文件版本 %d，本程序最高支持 %d", ErrPortableVersion, b.Version, PortableVersion)
	}
	if b.Encrypted {
		return nil, ErrPortableEncrypted
	}
	if mode != ImportOverwrite {
		mode = ImportSkip
	}

	pick := map[string]bool{}
	for _, e := range only {
		pick[strings.ToLower(strings.TrimSpace(e))] = true
	}

	existing, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	byEmail := map[string]*Account{}
	for i := range existing {
		byEmail[strings.ToLower(existing[i].Email)] = &existing[i]
	}

	res := &ImportResult{Outcomes: make([]ImportOutcome, 0, len(b.Accounts))}
	for i := range b.Accounts {
		pa := &b.Accounts[i]
		key := strings.ToLower(strings.TrimSpace(pa.Email))
		if len(pick) > 0 && !pick[key] {
			continue
		}

		cur, exists := byEmail[key]
		if exists && mode == ImportSkip {
			res.Skipped++
			res.Outcomes = append(res.Outcomes, ImportOutcome{Email: pa.Email, Action: "skipped"})
			continue
		}

		var ierr error
		if exists {
			ierr = s.applyPortable(cur, pa)
		} else {
			ierr = s.createFromPortable(pa)
		}
		switch {
		case ierr != nil:
			res.Failed++
			res.Outcomes = append(res.Outcomes, ImportOutcome{Email: pa.Email, Action: "failed", Error: ierr.Error()})
		case exists:
			res.Updated++
			res.Outcomes = append(res.Outcomes, ImportOutcome{Email: pa.Email, Action: "updated"})
		default:
			res.Created++
			res.Outcomes = append(res.Outcomes, ImportOutcome{Email: pa.Email, Action: "created"})
		}
	}
	return res, nil
}

// validPortable 做导入前的最小校验。
//
// 只拦"导进去必然用不了"的情形，不做格式美化：这是别的程序也可能生成的文件，
// 过度挑剔只会让它难用。
func validPortable(pa *PortableAccount) error {
	if strings.TrimSpace(pa.Email) == "" {
		return errors.New("缺少邮箱地址")
	}
	if strings.TrimSpace(pa.IMAPHost) == "" || pa.IMAPPort <= 0 {
		return errors.New("缺少 IMAP 主机或端口")
	}
	if strings.TrimSpace(pa.SMTPHost) == "" || pa.SMTPPort <= 0 {
		return errors.New("缺少 SMTP 主机或端口")
	}
	return nil
}

// createFromPortable 按导入数据新建账户。
func (s *Service) createFromPortable(pa *PortableAccount) error {
	if err := validPortable(pa); err != nil {
		return err
	}
	name := pa.Name
	if strings.TrimSpace(name) == "" {
		name = pa.Email
	}
	a := &Account{
		Name: name, Email: pa.Email, Username: pa.Username,
		AuthType:     AuthTypePassword,
		IMAPHost:     pa.IMAPHost,
		IMAPPort:     pa.IMAPPort,
		IMAPSecurity: pa.IMAPSecurity,
		SMTPHost:     pa.SMTPHost,
		SMTPPort:     pa.SMTPPort,
		SMTPSecurity: pa.SMTPSecurity,
		Status:       StatusNew,
		Enabled:      pa.Enabled,
	}
	// 没带密码的导入照样建账户，只是连不上——用户随后在界面里补一次密码即可。
	// 比整条拒掉好：服务器地址、端口、加密方式这些才是搬家时真正烦人的部分。
	if pa.Password != nil {
		enc, err := s.enc.Encrypt(*pa.Password)
		if err != nil {
			return err
		}
		a.PasswordEnc = enc
	}
	if err := s.applyPortableProxy(a, pa); err != nil {
		return err
	}
	return s.repo.Create(a)
}

// applyPortable 用导入数据覆盖已存在的账户。
func (s *Service) applyPortable(cur *Account, pa *PortableAccount) error {
	if err := validPortable(pa); err != nil {
		return err
	}
	if strings.TrimSpace(pa.Name) != "" {
		cur.Name = pa.Name
	}
	cur.Username = pa.Username
	cur.IMAPHost = pa.IMAPHost
	cur.IMAPPort = pa.IMAPPort
	cur.IMAPSecurity = pa.IMAPSecurity
	cur.SMTPHost = pa.SMTPHost
	cur.SMTPPort = pa.SMTPPort
	cur.SMTPSecurity = pa.SMTPSecurity
	cur.Enabled = pa.Enabled

	// ⚠ 只在导入数据**带了**密码时才动它。不带（nil）说明这份导出没勾"含密码"，
	// 此时把现有密码清空是灾难性的：用户本意是同步服务器设置，
	// 结果一个能用的账户变成连不上，而且原密码已经没了。
	if pa.Password != nil {
		enc, err := s.enc.Encrypt(*pa.Password)
		if err != nil {
			return err
		}
		cur.PasswordEnc = enc
	}
	if err := s.applyPortableProxy(cur, pa); err != nil {
		return err
	}
	return s.repo.Update(cur)
}

// applyPortableProxy 写入代理设置。代理密码与账户密码同理：nil 表示不动。
func (s *Service) applyPortableProxy(a *Account, pa *PortableAccount) error {
	a.ProxyType = pa.ProxyType
	a.ProxyHost = pa.ProxyHost
	a.ProxyPort = pa.ProxyPort
	a.ProxyUsername = pa.ProxyUsername
	if pa.ProxyPassword != nil {
		enc, err := s.enc.Encrypt(*pa.ProxyPassword)
		if err != nil {
			return err
		}
		a.ProxyPasswordEnc = enc
	}
	return nil
}

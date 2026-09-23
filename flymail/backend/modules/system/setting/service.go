package setting

import (
	"errors"
	"strconv"
)

// ErrNoEncryptor 表示进程未注入加密器，密文设置无法保存。
// 正常部署不会遇到（app 层总会注入），它防的是「悄悄存成明文」。
var ErrNoEncryptor = errors.New("未配置加密器，无法保存密文设置")

// Encryptor 是密文设置所需的加解密能力（*crypto.Encryptor 满足之）。
// 用接口而不是具体类型：setting 是系统模块，不该为了一个可选能力去依赖 internal/crypto。
type Encryptor interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// Service 封装设置业务逻辑。
type Service struct {
	repo *Repository
	// enc 为 nil 时密文设置整体不可用：写入被拒、读取返回空。
	// 不静默存明文——那会让一个本该加密的凭据以为自己安全地躺在库里。
	enc Encryptor
}

func NewService(repo *Repository) *Service { return &Service{repo: repo} }

// SetEncryptor 注入密文设置所需的加解密器（由 app 层传入，与账户凭据同一把密钥）。
func (s *Service) SetEncryptor(e Encryptor) { s.enc = e }

// GetInt 取整数设置；取不到或解析失败返回 def。
func (s *Service) GetInt(key string, def int) int {
	val, found, err := s.repo.Get(key)
	if err != nil || !found {
		return def
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return n
}

// GetString 取字符串设置；取不到或为空返回 def。
func (s *Service) GetString(key, def string) string {
	val, found, err := s.repo.Get(key)
	if err != nil || !found || val == "" {
		return def
	}
	return val
}

// SetMany 批量保存键值对。密文设置（见 secretKeys）在落库前加密。
//
// 密文设置传空串的语义是**清除**，不是「存一个加密后的空串」：管理员要撤掉
// 一个 client_secret 时，界面上能做的就是把输入框清空。
func (s *Service) SetMany(m map[string]string) error {
	for k, v := range m {
		if isSecretKey(k) {
			if v == "" {
				if err := s.repo.Set(k, ""); err != nil {
					return err
				}
				continue
			}
			if s.enc == nil {
				return ErrNoEncryptor
			}
			ciphertext, err := s.enc.Encrypt(v)
			if err != nil {
				return err
			}
			if err := s.repo.Set(k, ciphertext); err != nil {
				return err
			}
			continue
		}
		if err := s.repo.Set(k, v); err != nil {
			return err
		}
	}
	return nil
}

// GetSecret 解密并返回一个密文设置；未配置、无加密器或解密失败都返回空串。
//
// 解密失败最常见的成因是**换过 FLYMAIL_CRYPTO_ENCRYPTION_KEY**（账户密码也会一起解不开）。
// 这里不把错误往上抛：调用方是「取 OAuth 凭据」这种随时可能被调用的路径，
// 返回空串会退化成「未配置」——入口置灰、提示去设置页重填，比整个流程报错好懂。
func (s *Service) GetSecret(key string) string {
	if s.enc == nil {
		return ""
	}
	val, found, err := s.repo.Get(key)
	if err != nil || !found || val == "" {
		return ""
	}
	plain, err := s.enc.Decrypt(val)
	if err != nil {
		return ""
	}
	return plain
}

// All 返回所有已存储的设置；调用方负责补充默认值。
//
// ⚠ 密文设置**不在返回值里**：它们被替换成 <key>_set = "true"/"false"。
// 这是全仓唯一会把 settings 表整个交出去的地方，密文若随之出网，
// 就等于把一个加密存储退化成「多编码了一层的明文」。
func (s *Service) All() map[string]string {
	m, err := s.repo.All()
	if err != nil {
		return map[string]string{}
	}
	for _, k := range secretKeys {
		v := m[k]
		delete(m, k)
		m[k+SecretSetSuffix] = strconv.FormatBool(v != "")
	}
	return m
}

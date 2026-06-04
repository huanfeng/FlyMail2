package send

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/logger"
	coresmtp "flymail-core/smtp"
	"flymail-core/types"

	imapv2 "github.com/emersion/go-imap/v2"
	"go.uber.org/zap"

	"flymail/modules/email/account"
	"flymail/modules/email/folder"
)

// ErrNoRecipient 未提供收件人时返回此错误。
var ErrNoRecipient = errors.New("no recipient: To field is empty")

// AccountProvider 账户服务所需最小接口（方便测试注入 fake）。
type AccountProvider interface {
	SMTPConfig(id uint) (types.SMTPConfig, error)
	IMAPConfig(id uint) (types.IMAPConfig, error)
	Get(id uint) (*account.AccountResponse, error)
}

// FolderProvider 文件夹服务所需最小接口。
type FolderProvider interface {
	FindByType(accountID uint, folderType string) (*folder.Folder, error)
}

// Service 邮件发送服务。
type Service struct {
	accounts AccountProvider
	folders  FolderProvider
	sendFn   func(cfg types.SMTPConfig, from string, recipients []string, raw []byte) error
	appendFn func(cfg types.IMAPConfig, mailbox string, raw []byte) error
	now      func() time.Time
}

// NewService 使用真实 SMTP/IMAP 实现构建 Service。
func NewService(accounts AccountProvider, folders FolderProvider) *Service {
	s := &Service{
		accounts: accounts,
		folders:  folders,
		now:      time.Now,
	}
	s.sendFn = func(cfg types.SMTPConfig, from string, recipients []string, raw []byte) error {
		return coresmtp.NewClient(cfg).SendRaw(from, recipients, raw)
	}
	s.appendFn = func(cfg types.IMAPConfig, mailbox string, raw []byte) error {
		sess, err := coreimap.Dial(cfg)
		if err != nil {
			return err
		}
		defer sess.Close()
		return sess.Append(mailbox, []imapv2.Flag{imapv2.FlagSeen}, raw)
	}
	return s
}

// SetSenders 注入测试用 sendFn 和 appendFn。
func (s *Service) SetSenders(
	sendFn func(cfg types.SMTPConfig, from string, recipients []string, raw []byte) error,
	appendFn func(cfg types.IMAPConfig, mailbox string, raw []byte) error,
) {
	s.sendFn = sendFn
	s.appendFn = appendFn
}

// SetNow 注入测试用时钟函数。
func (s *Service) SetNow(fn func() time.Time) {
	s.now = fn
}

// Send 构建 RFC 5322 邮件并通过 SMTP 发送，成功后尽力 APPEND 到"已发送"文件夹。
func (s *Service) Send(req SendRequest) error {
	if len(req.To) == 0 {
		return ErrNoRecipient
	}

	acct, err := s.accounts.Get(req.AccountID)
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}
	from := acct.Email

	smtpCfg, err := s.accounts.SMTPConfig(req.AccountID)
	if err != nil {
		return fmt.Errorf("get smtp config: %w", err)
	}

	now := s.now()
	messageID := generateMessageID(now, from)

	raw, err := BuildRFC5322(from, req, messageID, now)
	if err != nil {
		return fmt.Errorf("build message: %w", err)
	}

	// 收件人 = To + Cc + Bcc
	recipients := make([]string, 0, len(req.To)+len(req.Cc)+len(req.Bcc))
	recipients = append(recipients, req.To...)
	recipients = append(recipients, req.Cc...)
	recipients = append(recipients, req.Bcc...)

	if err := s.sendFn(smtpCfg, from, recipients, raw); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	// 尽力 APPEND 到"已发送"（失败只记日志，不返回错误）
	sentFolder, err := s.folders.FindByType(req.AccountID, "sent")
	if err != nil {
		logger.Warn("send: 查找已发送文件夹失败",
			zap.Uint("account_id", req.AccountID), zap.Error(err))
	}
	if sentFolder != nil {
		imapCfg, err := s.accounts.IMAPConfig(req.AccountID)
		if err != nil {
			logger.Warn("send: 取 IMAP 配置以 APPEND 失败",
				zap.Uint("account_id", req.AccountID), zap.Error(err))
		} else {
			if err := s.appendFn(imapCfg, sentFolder.Path, raw); err != nil {
				logger.Warn("send: APPEND 到已发送文件夹失败",
					zap.String("folder", sentFolder.Path), zap.Error(err))
			}
		}
	}

	return nil
}

// generateMessageID 生成唯一 Message-ID（不含尖括号）。
func generateMessageID(t time.Time, from string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	domain := "localhost"
	if idx := strings.LastIndex(from, "@"); idx >= 0 {
		domain = from[idx+1:]
	}
	return fmt.Sprintf("%d.%s@%s", t.UnixNano(), hex.EncodeToString(b), domain)
}

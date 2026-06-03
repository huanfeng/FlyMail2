package imap

import (
	"fmt"

	imapv2 "github.com/emersion/go-imap/v2"
)

// Move 把给定 UID 的邮件移动到 mailbox。
// 优先使用 MOVE 扩展（RFC 6851，原子移动）；服务器不支持时回退为
// COPY 到目标 + 源标记 \Deleted + EXPUNGE。
//
// 注意：回退路径会 EXPUNGE 当前所选文件夹内所有 \Deleted 邮件（与 Delete 相同行为），
// 单用户简单客户端可接受。
func (s *Session) Move(mailbox string, uids ...imapv2.UID) error {
	if s.Client == nil {
		return fmt.Errorf("not connected")
	}
	if len(uids) == 0 {
		return nil
	}

	var set imapv2.UIDSet
	set.AddNum(uids...)

	if s.Client.Caps().Has(imapv2.CapMove) {
		_, err := s.Client.Move(set, mailbox).Wait()
		return err
	}

	// 回退：COPY 后删除源。
	if _, err := s.Client.Copy(set, mailbox).Wait(); err != nil {
		return err
	}
	return s.Delete(uids...)
}

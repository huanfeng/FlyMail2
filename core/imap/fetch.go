package imap

import (
	"bytes"
	"fmt"
	"strings"

	imapv2 "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"flymail-core/parser"
	"flymail-core/types"
)

// FetchOptions controls what to fetch.
type FetchOptions struct {
	// FetchBody requests the full RFC 5322 body for parsing text/html/attachments.
	// When false, only envelope metadata is returned (faster).
	//
	// 两种取法拿到的信封字段是同一套：整封抓取由 parser 从完整邮件头里填，
	// 元数据抓取由 parser 从 BODY.PEEK[HEADER.FIELDS (…)] 区段里填。
	// 不存在「没取正文所以某些字段缺」这回事。
	FetchBody bool
}

// FetchByUIDs fetches messages by specific UIDs from the currently selected folder.
func (s *Session) FetchByUIDs(uids []imapv2.UID, opts FetchOptions) ([]*types.ParsedEmail, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}
	if len(uids) == 0 {
		return nil, nil
	}

	var uidSet imapv2.UIDSet
	uidSet.AddNum(uids...)

	return s.doFetch(uidSet, opts)
}

// FetchByUIDRange fetches messages in a UID range [from, to].
// If to is 0, fetches from `from` to the end (*).
func (s *Session) FetchByUIDRange(from, to imapv2.UID, opts FetchOptions) ([]*types.ParsedEmail, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}

	var uidSet imapv2.UIDSet
	uidSet.AddRange(from, to)

	return s.doFetch(uidSet, opts)
}

// FetchBySeqRange fetches messages by sequence-number range [from, to].
// Useful when the server does not report UIDNEXT (e.g. NetEase 163), so the
// "last N messages" window must be expressed by sequence number instead of UID.
// The FETCH response still carries each message's real UID (UID is requested),
// so de-duplication anchors are preserved.
func (s *Session) FetchBySeqRange(from, to uint32, opts FetchOptions) ([]*types.ParsedEmail, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}

	var seqSet imapv2.SeqSet
	seqSet.AddRange(from, to)

	return s.doFetch(seqSet, opts)
}

// UIDSearch 在当前选中的文件夹执行 UID SEARCH，返回命中的 UID（服务器侧全文/头部检索）。
// 由调用方构造 criteria（go-imap v2 的 SearchCriteria：Text/Header/Flag/SentSince…）。
func (s *Session) UIDSearch(criteria *imapv2.SearchCriteria) ([]imapv2.UID, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}
	res, err := s.Client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	return res.AllUIDs(), nil
}

// SearchUnseenSince searches for unseen messages with UID >= startUID.
func (s *Session) SearchUnseenSince(startUID imapv2.UID) ([]imapv2.UID, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}

	uidSet := imapv2.UIDSet{}
	uidSet.AddRange(startUID, 0)

	criteria := &imapv2.SearchCriteria{
		UID:     []imapv2.UIDSet{uidSet},
		NotFlag: []imapv2.Flag{imapv2.FlagSeen},
	}

	res, err := s.Client.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return res.AllUIDs(), nil
}

// FetchRawMessage 取回单个 UID 的整封原始 RFC 5322 字节（BODY[]），供 M7 附件下载使用。
// 调用方拿到原始字节后可交给 parser.ExtractAttachments 解析出附件内容。
func (s *Session) FetchRawMessage(uid imapv2.UID) ([]byte, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}

	var uidSet imapv2.UIDSet
	uidSet.AddNum(uid)

	section := newFullBodySection()
	fetchOpts := &imapv2.FetchOptions{
		UID:         true,
		BodySection: []*imapv2.FetchItemBodySection{section},
	}

	fetchCmd := s.Client.Fetch(uidSet, fetchOpts)

	var raw []byte
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}
		buf, err := msg.Collect()
		if err != nil {
			continue
		}
		if body := buf.FindBodySection(section); body != nil {
			raw = body
		}
	}

	if err := fetchCmd.Close(); err != nil {
		return nil, fmt.Errorf("fetch close: %w", err)
	}
	if raw == nil {
		return nil, fmt.Errorf("message body not found for uid %d", uid)
	}
	return raw, nil
}

// newFullBodySection 构造「整封原文」的抓取区段（空 Specifier = BODY[]）。
//
// ⚠ Peek 不是可选项，这是这个函数存在的**全部理由**。
//
// `BODY[]` 会让服务端给邮件置上 \Seen，而走这条路径的是附件下载、原文查看、
// 以及**后台正文预取**——也就是说，少一个 PEEK，光是同步就会把用户邮箱里
// 抓过的未读邮件全部标成已读，并且是在服务端：手机、网页版、其它客户端
// 一起跟着变。未读状态一旦丢了就找不回来（没有"本来哪些是未读"的记录）。
//
// 标记已读必须是显式动作（回写队列里的 wbOpRead），绝不能是抓取的副作用。
//
// 收敛成一个构造函数而不是在两个调用点各写一次：这个缺陷的成因正是
// 「取头的那个区段记得加 Peek，整封抓取那两处各自漏了」。
func newFullBodySection() *imapv2.FetchItemBodySection {
	return &imapv2.FetchItemBodySection{Peek: true}
}

// envelopeHeaderSection 是元数据抓取取的头字段区段——**取代 ENVELOPE**。
//
// ── 为什么不用 ENVELOPE ─────────────────────────────────────────────────────
//
// ENVELOPE 是服务端替我们把邮件头解析成十个字段。省一点带宽，代价是把解析的
// 正确性交给了对方，而对方可能算错：
//
//	QQ   某些邮件只给九个字段，go-imap 严格按 RFC 解析直接报
//	     `expected SP, got ")"`，还**连带拆掉整条连接**——一封坏邮件让整个
//	     账户的同步永远跑不完（2026-09-16 线上事故）。MailKit 2018 年就为
//	     QQ 的同一类缺陷加过绕行，八年了 QQ 没修。
//	GreenMail  ENVELOPE 里不带 In-Reply-To（实测），会话归并本来就得靠头。
//
// 所以统一从头里取、自己解析：少一个出错来源，整类「某家服务商的信封不标准」
// 的兼容问题一起消失，而且与整封抓取走的是同一套解析代码。
//
// ⚠ 这里**不要**回头再加 Envelope: true。守卫见 envelope_free_test.go：
// 那条用例的假服务端一旦被问到 ENVELOPE 就返回 QQ 那种畸形值。
//
// ── ⚠ 为什么取整个头，而不是只列需要的几个字段 ─────────────────────────────
//
// `HEADER.FIELDS (…)` 只要几百字节，本该是首选。但 go-imap 把字段名一律写成
// **带引号的字符串**：
//
//	BODY.PEEK[HEADER.FIELDS ("Date" "Subject" "From" …)]
//
// RFC 3501 的 header-fld-name 是 astring，加引号完全合法，可是实测有服务器
// 只认不加引号的原子写法，对加引号的一律返回空内容：
//
//	            不加引号   加引号   整个 HEADER
//	GreenMail      157       0        362
//	QQ             326       2       1926
//	Gmail / 163    正常     正常      正常
//
// 偏偏 QQ 就是要修的那一家。而引号是 go-imap 编码器加的，调不掉。
// 「取了字段表却拿回空内容」是最坏的失败方式——不报错，只是所有邮件都没有
// 主题和发件人。既有的线程头区段其实一直踩着这个坑：docs/flymail/m10-threads.md
// 记的「GreenMail 对 HEADER.FIELDS 返回空」就是它，只是当时还有 ENVELOPE 兜底，
// 没暴露成事故。
//
// 所以取整个头。代价是每封多几 KB（实测 GreenMail 0.4KB、QQ 1.9KB、
// Gmail 5.5KB、163 最长 17KB），只在首次全量同步时明显；换来的是不依赖任何
// 服务端的字段表实现。PEEK 避免把邮件标成已读。
var envelopeHeaderSection = &imapv2.FetchItemBodySection{
	Specifier: imapv2.PartSpecifierHeader,
	Peek:      true,
}

func (s *Session) doFetch(numSet imapv2.NumSet, opts FetchOptions) ([]*types.ParsedEmail, error) {
	var bodySection *imapv2.FetchItemBodySection
	if opts.FetchBody {
		bodySection = newFullBodySection()
	}

	// ⚠ 没有 Envelope: true，是有意的——见 envelopeHeaderSection 的说明。
	fetchOpts := &imapv2.FetchOptions{
		UID:          true,
		InternalDate: true,
		Flags:        true,
		RFC822Size:   true,
	}
	if bodySection != nil {
		// 整封抓取时头就在 BODY[] 里，由 parser 一并解析
		fetchOpts.BodySection = []*imapv2.FetchItemBodySection{bodySection}
	} else {
		fetchOpts.BodySection = []*imapv2.FetchItemBodySection{envelopeHeaderSection}
	}

	fetchCmd := s.Client.Fetch(numSet, fetchOpts)

	var emails []*types.ParsedEmail
	for {
		msg := fetchCmd.Next()
		if msg == nil {
			break
		}

		buf, err := msg.Collect()
		if err != nil {
			continue
		}

		email := convertMessage(buf, bodySection)
		if email != nil {
			emails = append(emails, email)
		}
	}

	if err := fetchCmd.Close(); err != nil {
		return emails, fmt.Errorf("fetch close: %w", err)
	}
	return emails, nil
}

func convertMessage(buf *imapclient.FetchMessageBuffer, bodySection *imapv2.FetchItemBodySection) *types.ParsedEmail {
	if buf == nil {
		return nil
	}

	email := &types.ParsedEmail{
		UID:    uint32(buf.UID),
		SeqNum: buf.SeqNum,
		Date:   buf.InternalDate,
		Size:   buf.RFC822Size,
	}

	// Flags
	for _, f := range buf.Flags {
		email.Flags = append(email.Flags, string(f))
		switch f {
		case imapv2.FlagSeen:
			email.IsRead = true
		case imapv2.FlagFlagged:
			email.IsStarred = true
		}
	}

	// 信封字段（主题、收发件人、Message-ID、线程头）一律由 parser 从邮件头解析，
	// 不走服务端的 ENVELOPE——见 envelopeHeaderSection 的说明。
	if bodySection != nil {
		if body := buf.FindBodySection(bodySection); body != nil {
			parser.ParseBody(bytes.NewReader(body), email, true)
		}
	} else if hdr := buf.FindBodySection(envelopeHeaderSection); len(hdr) > 0 {
		_ = parser.ParseHeaders(bytes.NewReader(hdr), email)
	}

	return email
}

// ConvertIMAPAddresses converts go-imap/v2 addresses to core types.Address.
func ConvertIMAPAddresses(addrs []imapv2.Address) []types.Address {
	if len(addrs) == 0 {
		return nil
	}
	result := make([]types.Address, 0, len(addrs))
	for _, a := range addrs {
		addr := fmt.Sprintf("%s@%s", a.Mailbox, a.Host)
		name := parser.DecodeMIMEHeader(strings.TrimSpace(a.Name))
		result = append(result, types.Address{Name: name, Email: addr})
	}
	return result
}

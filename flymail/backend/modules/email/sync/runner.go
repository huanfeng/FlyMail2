package sync

import (
	"context"
	"errors"
	"time"

	"flymail-core/types"
)

const (
	// idleCloseInterval 非 IDLE 档账户一轮任务处理完后的空闲关闭等待。
	idleCloseInterval = 60 * time.Second
	// 连接失败后的后台重连退避区间（前台任务不受此约束，见 exec）。
	dialBackoffBase = 1 * time.Second
	dialBackoffMax  = 60 * time.Second
)

// errBreakerOpen 表示账户熔断打开、后台任务此刻被拒（前台任务不会收到此错误）。
var errBreakerOpen = errors.New("account circuit breaker open")

// errDialBackoff 表示上次连接失败后仍在退避窗口内，后台任务本轮跳过。
var errDialBackoff = errors.New("dial backoff in effect")

// task 是投递给 runner 的一个工作单元；run 在 runner 独占的连接上串行执行。
// reply 非 nil 时（前台任务）用于把执行结果回传给等待的调用方；后台任务通常为 nil。
type task struct {
	run   func(Session) error
	reply chan error
}

// runner 是单账户的连接持有者（Actor）：独占一条 IMAP 连接，串行执行前台/后台任务。
// 本文件只含骨架（任务队列 + 懒建连 + 空闲关闭 + 退避 + 熔断门控）；
// IDLE↔轮询循环在 Task 4 接入。
type runner struct {
	accountID uint
	cfg       func() (types.IMAPConfig, error)
	dial      func(types.IMAPConfig) (Session, error)
	breaker   *breaker

	fg chan task // 前台队列（交互：详情/附件/手动触发，优先）
	bg chan task // 后台队列（回写/IDLE 唤醒同步）

	// 可注入以便测试（默认见 newRunner）。
	idleClose time.Duration
	now       func() time.Time

	// 仅 runner goroutine 访问的重连退避状态。
	dialBackoff time.Duration
	nextDialAt  time.Time

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// newRunner 构建 runner（尚未启动，调用 start）。
func newRunner(accountID uint, cfg func() (types.IMAPConfig, error), dial func(types.IMAPConfig) (Session, error)) *runner {
	return &runner{
		accountID: accountID,
		cfg:       cfg,
		dial:      dial,
		breaker:   newBreaker(),
		fg:        make(chan task),
		bg:        make(chan task, 64),
		idleClose: idleCloseInterval,
		now:       time.Now,
	}
}

// start 在 parent ctx 下启动 runner goroutine。
func (r *runner) start(parent context.Context) {
	r.ctx, r.cancel = context.WithCancel(parent)
	r.done = make(chan struct{})
	go r.loop()
}

// stop 取消并等待 runner 退出。
func (r *runner) stop() {
	if r.cancel != nil {
		r.cancel()
	}
	if r.done != nil {
		<-r.done
	}
}

// submitForeground 投递前台任务并等待结果；ctx 超时/取消则返回其错误，
// 但任务仍会在 runner 上执行完（reply 有缓冲，孤儿结果被安全丢弃）。
func (r *runner) submitForeground(ctx context.Context, run func(Session) error) error {
	reply := make(chan error, 1)
	select {
	case r.fg <- task{run: run, reply: reply}:
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
	select {
	case err := <-reply:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}

// submitBackground 非阻塞投递后台任务；队列满则丢弃（回写已持久化、丢内存任务不丢数据）。
// 返回是否入队成功。
func (r *runner) submitBackground(run func(Session) error) bool {
	select {
	case r.bg <- task{run: run}:
		return true
	case <-r.ctx.Done():
		return false
	default:
		return false
	}
}

// loop 是 runner 主循环：无连接时阻塞等任务；有连接时前台优先、其次后台、再空闲关闭。
func (r *runner) loop() {
	defer close(r.done)
	var sess Session
	defer func() {
		if sess != nil {
			_ = sess.Close()
		}
	}()

	idleTimer := time.NewTimer(r.idleClose)
	stopTimer(idleTimer)
	defer idleTimer.Stop()

	for {
		if sess == nil {
			// 无连接：阻塞等任务，不计空闲关闭（不空转持有连接）。
			select {
			case <-r.ctx.Done():
				return
			case t := <-r.fg:
				r.exec(&sess, t, true)
			case t := <-r.bg:
				r.exec(&sess, t, false)
			}
			continue
		}
		// 有连接：前台优先 → 后台 → 空闲关闭 → 退出。
		idleTimer.Reset(r.idleClose)
		select {
		case <-r.ctx.Done():
			stopTimer(idleTimer)
			return
		case t := <-r.fg:
			stopTimer(idleTimer)
			r.exec(&sess, t, true)
		case t := <-r.bg:
			stopTimer(idleTimer)
			r.execPreferFG(&sess, t)
		case <-idleTimer.C:
			_ = sess.Close()
			sess = nil
		}
	}
}

// execPreferFG 在执行一个已选中的后台任务前，先排干当前就绪的前台任务（严格前台优先）。
func (r *runner) execPreferFG(sess *Session, bg task) {
	for {
		select {
		case ft := <-r.fg:
			r.exec(sess, ft, true)
		default:
			r.exec(sess, bg, false)
			return
		}
	}
}

// exec 在 runner 连接上执行一个任务：懒建连、执行、按结果维护连接/熔断/退避，并回传结果。
func (r *runner) exec(sess *Session, t task, foreground bool) {
	// 后台任务受熔断与退避门控；前台任务穿透（用户在等，给一次立即恢复的机会）。
	if !foreground {
		if !r.breaker.AllowBackground() {
			reply(t, errBreakerOpen)
			return
		}
		if !r.nextDialAt.IsZero() && *sess == nil && r.now().Before(r.nextDialAt) {
			reply(t, errDialBackoff)
			return
		}
	}

	if *sess == nil {
		s, err := r.dialConn()
		if err != nil {
			r.onDialFailure()
			reply(t, err)
			return
		}
		r.onDialSuccess()
		*sess = s
	}

	if err := t.run(*sess); err != nil {
		// 任务失败：连接状态不确定，关闭以便下轮重建（延续旧代码「出错即重连」语义）。
		_ = (*sess).Close()
		*sess = nil
		if !foreground {
			r.breaker.RecordFailure()
		}
		reply(t, err)
		return
	}
	r.breaker.RecordSuccess()
	reply(t, nil)
}

// dialConn 取配置并建连。
func (r *runner) dialConn() (Session, error) {
	cfg, err := r.cfg()
	if err != nil {
		return nil, err
	}
	return r.dial(cfg)
}

// onDialFailure 记熔断失败并推进重连退避（1s→60s 封顶）。
func (r *runner) onDialFailure() {
	r.breaker.RecordFailure()
	if r.dialBackoff == 0 {
		r.dialBackoff = dialBackoffBase
	} else {
		r.dialBackoff *= 2
		if r.dialBackoff > dialBackoffMax {
			r.dialBackoff = dialBackoffMax
		}
	}
	r.nextDialAt = r.now().Add(r.dialBackoff)
}

// onDialSuccess 复位重连退避。
func (r *runner) onDialSuccess() {
	r.dialBackoff = 0
	r.nextDialAt = time.Time{}
}

// reply 把结果非阻塞回传（reply 有缓冲，无接收方时安全丢弃）。
func reply(t task, err error) {
	if t.reply != nil {
		t.reply <- err
	}
}

package sync

import (
	"context"
	"errors"
	"time"

	coreimap "flymail-core/imap"
	"flymail-core/types"
)

// runnerHost 提供 runner 所需的同步动作（Task 5 由 Manager 实现，内部据 accountID
// 路由到 folders/messages 服务）。host 为 nil 时 runner 退化为纯任务服务器（Task 3）。
type runnerHost interface {
	// FullSync 执行一轮全文件夹增量同步；yield 在每个文件夹边界被调用，供 runner 让位前台任务。
	FullSync(accountID uint, sess Session, yield func()) error
	// InboxSync 只增量同步收件箱（IDLE 唤醒用）。
	InboxSync(accountID uint, sess Session) error
	// SelectInbox 为进入 IDLE 选中收件箱；ok=false 表示无收件箱可 IDLE。
	SelectInbox(accountID uint, sess Session) (ok bool, err error)
	// PollInterval 返回当前轮询间隔。
	PollInterval() time.Duration
	// IDLEAllowed 报告该账户是否获得常驻 IDLE 名额（超额账户降为轮询模式）。
	IDLEAllowed(accountID uint) bool
}

const (
	// idleCloseInterval 非 IDLE 档账户一轮任务处理完后的空闲关闭等待。
	idleCloseInterval = 60 * time.Second
	// 连接失败后的后台重连退避区间（前台任务不受此约束，见 exec）。
	dialBackoffBase = 1 * time.Second
	dialBackoffMax  = 60 * time.Second
	// syncSlotRetry 抢不到全局同步名额时的短重试延迟。
	syncSlotRetry = 5 * time.Second
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
	host      runnerHost // nil = 纯任务模式（无轮询/IDLE）

	fg     chan task     // 前台队列（交互：详情/附件/手动触发，优先）
	bg     chan task     // 后台队列（回写/IDLE 唤醒同步）
	idleCh chan struct{} // IDLE 新邮件事件唤醒（连接的 IDLE 回调写入）

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
		idleCh:    make(chan struct{}, 1),
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

// loop 是 runner 主循环，统一处理任务服务与（host 模式下的）IDLE↔轮询。
// 唤醒源：前台队列 > 后台队列 > IDLE 新邮件 > 轮询 tick > 29min 刷新 > 空闲关闭 > ctx。
// host==nil 时轮询/IDLE 相关通道恒为 nil，退化为纯任务服务器（见 Task 3）。
func (r *runner) loop() {
	defer close(r.done)
	var sess Session
	defer func() {
		if sess != nil {
			_ = sess.Close()
		}
	}()

	hasHost := r.host != nil

	idleTimer := time.NewTimer(r.idleClose)
	stopTimer(idleTimer)
	defer idleTimer.Stop()

	refreshTimer := time.NewTimer(idleRefreshInterval)
	stopTimer(refreshTimer)
	defer refreshTimer.Stop()

	// 轮询计时器：host 模式下首轮加错峰相位，之后每 pollInterval 触发一次全量同步。
	pollTimer := time.NewTimer(time.Hour)
	stopTimer(pollTimer)
	defer pollTimer.Stop()
	if hasHost {
		pollTimer.Reset(r.pollPhase())
	}

	// 当前 IDLE 句柄（仅 host 模式、连接可 IDLE 且空闲时非 nil）。
	var idleHandle *coreimap.IdleHandle

	// stopIdle 结束当前 IDLE。返回 false 表示 Stop 失败（连接脏，调用方应关闭重建）。
	stopIdle := func(reason string) bool {
		stopTimer(refreshTimer)
		if idleHandle == nil {
			return true
		}
		err := idleHandle.Stop(reason)
		idleHandle = nil
		return err == nil
	}

	for {
		if r.ctx.Err() != nil {
			stopIdle("shutdown")
			return
		}
		stopTimer(idleTimer)

		// 有连接且可 IDLE、有 IDLE 名额且当前空闲：进入 IDLE（选中收件箱→StartIDLE→装刷新计时）。
		if hasHost && sess != nil && idleHandle == nil && sess.CanIDLE() && r.host.IDLEAllowed(r.accountID) {
			if ok, _ := r.host.SelectInbox(r.accountID, sess); ok {
				if h, err := sess.StartIDLE(); err == nil {
					idleHandle = h
					refreshTimer.Reset(idleRefreshInterval)
				}
			}
		}

		// 非 IDLE 连接才计空闲关闭（IDLE 连接靠 IDLE 保活）。
		var idleCloseC <-chan time.Time
		if sess != nil && idleHandle == nil {
			idleTimer.Reset(r.idleClose)
			idleCloseC = idleTimer.C
		}
		var idleDone <-chan error
		var refreshC <-chan time.Time
		if idleHandle != nil {
			idleDone = idleHandle.Done()
			refreshC = refreshTimer.C
		}
		var pollC <-chan time.Time
		if hasHost {
			pollC = pollTimer.C
		}

		select {
		case <-r.ctx.Done():
			stopIdle("shutdown")
			return
		case t := <-r.fg:
			if !stopIdle("task") && sess != nil {
				_ = sess.Close()
				sess = nil
			}
			r.exec(&sess, t, true)
		case t := <-r.bg:
			if !stopIdle("task") && sess != nil {
				_ = sess.Close()
				sess = nil
			}
			r.execPreferFG(&sess, t)
		case <-r.idleCh:
			if !stopIdle("new-mail") && sess != nil {
				_ = sess.Close()
				sess = nil
			}
			if sess != nil {
				if err := r.host.InboxSync(r.accountID, sess); err != nil {
					_ = sess.Close()
					sess = nil
					r.breaker.RecordFailure()
				} else {
					r.breaker.RecordSuccess()
				}
			}
		case <-pollC:
			if !stopIdle("poll") && sess != nil {
				_ = sess.Close()
				sess = nil
			}
			pollTimer.Reset(r.doPoll(&sess))
		case <-refreshC:
			// 29min 刷新：结束 IDLE，下一轮循环重新进入 IDLE。
			_ = stopIdle("refresh")
		case <-idleDone:
			// IDLE 连接被服务器关闭：丢弃连接，下一轮由任务/轮询重建。
			stopIdle("done")
			if sess != nil {
				_ = sess.Close()
				sess = nil
			}
			r.breaker.RecordFailure()
		case <-idleCloseC:
			_ = sess.Close()
			sess = nil
		}
	}
}

// doPoll 执行一轮全量同步（host 模式），返回下次轮询前应等待的时长：
// 熔断/退避门控 → 懒建连 → FullSync（文件夹边界让位前台）。抢不到全局同步名额则短延迟重试。
func (r *runner) doPoll(sess *Session) time.Duration {
	interval := r.host.PollInterval()
	if !r.breaker.AllowBackground() {
		return interval
	}
	if *sess == nil {
		if !r.nextDialAt.IsZero() && r.now().Before(r.nextDialAt) {
			return interval
		}
		s, err := r.dialConn()
		if err != nil {
			r.onDialFailure()
			return interval
		}
		r.onDialSuccess()
		*sess = s
	}
	err := r.host.FullSync(r.accountID, *sess, func() { r.drainForeground(sess) })
	if err != nil {
		if errors.Is(err, errSyncSlotBusy) {
			// 没抢到全局同步名额：连接保留，稍后重试（不算失败）。
			return syncSlotRetry
		}
		_ = (*sess).Close()
		*sess = nil
		r.onDialFailure()
		return interval
	}
	r.breaker.RecordSuccess()
	return interval
}

// drainForeground 非阻塞排干当前就绪的前台任务（全量同步在文件夹边界调用，实现协作让位）。
func (r *runner) drainForeground(sess *Session) {
	for {
		select {
		case ft := <-r.fg:
			r.exec(sess, ft, true)
		default:
			return
		}
	}
}

// pollPhase 返回首轮同步的错峰相位：hash(accountID) % pollInterval，避免重启后众账户同时开跑。
func (r *runner) pollPhase() time.Duration {
	pi := r.host.PollInterval()
	if pi <= 0 {
		return 0
	}
	h := uint64(r.accountID) * 2654435761
	return time.Duration(h % uint64(pi))
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

// dialConn 取配置并建连；host 模式下顺带装 IDLE 新邮件回调（唤醒主循环去同步收件箱）。
func (r *runner) dialConn() (Session, error) {
	cfg, err := r.cfg()
	if err != nil {
		return nil, err
	}
	s, err := r.dial(cfg)
	if err != nil {
		return nil, err
	}
	if r.host != nil {
		s.SetIDLEHandler(func(coreimap.IDLEEvent) {
			select {
			case r.idleCh <- struct{}{}:
			default:
			}
		})
	}
	return s, nil
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

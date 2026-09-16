package imap

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// StartIDLE enters IDLE mode on the currently selected mailbox.
// It blocks until IDLE is interrupted (via StopIDLE on the returned IdleHandle)
// or the server drops the connection.
//
// Server updates during IDLE are delivered through the IDLEEvent handler
// registered via SetIDLEHandler.
type IdleHandle struct {
	cmd  idleCommand
	done chan error
	// restore 把连接的空闲读上限从 IDLE 的长值改回命令期的短值。
	restore func()
}

// idleCommand abstracts the idle command for testability.
type idleCommand interface {
	Close() error
	Wait() error
}

// StartIDLE begins an IDLE session. Returns an IdleHandle that must be
// stopped via Stop() before issuing other commands on this Session.
func (s *Session) StartIDLE() (*IdleHandle, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("not connected")
	}

	// ⚠ 必须在发出 IDLE **之前**放宽读上限。
	//
	// 连接层给「等下一条响应的第一个字节」加了 5 分钟上限（见 timeout.go），
	// 而 IDLE 的本意就是长时间不说话——不放宽的话，每条 IDLE 连接都会在 5 分钟
	// 整点被自己的超时打断，表现为反复重连，比原来的缺陷更显眼。
	// 放在 Idle() 之后则留下一个窗口：那期间到期就会把刚建立的 IDLE 打掉。
	restore := func() {}
	if s.conn != nil {
		s.conn.setIdleTimeout(idleHoldTimeout)
		restore = func() { s.conn.setIdleTimeout(connIdleTimeout) }
	}

	cmd, err := s.Client.Idle()
	if err != nil {
		restore()
		return nil, fmt.Errorf("start IDLE failed: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	return &IdleHandle{cmd: cmd, done: done, restore: restore}, nil
}

// Stop terminates the IDLE session. Must be called before issuing other
// IMAP commands on the same Session.
func (h *IdleHandle) Stop(reason string) error {
	if h.cmd == nil {
		return nil
	}
	// 退出 IDLE 就要把读上限收回来：接下来发的是普通命令，
	// 再按 IDLE 的 35 分钟等下去，静默掐断又变成不可发现的了。
	if h.restore != nil {
		h.restore()
	}

	if err := h.cmd.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		// Log but don't fail — the important thing is that IDLE ends
		_ = err
	}

	// Wait for the IDLE goroutine to finish
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		return fmt.Errorf("IDLE stop timeout (%s)", reason)
	}

	return nil
}

// Done returns a channel that receives an error when IDLE terminates
// unexpectedly (e.g. server dropped the connection).
func (h *IdleHandle) Done() <-chan error {
	return h.done
}

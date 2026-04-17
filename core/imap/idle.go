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

	cmd, err := s.Client.Idle()
	if err != nil {
		return nil, fmt.Errorf("start IDLE failed: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	return &IdleHandle{cmd: cmd, done: done}, nil
}

// Stop terminates the IDLE session. Must be called before issuing other
// IMAP commands on the same Session.
func (h *IdleHandle) Stop(reason string) error {
	if h.cmd == nil {
		return nil
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

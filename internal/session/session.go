package session

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"
)

type Session struct {
	ID        string
	State     State
	StartedAt time.Time

	ContainerID string

	Stdin  io.WriteCloser
	Output io.Reader

	Stdout strings.Builder
	Stderr strings.Builder

	ctx    context.Context
	cancel context.CancelFunc

	done     chan struct{}
	doneOnce sync.Once
	mu       sync.Mutex
	activeWS int
	timer    *time.Timer
}

func New(
	id string,
	containerID string,
	stdin io.WriteCloser,
	output io.Reader,
	ctx context.Context,
	cancel context.CancelFunc,
) *Session {
	return &Session{
		ID:          id,
		ContainerID: containerID,
		State:       StateRunning,
		StartedAt:   time.Now(),
		Stdin:       stdin,
		Output:      output,
		ctx:         ctx,
		cancel:      cancel,
		done:        make(chan struct{}),
	}
}

// --------------------
// Input handling
// --------------------

func (s *Session) WriteInput(data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State != StateRunning && s.State != StateWaitingInput {
		return fmt.Errorf("session not accepting input (state=%s)", s.State)
	}

	_, err := s.Stdin.Write([]byte(data))
	return err
}

func (s *Session) CloseInput() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Mark intent: no more input
	if s.State == StateRunning {
		s.State = StateWaitingInput
	}

	_ = s.Stdin.Close()
}

// --------------------
// Lifecycle handling
// --------------------

func (s *Session) MarkFinished() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StateFinished || s.State == StateClosed {
		return
	}

	s.State = StateFinished
	s.signalDone()
}

func (s *Session) MarkTerminated() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StateFinished || s.State == StateClosed {
		return
	}

	s.State = StateTerminated
	s.signalDone()
}

func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StateClosed {
		return
	}

	s.State = StateClosed
	s.signalDone()
}

// --------------------
// Synchronization
// --------------------

func (s *Session) signalDone() {
	s.doneOnce.Do(func() {
		close(s.done)
	})
}

func (s *Session) Done() <-chan struct{} {
	return s.done
}

// Wait blocks until the session finishes or terminates
func (s *Session) Wait() {
	<-s.done
}

// --------------------
// WebSocket handling
// --------------------

func (s *Session) AttachWS() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeWS++
	if s.timer != nil {
		log.Printf("Session %s: WebSocket attached, stopping timer.", s.ID)
		s.timer.Stop()
	}
}

func (s *Session) DetachWS() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeWS--
	if s.activeWS == 0 {
		log.Printf("Session %s: Last WebSocket detached, starting 1-minute termination timer.", s.ID)
		s.timer = time.AfterFunc(1*time.Minute, func() {
			log.Printf("Session %s: Termination timer fired.", s.ID)
			s.Stop()
		})
	}
	return s.activeWS == 0
}

func (s *Session) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StateFinished || s.State == StateTerminated {
		return
	}

	log.Printf("Session %s: Stopping session.", s.ID)
	s.State = StateTerminated
	s.cancel() // 🔥 THIS IS THE KEY
	s.signalDone()
}

func (s *Session) ActiveWSCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeWS
}

func (s *Session) Context() context.Context {
	return s.ctx
}

func (s *Session) Cancel() {
	s.cancel()
}

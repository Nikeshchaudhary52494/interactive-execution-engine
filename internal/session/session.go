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

const (
	MaxOutputBytes = 1 << 20 // 1 MB
)

type Session struct {
	ID        string
	State     State
	StartedAt time.Time

	Language string
	Code     string

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

	lastActivity time.Time
	idleTimeout  time.Duration
	idleTimer    *time.Timer
}

func New(
	id string,
	containerID string,
	stdin io.WriteCloser,
	output io.Reader,
	ctx context.Context,
	cancel context.CancelFunc,
) *Session {
	s := &Session{
		ID:           id,
		ContainerID:  containerID,
		State:        StateRunning,
		StartedAt:    time.Now(),
		Stdin:        stdin,
		Output:       output,
		ctx:          ctx,
		cancel:       cancel,
		done:         make(chan struct{}),
		idleTimeout:  30 * time.Second,
		lastActivity: time.Now(),
	}
	s.startIdleWatcher()
	return s
}

//
// ---------------- Output handling (SAFE) ----------------
//

func (s *Session) AppendOutput(data []byte) {
	s.mu.Lock()
	s.Stdout.Write(data)

	overflow := s.Stdout.Len() > MaxOutputBytes
	s.lastActivity = time.Now()
	s.idleTimer.Reset(s.idleTimeout)
	s.mu.Unlock()

	if overflow {
		log.Printf("Session %s: output limit exceeded", s.ID)
		s.Stop()
	}
}

func (s *Session) GetStdout() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Stdout.String()
}

func (s *Session) GetStderr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Stderr.String()
}

func (s *Session) StdoutWriter() io.Writer {
	return &safeWriter{s: s, isStderr: false}
}

func (s *Session) StderrWriter() io.Writer {
	return &safeWriter{s: s, isStderr: true}
}

type safeWriter struct {
	s        *Session
	isStderr bool
}

func (w *safeWriter) Write(p []byte) (n int, err error) {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()

	if w.isStderr {
		n, err = w.s.Stderr.Write(p)
	} else {
		n, err = w.s.Stdout.Write(p)
		w.s.lastActivity = time.Now()
		if w.s.idleTimer != nil {
			w.s.idleTimer.Reset(w.s.idleTimeout)
		}
	}

	overflow := false
	if !w.isStderr && w.s.Stdout.Len() > MaxOutputBytes {
		overflow = true
	}
	if w.isStderr && w.s.Stderr.Len() > MaxOutputBytes {
		overflow = true
	}

	if overflow {
		go func() {
			log.Printf("Session %s: output limit exceeded", w.s.ID)
			w.s.Stop()
		}()
	}
	return
}

//
// ---------------- Input handling ----------------
//

func (s *Session) WriteInput(data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State != StateRunning && s.State != StateWaitingInput {
		return fmt.Errorf("session not accepting input (state=%s)", s.State)
	}

	s.lastActivity = time.Now()
	if s.idleTimer != nil {
		s.idleTimer.Reset(s.idleTimeout)
	}

	_, err := s.Stdin.Write([]byte(data))
	return err
}

//
// ---------------- Lifecycle handling ----------------
//

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

func (s *Session) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StateFinished || s.State == StateTerminated {
		return
	}

	log.Printf("Session %s: Stopping session.", s.ID)
	s.State = StateTerminated
	if s.cancel != nil {
		s.cancel()
	}
	s.signalDone()
}

func (s *Session) Context() context.Context {
	return s.ctx
}

func (s *Session) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

//
// ---------------- WS handling ----------------
//

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

func (s *Session) ActiveWSCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeWS
}

//
// ---------------- Idle timeout ----------------
//

func (s *Session) startIdleWatcher() {
	s.idleTimer = time.AfterFunc(s.idleTimeout, func() {
		log.Printf("Session %s idle timeout", s.ID)
		s.Stop()
	})
}

func (s *Session) StopIdleWatcher() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
}

//
// ---------------- Synchronization ----------------
//

func (s *Session) signalDone() {
	s.doneOnce.Do(func() {
		close(s.done)
	})
}

func (s *Session) Done() <-chan struct{} {
	return s.done
}

func NewPending(id, lang, code string) *Session {
	s := &Session{
		ID:           id,
		State:        StateWaiting,
		Language:  lang,
		Code:      code,
		StartedAt: time.Now(),
		done:      make(chan struct{}),
		idleTimeout: 30 * time.Second,
		lastActivity: time.Now(),
	}
	s.startIdleWatcher()
	return s
}

func (s *Session) MarkRunning() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = StateRunning
}

func (s *Session) SetRuntime(
	containerID string,
	stdin io.WriteCloser,
	output io.Reader,
	ctx context.Context,
	cancel context.CancelFunc,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ContainerID = containerID
	s.Stdin = stdin
	s.Output = output
	s.ctx = ctx
	s.cancel = cancel
}

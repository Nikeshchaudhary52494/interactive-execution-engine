package engine

import (
	"context"
	"log"

	"execution-engine/internal/executor"
	"execution-engine/internal/modules"
	"execution-engine/internal/session"
)

type engine struct {
	executor *executor.DockerExecutor
	sessions *session.Manager
}

func New(exec *executor.DockerExecutor) *engine {
	return &engine{
		executor: exec,
		sessions: session.NewManager(),
	}
}

func (e *engine) StartSession(
	ctx context.Context,
	req modules.ExecuteRequest,
) (*session.Session, error) {

	sess, err := e.executor.StartSession(ctx, req.Language, req.Code)
	if err != nil {
		return nil, err
	}

	e.sessions.Add(sess)
	log.Printf("Engine: Session %s added to the manager", sess.ID)

	// 🔥 AUTO-REMOVE when done
	go func() {
		<-sess.Done()
		log.Printf("Engine: Session %s is done, removing from manager", sess.ID)
		e.sessions.Remove(sess.ID)
	}()

	return sess, nil
}

func (e *engine) GetSession(id string) (*session.Session, bool) {
	return e.sessions.Get(id)
}

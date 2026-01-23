package executor

import (
	"context"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"

	"execution-engine/internal/session"
)

func (d *DockerExecutor) watchSession(
	s *session.Session,
	tempDir string,
) {
	defer os.RemoveAll(tempDir)

	// ---------------- stream stdout ----------------
	go func() {
		_, _ = stdcopy.StdCopy(s.StdoutWriter(), s.StderrWriter(), s.Output)
	}()

	waitCh, _ := d.cli.ContainerWait(
		context.Background(),
		s.ContainerID,
		container.WaitConditionNotRunning,
	)

	select {
	case <-waitCh:
		s.MarkFinished()

	case <-s.Context().Done(): // 🔥 session cancelled
		_ = d.cli.ContainerKill(
			context.Background(),
			s.ContainerID,
			"KILL",
		)
		s.MarkTerminated()
	}

	// ALWAYS remove container
	_ = d.cli.ContainerRemove(
		context.Background(),
		s.ContainerID,
		container.RemoveOptions{Force: true},
	)
}

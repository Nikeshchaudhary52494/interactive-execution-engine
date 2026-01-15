package executor

import (
	"context"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"

	"execution-engine/internal/language"
	"execution-engine/internal/session"
)

func (d *DockerExecutor) StartSession(
	ctx context.Context,
	lang string,
	code string,
) (*session.Session, error) {

	spec, err := language.Resolve(lang)
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "exec-*")
	if err != nil {
		return nil, err
	}

	codePath := filepath.Join(tempDir, spec.FileName)
	if err := os.WriteFile(codePath, []byte(code), 0644); err != nil {
		return nil, err
	}

	resp, err := d.cli.ContainerCreate(
		ctx,
		&container.Config{
			Image:        spec.Image,
			Cmd:          spec.RunCommand,
			WorkingDir:   "/workspace",
			OpenStdin:    true,
			AttachStdin:  true,
			StdinOnce:    false,
			AttachStdout: true,
			AttachStderr: true,
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: tempDir,
					Target: "/workspace",
				},
			},
		},
		nil, nil, "",
	)
	if err != nil {
		return nil, err
	}

	attach, err := d.cli.ContainerAttach(
		ctx,
		resp.ID,
		container.AttachOptions{
			Stream: true,
			Stdin:  true,
			Stdout: true,
			Stderr: true,
		},
	)
	if err != nil {
		return nil, err
	}

	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, err
	}

	sessCtx, cancel := context.WithCancel(context.Background())

	sess := session.New(
		session.NewID(),
		resp.ID,
		attach.Conn,
		attach.Reader,
		sessCtx,
		cancel,
	)

	go d.watchSession(sess, tempDir)

	return sess, nil
}

package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/uuid"

	"execution-engine/internal/engine"
	"execution-engine/internal/language"
)

const (
	workspaceDir = "/workspace"
	resultFile   = "result.json"
)

type DockerExecutor struct {
	cli *client.Client
}

func NewDockerExecutor() (*DockerExecutor, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &DockerExecutor{cli: cli}, nil
}

func (d *DockerExecutor) Run(
	ctx context.Context,
	lang string,
	code string,
) (*engine.ExecuteResult, error) {
	spec, err := language.Resolve(lang)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve language spec: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "exec-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	codeFilePath := filepath.Join(tempDir, spec.FileName)
	if err := os.WriteFile(codeFilePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write code to file: %w", err)
	}

	return d.runContainer(ctx, tempDir, spec)
}

func (d *DockerExecutor) runContainer(
	ctx context.Context,
	tempDir string,
	spec language.Spec,
) (*engine.ExecuteResult, error) {
	// ---- create container ----
	id := uuid.New().String()
	containerName := "exec-" + id
	resp, err := d.cli.ContainerCreate(
		ctx,
		&container.Config{
			Image:           spec.Image,
			Cmd:             spec.RunCommand,
			WorkingDir:      workspaceDir,
			Tty:             false,
			NetworkDisabled: true,
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: tempDir,
					Target: workspaceDir,
				},
			},
			Resources: container.Resources{
				Memory: 200 * 1024 * 1024, // 200MB
			},
		},
		nil,
		nil,
		containerName,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	defer func() {
		d.cli.ContainerRemove(
			context.Background(),
			resp.ID,
			container.RemoveOptions{Force: true},
		)
	}()

	startTime := time.Now()

	// ---- run container ----
	err = d.cli.ContainerStart(
		ctx,
		resp.ID,
		container.StartOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// ---- wait for completion ----
	statusCh, errCh := d.cli.ContainerWait(
		ctx,
		resp.ID,
		container.WaitConditionNotRunning,
	)

	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("error waiting for container: %w", err)
		}
	case status := <-statusCh:
		duration := time.Since(startTime)

		// ---- get logs ----
		logReader, err := d.cli.ContainerLogs(
			ctx,
			resp.ID,
			container.LogsOptions{
				ShowStdout: true,
				ShowStderr: true,
				Follow:     false,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get container logs: %w", err)
		}
		defer logReader.Close()

		var stdout, stderr strings.Builder
		if _, err := stdcopy.StdCopy(&stdout, &stderr, logReader); err != nil {
			return nil, fmt.Errorf("failed to copy logs: %w", err)
		}

		return &engine.ExecuteResult{
			ExitCode:   int(status.StatusCode),
			Stdout:     stdout.String(),
			Stderr:     stderr.String(),
			DurationMs: duration.Milliseconds(),
			TimedOut:   false,
		}, nil

	case <-ctx.Done():
		duration := time.Since(startTime)
		return &engine.ExecuteResult{
			DurationMs: duration.Milliseconds(),
			TimedOut:   true,
		}, nil
	}
	return nil, nil
}
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

	"execution-engine/internal/engine"
	"execution-engine/internal/language"
)

const (
	workspaceDir = "/workspace"
)

type DockerExecutor struct {
	cli *client.Client
}

func NewDockerExecutor() (*DockerExecutor, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
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
		return nil, fmt.Errorf("resolve language: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "exec-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	codePath := filepath.Join(tempDir, spec.FileName)
	if err := os.WriteFile(codePath, []byte(code), 0644); err != nil {
		return nil, fmt.Errorf("write code file: %w", err)
	}

	return d.runContainer(ctx, tempDir, spec)
}

func (d *DockerExecutor) runContainer(
	ctx context.Context,
	tempDir string,
	spec language.Spec,
) (*engine.ExecuteResult, error) {

	startTime := time.Now()

	// ---------- CREATE CONTAINER ----------
	createResp, err := d.cli.ContainerCreate(
		ctx,
		&container.Config{
			Image:           spec.Image,
			Cmd:             spec.RunCommand,
			WorkingDir:      workspaceDir,
			Tty:             false,
			OpenStdin:       false, // non-interactive for now
			AttachStdout:    true,
			AttachStderr:    true,
			NetworkDisabled: true,
		},
		&container.HostConfig{
			AutoRemove: false,
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
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("container create: %w", err)
	}

	containerID := createResp.ID

	// ---------- ALWAYS CLEANUP ----------
	defer func() {
		_ = d.cli.ContainerRemove(
			context.Background(),
			containerID,
			container.RemoveOptions{Force: true},
		)
	}()

	// ---------- ATTACH (BEFORE START) ----------
	attachResp, err := d.cli.ContainerAttach(
		ctx,
		containerID,
		container.AttachOptions{
			Stream: true,
			Stdout: true,
			Stderr: true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("container attach: %w", err)
	}
	defer attachResp.Close()

	var stdoutBuf, stderrBuf strings.Builder

	outputDone := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader)
		outputDone <- err
	}()

	// ---------- START ----------
	if err := d.cli.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("container start: %w", err)
	}

	// ---------- WAIT / TIMEOUT ----------
	waitCh, errCh := d.cli.ContainerWait(
		ctx,
		containerID,
		container.WaitConditionNotRunning,
	)

	var (
		exitCode int
		timedOut bool
	)

	select {
	case err := <-errCh:
		if err != nil {
			return nil, fmt.Errorf("container wait error: %w", err)
		}

	case status := <-waitCh:
		exitCode = int(status.StatusCode)

	case <-ctx.Done():
		timedOut = true

		// force kill
		_ = d.cli.ContainerKill(
			context.Background(),
			containerID,
			"KILL",
		)

		// wait until container actually stops
		<-waitCh
	}

	// ---------- DRAIN OUTPUT ----------
	<-outputDone

	duration := time.Since(startTime)

	return &engine.ExecuteResult{
		ExitCode:   exitCode,
		Stdout:     stdoutBuf.String(),
		Stderr:     stderrBuf.String(),
		DurationMs: duration.Milliseconds(),
		TimedOut:   timedOut,
	}, nil
}

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// ensureImage ensures the Docker image exists locally.
// If not present, it pulls it.
func ensureImage(
	ctx context.Context,
	cli *client.Client,
	imageName string,
) error {

	// 1️⃣ Check if image already exists
	_, _, err := cli.ImageInspectWithRaw(ctx, imageName)
	if err == nil {
		return nil
	}

	// 2️⃣ Pull image
	reader, err := cli.ImagePull(
		ctx,
		imageName,
		image.PullOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer reader.Close()

	// 3️⃣ Drain pull output (required)
	dec := json.NewDecoder(reader)
	for {
		var msg map[string]interface{}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("image pull decode error: %w", err)
		}
	}

	return nil
}

package executor

import (
	"context"
	"log"

	"execution-engine/internal/language"
)

// PreloadImages pulls all required images before server starts
func (d *DockerExecutor) PreloadImages(ctx context.Context) error {
	specs := language.AllSpecs()

	log.Println("🔄 Preloading Docker images...")

	for _, spec := range specs {
		log.Printf("➡️  checking image: %s (%s)", spec.Image, spec.Name)

		if err := ensureImage(ctx, d.cli, spec.Image); err != nil {
			return err
		}

		log.Printf("✅ ready: %s", spec.Image)
	}

	log.Println("🎉 All Docker images are ready")
	return nil
}

package main

import (
	"context"
	"log"
	"time"

	"execution-engine/internal/api"
	"execution-engine/internal/engine"
	"execution-engine/internal/executor"
)

func main() {
	// ---- bootstrap docker executor ----
	dockerExec, err := executor.NewDockerExecutor()
	if err != nil {
		panic(err)
	}

	// ---- preload docker images ----
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if err := dockerExec.PreloadImages(ctx); err != nil {
		log.Fatalf("❌ failed to preload images: %v", err)
	}

	// ---- engine ----
	eng := engine.New(dockerExec)

	// ---- router ----
	r := api.New(eng)

	log.Println("🚀 Server started on :8080")
	if err := r.Run(":8080"); err != nil {
		panic(err)
	}
}

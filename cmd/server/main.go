package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"execution-engine/internal/engine"
	"execution-engine/internal/executor"
)

func main() {
	// ---- bootstrap docker executor ----
	dockerExec, err := executor.NewDockerExecutor()
	if err != nil {
		log.Fatalf("failed to init docker executor: %v", err)
	}

	// ---- engine ----
	execEngine := engine.New(dockerExec)

	// ---- routes ----
	http.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req engine.ExecuteRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}

		if req.TimeLimitMs <= 0 {
			req.TimeLimitMs = 2000
		}

		ctx, cancel := context.WithTimeout(
			r.Context(),
			time.Duration(req.TimeLimitMs)*time.Millisecond,
		)
		defer cancel()

		result, err := execEngine.Execute(ctx, req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	log.Println("🚀 execution engine listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

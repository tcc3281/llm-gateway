package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"llm-gateway/internal/alert"
	"llm-gateway/internal/config"
	"llm-gateway/internal/db"
	"llm-gateway/internal/telemetry"
)

// Worker Plane Entrypoint — Asynchronous Alert Engine & Audit Notification Worker
func main() {
	log.Println("[Worker Plane] Starting LLM Gateway Alert & Audit Worker...")

	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Config load error: %v", err)
	}

	// 2. PostgreSQL Connection
	pg, err := connectPostgresWithRetry(cfg, 30)
	if err != nil {
		log.Fatalf("Fatal: cannot connect to PostgreSQL: %v", err)
	}
	defer pg.Close()

	// 3. Initialize Alert Engine (Queue size 10,000 for dedicated worker)
	alertEngine := alert.NewEngine(pg, 10000)
	defer alertEngine.Close()

	// 4. Lightweight Health & Metrics Server for Worker
	workerPort := os.Getenv("WORKER_PORT")
	if workerPort == "" {
		workerPort = "8082"
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", telemetry.PrometheusHandler())
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"status": "ok", "service": "llm-gateway-worker", "dropped_events": %d}`, alertEngine.DroppedCount())))
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", workerPort),
		Handler: mux,
	}

	go func() {
		log.Printf("[Worker Plane] Worker health & metrics listening on http://0.0.0.0:%s", workerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Worker HTTP server error: %v", err)
		}
	}()

	log.Println("[Worker Plane] Background worker loops active: anti-spam cooldown, circuit breaker, webhook dispatcher.")

	// 5. Graceful Shutdown Signal Handler
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[Worker Plane] Shutting down Alert & Audit Worker gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = srv.Shutdown(ctx)
	alertEngine.Close()

	log.Println("[Worker Plane] Worker stopped clean.")
}

func connectPostgresWithRetry(cfg *config.Config, attempts int) (*db.PostgresDB, error) {
	var lastErr error
	for i := 1; i <= attempts; i++ {
		pg, err := db.NewPostgres(cfg)
		if err == nil {
			return pg, nil
		}
		lastErr = err
		log.Printf("Waiting for PostgreSQL (attempt %d/%d): %v", i, attempts, err)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("postgres unavailable after %d attempts: %w", attempts, lastErr)
}

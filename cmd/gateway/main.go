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

	"llm-gateway/internal/analytics"
	"llm-gateway/internal/auth"
	"llm-gateway/internal/config"
	"llm-gateway/internal/db"
	"llm-gateway/internal/handler"
	"llm-gateway/internal/provider"
	"llm-gateway/internal/service"
)

func main() {
	log.Println("Starting LLM Gateway Server...")

	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. Initialize PostgreSQL (retry until the container dependency is ready)
	pg, err := connectPostgresWithRetry(cfg, 30)
	if err != nil {
		log.Fatalf("Fatal: cannot connect to PostgreSQL: %v", err)
	}
	defer pg.Close()

	// 3. Initialize Redis (optional — rate limiting degrades gracefully if absent)
	redisClient, err := db.NewRedis(cfg)
	if err != nil {
		log.Printf("Warning: Redis connection error (rate limiting disabled): %v", err)
	}

	// 4. Initialize Services & Components
	keyStore := provider.NewKeyStore(pg, redisClient, cfg.ProviderMasterKey)
	authService := auth.NewAuthService(pg, redisClient, cfg.AdminAPIKey)
	providerClient := provider.NewProviderClient()
	costCalc := analytics.NewCostCalculator(pg)
	telemetryLogger := analytics.NewTelemetryLogger(pg, 10000)
	defer telemetryLogger.Close()

	gwService := service.NewGatewayService(providerClient, keyStore, costCalc, telemetryLogger, redisClient, pg)

	// 5. Initialize Handlers
	gwHandler := handler.NewGatewayHandler(authService, gwService, redisClient, pg)
	adminHandler := handler.NewAdminHandler(pg, authService, keyStore, costCalc, redisClient)

	// 6. Router Setup: Public Gateway Mux (Port 8080)
	publicMux := http.NewServeMux()
	publicMux.HandleFunc("POST /v1/chat/completions", gwHandler.HandleOpenAIChatCompletions)
	publicMux.HandleFunc("POST /v1/messages", gwHandler.HandleAnthropicMessages)
	publicMux.HandleFunc("GET /v1/models", gwHandler.HandleListModels)
	publicMux.HandleFunc("GET /v1/models/{id}", gwHandler.HandleGetModel)
	publicMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "llm-gateway-public"}`))
	})

	// 7. Router Setup: Admin Management Mux (Port 8081)
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /admin/users", adminHandler.ListUsers)
	adminMux.HandleFunc("GET /users", adminHandler.ListUsers)
	adminMux.HandleFunc("POST /admin/users", adminHandler.CreateUser)
	adminMux.HandleFunc("POST /users", adminHandler.CreateUser)
	adminMux.HandleFunc("GET /admin/keys", adminHandler.ListAPIKeys)
	adminMux.HandleFunc("GET /keys", adminHandler.ListAPIKeys)
	adminMux.HandleFunc("POST /admin/keys", adminHandler.CreateAPIKey)
	adminMux.HandleFunc("POST /keys", adminHandler.CreateAPIKey)
	adminMux.HandleFunc("DELETE /admin/keys", adminHandler.RevokeAPIKey)
	adminMux.HandleFunc("DELETE /keys", adminHandler.RevokeAPIKey)
	adminMux.HandleFunc("GET /admin/providers", adminHandler.ListProviderKeys)
	adminMux.HandleFunc("GET /providers", adminHandler.ListProviderKeys)
	adminMux.HandleFunc("POST /admin/providers", adminHandler.SetProviderKey)
	adminMux.HandleFunc("POST /providers", adminHandler.SetProviderKey)
	adminMux.HandleFunc("DELETE /admin/providers", adminHandler.DeleteProviderKey)
	adminMux.HandleFunc("DELETE /providers", adminHandler.DeleteProviderKey)
	adminMux.HandleFunc("GET /admin/analytics/stats", adminHandler.GetUsageStats)
	adminMux.HandleFunc("GET /analytics/stats", adminHandler.GetUsageStats)
	adminMux.HandleFunc("GET /admin/models", adminHandler.GetModels)
	adminMux.HandleFunc("GET /models", adminHandler.GetModels)
	adminMux.HandleFunc("POST /admin/models", adminHandler.UpsertModel)
	adminMux.HandleFunc("POST /models", adminHandler.UpsertModel)
	adminMux.HandleFunc("DELETE /admin/models", adminHandler.DeleteModel)
	adminMux.HandleFunc("DELETE /models", adminHandler.DeleteModel)
	adminMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "llm-gateway-admin"}`))
	})

	publicServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      publicMux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	adminServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.AdminPort),
		Handler:      adminMux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 8. Start Both HTTP Servers in Goroutines
	go func() {
		log.Printf("🚀 Public LLM Gateway running on port %s (Bearer Auth required)", cfg.Port)
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Public Gateway server error: %v", err)
		}
	}()

	go func() {
		log.Printf("🛠 Admin Management Server running on port %s (Admin Key required)", cfg.AdminPort)
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Admin server error: %v", err)
		}
	}()

	// 9. Graceful Shutdown Signal Handler
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down LLM Gateway servers gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := publicServer.Shutdown(ctx); err != nil {
		log.Printf("Public server forced to shutdown: %v", err)
	}
	if err := adminServer.Shutdown(ctx); err != nil {
		log.Printf("Admin server forced to shutdown: %v", err)
	}

	log.Println("LLM Gateway servers stopped clean.")
}

// connectPostgresWithRetry waits for PostgreSQL to become available in the
// docker-compose stack before giving up.
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


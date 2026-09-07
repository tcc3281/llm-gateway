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
	"llm-gateway/internal/cache"
	"llm-gateway/internal/config"
	"llm-gateway/internal/db"
	"llm-gateway/internal/handler"
	"llm-gateway/internal/provider"
	"llm-gateway/internal/service"
	"llm-gateway/internal/telemetry"
	"llm-gateway/internal/web"
)

func main() {
	log.Println("Starting LLM Gateway Server...")

	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 1b. Initialize OpenTelemetry Tracing
	otelShutdown, err := telemetry.Setup(context.Background(), "llm-gateway", cfg.OTelExporter, cfg.OTelEndpoint)
	if err != nil {
		log.Printf("Warning: OTel initialization error (tracing disabled): %v", err)
	} else {
		defer func() {
			sCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := otelShutdown(sCtx); err != nil {
				log.Printf("Error shutting down OTel: %v", err)
			}
		}()
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

	globalCacheTTL := time.Duration(cfg.ResponseCacheTTL) * time.Second
	responseCache := cache.NewResponseCache(redisClient, globalCacheTTL)
	gwService := service.NewGatewayService(providerClient, keyStore, costCalc, telemetryLogger, redisClient, pg, responseCache)

	// Refresh each provider's discovered model catalog hourly in the background
	// so new upstream models resolve without an admin touching the UI.
	go gwService.StartModelDiscoveryLoop(context.Background(), time.Hour)

	// 5. Initialize Handlers
	gwHandler := handler.NewGatewayHandler(authService, gwService, redisClient, pg)
	adminHandler := handler.NewAdminHandler(pg, authService, keyStore, costCalc, redisClient, cfg.AdminAllowedIPs, cfg, gwService)

	// 6. Router Setup: Public Gateway Mux (Port 8080)
	publicMux := http.NewServeMux()
	publicMux.HandleFunc("POST /v1/chat/completions", gwHandler.HandleOpenAIChatCompletions)
	publicMux.HandleFunc("POST /v1/messages", gwHandler.HandleAnthropicMessages)
	publicMux.HandleFunc("POST /v1/embeddings", gwHandler.HandleEmbeddings)
	publicMux.HandleFunc("GET /v1/models", gwHandler.HandleListModels)
	publicMux.HandleFunc("GET /v1/models/{id}", gwHandler.HandleGetModel)
	publicMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "llm-gateway-public"}`))
	})

	// 7. Router Setup: Admin Management Mux (Port 8081)
	adminMux := http.NewServeMux()
	// Only the /admin/* paths are registered as API routes: the bare paths
	// (/users, /keys, /providers, /models, /analytics/*) are owned by the React
	// SPA for client-side routing, so no alias is registered for them.
	adminMux.HandleFunc("GET /admin/users", adminHandler.ListUsers)
	adminMux.HandleFunc("POST /admin/users", adminHandler.CreateUser)
	adminMux.HandleFunc("PATCH /admin/users", adminHandler.UpdateUser)
	adminMux.HandleFunc("DELETE /admin/users", adminHandler.DeleteUser)
	adminMux.HandleFunc("GET /admin/keys", adminHandler.ListAPIKeys)
	adminMux.HandleFunc("POST /admin/keys", adminHandler.CreateAPIKey)
	adminMux.HandleFunc("PATCH /admin/keys", adminHandler.UpdateAPIKey)
	adminMux.HandleFunc("DELETE /admin/keys", adminHandler.RevokeAPIKey)
	adminMux.HandleFunc("POST /admin/keys/reactivate", adminHandler.ReactivateAPIKey)
	adminMux.HandleFunc("GET /admin/providers", adminHandler.ListProviderKeys)
	adminMux.HandleFunc("POST /admin/providers", adminHandler.SetProviderKey)
	adminMux.HandleFunc("DELETE /admin/providers", adminHandler.DeleteProviderKey)
	adminMux.HandleFunc("GET /admin/analytics/stats", adminHandler.GetUsageStats)
	adminMux.HandleFunc("GET /admin/analytics/dashboard", adminHandler.GetDashboardStats)
	adminMux.HandleFunc("GET /admin/models", adminHandler.GetModels)
	adminMux.HandleFunc("GET /admin/models/catalog", adminHandler.GetModelCatalog)
	adminMux.HandleFunc("POST /admin/models", adminHandler.UpsertModel)
	adminMux.HandleFunc("POST /admin/models/sync", adminHandler.SyncModelsFromCatalog)
	adminMux.HandleFunc("POST /admin/models/discover", adminHandler.DiscoverModels)
	adminMux.HandleFunc("DELETE /admin/models", adminHandler.DeleteModel)
	adminMux.HandleFunc("GET /admin/logs", adminHandler.GetUsageLogs)
	adminMux.HandleFunc("GET /admin/system/info", adminHandler.GetSystemInfo)
	adminMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "llm-gateway-admin"}`))
	})
	// SPA: catch-all for GET requests not matched by a specific /admin/* route.
	// Client-side routing in the React app owns every path below this handler.
	adminMux.Handle("/", web.Handler())

	publicServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      logRequests(publicMux),
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

// logRequests wraps a mux with a one-line access log so every request Claude
// Code / SDK clients send to the public gateway is visible in `docker compose
// logs gateway` — including calls to unregistered paths (e.g. count_tokens)
// that never reach HandleRequest and therefore leave no telemetry trace.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		log.Printf("HTTP %s %s -> %d (from %s)", r.Method, r.URL.Path, rec.status, r.RemoteAddr)
	})
}

// statusRecorder captures the response status for the access log. It defaults
// to 200 so streaming responses (which only call Write) log correctly.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Flush preserves the underlying writer's http.Flusher so the gateway's SSE
// streaming path (which asserts w.(http.Flusher)) keeps working behind the
// access-log wrapper. Embedding only http.ResponseWriter would drop Flush.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
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

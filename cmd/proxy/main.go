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
	"llm-gateway/internal/analytics"
	"llm-gateway/internal/auth"
	"llm-gateway/internal/cache"
	"llm-gateway/internal/config"
	"llm-gateway/internal/db"
	"llm-gateway/internal/docs"
	"llm-gateway/internal/handler"
	"llm-gateway/internal/permission"
	"llm-gateway/internal/provider"
	"llm-gateway/internal/proxy"
	"llm-gateway/internal/telemetry"
)

// Data Plane Entrypoint — High-Performance Proxy Core (Pure Stateless Proxy)
func main() {
	log.Println("[Data Plane] Starting LLM Gateway High-Performance Proxy Core...")

	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Config load error: %v", err)
	}

	// 2. OpenTelemetry Tracing Setup
	otelShutdown, err := telemetry.Setup(context.Background(), "llm-gateway-proxy", cfg.OTelExporter, cfg.OTelEndpoint)
	if err != nil {
		log.Printf("Warning: OpenTelemetry init failed: %v", err)
	} else {
		defer func() {
			sCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := otelShutdown(sCtx); err != nil {
				log.Printf("Warning: OTel shutdown error: %v", err)
			}
		}()
	}

	// 3. PostgreSQL Connection
	pg, err := connectPostgresWithRetry(cfg, 30)
	if err != nil {
		log.Fatalf("Fatal: cannot connect to PostgreSQL: %v", err)
	}
	defer pg.Close()

	// 4. Redis Client (Fast in-memory cache for API key validation & rate limits)
	redisClient, err := db.NewRedis(cfg)
	if err != nil {
		log.Printf("Warning: Redis connection error: %v", err)
	}

	// 5. Initialize Components
	keyStore := provider.NewKeyStore(pg, redisClient, cfg.ProviderMasterKey)
	authService := auth.NewAuthService(pg, redisClient, cfg.AdminAPIKey)
	providerClient := provider.NewProviderClient()
	costCalc := analytics.NewCostCalculator(pg)
	telemetryLogger := analytics.NewTelemetryLogger(pg, 10000)
	defer telemetryLogger.Close()

	globalCacheTTL := time.Duration(cfg.ResponseCacheTTL) * time.Second
	responseCache := cache.NewResponseCache(redisClient, globalCacheTTL)
	permEngine := permission.NewPermissionEngine(pg)
	alertEngine := alert.NewEngine(pg, 5000)
	defer alertEngine.Close()

	// 6. Data Plane Proxy Engine
	proxyEngine := proxy.NewEngine(
		providerClient,
		keyStore,
		costCalc,
		telemetryLogger,
		redisClient,
		pg,
		responseCache,
		permEngine,
		cfg.ContentCapturePolicy,
		cfg.SLOMaxTTFTMs,
		cfg.SLOMaxTBTMs,
		cfg.AgentMaxToolCalls,
	)
	proxyEngine.SetAlertEngine(alertEngine)

	// Refresh discovered model catalog hourly in the background
	go proxyEngine.StartModelDiscoveryLoop(context.Background(), time.Hour)

	// Start in-memory plugin cache background sync & distributed Redis Pub/Sub listener
	if pe := proxyEngine.PluginEngine(); pe != nil {
		pe.StartBackgroundSync(context.Background(), 30*time.Second)
		if redisClient != nil {
			pe.StartRedisSubscriber(context.Background(), redisClient)
		}
	}

	// 7. Handlers & Public Mux
	gwHandler := handler.NewGatewayHandler(authService, proxyEngine, redisClient, pg)

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", telemetry.PrometheusHandler())
	mux.HandleFunc("POST /v1/chat/completions", gwHandler.HandleOpenAIChatCompletions)
	mux.HandleFunc("POST /v1/messages", gwHandler.HandleAnthropicMessages)
	mux.HandleFunc("POST /v1/embeddings", gwHandler.HandleEmbeddings)
	mux.HandleFunc("GET /v1/models", gwHandler.HandleListModels)
	mux.HandleFunc("GET /v1/models/{id}", gwHandler.HandleGetModel)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "llm-gateway-data-plane"}`))
	})
	docs.RegisterRoutes(mux, true)

	proxyServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      logRequests(mux),
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 300 * time.Second,
	}

	go func() {
		log.Printf("[Data Plane] Proxy Core listening on http://0.0.0.0:%s (AI Traffic only)", cfg.Port)
		if err := proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Proxy server error: %v", err)
		}
	}()

	// 8. Graceful Shutdown Signal Handler
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[Data Plane] Shutting down Proxy Core gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := proxyServer.Shutdown(ctx); err != nil {
		log.Printf("Proxy server forced to shutdown: %v", err)
	}

	log.Println("[Data Plane] Proxy Core stopped clean.")
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		log.Printf("HTTP %s %s -> %d (from %s)", r.Method, r.URL.Path, rec.status, r.RemoteAddr)
	})
}

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

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
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

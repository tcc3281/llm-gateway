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
	cfg.Validate()

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
	permEngine := permission.NewPermissionEngine(pg)
	alertEngine := alert.NewEngine(pg, 5000)
	defer alertEngine.Close()

	gwService := service.NewGatewayService(
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
	gwService.SetAlertEngine(alertEngine)

	// Refresh each provider's discovered model catalog hourly in the background
	// so new upstream models resolve without an admin touching the UI.
	go gwService.StartModelDiscoveryLoop(context.Background(), time.Hour)

	// Start in-memory plugin cache background sync & distributed Redis Pub/Sub listener
	if pe := gwService.PluginEngine(); pe != nil {
		pe.StartBackgroundSync(context.Background(), 30*time.Second)
		if redisClient != nil {
			pe.StartRedisSubscriber(context.Background(), redisClient)
		}
	}

	// 5. Initialize Handlers
	gwHandler := handler.NewGatewayHandler(authService, gwService, redisClient, pg)
	adminHandler := handler.NewAdminHandler(pg, authService, keyStore, costCalc, redisClient, cfg.AdminAllowedIPs, cfg, gwService, permEngine, alertEngine)

	// 6. Router Setup: Public Gateway Mux (Port 8080)
	publicMux := http.NewServeMux()
	publicMux.Handle("GET /metrics", telemetry.PrometheusHandler())
	publicMux.HandleFunc("POST /v1/chat/completions", gwHandler.HandleOpenAIChatCompletions)
	publicMux.HandleFunc("POST /v1/messages", gwHandler.HandleAnthropicMessages)
	publicMux.HandleFunc("POST /v1/embeddings", gwHandler.HandleEmbeddings)
	publicMux.HandleFunc("GET /v1/models", gwHandler.HandleListModels)
	publicMux.HandleFunc("GET /v1/models/{id}", gwHandler.HandleGetModel)
	publicMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "llm-gateway"}`))
	})
	docs.RegisterRoutes(publicMux, false)

	// 7. Router Setup: Admin Management Mux (Port 8081)
	adminMux := http.NewServeMux()
	docs.RegisterRoutes(adminMux, false)
	adminMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "llm-gateway-admin"}`))
	})
	adminMux.Handle("GET /metrics", telemetry.PrometheusHandler())

	// Register admin API routes on both muxes so single-port environments (like Cloud Run)
	// can access both Data Plane (/v1/*) and Control Plane (/admin/* & Web UI):
	registerAdminRoutes(adminMux, adminHandler)
	registerAdminRoutes(publicMux, adminHandler)

	// SPA: catch-all for GET requests not matched by a specific /v1/* or /admin/* route.
	// Client-side routing in the React app owns every path below this handler.
	adminMux.Handle("/", web.Handler())
	publicMux.Handle("/", web.Handler())

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
		log.Printf("🚀 Public LLM Gateway running on port %s (Bearer Auth required, Web UI enabled)", cfg.Port)
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Public Gateway server error: %v", err)
		}
	}()

	if cfg.AdminPort != "" && cfg.AdminPort != cfg.Port {
		go func() {
			log.Printf("🛠 Admin Management Server running on port %s (Admin Key required)", cfg.AdminPort)
			if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Admin server error: %v", err)
			}
		}()
	}

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
	if cfg.AdminPort != "" && cfg.AdminPort != cfg.Port {
		if err := adminServer.Shutdown(ctx); err != nil {
			log.Printf("Admin server forced to shutdown: %v", err)
		}
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

func registerAdminRoutes(mux *http.ServeMux, adminHandler *handler.AdminHandler) {
	mux.HandleFunc("POST /admin/auth/login", adminHandler.Login)
	mux.HandleFunc("GET /admin/auth/me", adminHandler.GetCurrentUser)
	mux.HandleFunc("POST /admin/auth/password", adminHandler.SetPassword)

	// Member & Team Analytics ("ai dùng gì, dùng bao nhiêu")
	mux.HandleFunc("GET /admin/analytics/members", adminHandler.GetMemberUsageAnalytics)
	mux.HandleFunc("GET /admin/analytics/members/{id}", adminHandler.GetMemberDetail)

	mux.HandleFunc("GET /admin/users", adminHandler.ListUsers)
	mux.HandleFunc("POST /admin/users", adminHandler.CreateUser)
	mux.HandleFunc("PATCH /admin/users", adminHandler.UpdateUser)
	mux.HandleFunc("DELETE /admin/users", adminHandler.DeleteUser)
	mux.HandleFunc("GET /admin/keys", adminHandler.ListAPIKeys)
	mux.HandleFunc("POST /admin/keys", adminHandler.CreateAPIKey)
	mux.HandleFunc("PATCH /admin/keys", adminHandler.UpdateAPIKey)
	mux.HandleFunc("DELETE /admin/keys", adminHandler.RevokeAPIKey)
	mux.HandleFunc("POST /admin/keys/reactivate", adminHandler.ReactivateAPIKey)
	mux.HandleFunc("GET /admin/keys/{id}/models", adminHandler.ListKeyModelPolicies)
	mux.HandleFunc("POST /admin/keys/{id}/models", adminHandler.CreateKeyModelPolicy)
	mux.HandleFunc("DELETE /admin/keys/{id}/models/{pid}", adminHandler.DeleteKeyModelPolicy)
	mux.HandleFunc("GET /admin/groups", adminHandler.ListGroups)
	mux.HandleFunc("POST /admin/groups", adminHandler.CreateGroup)
	mux.HandleFunc("GET /admin/groups/{id}", adminHandler.GetGroup)
	mux.HandleFunc("PATCH /admin/groups/{id}", adminHandler.UpdateGroup)
	mux.HandleFunc("DELETE /admin/groups/{id}", adminHandler.DeleteGroup)
	mux.HandleFunc("GET /admin/groups/{id}/policies", adminHandler.ListGroupPolicies)
	mux.HandleFunc("POST /admin/groups/{id}/policies", adminHandler.CreateGroupPolicy)
	mux.HandleFunc("PATCH /admin/groups/{id}/policies/{pid}", adminHandler.UpdateGroupPolicy)
	mux.HandleFunc("DELETE /admin/groups/{id}/policies/{pid}", adminHandler.DeleteGroupPolicy)
	mux.HandleFunc("GET /admin/iam/actions", adminHandler.GetIAMActions)
	mux.HandleFunc("GET /admin/groups/{id}/members", adminHandler.ListGroupMembers)
	mux.HandleFunc("POST /admin/groups/{id}/members", adminHandler.AddGroupMember)
	mux.HandleFunc("PUT /admin/groups/{id}/members/{uid}/role", adminHandler.SetGroupMemberRole)
	mux.HandleFunc("DELETE /admin/groups/{id}/members/{uid}", adminHandler.RemoveGroupMember)
	mux.HandleFunc("GET /admin/tenants", adminHandler.ListTenants)
	mux.HandleFunc("POST /admin/tenants", adminHandler.CreateTenant)
	mux.HandleFunc("GET /admin/tenants/{id}", adminHandler.GetTenant)
	mux.HandleFunc("PATCH /admin/tenants/{id}", adminHandler.UpdateTenant)
	mux.HandleFunc("DELETE /admin/tenants/{id}", adminHandler.DeleteTenant)
	mux.HandleFunc("POST /admin/tenants/{id}/topup", adminHandler.TopUpCredit)
	mux.HandleFunc("GET /admin/tenants/{id}/transactions", adminHandler.ListTenantTransactions)
	mux.HandleFunc("GET /admin/audit-logs", adminHandler.ListAuditLogs)
	mux.HandleFunc("GET /admin/providers", adminHandler.ListProviderKeys)
	mux.HandleFunc("POST /admin/providers", adminHandler.SetProviderKey)
	mux.HandleFunc("DELETE /admin/providers", adminHandler.DeleteProviderKey)
	mux.HandleFunc("GET /admin/analytics/stats", adminHandler.GetUsageStats)
	mux.HandleFunc("GET /admin/analytics/dashboard", adminHandler.GetDashboardStats)
	mux.HandleFunc("GET /admin/models", adminHandler.GetModels)
	mux.HandleFunc("GET /admin/models/catalog", adminHandler.GetModelCatalog)
	mux.HandleFunc("POST /admin/models", adminHandler.UpsertModel)
	mux.HandleFunc("POST /admin/models/sync", adminHandler.SyncModelsFromCatalog)
	mux.HandleFunc("POST /admin/models/discover", adminHandler.DiscoverModels)
	mux.HandleFunc("DELETE /admin/models", adminHandler.DeleteModel)
	mux.HandleFunc("GET /admin/virtual-models", adminHandler.ListVirtualModels)
	mux.HandleFunc("POST /admin/virtual-models", adminHandler.CreateVirtualModel)
	mux.HandleFunc("GET /admin/virtual-models/{id}", adminHandler.GetVirtualModel)
	mux.HandleFunc("PUT /admin/virtual-models/{id}", adminHandler.UpdateVirtualModel)
	mux.HandleFunc("DELETE /admin/virtual-models/{id}", adminHandler.DeleteVirtualModel)
	mux.HandleFunc("GET /admin/plugins", adminHandler.ListPlugins)
	mux.HandleFunc("GET /admin/plugins/{name}", adminHandler.GetPlugin)
	mux.HandleFunc("GET /admin/plugin-bindings", adminHandler.ListPluginBindings)
	mux.HandleFunc("POST /admin/plugin-bindings", adminHandler.CreatePluginBinding)
	mux.HandleFunc("GET /admin/plugin-bindings/{id}", adminHandler.GetPluginBinding)
	mux.HandleFunc("PUT /admin/plugin-bindings/{id}", adminHandler.UpdatePluginBinding)
	mux.HandleFunc("DELETE /admin/plugin-bindings/{id}", adminHandler.DeletePluginBinding)
	mux.HandleFunc("GET /admin/logs", adminHandler.GetUsageLogs)
	mux.HandleFunc("GET /admin/logs/{id}", adminHandler.GetUsageLogDetail)
	mux.HandleFunc("GET /admin/alerts/rules", adminHandler.ListAlertRules)
	mux.HandleFunc("POST /admin/alerts/rules", adminHandler.CreateAlertRule)
	mux.HandleFunc("PATCH /admin/alerts/rules/{id}", adminHandler.UpdateAlertRule)
	mux.HandleFunc("DELETE /admin/alerts/rules/{id}", adminHandler.DeleteAlertRule)
	mux.HandleFunc("GET /admin/alerts/history", adminHandler.ListAlertHistory)
	mux.HandleFunc("POST /admin/alerts/test", adminHandler.TestAlertWebhook)
	mux.HandleFunc("GET /admin/system/info", adminHandler.GetSystemInfo)
}

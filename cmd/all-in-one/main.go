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
	"llm-gateway/internal/web"
)

// All-in-One Entrypoint — Runs both Data Plane (Proxy) and Control Plane (Admin) concurrently.
// Ideal for local development, staging, or unified container deployments.
func main() {
	log.Println("[All-in-One] Starting LLM Gateway (Data Plane + Control Plane)...")

	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Config load error: %v", err)
	}

	// 2. OpenTelemetry Tracing Setup
	otelShutdown, err := telemetry.Setup(context.Background(), "llm-gateway", cfg.OTelExporter, cfg.OTelEndpoint)
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

	// 4. Redis Client
	redisClient, err := db.NewRedis(cfg)
	if err != nil {
		log.Printf("Warning: Redis connection error: %v", err)
	}

	// 5. Initialize Core Components
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

	go proxyEngine.StartModelDiscoveryLoop(context.Background(), time.Hour)

	// Start in-memory plugin cache background sync & distributed Redis Pub/Sub listener
	if pe := proxyEngine.PluginEngine(); pe != nil {
		pe.StartBackgroundSync(context.Background(), 30*time.Second)
		if redisClient != nil {
			pe.StartRedisSubscriber(context.Background(), redisClient)
		}
	}

	// 6. Initialize Handlers
	gwHandler := handler.NewGatewayHandler(authService, proxyEngine, redisClient, pg)
	adminHandler := handler.NewAdminHandler(pg, authService, keyStore, costCalc, redisClient, cfg.AdminAllowedIPs, cfg, proxyEngine, permEngine, alertEngine)

	// 7. Router Setup: Public Gateway Mux (Data Plane, Port 8080)
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
		w.Write([]byte(`{"status": "ok", "service": "llm-gateway-public"}`))
	})
	docs.RegisterRoutes(publicMux, true)

	// 8. Router Setup: Admin Management Mux (Control Plane, Port 8081)
	adminMux := http.NewServeMux()
	docs.RegisterRoutes(adminMux, false)
	adminMux.HandleFunc("POST /admin/auth/login", adminHandler.Login)
	adminMux.HandleFunc("GET /admin/auth/me", adminHandler.GetCurrentUser)
	adminMux.HandleFunc("POST /admin/auth/password", adminHandler.SetPassword)

	adminMux.HandleFunc("GET /admin/analytics/members", adminHandler.GetMemberUsageAnalytics)
	adminMux.HandleFunc("GET /admin/analytics/members/{id}", adminHandler.GetMemberDetail)

	adminMux.HandleFunc("GET /admin/users", adminHandler.ListUsers)
	adminMux.HandleFunc("POST /admin/users", adminHandler.CreateUser)
	adminMux.HandleFunc("PATCH /admin/users", adminHandler.UpdateUser)
	adminMux.HandleFunc("DELETE /admin/users", adminHandler.DeleteUser)
	adminMux.HandleFunc("GET /admin/keys", adminHandler.ListAPIKeys)
	adminMux.HandleFunc("POST /admin/keys", adminHandler.CreateAPIKey)
	adminMux.HandleFunc("PATCH /admin/keys", adminHandler.UpdateAPIKey)
	adminMux.HandleFunc("DELETE /admin/keys", adminHandler.RevokeAPIKey)
	adminMux.HandleFunc("POST /admin/keys/reactivate", adminHandler.ReactivateAPIKey)
	adminMux.HandleFunc("GET /admin/keys/{id}/models", adminHandler.ListKeyModelPolicies)
	adminMux.HandleFunc("POST /admin/keys/{id}/models", adminHandler.CreateKeyModelPolicy)
	adminMux.HandleFunc("DELETE /admin/keys/{id}/models/{pid}", adminHandler.DeleteKeyModelPolicy)
	adminMux.HandleFunc("GET /admin/groups", adminHandler.ListGroups)
	adminMux.HandleFunc("POST /admin/groups", adminHandler.CreateGroup)
	adminMux.HandleFunc("GET /admin/groups/{id}", adminHandler.GetGroup)
	adminMux.HandleFunc("PATCH /admin/groups/{id}", adminHandler.UpdateGroup)
	adminMux.HandleFunc("DELETE /admin/groups/{id}", adminHandler.DeleteGroup)
	adminMux.HandleFunc("GET /admin/groups/{id}/policies", adminHandler.ListGroupPolicies)
	adminMux.HandleFunc("POST /admin/groups/{id}/policies", adminHandler.CreateGroupPolicy)
	adminMux.HandleFunc("PATCH /admin/groups/{id}/policies/{pid}", adminHandler.UpdateGroupPolicy)
	adminMux.HandleFunc("DELETE /admin/groups/{id}/policies/{pid}", adminHandler.DeleteGroupPolicy)
	adminMux.HandleFunc("GET /admin/iam/actions", adminHandler.GetIAMActions)
	adminMux.HandleFunc("GET /admin/groups/{id}/members", adminHandler.ListGroupMembers)
	adminMux.HandleFunc("POST /admin/groups/{id}/members", adminHandler.AddGroupMember)
	adminMux.HandleFunc("PUT /admin/groups/{id}/members/{uid}/role", adminHandler.SetGroupMemberRole)
	adminMux.HandleFunc("DELETE /admin/groups/{id}/members/{uid}", adminHandler.RemoveGroupMember)
	adminMux.HandleFunc("GET /admin/tenants", adminHandler.ListTenants)
	adminMux.HandleFunc("POST /admin/tenants", adminHandler.CreateTenant)
	adminMux.HandleFunc("GET /admin/tenants/{id}", adminHandler.GetTenant)
	adminMux.HandleFunc("PATCH /admin/tenants/{id}", adminHandler.UpdateTenant)
	adminMux.HandleFunc("DELETE /admin/tenants/{id}", adminHandler.DeleteTenant)
	adminMux.HandleFunc("POST /admin/tenants/{id}/topup", adminHandler.TopUpCredit)
	adminMux.HandleFunc("GET /admin/tenants/{id}/transactions", adminHandler.ListTenantTransactions)
	adminMux.HandleFunc("GET /admin/audit-logs", adminHandler.ListAuditLogs)
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
	adminMux.HandleFunc("GET /admin/virtual-models", adminHandler.ListVirtualModels)
	adminMux.HandleFunc("POST /admin/virtual-models", adminHandler.CreateVirtualModel)
	adminMux.HandleFunc("GET /admin/virtual-models/{id}", adminHandler.GetVirtualModel)
	adminMux.HandleFunc("PUT /admin/virtual-models/{id}", adminHandler.UpdateVirtualModel)
	adminMux.HandleFunc("DELETE /admin/virtual-models/{id}", adminHandler.DeleteVirtualModel)
	adminMux.HandleFunc("GET /admin/plugins", adminHandler.ListPlugins)
	adminMux.HandleFunc("GET /admin/plugins/{name}", adminHandler.GetPlugin)
	adminMux.HandleFunc("GET /admin/plugin-bindings", adminHandler.ListPluginBindings)
	adminMux.HandleFunc("POST /admin/plugin-bindings", adminHandler.CreatePluginBinding)
	adminMux.HandleFunc("GET /admin/plugin-bindings/{id}", adminHandler.GetPluginBinding)
	adminMux.HandleFunc("PUT /admin/plugin-bindings/{id}", adminHandler.UpdatePluginBinding)
	adminMux.HandleFunc("DELETE /admin/plugin-bindings/{id}", adminHandler.DeletePluginBinding)
	adminMux.HandleFunc("GET /admin/logs", adminHandler.GetUsageLogs)
	adminMux.HandleFunc("GET /admin/logs/{id}", adminHandler.GetUsageLogDetail)
	adminMux.HandleFunc("GET /admin/alerts/rules", adminHandler.ListAlertRules)
	adminMux.HandleFunc("POST /admin/alerts/rules", adminHandler.CreateAlertRule)
	adminMux.HandleFunc("PATCH /admin/alerts/rules/{id}", adminHandler.UpdateAlertRule)
	adminMux.HandleFunc("DELETE /admin/alerts/rules/{id}", adminHandler.DeleteAlertRule)
	adminMux.HandleFunc("GET /admin/alerts/history", adminHandler.ListAlertHistory)
	adminMux.HandleFunc("POST /admin/alerts/test", adminHandler.TestAlertWebhook)
	adminMux.HandleFunc("GET /admin/system/info", adminHandler.GetSystemInfo)
	adminMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "llm-gateway-admin"}`))
	})
	adminMux.Handle("GET /metrics", telemetry.PrometheusHandler())
	adminMux.Handle("/", web.Handler())

	// 9. Start Servers
	publicServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      logRequests(publicMux),
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 300 * time.Second,
	}

	adminServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.AdminPort),
		Handler:      adminMux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		log.Printf("[All-in-One] Public API Gateway listening on http://0.0.0.0:%s", cfg.Port)
		if err := publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Public server error: %v", err)
		}
	}()

	go func() {
		log.Printf("[All-in-One] Admin Management & Web UI listening on http://0.0.0.0:%s", cfg.AdminPort)
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Admin server error: %v", err)
		}
	}()

	// 10. Graceful Shutdown Signal Handler
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[All-in-One] Shutting down LLM Gateway servers gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := publicServer.Shutdown(ctx); err != nil {
		log.Printf("Public server forced to shutdown: %v", err)
	}
	if err := adminServer.Shutdown(ctx); err != nil {
		log.Printf("Admin server forced to shutdown: %v", err)
	}

	log.Println("[All-in-One] LLM Gateway servers stopped clean.")
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

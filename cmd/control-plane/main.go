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
	"llm-gateway/internal/config"
	"llm-gateway/internal/db"
	"llm-gateway/internal/docs"
	"llm-gateway/internal/handler"
	"llm-gateway/internal/permission"
	"llm-gateway/internal/provider"
	"llm-gateway/internal/telemetry"
	"llm-gateway/internal/web"
)

// Control Plane Entrypoint — IAM, Multi-Tenant Management, Billing, Alert Policies & Dashboard Web UI
func main() {
	log.Println("[Control Plane] Starting LLM Gateway IAM & Management Service...")

	// 1. Load Config
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Config load error: %v", err)
	}

	// 2. PostgreSQL Connection (Source of Truth for IAM, Policies, Tenants)
	pg, err := connectPostgresWithRetry(cfg, 30)
	if err != nil {
		log.Fatalf("Fatal: cannot connect to PostgreSQL: %v", err)
	}
	defer pg.Close()

	// 3. Redis Client (Config cache invalidation & session store)
	redisClient, err := db.NewRedis(cfg)
	if err != nil {
		log.Printf("Warning: Redis connection error: %v", err)
	}

	// 4. Initialize Control Plane Components
	keyStore := provider.NewKeyStore(pg, redisClient, cfg.ProviderMasterKey)
	authService := auth.NewAuthService(pg, redisClient, cfg.AdminAPIKey)
	costCalc := analytics.NewCostCalculator(pg)
	permEngine := permission.NewPermissionEngine(pg)
	alertEngine := alert.NewEngine(pg, 5000)
	defer alertEngine.Close()

	// 5. Admin Handler (Pass nil for gwService in pure Control Plane mode)
	adminHandler := handler.NewAdminHandler(
		pg,
		authService,
		keyStore,
		costCalc,
		redisClient,
		cfg.AdminAllowedIPs,
		cfg,
		nil, // gwService is nil in standalone control-plane
		permEngine,
		alertEngine,
	)

	// 6. Router Setup: Admin Management Mux (Port 8081)
	mux := http.NewServeMux()

	// Auth & Profile
	mux.HandleFunc("POST /admin/auth/login", adminHandler.Login)
	mux.HandleFunc("GET /admin/auth/me", adminHandler.GetCurrentUser)
	mux.HandleFunc("POST /admin/auth/password", adminHandler.SetPassword)

	// Member & Team Analytics
	mux.HandleFunc("GET /admin/analytics/members", adminHandler.GetMemberUsageAnalytics)
	mux.HandleFunc("GET /admin/analytics/members/{id}", adminHandler.GetMemberDetail)

	// Users & Keys
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

	// Groups & IAM Policies
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

	// Tenants & B2B Billing
	mux.HandleFunc("GET /admin/tenants", adminHandler.ListTenants)
	mux.HandleFunc("POST /admin/tenants", adminHandler.CreateTenant)
	mux.HandleFunc("GET /admin/tenants/{id}", adminHandler.GetTenant)
	mux.HandleFunc("PATCH /admin/tenants/{id}", adminHandler.UpdateTenant)
	mux.HandleFunc("DELETE /admin/tenants/{id}", adminHandler.DeleteTenant)
	mux.HandleFunc("POST /admin/tenants/{id}/topup", adminHandler.TopUpCredit)
	mux.HandleFunc("GET /admin/tenants/{id}/transactions", adminHandler.ListTenantTransactions)

	// Providers & Models
	mux.HandleFunc("GET /admin/providers", adminHandler.ListProviderKeys)
	mux.HandleFunc("POST /admin/providers", adminHandler.SetProviderKey)
	mux.HandleFunc("DELETE /admin/providers", adminHandler.DeleteProviderKey)
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

	// Observability & Auditing
	mux.HandleFunc("GET /admin/analytics/stats", adminHandler.GetUsageStats)
	mux.HandleFunc("GET /admin/analytics/dashboard", adminHandler.GetDashboardStats)
	mux.HandleFunc("GET /admin/logs", adminHandler.GetUsageLogs)
	mux.HandleFunc("GET /admin/logs/{id}", adminHandler.GetUsageLogDetail)
	mux.HandleFunc("GET /admin/audit-logs", adminHandler.ListAuditLogs)

	// Alerts & Webhook Management
	mux.HandleFunc("GET /admin/alerts/rules", adminHandler.ListAlertRules)
	mux.HandleFunc("POST /admin/alerts/rules", adminHandler.CreateAlertRule)
	mux.HandleFunc("PATCH /admin/alerts/rules/{id}", adminHandler.UpdateAlertRule)
	mux.HandleFunc("DELETE /admin/alerts/rules/{id}", adminHandler.DeleteAlertRule)
	mux.HandleFunc("GET /admin/alerts/history", adminHandler.ListAlertHistory)
	mux.HandleFunc("POST /admin/alerts/test", adminHandler.TestAlertWebhook)

	// System & Health
	mux.HandleFunc("GET /admin/system/info", adminHandler.GetSystemInfo)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok", "service": "llm-gateway-control-plane"}`))
	})
	mux.Handle("GET /metrics", telemetry.PrometheusHandler())
	docs.RegisterRoutes(mux, false)

	// SPA Dashboard Web UI Catch-all
	mux.Handle("/", web.Handler())

	adminServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.AdminPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		log.Printf("[Control Plane] Admin & Dashboard listening on http://0.0.0.0:%s (Management UI)", cfg.AdminPort)
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Control Plane server error: %v", err)
		}
	}()

	// 7. Graceful Shutdown Signal Handler
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[Control Plane] Shutting down Control Plane gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := adminServer.Shutdown(ctx); err != nil {
		log.Printf("Control Plane server forced to shutdown: %v", err)
	}

	log.Println("[Control Plane] Control Plane stopped clean.")
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

package proxy

import (
	"context"
	"time"

	"llm-gateway/internal/analytics"
	"llm-gateway/internal/cache"
	"llm-gateway/internal/db"
	"llm-gateway/internal/permission"
	"llm-gateway/internal/provider"
	"llm-gateway/internal/service"
)

// Engine represents the High-Performance Proxy Core (Data Plane).
type Engine = service.GatewayService

// ProxyParams encapsulates incoming completion or message parameters.
type ProxyParams = service.ProxyParams

// NewEngine instantiates the Data Plane proxy engine.
func NewEngine(
	p *provider.ProviderClient,
	k *provider.KeyStore,
	c *analytics.CostCalculator,
	t *analytics.TelemetryLogger,
	redis *db.RedisClient,
	pg *db.PostgresDB,
	rc *cache.ResponseCache,
	permEngine *permission.PermissionEngine,
	contentCapturePolicy string,
	sloMaxTTFTMs int,
	sloMaxTBTMs int,
	agentMaxToolCalls int,
) *Engine {
	return service.NewGatewayService(
		p, k, c, t, redis, pg, rc, permEngine,
		contentCapturePolicy, sloMaxTTFTMs, sloMaxTBTMs, agentMaxToolCalls,
	)
}

// StartModelDiscovery runs model synchronization in the background.
func StartModelDiscovery(ctx context.Context, engine *Engine, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	go engine.StartModelDiscoveryLoop(ctx, interval)
}

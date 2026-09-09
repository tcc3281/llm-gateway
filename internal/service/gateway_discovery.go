package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"llm-gateway/internal/model"
	"llm-gateway/internal/provider"
)

// ListAvailableModels returns the union of:
//   - models configured in the DB model_pricings table (authoritative; also the
//     only source for Anthropic, which has no public models-list API), and
//   - models advertised live by each provider that has a key configured.
//
// The result is cached in Redis for modelsCacheTTL. A cold start performs the
// live fetches inline; after that the endpoint is served from cache. Providers
// without a key, or whose list API fails, are skipped rather than failing the
// whole request.
func (g *GatewayService) ListAvailableModels(ctx context.Context) ([]model.ProviderModel, error) {
	if g.redis != nil {
		if cached, err := g.redis.GetCachedModels(ctx); err == nil && len(cached) > 0 {
			return cached, nil
		}
	}

	models := make([]model.ProviderModel, 0, 32)
	seen := make(map[string]bool)

	// 1. Admin-configured models from the pricing table.
	if g.pg != nil {
		if dbModels, err := g.pg.GetAllModelsList(); err == nil {
			for _, m := range dbModels {
				if seen[m.ModelName] {
					continue
				}
				seen[m.ModelName] = true
				created := m.UpdatedAt.Unix()
				if created <= 0 {
					created = 1700000000
				}
				models = append(models, model.ProviderModel{
					ID:       m.ModelName,
					Object:   "model",
					Created:  created,
					OwnedBy:  m.Provider,
					Provider: m.Provider,
				})
			}
		}

		// 1b. Active Virtual Models (v0.3.0)
		if vms, err := g.pg.ListVirtualModels(nil); err == nil {
			for _, vm := range vms {
				if !vm.IsActive || seen[vm.Name] {
					continue
				}
				seen[vm.Name] = true
				created := vm.CreatedAt.Unix()
				if created <= 0 {
					created = 1700000000
				}
				models = append(models, model.ProviderModel{
					ID:       vm.Name,
					Object:   "model",
					Created:  created,
					OwnedBy:  "virtual",
					Provider: "virtual",
				})
			}
		}
	}

	// 2. Live model lists from each provider that has a key configured (concurrent fetch).
	providers := []provider.ProviderType{
		provider.ProviderOpenAI,
		provider.ProviderAnthropic,
		provider.ProviderGemini,
	}

	type providerModelsResult struct {
		provider provider.ProviderType
		ids      []string
	}

	resChan := make(chan providerModelsResult, len(providers))
	var wg sync.WaitGroup

	for _, p := range providers {
		wg.Add(1)
		go func(pt provider.ProviderType) {
			defer wg.Done()
			key, err := g.keyStore.GetKey(ctx, string(pt))
			if err != nil {
				return // no key configured — nothing to fetch
			}
			ids, err := g.providerClient.ListModels(ctx, pt, key)
			if err != nil {
				return // upstream list API failed — keep the configured set
			}
			resChan <- providerModelsResult{provider: pt, ids: ids}
		}(p)
	}

	wg.Wait()
	close(resChan)

	nowUnix := time.Now().Unix()
	for res := range resChan {
		for _, id := range res.ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			models = append(models, model.ProviderModel{
				ID:       id,
				Object:   "model",
				Created:  nowUnix,
				OwnedBy:  string(res.provider),
				Provider: string(res.provider),
			})
		}
	}

	// Cache what we got. An empty set (no DB rows, no reachable provider) is
	// cached briefly so a transient outage doesn't hammer upstream on every miss.
	ttl := modelsCacheTTL
	if len(models) == 0 {
		ttl = modelsEmptyCache
	}
	if g.redis != nil {
		_ = g.redis.SetCachedModels(ctx, models, ttl)
	}
	return models, nil
}

// DiscoverProvider fetches one provider's /models API and persists the result
// into the provider_models catalog, then refreshes the routing cache. Returns
// the number of models stored.
func (g *GatewayService) DiscoverProvider(ctx context.Context, providerName string) (int, error) {
	pc, err := g.keyStore.GetProviderInfo(ctx, providerName)
	if err != nil {
		return 0, err
	}
	n, err := g.discoverProvider(ctx, *pc)
	if err != nil {
		return 0, err
	}
	if g.costCalc != nil {
		_ = g.costCalc.RefreshCatalog()
	}
	return n, nil
}

// DiscoverAllModels refreshes the persisted catalog for every configured
// provider. Returns the total number of models stored and per-provider errors;
// a provider whose /models API fails is skipped, not fatal.
func (g *GatewayService) DiscoverAllModels(ctx context.Context) (int, map[string]string) {
	providers, err := g.keyStore.ListActiveProviders()
	if err != nil {
		return 0, map[string]string{"": err.Error()}
	}
	total := 0
	errs := make(map[string]string)
	for _, pc := range providers {
		n, derr := g.discoverProvider(ctx, pc)
		if derr != nil {
			errs[pc.Provider] = derr.Error()
			continue
		}
		total += n
	}
	if g.costCalc != nil {
		_ = g.costCalc.RefreshCatalog()
	}
	return total, errs
}

// discoverProvider fetches and persists one provider's model list.
func (g *GatewayService) discoverProvider(ctx context.Context, pc provider.ProviderConfig) (int, error) {
	key, err := g.keyStore.GetKey(ctx, pc.Provider)
	if err != nil {
		return 0, err
	}

	var ids []string
	if pc.BaseURL != "" {
		// Custom OpenAI/Anthropic-compatible provider.
		ids, err = g.providerClient.ListModelsCustom(ctx, provider.ProviderType(pc.Protocol), pc.BaseURL, key)
	} else {
		ids, err = g.providerClient.ListModels(ctx, provider.ProviderType(pc.Provider), key)
	}
	if err != nil {
		return 0, err
	}
	if g.pg == nil {
		return 0, errors.New("database unavailable")
	}
	if err := g.pg.ReplaceProviderModels(pc.Provider, ids); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// StartModelDiscoveryLoop refreshes the catalog on a fixed interval and once at
// startup, so newly configured providers and upstream model additions are picked
// up without hardcoded model lists.
func (g *GatewayService) StartModelDiscoveryLoop(ctx context.Context, interval time.Duration) {
	g.DiscoverAllModels(ctx) // first pass at startup
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			g.DiscoverAllModels(ctx)
		}
	}
}

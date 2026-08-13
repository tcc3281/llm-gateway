package analytics

import (
	"fmt"
	"log"
	"sync"
	"time"

	"llm-gateway/internal/db"
	"llm-gateway/internal/model"
)

type CostCalculator struct {
	pg               *db.PostgresDB
	mu               sync.RWMutex
	pricings         map[string]model.ModelPricing
	providersByModel map[string]string
	catalogByModel   map[string]string
}

func NewCostCalculator(pg *db.PostgresDB) *CostCalculator {
	c := &CostCalculator{
		pg:               pg,
		pricings:         make(map[string]model.ModelPricing),
		providersByModel: make(map[string]string),
		catalogByModel:   make(map[string]string),
	}
	_ = c.RefreshPricing()
	_ = c.RefreshCatalog()

	// Keep the in-memory pricing + catalog caches in sync with the DB so admin
	// edits and provider model discovery take effect without a restart.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := c.RefreshPricing(); err != nil {
				log.Printf("Warning: failed to refresh pricing cache: %v", err)
			}
			if err := c.RefreshCatalog(); err != nil {
				log.Printf("Warning: failed to refresh catalog cache: %v", err)
			}
		}
	}()

	return c
}

func (c *CostCalculator) RefreshPricing() error {
	if c.pg == nil {
		return nil
	}
	pricings, err := c.pg.GetPricings()
	if err != nil {
		return err
	}

	// Reverse index (model → provider) so the gateway can route a request by
	// the model's registered provider instead of guessing from the name prefix.
	// If a model is registered under several providers (e.g. across catalog
	// syncs), the last one wins.
	providersByModel := make(map[string]string, len(pricings))
	for _, p := range pricings {
		providersByModel[p.ModelName] = p.Provider
	}

	c.mu.Lock()
	c.pricings = pricings
	c.providersByModel = providersByModel
	c.mu.Unlock()
	return nil
}

// RefreshCatalog reloads the discovered provider_models catalog into the routing
// cache. Called after discovery and on the background ticker alongside pricing.
func (c *CostCalculator) RefreshCatalog() error {
	if c.pg == nil {
		return nil
	}
	rows, err := c.pg.ListCatalogModels()
	if err != nil {
		return err
	}
	byModel := make(map[string]string, len(rows))
	for _, r := range rows {
		byModel[r.ModelName] = r.Provider
	}
	c.mu.Lock()
	c.catalogByModel = byModel
	c.mu.Unlock()
	return nil
}

// LookupProviderByModel reports the provider a model is known under: an explicit
// registration on the Models page (model_pricings) wins over a catalog row
// discovered from a provider's /models API (provider_models). ok is false when
// the model is in neither, so callers can reject unknown models instead of
// guessing an upstream from the name prefix.
func (c *CostCalculator) LookupProviderByModel(modelName string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if p, ok := c.providersByModel[modelName]; ok {
		return p, true
	}
	p, ok := c.catalogByModel[modelName]
	return p, ok
}

type CostResult struct {
	Cost                float64
	PromptPriceUsed     float64
	CompletionPriceUsed float64
	PricingSource       string
	PricingFallback     bool
}

func (c *CostCalculator) CalculateCost(provider, modelName string, promptTokens, completionTokens, cachedInputTokens, reasoningTokens int) CostResult {
	c.mu.RLock()
	key := fmt.Sprintf("%s:%s", provider, modelName)
	pricing, exists := c.pricings[key]
	c.mu.RUnlock()

	var promptPriceUsed, completionPriceUsed float64
	var pricingSource string
	var pricingFallback bool

	if !exists {
		// Default fallback estimations if pricing not in DB
		promptPriceUsed = 1.0     // $1.00 / 1M
		completionPriceUsed = 3.0 // $3.00 / 1M
		pricingSource = "fallback"
		pricingFallback = true
	} else {
		promptPriceUsed = pricing.PromptPricePer1M
		completionPriceUsed = pricing.CompletionPricePer1M
		pricingSource = "database"
		pricingFallback = false
	}

	// OpenAI/Anthropic: 50% discount for cached input tokens
	regularInputTokens := promptTokens - cachedInputTokens
	if regularInputTokens < 0 {
		regularInputTokens = 0
	}
	promptCost := (float64(regularInputTokens) / 1_000_000.0) * promptPriceUsed
	cachedPromptCost := (float64(cachedInputTokens) / 1_000_000.0) * (promptPriceUsed * 0.5)

	// Reasoning tokens are billed at standard completion rate
	completionCost := (float64(completionTokens) / 1_000_000.0) * completionPriceUsed

	totalCost := promptCost + cachedPromptCost + completionCost

	return CostResult{
		Cost:                totalCost,
		PromptPriceUsed:     promptPriceUsed,
		CompletionPriceUsed: completionPriceUsed,
		PricingSource:       pricingSource,
		PricingFallback:     pricingFallback,
	}
}

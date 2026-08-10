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
	pg       *db.PostgresDB
	mu       sync.RWMutex
	pricings map[string]model.ModelPricing
}

func NewCostCalculator(pg *db.PostgresDB) *CostCalculator {
	c := &CostCalculator{
		pg:       pg,
		pricings: make(map[string]model.ModelPricing),
	}
	_ = c.RefreshPricing()

	// Keep the in-memory pricing table in sync with model_pricings so admin
	// edits take effect without a restart.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := c.RefreshPricing(); err != nil {
				log.Printf("Warning: failed to refresh pricing cache: %v", err)
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

	c.mu.Lock()
	c.pricings = pricings
	c.mu.Unlock()
	return nil
}

func (c *CostCalculator) CalculateCost(provider, modelName string, promptTokens, completionTokens int) float64 {
	c.mu.RLock()
	key := fmt.Sprintf("%s:%s", provider, modelName)
	pricing, exists := c.pricings[key]
	c.mu.RUnlock()

	if !exists {
		// Default fallback estimations if pricing not in DB
		return (float64(promptTokens)/1_000_000.0 * 1.0) + (float64(completionTokens)/1_000_000.0 * 3.0)
	}

	promptCost := (float64(promptTokens) / 1_000_000.0) * pricing.PromptPricePer1M
	completionCost := (float64(completionTokens) / 1_000_000.0) * pricing.CompletionPricePer1M

	return promptCost + completionCost
}

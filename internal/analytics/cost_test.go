package analytics

import (
	"math"
	"testing"

	"llm-gateway/internal/model"
)

func TestCalculateCost_KnownModel(t *testing.T) {
	calc := &CostCalculator{
		pricings: map[string]model.ModelPricing{
			"openai:gpt-4o": {
				Provider:            "openai",
				ModelName:           "gpt-4o",
				PromptPricePer1M:     2.50, // $2.50 / 1M
				CompletionPricePer1M: 10.00, // $10.00 / 1M
			},
		},
	}

	// 100,000 prompt tokens, 50,000 completion tokens
	// Prompt cost: 0.1 * 2.50 = $0.25
	// Completion cost: 0.05 * 10.00 = $0.50
	// Total = $0.75
	res := calc.CalculateCost("openai", "gpt-4o", 100_000, 50_000, 0, 0)
	if res.PricingFallback {
		t.Errorf("expected database pricing, got fallback")
	}
	if res.PricingSource != "database" {
		t.Errorf("expected source database, got %s", res.PricingSource)
	}
	if math.Abs(res.Cost-0.75) > 1e-6 {
		t.Errorf("expected cost 0.75, got %f", res.Cost)
	}
}

func TestCalculateCost_PromptCachingDiscount(t *testing.T) {
	customPrice := 0.50 // explicit $0.50 / 1M
	calc := &CostCalculator{
		pricings: map[string]model.ModelPricing{
			"anthropic:claude-3-5-sonnet": {
				Provider:            "anthropic",
				ModelName:           "claude-3-5-sonnet",
				PromptPricePer1M:     3.00,  // $3.00 / 1M
				CompletionPricePer1M: 15.00, // $15.00 / 1M
			},
			"openai:gpt-4o": {
				Provider:            "openai",
				ModelName:           "gpt-4o",
				PromptPricePer1M:     2.50, // $2.50 / 1M
				CompletionPricePer1M: 10.00,
			},
			"deepseek:deepseek-chat": {
				Provider:            "deepseek",
				ModelName:           "deepseek-chat",
				PromptPricePer1M:     1.00,
				CompletionPricePer1M: 2.00,
				CacheReadPricePer1M:  &customPrice,
			},
		},
	}

	// 1. Anthropic: 90% discount (10% rate)
	// 1,000,000 total prompt tokens, of which 800,000 are cached.
	// regular input = 200,000 -> 0.2 * $3.00 = $0.60
	// cached input = 800,000 -> 0.8 * ($3.00 * 0.10) = 0.8 * $0.30 = $0.24
	// completion = 100,000 -> 0.1 * $15.00 = $1.50
	// Total cost = 0.60 + 0.24 + 1.50 = $2.34
	resAnthropic := calc.CalculateCost("anthropic", "claude-3-5-sonnet", 1_000_000, 100_000, 800_000, 0)
	expectedAnthropic := 0.60 + 0.24 + 1.50
	if math.Abs(resAnthropic.Cost-expectedAnthropic) > 1e-6 {
		t.Errorf("expected anthropic cost %f, got %f", expectedAnthropic, resAnthropic.Cost)
	}

	// 2. OpenAI: 50% default discount
	// 1,000,000 prompt, 500,000 cached.
	// regular input = 500,000 -> 0.5 * 2.50 = 1.25
	// cached input = 500,000 -> 0.5 * (2.50 * 0.50) = 0.625
	// completion = 0
	// Total = 1.875
	resOpenAI := calc.CalculateCost("openai", "gpt-4o", 1_000_000, 0, 500_000, 0)
	expectedOpenAI := 1.25 + 0.625
	if math.Abs(resOpenAI.Cost-expectedOpenAI) > 1e-6 {
		t.Errorf("expected openai cost %f, got %f", expectedOpenAI, resOpenAI.Cost)
	}

	// 3. Custom CacheReadPricePer1M takes precedence
	// 1,000,000 prompt, 400,000 cached with custom cache price = $0.50 / 1M
	// regular = 600,000 * $1.00 = 0.60
	// cached = 400,000 * $0.50 = 0.20
	// Total = 0.80
	resCustom := calc.CalculateCost("deepseek", "deepseek-chat", 1_000_000, 0, 400_000, 0)
	expectedCustom := 0.60 + 0.20
	if math.Abs(resCustom.Cost-expectedCustom) > 1e-6 {
		t.Errorf("expected custom cache cost %f, got %f", expectedCustom, resCustom.Cost)
	}
}

func TestCalculateCost_FallbackPricing(t *testing.T) {
	calc := &CostCalculator{
		pricings: make(map[string]model.ModelPricing),
	}

	// Unknown model should use fallback: prompt=$1.00/1M, completion=$3.00/1M
	res := calc.CalculateCost("custom", "unknown-model", 1_000_000, 1_000_000, 0, 0)
	if !res.PricingFallback {
		t.Errorf("expected pricing fallback to be true")
	}
	if res.PricingSource != "fallback" {
		t.Errorf("expected pricing source to be fallback, got %s", res.PricingSource)
	}
	// 1M * 1.00 + 1M * 3.00 = 4.00
	if math.Abs(res.Cost-4.00) > 1e-6 {
		t.Errorf("expected cost 4.00, got %f", res.Cost)
	}
}

func TestCalculateCost_ReasoningTokens(t *testing.T) {
	calc := &CostCalculator{
		pricings: map[string]model.ModelPricing{
			"deepseek:deepseek-r1": {
				Provider:            "deepseek",
				ModelName:           "deepseek-r1",
				PromptPricePer1M:     0.55,
				CompletionPricePer1M: 2.19,
			},
		},
	}

	// In DeepSeek and OpenAI o1, total completion includes reasoning tokens
	res := calc.CalculateCost("deepseek", "deepseek-r1", 10_000, 5_000, 0, 2_000)
	// Prompt: 0.01 * 0.55 = 0.0055
	// Completion (5,000 tokens): 0.005 * 2.19 = 0.01095
	expected := 0.0055 + 0.01095
	if math.Abs(res.Cost-expected) > 1e-6 {
		t.Errorf("expected cost %f, got %f", expected, res.Cost)
	}
}

func TestEstimateMaxCost(t *testing.T) {
	pricing := model.ModelPricing{
		Provider:             "openai",
		ModelName:            "gpt-4o",
		PromptPricePer1M:     2.50,
		CompletionPricePer1M: 10.00,
	}

	// 1,000 prompt tokens, 2,000 max output tokens
	// Prompt: (1,000 / 1M) * 2.50 = 0.0025
	// Output: (2,000 / 1M) * 10.00 = 0.02
	// Total: 0.0225
	est := EstimateMaxCost(pricing, 1_000, 2_000)
	expected := 0.0025 + 0.02
	if math.Abs(est-expected) > 1e-6 {
		t.Errorf("expected max cost %f, got %f", expected, est)
	}

	// Default tokens when zero
	estZero := EstimateMaxCost(pricing, 0, 0)
	// Default prompt=500 -> 0.00125, Default output=2048 -> 0.02048 -> 0.02173
	if estZero <= 0 {
		t.Errorf("expected positive cost estimate with default tokens, got %f", estZero)
	}
}

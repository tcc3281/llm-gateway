package builtin

import (
	"math/rand"
	"strings"
	"time"

	"llm-gateway/internal/plugin"
)

// CanaryTrafficSplitterPlugin splits traffic dynamically between models/providers for A/B testing.
type CanaryTrafficSplitterPlugin struct {
	rng *rand.Rand
}

func NewCanaryTrafficSplitterPlugin() *CanaryTrafficSplitterPlugin {
	return &CanaryTrafficSplitterPlugin{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (p *CanaryTrafficSplitterPlugin) Name() string {
	return "canary-traffic-splitter"
}

func (p *CanaryTrafficSplitterPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreRouting}
}

func (p *CanaryTrafficSplitterPlugin) ExecPreRouting(pctx *plugin.PluginContext, cfg map[string]any) error {
	if pctx.CanReq == nil {
		return nil
	}

	canaryModel := strings.TrimSpace(getStringConfig(cfg, "canary_model", ""))
	if canaryModel == "" {
		return nil
	}

	canaryPercentage := getIntConfig(cfg, "canary_percentage", 10)
	if canaryPercentage <= 0 {
		return nil
	}
	if canaryPercentage > 100 {
		canaryPercentage = 100
	}

	// Roll 0..99
	roll := p.rng.Intn(100)
	if roll < canaryPercentage {
		// Traffic routed to canary
		pctx.CanReq.Model = canaryModel
		pctx.Model = canaryModel

		canaryProvider := strings.TrimSpace(getStringConfig(cfg, "canary_provider", ""))
		if canaryProvider != "" {
			pctx.Provider = canaryProvider
		}

		if pctx.ExtraHeaders == nil {
			pctx.ExtraHeaders = make(map[string]string)
		}
		pctx.ExtraHeaders["X-Canary-Experiment"] = "active"
		pctx.ExtraHeaders["X-Canary-Target-Model"] = canaryModel
	}

	return nil
}

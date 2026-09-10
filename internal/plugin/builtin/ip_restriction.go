package builtin

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"llm-gateway/internal/plugin"
)

// IPRestrictionPlugin enforces IP whitelisting and blacklisting at pre_auth stage.
type IPRestrictionPlugin struct{}

func NewIPRestrictionPlugin() *IPRestrictionPlugin {
	return &IPRestrictionPlugin{}
}

func (p *IPRestrictionPlugin) Name() string {
	return "ip-restriction"
}

func (p *IPRestrictionPlugin) Hooks() []plugin.HookType {
	return []plugin.HookType{plugin.HookPreAuth}
}

func (p *IPRestrictionPlugin) ExecPreAuth(pctx *plugin.PluginContext, cfg map[string]any) error {
	clientIP := strings.TrimSpace(pctx.ClientIP)
	if clientIP == "" {
		return nil
	}

	parsedIP := net.ParseIP(clientIP)
	if parsedIP == nil {
		return nil
	}

	denylist := getStringSliceConfig(cfg, "denylist")
	allowlist := getStringSliceConfig(cfg, "allowlist")

	// 1. Check Denylist first
	for _, entry := range denylist {
		if matchIPOrCIDR(parsedIP, entry) {
			p.blockRequest(pctx, fmt.Sprintf("Access denied: IP %s is blocked by security denylist policy", clientIP))
			return nil
		}
	}

	// 2. Check Allowlist if configured
	if len(allowlist) > 0 {
		allowed := false
		for _, entry := range allowlist {
			if matchIPOrCIDR(parsedIP, entry) {
				allowed = true
				break
			}
		}
		if !allowed {
			p.blockRequest(pctx, fmt.Sprintf("Access denied: IP %s is not in the authorized allowlist", clientIP))
			return nil
		}
	}

	return nil
}

func matchIPOrCIDR(ip net.IP, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}

	if strings.Contains(pattern, "/") {
		_, ipNet, err := net.ParseCIDR(pattern)
		if err == nil && ipNet.Contains(ip) {
			return true
		}
	} else {
		targetIP := net.ParseIP(pattern)
		if targetIP != nil && targetIP.Equal(ip) {
			return true
		}
	}
	return false
}

func (p *IPRestrictionPlugin) blockRequest(pctx *plugin.PluginContext, message string) {
	pctx.Stop = true
	pctx.StopStatusCode = http.StatusForbidden
	errBody := map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "permission_denied",
			"code":    "ip_forbidden",
		},
	}
	pctx.StopErrorBody, _ = json.Marshal(errBody)
}

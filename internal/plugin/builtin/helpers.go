package builtin

// getBoolConfig safely retrieves a boolean configuration value with a default fallback.
func getBoolConfig(cfg map[string]any, key string, defaultVal bool) bool {
	if cfg == nil {
		return defaultVal
	}
	v, exists := cfg[key]
	if !exists {
		return defaultVal
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return defaultVal
}

// getStringConfig safely retrieves a string configuration value with a default fallback.
func getStringConfig(cfg map[string]any, key string, defaultVal string) string {
	if cfg == nil {
		return defaultVal
	}
	v, exists := cfg[key]
	if !exists {
		return defaultVal
	}
	if s, ok := v.(string); ok {
		return s
	}
	return defaultVal
}

// getIntConfig safely retrieves an integer configuration value from int or float64.
func getIntConfig(cfg map[string]any, key string, defaultVal int) int {
	if cfg == nil {
		return defaultVal
	}
	v, exists := cfg[key]
	if !exists {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return defaultVal
	}
}

// getFloatConfig safely retrieves a float64 configuration value from float64 or int.
func getFloatConfig(cfg map[string]any, key string, defaultVal float64) float64 {
	if cfg == nil {
		return defaultVal
	}
	v, exists := cfg[key]
	if !exists {
		return defaultVal
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return defaultVal
	}
}

// getStringSliceConfig safely retrieves a string slice configuration value.
func getStringSliceConfig(cfg map[string]any, key string) []string {
	if cfg == nil {
		return nil
	}
	v, exists := cfg[key]
	if !exists {
		return nil
	}
	switch items := v.(type) {
	case []string:
		return items
	case []any:
		var result []string
		for _, item := range items {
			if s, ok := item.(string); ok && s != "" {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port         string
	AdminPort    string
	Env          string
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	DBSSLMode    string
	RedisHost         string
	RedisPass         string
	AdminAPIKey       string
	ProviderMasterKey string // encrypts provider keys at rest; empty = plaintext (dev only)
	AdminAllowedIPs      string // comma-separated allowed IPs or CIDR blocks
	ResponseCacheTTL     int    // CACHE_RESPONSE_TTL_SECONDS, default 3600
	OTelExporter         string // OTEL_EXPORTER: "otlp" | "stdout" | "none"
	OTelEndpoint         string // OTEL_EXPORTER_OTLP_ENDPOINT: default "localhost:4317"
	ContentCapturePolicy string // OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT: "none" | "event_only"
	SLOMaxTTFTMs         int    // SLO_MAX_TTFT_MS: default 500ms
	SLOMaxTBTMs          int    // SLO_MAX_TBT_MS: default 250ms
	AgentMaxToolCalls    int    // AGENT_MAX_TOOL_CALLS: default 10
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:                 getEnv("PORT", "8080"),
		AdminPort:            getEnv("ADMIN_PORT", "8081"),
		Env:                  getEnv("ENV", "development"),
		DBHost:               getEnv("DB_HOST", "localhost"),
		DBPort:               getEnv("DB_PORT", "5432"),
		DBUser:               getEnv("DB_USER", "llm_user"),
		DBPassword:           getEnv("DB_PASSWORD", "llm_password"),
		DBName:               getEnv("DB_NAME", "llm_gateway"),
		DBSSLMode:            getEnv("DB_SSLMODE", "disable"),
		RedisHost:            getEnv("REDIS_HOST", "localhost:6379"),
		RedisPass:            getEnv("REDIS_PASSWORD", ""),
		AdminAPIKey:          getEnv("ADMIN_API_KEY", "sk-admin-secret-key-change-me"),
		ProviderMasterKey:    getEnv("PROVIDER_MASTER_KEY", ""),
		AdminAllowedIPs:      getEnv("ADMIN_ALLOWED_IPS", ""),
		ResponseCacheTTL:     getEnvAsInt("CACHE_RESPONSE_TTL_SECONDS", 3600),
		OTelExporter:         getEnv("OTEL_EXPORTER", "otlp"),
		OTelEndpoint:         getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		ContentCapturePolicy: getEnv("OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT", "none"),
		SLOMaxTTFTMs:         getEnvAsInt("SLO_MAX_TTFT_MS", 500),
		SLOMaxTBTMs:          getEnvAsInt("SLO_MAX_TBT_MS", 250),
		AgentMaxToolCalls:    getEnvAsInt("AGENT_MAX_TOOL_CALLS", 10),
	}

	return cfg, nil
}

func (c *Config) GetDSN() string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.DBSSLMode)
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	str := getEnv(key, "")
	if val, err := strconv.Atoi(str); err == nil {
		return val
	}
	return fallback
}

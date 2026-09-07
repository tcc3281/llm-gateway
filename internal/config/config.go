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
	AdminAllowedIPs   string // comma-separated allowed IPs or CIDR blocks
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		Port:              getEnv("PORT", "8080"),
		AdminPort:         getEnv("ADMIN_PORT", "8081"),
		Env:               getEnv("ENV", "development"),
		DBHost:            getEnv("DB_HOST", "localhost"),
		DBPort:            getEnv("DB_PORT", "5432"),
		DBUser:            getEnv("DB_USER", "llm_user"),
		DBPassword:        getEnv("DB_PASSWORD", "llm_password"),
		DBName:            getEnv("DB_NAME", "llm_gateway"),
		DBSSLMode:         getEnv("DB_SSLMODE", "disable"),
		RedisHost:         getEnv("REDIS_HOST", "localhost:6379"),
		RedisPass:         getEnv("REDIS_PASSWORD", ""),
		AdminAPIKey:       getEnv("ADMIN_API_KEY", "sk-admin-secret-key-change-me"),
		ProviderMasterKey: getEnv("PROVIDER_MASTER_KEY", ""),
		AdminAllowedIPs:   getEnv("ADMIN_ALLOWED_IPS", ""),
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

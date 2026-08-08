package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/config"
)

func validValues() map[string]string {
	return map[string]string{
		"APP_HTTP_ADDR":      ":8080",
		"POSTGRES_DSN":       "postgres://exchange:secret@postgres:5432/exchange?sslmode=disable",
		"REDIS_ADDR":         "redis:6379",
		"NATS_URL":           "nats://nats:4222",
		"DEPENDENCY_TIMEOUT": "1s",
		"SHUTDOWN_TIMEOUT":   "10s",
	}
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func TestLoadReturnsValidatedConfig(t *testing.T) {
	cfg, err := config.Load(lookup(validValues()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.RedisAddr != "redis:6379" || cfg.NATSURL != "nats://nats:4222" {
		t.Fatalf("Load() addresses = %#v", cfg)
	}
	if cfg.DependencyTimeout != time.Second || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("Load() timeouts = %s, %s", cfg.DependencyTimeout, cfg.ShutdownTimeout)
	}
}

func TestLoadRejectsMissingValueWithoutLeakingSecrets(t *testing.T) {
	values := validValues()
	delete(values, "REDIS_ADDR")
	_, err := config.Load(lookup(values))
	if err == nil || !strings.Contains(err.Error(), "REDIS_ADDR") {
		t.Fatalf("Load() error = %v", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("Load() leaked secret: %v", err)
	}
}

func TestLoadRejectsInvalidURLsAndTimeouts(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "postgres scheme", key: "POSTGRES_DSN", value: "mysql://db"},
		{name: "nats scheme", key: "NATS_URL", value: "http://nats:4222"},
		{name: "zero dependency timeout", key: "DEPENDENCY_TIMEOUT", value: "0s"},
		{name: "negative shutdown timeout", key: "SHUTDOWN_TIMEOUT", value: "-1s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := validValues()
			values[tt.key] = tt.value
			if _, err := config.Load(lookup(values)); err == nil {
				t.Fatalf("Load() error = nil")
			}
		})
	}
}

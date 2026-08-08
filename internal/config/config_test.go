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

func TestLoadPreservesAcceptedPostgresAndRedisValues(t *testing.T) {
	values := validValues()
	const postgresDSN = "postgresql://week_one_user:p%40ss%3Aword@postgres/week_one_db?sslmode=disable"
	const redisAddr = "[::1]:65535"
	values["POSTGRES_DSN"] = postgresDSN
	values["REDIS_ADDR"] = redisAddr

	cfg, err := config.Load(lookup(values))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.PostgresDSN != postgresDSN {
		t.Fatal("Load() changed the accepted POSTGRES_DSN")
	}
	if cfg.RedisAddr != redisAddr {
		t.Fatal("Load() changed the accepted REDIS_ADDR")
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

func TestLoadDoesNotLeakInvalidAddressValues(t *testing.T) {
	const secret = "secret-address-value"

	tests := []struct {
		name string
		key  string
	}{
		{name: "http address", key: "APP_HTTP_ADDR"},
		{name: "redis address", key: "REDIS_ADDR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := validValues()
			values[tt.key] = secret

			_, err := config.Load(lookup(values))
			if err == nil {
				t.Fatal("Load() error = nil")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("Load() leaked address value: %v", err)
			}
		})
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

func TestLoadRejectsStructurallyInvalidPostgresDSNs(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		{name: "scheme only", dsn: "postgres://"},
		{name: "opaque URL", dsn: "postgres:week_one_db"},
		{name: "empty hostname", dsn: "postgres://:5432/week_one_db"},
		{name: "empty database", dsn: "postgres://postgres:5432/"},
		{name: "non-numeric port", dsn: "postgres://postgres:not-a-port/week_one_db"},
		{name: "zero port", dsn: "postgres://postgres:0/week_one_db"},
		{name: "out of range port", dsn: "postgres://postgres:65536/week_one_db"},
		{name: "pgx incompatible option", dsn: "postgres://postgres:5432/week_one_db?connect_timeout=not-a-number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := validValues()
			values["POSTGRES_DSN"] = tt.dsn
			if _, err := config.Load(lookup(values)); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func TestLoadRejectsInvalidRedisAddresses(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{name: "empty hostname", address: ":6379"},
		{name: "zero port", address: "redis:0"},
		{name: "out of range port", address: "redis:65536"},
		{name: "non-numeric port", address: "redis:not-a-port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := validValues()
			values["REDIS_ADDR"] = tt.address
			if _, err := config.Load(lookup(values)); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func TestLoadDoesNotLeakInvalidPostgresOrRedisValues(t *testing.T) {
	const secret = "dependency-config-secret"
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "postgres", key: "POSTGRES_DSN", value: "postgres://user:" + secret + "@:5432/week_one_db"},
		{name: "redis", key: "REDIS_ADDR", value: secret + ":0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := validValues()
			values[tt.key] = tt.value
			_, err := config.Load(lookup(values))
			if err == nil {
				t.Fatal("Load() error = nil")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("Load() leaked invalid dependency value: %v", err)
			}
		})
	}
}

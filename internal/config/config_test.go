package config_test

import (
	"strings"
	"testing"
	"time"

	"exchange-lab/internal/config"
)

func validValues() map[string]string {
	return map[string]string{
		"APP_HTTP_ADDR":        ":8080",
		"POSTGRES_DSN":         "postgres://exchange:secret@postgres:5432/exchange?sslmode=disable",
		"POSTGRES_MIN_CONNS":   "1",
		"POSTGRES_MAX_CONNS":   "10",
		"DATABASE_TIMEOUT":     "2s",
		"REDIS_ADDR":           "redis:6379",
		"CACHE_TIMEOUT":        "200ms",
		"INSTRUMENT_CACHE_TTL": "5m",
		"TICKER_CACHE_TTL":     "5s",
		"NATS_URL":             "nats://nats:4222",
		"DEPENDENCY_TIMEOUT":   "1s",
		"REQUEST_TIMEOUT":      "2s",
		"SHUTDOWN_TIMEOUT":     "10s",
		"AUTH_SESSION_TTL":     "24h",
		"AUTH_COOKIE_SECURE":   "false",
		"LOGIN_RATE_LIMIT":     "5",
		"LOGIN_RATE_WINDOW":    "10m",
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
	if cfg.DependencyTimeout != time.Second || cfg.DatabaseTimeout != 2*time.Second || cfg.RequestTimeout != 2*time.Second || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("Load() timeouts = dependency %s, database %s, request %s, shutdown %s", cfg.DependencyTimeout, cfg.DatabaseTimeout, cfg.RequestTimeout, cfg.ShutdownTimeout)
	}
	if cfg.PostgresMinConns != 1 || cfg.PostgresMaxConns != 10 {
		t.Fatalf("Load() PostgreSQL limits = min %d max %d", cfg.PostgresMinConns, cfg.PostgresMaxConns)
	}
	if cfg.CacheTimeout != 200*time.Millisecond || cfg.InstrumentCacheTTL != 5*time.Minute || cfg.TickerCacheTTL != 5*time.Second {
		t.Fatalf("Load() cache config = timeout %s instrument %s ticker %s", cfg.CacheTimeout, cfg.InstrumentCacheTTL, cfg.TickerCacheTTL)
	}
	if cfg.AuthSessionTTL != 24*time.Hour || cfg.AuthCookieSecure || cfg.LoginRateLimit != 5 || cfg.LoginRateWindow != 10*time.Minute {
		t.Fatalf("Load() auth config = %#v", cfg)
	}
}

func TestLoadDatabaseReturnsOnlyValidatedDatabaseSettings(t *testing.T) {
	values := validValues()
	database, err := config.LoadDatabase(lookup(values))
	if err != nil {
		t.Fatalf("LoadDatabase() error = %v", err)
	}
	if database.PostgresDSN != values["POSTGRES_DSN"] || database.MinConns != 1 || database.MaxConns != 10 || database.Timeout != 2*time.Second {
		t.Fatalf("LoadDatabase() = %#v", database)
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
		{name: "zero database timeout", key: "DATABASE_TIMEOUT", value: "0s"},
		{name: "zero max connections", key: "POSTGRES_MAX_CONNS", value: "0"},
		{name: "negative min connections", key: "POSTGRES_MIN_CONNS", value: "-1"},
		{name: "min exceeds max", key: "POSTGRES_MIN_CONNS", value: "11"},
		{name: "invalid request timeout", key: "REQUEST_TIMEOUT", value: "later"},
		{name: "zero cache timeout", key: "CACHE_TIMEOUT", value: "0s"},
		{name: "negative instrument cache TTL", key: "INSTRUMENT_CACHE_TTL", value: "-1s"},
		{name: "invalid ticker cache TTL", key: "TICKER_CACHE_TTL", value: "later"},
		{name: "negative shutdown timeout", key: "SHUTDOWN_TIMEOUT", value: "-1s"},
		{name: "short auth session", key: "AUTH_SESSION_TTL", value: "30s"},
		{name: "invalid secure cookie", key: "AUTH_COOKIE_SECURE", value: "sometimes"},
		{name: "zero login limit", key: "LOGIN_RATE_LIMIT", value: "0"},
		{name: "short login window", key: "LOGIN_RATE_WINDOW", value: "500ms"},
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

func TestLoadPreservesAcceptedNATSURLs(t *testing.T) {
	tests := []struct {
		name    string
		natsURL string
	}{
		{name: "default port omitted", natsURL: "nats://nats"},
		{name: "default port explicit", natsURL: "nats://nats:4222"},
		{name: "minimum port", natsURL: "nats://nats:1"},
		{name: "maximum port", natsURL: "nats://nats:65535"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := validValues()
			values["NATS_URL"] = tt.natsURL
			cfg, err := config.Load(lookup(values))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.NATSURL != tt.natsURL {
				t.Fatal("Load() changed the accepted NATS_URL")
			}
		})
	}
}

func TestLoadRejectsStructurallyInvalidNATSURLs(t *testing.T) {
	tests := []struct {
		name    string
		natsURL string
	}{
		{name: "scheme only", natsURL: "nats://"},
		{name: "opaque URL", natsURL: "nats:server"},
		{name: "empty hostname", natsURL: "nats://:4222"},
		{name: "empty port", natsURL: "nats://nats:"},
		{name: "zero port", natsURL: "nats://nats:0"},
		{name: "out of range port", natsURL: "nats://nats:65536"},
		{name: "non-numeric port", natsURL: "nats://nats:not-a-port"},
		{name: "malformed port", natsURL: "nats://nats:12:34"},
		{name: "malformed IPv6 host", natsURL: "nats://[::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := validValues()
			values["NATS_URL"] = tt.natsURL
			if _, err := config.Load(lookup(values)); err == nil {
				t.Fatal("Load() error = nil")
			}
		})
	}
}

func TestLoadDoesNotLeakInvalidNATSURL(t *testing.T) {
	const secret = "nats-url-secret"
	values := validValues()
	values["NATS_URL"] = "nats://probe:" + secret + "@:0"

	_, err := config.Load(lookup(values))
	if err == nil {
		t.Fatal("Load() error = nil")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), values["NATS_URL"]) {
		t.Fatalf("Load() leaked invalid NATS URL: %v", err)
	}
}

func TestLoadMarketWorkerReturnsBoundedRuntimeConfig(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"MARKET_WS_URL":             "ws://market-simulator:8090/ws",
		"MARKET_MAX_MESSAGE_BYTES":  "1048576",
		"MARKET_MAX_RECONNECTS":     "5",
		"MARKET_INITIAL_BACKOFF":    "250ms",
		"MARKET_MAX_BACKOFF":        "5s",
		"MARKET_HEARTBEAT_INTERVAL": "10s",
		"MARKET_OPERATION_TIMEOUT":  "2s",
		"REDIS_ADDR":                "redis:6379",
		"CACHE_TIMEOUT":             "200ms",
		"INSTRUMENT_CACHE_TTL":      "5m",
		"TICKER_CACHE_TTL":          "5s",
		"NATS_URL":                  "nats://nats:4222",
	}
	cfg, err := config.LoadMarketWorker(lookup(values))
	if err != nil {
		t.Fatalf("LoadMarketWorker() error = %v", err)
	}
	if cfg.WebSocketURL != values["MARKET_WS_URL"] || cfg.MaxMessageBytes != 1<<20 || cfg.MaxReconnects != 5 || cfg.InitialBackoff != 250*time.Millisecond || cfg.MaxBackoff != 5*time.Second {
		t.Fatalf("LoadMarketWorker() = %#v", cfg)
	}
}

func TestLoadMarketWorkerRejectsUnboundedOrSensitiveConfig(t *testing.T) {
	t.Parallel()

	base := map[string]string{
		"MARKET_WS_URL": "ws://market-simulator:8090/ws", "MARKET_MAX_MESSAGE_BYTES": "1048576",
		"MARKET_MAX_RECONNECTS": "5", "MARKET_INITIAL_BACKOFF": "250ms", "MARKET_MAX_BACKOFF": "5s",
		"MARKET_HEARTBEAT_INTERVAL": "10s", "MARKET_OPERATION_TIMEOUT": "2s", "REDIS_ADDR": "redis:6379",
		"CACHE_TIMEOUT": "200ms", "INSTRUMENT_CACHE_TTL": "5m", "TICKER_CACHE_TTL": "5s", "NATS_URL": "nats://nats:4222",
	}
	tests := map[string][2]string{
		"http websocket":        {"MARKET_WS_URL", "http://simulator/ws"},
		"websocket credentials": {"MARKET_WS_URL", "ws://user:secret@simulator/ws"},
		"oversized message":     {"MARKET_MAX_MESSAGE_BYTES", "16777217"},
		"unbounded reconnects":  {"MARKET_MAX_RECONNECTS", "0"},
		"too many reconnects":   {"MARKET_MAX_RECONNECTS", "21"},
		"backoff inverted":      {"MARKET_INITIAL_BACKOFF", "10s"},
	}
	for name, mutation := range tests {
		name, mutation := name, mutation
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			values := make(map[string]string, len(base))
			for key, value := range base {
				values[key] = value
			}
			values[mutation[0]] = mutation[1]
			_, err := config.LoadMarketWorker(lookup(values))
			if err == nil {
				t.Fatal("LoadMarketWorker() error = nil")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("LoadMarketWorker() leaked value: %v", err)
			}
		})
	}
}

func TestLoadMarketSimulatorReturnsValidatedConfig(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"SIMULATOR_HTTP_ADDR": ":8090",
		"SIMULATOR_INTERVAL":  "250ms",
		"SHUTDOWN_TIMEOUT":    "10s",
	}
	cfg, err := config.LoadMarketSimulator(lookup(values))
	if err != nil {
		t.Fatalf("LoadMarketSimulator() error = %v", err)
	}
	if cfg.HTTPAddr != ":8090" || cfg.Interval != 250*time.Millisecond || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("LoadMarketSimulator() = %#v", cfg)
	}
	values["SIMULATOR_INTERVAL"] = "0s"
	if _, err := config.LoadMarketSimulator(lookup(values)); err == nil {
		t.Fatal("LoadMarketSimulator(zero interval) error = nil")
	}
}

func TestLoadCandleWorkerReturnsValidatedBoundedConfig(t *testing.T) {
	t.Parallel()

	values := validValues()
	values["CANDLE_HTTP_ADDR"] = ":8091"
	values["CANDLE_PRICE_SCALE"] = "2"
	values["CANDLE_QUANTITY_SCALE"] = "3"
	values["CANDLE_OPERATION_TIMEOUT"] = "2s"
	cfg, err := config.LoadCandleWorker(lookup(values))
	if err != nil {
		t.Fatalf("LoadCandleWorker() error = %v", err)
	}
	if cfg.HTTPAddr != ":8091" || cfg.PriceScale != 2 || cfg.QuantityScale != 3 || cfg.OperationTimeout != 2*time.Second || cfg.Database.MaxConns != 10 {
		t.Fatalf("LoadCandleWorker() = %#v", cfg)
	}
	values["CANDLE_PRICE_SCALE"] = "19"
	if _, err := config.LoadCandleWorker(lookup(values)); err == nil {
		t.Fatal("LoadCandleWorker(invalid scale) error = nil")
	}
}

// Package config validates environment configuration for every executable.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config contains validated API and shared infrastructure settings.
type Config struct {
	HTTPAddr           string
	PostgresDSN        string
	PostgresMinConns   int32
	PostgresMaxConns   int32
	DatabaseTimeout    time.Duration
	RedisAddr          string
	CacheTimeout       time.Duration
	InstrumentCacheTTL time.Duration
	TickerCacheTTL     time.Duration
	NATSURL            string
	DependencyTimeout  time.Duration
	RequestTimeout     time.Duration
	ShutdownTimeout    time.Duration
	AuthSessionTTL     time.Duration
	AuthCookieSecure   bool
	LoginRateLimit     uint64
	LoginRateWindow    time.Duration
}

// DatabaseConfig is the configuration surface required by one-shot database
// jobs such as migrations, backup checks and reconciliation.
type DatabaseConfig struct {
	PostgresDSN string
	MinConns    int32
	MaxConns    int32
	Timeout     time.Duration
}

// MarketWorkerConfig bounds feed reconnects, heartbeats and downstream writes.
type MarketWorkerConfig struct {
	WebSocketURL       string
	MaxMessageBytes    int64
	MaxReconnects      uint
	InitialBackoff     time.Duration
	MaxBackoff         time.Duration
	HeartbeatInterval  time.Duration
	OperationTimeout   time.Duration
	RedisAddr          string
	CacheTimeout       time.Duration
	InstrumentCacheTTL time.Duration
	TickerCacheTTL     time.Duration
	NATSURL            string
}

// MarketSimulatorConfig defines the local deterministic feed endpoint and cadence.
type MarketSimulatorConfig struct {
	HTTPAddr        string
	Interval        time.Duration
	ShutdownTimeout time.Duration
}

// CandleWorkerConfig contains candle scales, persistence and publication settings.
type CandleWorkerConfig struct {
	HTTPAddr         string
	Database         DatabaseConfig
	NATSURL          string
	PriceScale       uint8
	QuantityScale    uint8
	OperationTimeout time.Duration
	ShutdownTimeout  time.Duration
}

// LoadOS validates API configuration from the current process environment.
func LoadOS() (Config, error) {
	return Load(os.LookupEnv)
}

// LoadMarketWorker validates market worker settings from a lookup function.
func LoadMarketWorker(lookup func(string) (string, bool)) (MarketWorkerConfig, error) {
	read := requiredReader(lookup)
	webSocketURL, err := read("MARKET_WS_URL")
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	parsedWebSocket, err := url.Parse(webSocketURL)
	if err != nil || (parsedWebSocket.Scheme != "ws" && parsedWebSocket.Scheme != "wss") || parsedWebSocket.Hostname() == "" || parsedWebSocket.User != nil {
		return MarketWorkerConfig{}, fmt.Errorf("MARKET_WS_URL is invalid")
	}
	maxMessageBytes, err := readBoundedUint(read, "MARKET_MAX_MESSAGE_BYTES", 1, 16<<20)
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	maxReconnects, err := readBoundedUint(read, "MARKET_MAX_RECONNECTS", 1, 20)
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	initialBackoff, err := readDuration(read, "MARKET_INITIAL_BACKOFF")
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	maxBackoff, err := readDuration(read, "MARKET_MAX_BACKOFF")
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	if initialBackoff > maxBackoff {
		return MarketWorkerConfig{}, fmt.Errorf("MARKET_INITIAL_BACKOFF must not exceed MARKET_MAX_BACKOFF")
	}
	heartbeatInterval, err := readDuration(read, "MARKET_HEARTBEAT_INTERVAL")
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	operationTimeout, err := readDuration(read, "MARKET_OPERATION_TIMEOUT")
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	redisAddr, err := read("REDIS_ADDR")
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	redisHost, redisPort, err := net.SplitHostPort(redisAddr)
	if err != nil || redisHost == "" || !validPort(redisPort) {
		return MarketWorkerConfig{}, fmt.Errorf("REDIS_ADDR is invalid")
	}
	cacheTimeout, err := readDuration(read, "CACHE_TIMEOUT")
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	instrumentCacheTTL, err := readDuration(read, "INSTRUMENT_CACHE_TTL")
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	tickerCacheTTL, err := readDuration(read, "TICKER_CACHE_TTL")
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	natsURL, err := read("NATS_URL")
	if err != nil {
		return MarketWorkerConfig{}, err
	}
	parsedNATS, err := url.Parse(natsURL)
	if err != nil || parsedNATS.Scheme != "nats" || parsedNATS.Opaque != "" || parsedNATS.Hostname() == "" || !validOptionalURLPort(parsedNATS) {
		return MarketWorkerConfig{}, fmt.Errorf("NATS_URL is invalid")
	}
	return MarketWorkerConfig{
		WebSocketURL: webSocketURL, MaxMessageBytes: int64(maxMessageBytes), MaxReconnects: uint(maxReconnects),
		InitialBackoff: initialBackoff, MaxBackoff: maxBackoff, HeartbeatInterval: heartbeatInterval, OperationTimeout: operationTimeout,
		RedisAddr: redisAddr, CacheTimeout: cacheTimeout, InstrumentCacheTTL: instrumentCacheTTL, TickerCacheTTL: tickerCacheTTL,
		NATSURL: natsURL,
	}, nil
}

// LoadMarketSimulator validates simulator settings from a lookup function.
func LoadMarketSimulator(lookup func(string) (string, bool)) (MarketSimulatorConfig, error) {
	read := requiredReader(lookup)
	httpAddr, err := read("SIMULATOR_HTTP_ADDR")
	if err != nil {
		return MarketSimulatorConfig{}, err
	}
	if _, err := net.ResolveTCPAddr("tcp", httpAddr); err != nil {
		return MarketSimulatorConfig{}, fmt.Errorf("SIMULATOR_HTTP_ADDR is invalid")
	}
	interval, err := readDuration(read, "SIMULATOR_INTERVAL")
	if err != nil {
		return MarketSimulatorConfig{}, err
	}
	shutdownTimeout, err := readDuration(read, "SHUTDOWN_TIMEOUT")
	if err != nil {
		return MarketSimulatorConfig{}, err
	}
	return MarketSimulatorConfig{HTTPAddr: httpAddr, Interval: interval, ShutdownTimeout: shutdownTimeout}, nil
}

// LoadCandleWorker validates candle worker settings from a lookup function.
func LoadCandleWorker(lookup func(string) (string, bool)) (CandleWorkerConfig, error) {
	read := requiredReader(lookup)
	httpAddr, err := read("CANDLE_HTTP_ADDR")
	if err != nil {
		return CandleWorkerConfig{}, err
	}
	if _, err := net.ResolveTCPAddr("tcp", httpAddr); err != nil {
		return CandleWorkerConfig{}, fmt.Errorf("CANDLE_HTTP_ADDR is invalid")
	}
	database, err := loadDatabase(read)
	if err != nil {
		return CandleWorkerConfig{}, err
	}
	natsURL, err := read("NATS_URL")
	if err != nil {
		return CandleWorkerConfig{}, err
	}
	parsedNATS, err := url.Parse(natsURL)
	if err != nil || parsedNATS.Scheme != "nats" || parsedNATS.Opaque != "" || parsedNATS.Hostname() == "" || !validOptionalURLPort(parsedNATS) {
		return CandleWorkerConfig{}, fmt.Errorf("NATS_URL is invalid")
	}
	priceScale, err := readBoundedUint(read, "CANDLE_PRICE_SCALE", 0, 18)
	if err != nil {
		return CandleWorkerConfig{}, err
	}
	quantityScale, err := readBoundedUint(read, "CANDLE_QUANTITY_SCALE", 0, 18)
	if err != nil {
		return CandleWorkerConfig{}, err
	}
	operationTimeout, err := readDuration(read, "CANDLE_OPERATION_TIMEOUT")
	if err != nil {
		return CandleWorkerConfig{}, err
	}
	shutdownTimeout, err := readDuration(read, "SHUTDOWN_TIMEOUT")
	if err != nil {
		return CandleWorkerConfig{}, err
	}
	return CandleWorkerConfig{
		HTTPAddr: httpAddr, Database: database, NATSURL: natsURL,
		PriceScale: uint8(priceScale), QuantityScale: uint8(quantityScale),
		OperationTimeout: operationTimeout, ShutdownTimeout: shutdownTimeout,
	}, nil
}

// Load validates API configuration from an injected environment lookup.
func Load(lookup func(string) (string, bool)) (Config, error) {
	read := func(key string) (string, error) {
		value, ok := lookup(key)
		if !ok || value == "" {
			return "", fmt.Errorf("required environment variable %s is missing", key)
		}
		return value, nil
	}

	httpAddr, err := read("APP_HTTP_ADDR")
	if err != nil {
		return Config{}, err
	}
	if _, err := net.ResolveTCPAddr("tcp", httpAddr); err != nil {
		return Config{}, fmt.Errorf("APP_HTTP_ADDR is invalid")
	}

	database, err := loadDatabase(read)
	if err != nil {
		return Config{}, err
	}

	redisAddr, err := read("REDIS_ADDR")
	if err != nil {
		return Config{}, err
	}
	redisHost, redisPort, err := net.SplitHostPort(redisAddr)
	if err != nil || redisHost == "" || !validPort(redisPort) {
		return Config{}, fmt.Errorf("REDIS_ADDR is invalid")
	}
	cacheTimeout, err := readDuration(read, "CACHE_TIMEOUT")
	if err != nil {
		return Config{}, err
	}
	instrumentCacheTTL, err := readDuration(read, "INSTRUMENT_CACHE_TTL")
	if err != nil {
		return Config{}, err
	}
	tickerCacheTTL, err := readDuration(read, "TICKER_CACHE_TTL")
	if err != nil {
		return Config{}, err
	}

	natsURL, err := read("NATS_URL")
	if err != nil {
		return Config{}, err
	}
	parsedNATS, err := url.Parse(natsURL)
	if err != nil || parsedNATS.Scheme != "nats" {
		return Config{}, fmt.Errorf("NATS_URL must use nats scheme")
	}
	if parsedNATS.Opaque != "" || parsedNATS.Hostname() == "" || !validOptionalURLPort(parsedNATS) {
		return Config{}, fmt.Errorf("NATS_URL is invalid")
	}

	dependencyTimeout, err := readDuration(read, "DEPENDENCY_TIMEOUT")
	if err != nil {
		return Config{}, err
	}
	requestTimeout, err := readDuration(read, "REQUEST_TIMEOUT")
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := readDuration(read, "SHUTDOWN_TIMEOUT")
	if err != nil {
		return Config{}, err
	}
	authSessionTTL, err := readDuration(read, "AUTH_SESSION_TTL")
	if err != nil || authSessionTTL < time.Minute || authSessionTTL > 30*24*time.Hour {
		return Config{}, fmt.Errorf("AUTH_SESSION_TTL is invalid")
	}
	authCookieSecureValue, err := read("AUTH_COOKIE_SECURE")
	if err != nil {
		return Config{}, err
	}
	authCookieSecure, err := strconv.ParseBool(authCookieSecureValue)
	if err != nil {
		return Config{}, fmt.Errorf("AUTH_COOKIE_SECURE is invalid")
	}
	loginRateLimit, err := readBoundedUint(read, "LOGIN_RATE_LIMIT", 1, 100)
	if err != nil {
		return Config{}, err
	}
	loginRateWindow, err := readDuration(read, "LOGIN_RATE_WINDOW")
	if err != nil || loginRateWindow < time.Second || loginRateWindow > 24*time.Hour {
		return Config{}, fmt.Errorf("LOGIN_RATE_WINDOW is invalid")
	}

	return Config{
		HTTPAddr:           httpAddr,
		PostgresDSN:        database.PostgresDSN,
		PostgresMinConns:   database.MinConns,
		PostgresMaxConns:   database.MaxConns,
		DatabaseTimeout:    database.Timeout,
		RedisAddr:          redisAddr,
		CacheTimeout:       cacheTimeout,
		InstrumentCacheTTL: instrumentCacheTTL,
		TickerCacheTTL:     tickerCacheTTL,
		NATSURL:            natsURL,
		DependencyTimeout:  dependencyTimeout,
		RequestTimeout:     requestTimeout,
		ShutdownTimeout:    shutdownTimeout,
		AuthSessionTTL:     authSessionTTL,
		AuthCookieSecure:   authCookieSecure,
		LoginRateLimit:     loginRateLimit,
		LoginRateWindow:    loginRateWindow,
	}, nil
}

func requiredReader(lookup func(string) (string, bool)) func(string) (string, error) {
	return func(key string) (string, error) {
		value, ok := lookup(key)
		if !ok || value == "" {
			return "", fmt.Errorf("required environment variable %s is missing", key)
		}
		return value, nil
	}
}

func readBoundedUint(read func(string) (string, error), key string, minimum, maximum uint64) (uint64, error) {
	value, err := read(key)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", key, minimum, maximum)
	}
	return parsed, nil
}

// LoadDatabase validates the database-only environment used by one-shot jobs.
func LoadDatabase(lookup func(string) (string, bool)) (DatabaseConfig, error) {
	read := func(key string) (string, error) {
		value, ok := lookup(key)
		if !ok || value == "" {
			return "", fmt.Errorf("required environment variable %s is missing", key)
		}
		return value, nil
	}
	return loadDatabase(read)
}

func loadDatabase(read func(string) (string, error)) (DatabaseConfig, error) {
	postgresDSN, err := read("POSTGRES_DSN")
	if err != nil {
		return DatabaseConfig{}, err
	}
	if err := validatePostgresDSN(postgresDSN); err != nil {
		return DatabaseConfig{}, err
	}
	minConns, err := readConnectionLimit(read, "POSTGRES_MIN_CONNS", true)
	if err != nil {
		return DatabaseConfig{}, err
	}
	maxConns, err := readConnectionLimit(read, "POSTGRES_MAX_CONNS", false)
	if err != nil {
		return DatabaseConfig{}, err
	}
	if minConns > maxConns {
		return DatabaseConfig{}, fmt.Errorf("POSTGRES_MIN_CONNS must not exceed POSTGRES_MAX_CONNS")
	}
	timeout, err := readDuration(read, "DATABASE_TIMEOUT")
	if err != nil {
		return DatabaseConfig{}, err
	}
	return DatabaseConfig{
		PostgresDSN: postgresDSN,
		MinConns:    minConns,
		MaxConns:    maxConns,
		Timeout:     timeout,
	}, nil
}

func validatePostgresDSN(postgresDSN string) error {
	postgresURL, err := url.Parse(postgresDSN)
	if err != nil || (postgresURL.Scheme != "postgres" && postgresURL.Scheme != "postgresql") {
		return fmt.Errorf("POSTGRES_DSN must use postgres scheme")
	}
	if postgresURL.Opaque != "" || postgresURL.Hostname() == "" || strings.Trim(postgresURL.Path, "/") == "" {
		return fmt.Errorf("POSTGRES_DSN is invalid")
	}
	if !validOptionalURLPort(postgresURL) {
		return fmt.Errorf("POSTGRES_DSN is invalid")
	}
	if _, err := pgxpool.ParseConfig(postgresDSN); err != nil {
		return fmt.Errorf("POSTGRES_DSN is invalid")
	}
	return nil
}

func readConnectionLimit(read func(string) (string, error), key string, allowZero bool) (int32, error) {
	value, err := read(key)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 0 || (!allowZero && parsed == 0) {
		return 0, fmt.Errorf("%s must be a valid connection limit", key)
	}
	return int32(parsed), nil
}

func validOptionalURLPort(parsedURL *url.URL) bool {
	host := parsedURL.Host
	if strings.HasPrefix(host, "[") {
		closingBracket := strings.LastIndex(host, "]")
		if closingBracket == -1 {
			return false
		}
		suffix := host[closingBracket+1:]
		if suffix == "" {
			return true
		}
		return strings.HasPrefix(suffix, ":") && validPort(strings.TrimPrefix(suffix, ":"))
	}
	if !strings.Contains(host, ":") {
		return true
	}
	_, port, err := net.SplitHostPort(host)
	return err == nil && validPort(port)
}

func validPort(port string) bool {
	numericPort, err := strconv.Atoi(port)
	return err == nil && numericPort >= 1 && numericPort <= 65535
}

func readDuration(read func(string) (string, error), key string) (time.Duration, error) {
	value, err := read(key)
	if err != nil {
		return 0, err
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return duration, nil
}

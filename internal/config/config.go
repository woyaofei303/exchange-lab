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

type Config struct {
	HTTPAddr          string
	PostgresDSN       string
	RedisAddr         string
	NATSURL           string
	DependencyTimeout time.Duration
	ShutdownTimeout   time.Duration
}

func LoadOS() (Config, error) {
	return Load(os.LookupEnv)
}

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

	postgresDSN, err := read("POSTGRES_DSN")
	if err != nil {
		return Config{}, err
	}
	postgresURL, err := url.Parse(postgresDSN)
	if err != nil || (postgresURL.Scheme != "postgres" && postgresURL.Scheme != "postgresql") {
		return Config{}, fmt.Errorf("POSTGRES_DSN must use postgres scheme")
	}
	if postgresURL.Opaque != "" || postgresURL.Hostname() == "" || strings.Trim(postgresURL.Path, "/") == "" {
		return Config{}, fmt.Errorf("POSTGRES_DSN is invalid")
	}
	if !validOptionalURLPort(postgresURL) {
		return Config{}, fmt.Errorf("POSTGRES_DSN is invalid")
	}
	if _, err := pgxpool.ParseConfig(postgresDSN); err != nil {
		return Config{}, fmt.Errorf("POSTGRES_DSN is invalid")
	}

	redisAddr, err := read("REDIS_ADDR")
	if err != nil {
		return Config{}, err
	}
	redisHost, redisPort, err := net.SplitHostPort(redisAddr)
	if err != nil || redisHost == "" || !validPort(redisPort) {
		return Config{}, fmt.Errorf("REDIS_ADDR is invalid")
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
	shutdownTimeout, err := readDuration(read, "SHUTDOWN_TIMEOUT")
	if err != nil {
		return Config{}, err
	}

	return Config{
		HTTPAddr:          httpAddr,
		PostgresDSN:       postgresDSN,
		RedisAddr:         redisAddr,
		NATSURL:           natsURL,
		DependencyTimeout: dependencyTimeout,
		ShutdownTimeout:   shutdownTimeout,
	}, nil
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

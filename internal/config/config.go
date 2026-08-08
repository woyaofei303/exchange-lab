package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"time"
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

	redisAddr, err := read("REDIS_ADDR")
	if err != nil {
		return Config{}, err
	}
	if _, _, err := net.SplitHostPort(redisAddr); err != nil {
		return Config{}, fmt.Errorf("REDIS_ADDR is invalid")
	}

	natsURL, err := read("NATS_URL")
	if err != nil {
		return Config{}, err
	}
	parsedNATS, err := url.Parse(natsURL)
	if err != nil || parsedNATS.Scheme != "nats" || parsedNATS.Host == "" {
		return Config{}, fmt.Errorf("NATS_URL must use nats scheme")
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

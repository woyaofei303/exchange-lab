// Command composeverify 检查渲染后的 Compose 配置安全规则。
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
)

type expectations struct {
	postgresDSN      string
	postgresDatabase string
	postgresUser     string
	postgresPassword string
}

type composeConfig struct {
	Services map[string]composeService `json:"services"`
}

type composeService struct {
	DependsOn   map[string]composeDependency `json:"depends_on"`
	Environment map[string]any               `json:"environment"`
	Command     []string                     `json:"command"`
}

type composeDependency struct {
	Condition string `json:"condition"`
}

func verifyConfig(payload []byte, expected expectations) error {
	var config composeConfig
	if err := json.Unmarshal(payload, &config); err != nil {
		return errors.New("compose configuration JSON is invalid")
	}

	api, ok := config.Services["api"]
	if !ok {
		return errors.New("compose contract requires services.api")
	}
	for _, dependency := range []string{"postgres", "redis", "nats"} {
		if api.DependsOn[dependency].Condition != "service_started" {
			return fmt.Errorf("compose contract requires services.api.depends_on.%s.condition service_started", dependency)
		}
	}
	if api.DependsOn["migrate"].Condition != "service_completed_successfully" {
		return errors.New("compose contract requires services.api.depends_on.migrate.condition service_completed_successfully")
	}

	postgresDSN, err := requiredEnvironment(api, "services.api.environment.POSTGRES_DSN", "POSTGRES_DSN")
	if err != nil {
		return err
	}
	migrate, ok := config.Services["migrate"]
	if !ok {
		return errors.New("compose contract requires services.migrate")
	}
	if migrate.DependsOn["postgres"].Condition != "service_healthy" {
		return errors.New("compose contract requires services.migrate.depends_on.postgres.condition service_healthy")
	}
	migrationDSN, err := requiredEnvironment(migrate, "services.migrate.environment.POSTGRES_DSN", "POSTGRES_DSN")
	if err != nil {
		return err
	}
	if migrationDSN != postgresDSN {
		return errors.New("compose contract requires API and migration PostgreSQL DSNs to match")
	}
	postgres, ok := config.Services["postgres"]
	if !ok {
		return errors.New("compose contract requires services.postgres")
	}
	postgresDatabase, err := requiredEnvironment(postgres, "services.postgres.environment.POSTGRES_DB", "POSTGRES_DB")
	if err != nil {
		return err
	}
	postgresUser, err := requiredEnvironment(postgres, "services.postgres.environment.POSTGRES_USER", "POSTGRES_USER")
	if err != nil {
		return err
	}
	postgresPassword, err := requiredEnvironment(postgres, "services.postgres.environment.POSTGRES_PASSWORD", "POSTGRES_PASSWORD")
	if err != nil {
		return err
	}
	redis, ok := config.Services["redis"]
	if !ok {
		return errors.New("compose contract requires services.redis")
	}
	wantRedisCommand := []string{"redis-server", "--appendonly", "yes", "--maxmemory", "128mb", "--maxmemory-policy", "noeviction"}
	if !slices.Equal(redis.Command, wantRedisCommand) {
		return errors.New("compose contract requires bounded Redis memory with noeviction policy")
	}
	if _, ok := config.Services["market-simulator"]; !ok {
		return errors.New("compose contract requires services.market-simulator")
	}
	marketWorker, ok := config.Services["market-worker"]
	if !ok {
		return errors.New("compose contract requires services.market-worker")
	}
	for _, dependency := range []string{"market-simulator", "candle-worker", "price-worker", "redis", "nats"} {
		if marketWorker.DependsOn[dependency].Condition != "service_healthy" {
			return fmt.Errorf("compose contract requires services.market-worker.depends_on.%s.condition service_healthy", dependency)
		}
	}
	marketURL, err := requiredEnvironment(marketWorker, "services.market-worker.environment.MARKET_WS_URL", "MARKET_WS_URL")
	if err != nil {
		return err
	}
	if marketURL != "ws://market-simulator:8090/ws" {
		return errors.New("compose contract requires the internal market simulator WebSocket URL")
	}
	maxReconnects, err := requiredEnvironment(marketWorker, "services.market-worker.environment.MARKET_MAX_RECONNECTS", "MARKET_MAX_RECONNECTS")
	if err != nil {
		return err
	}
	parsedReconnects, err := strconv.ParseUint(maxReconnects, 10, 8)
	if err != nil || parsedReconnects == 0 || parsedReconnects > 20 {
		return errors.New("compose contract requires bounded market reconnect attempts")
	}
	candleWorker, ok := config.Services["candle-worker"]
	if !ok {
		return errors.New("compose contract requires services.candle-worker")
	}
	if candleWorker.DependsOn["migrate"].Condition != "service_completed_successfully" || candleWorker.DependsOn["nats"].Condition != "service_healthy" {
		return errors.New("compose contract requires candle worker migration and NATS readiness gates")
	}
	candleDSN, err := requiredEnvironment(candleWorker, "services.candle-worker.environment.POSTGRES_DSN", "POSTGRES_DSN")
	if err != nil {
		return err
	}
	if candleDSN != postgresDSN {
		return errors.New("compose contract requires API, migration, and candle worker PostgreSQL DSNs to match")
	}
	eventWorker, ok := config.Services["event-worker"]
	if !ok {
		return errors.New("compose contract requires services.event-worker")
	}
	if eventWorker.DependsOn["migrate"].Condition != "service_completed_successfully" || eventWorker.DependsOn["nats"].Condition != "service_healthy" {
		return errors.New("compose contract requires event worker migration and NATS readiness gates")
	}
	eventDSN, err := requiredEnvironment(eventWorker, "services.event-worker.environment.POSTGRES_DSN", "POSTGRES_DSN")
	if err != nil {
		return err
	}
	if eventDSN != postgresDSN {
		return errors.New("compose contract requires event worker PostgreSQL DSN to match")
	}
	for _, workerName := range []string{"price-worker", "risk-worker", "funding-worker"} {
		worker, ok := config.Services[workerName]
		if !ok {
			return fmt.Errorf("compose contract requires services.%s", workerName)
		}
		if worker.DependsOn["migrate"].Condition != "service_completed_successfully" {
			return fmt.Errorf("compose contract requires services.%s migration readiness gate", workerName)
		}
		workerDSN, err := requiredEnvironment(worker, "services."+workerName+".environment.POSTGRES_DSN", "POSTGRES_DSN")
		if err != nil {
			return err
		}
		if workerDSN != postgresDSN {
			return fmt.Errorf("compose contract requires %s PostgreSQL DSN to match", workerName)
		}
	}
	if config.Services["price-worker"].DependsOn["nats"].Condition != "service_healthy" {
		return errors.New("compose contract requires price worker NATS readiness gate")
	}
	web, ok := config.Services["web"]
	if !ok {
		return errors.New("compose contract requires services.web")
	}
	if web.DependsOn["api"].Condition != "service_healthy" {
		return errors.New("compose contract requires services.web.depends_on.api.condition service_healthy")
	}

	if expected.postgresDSN != "" && postgresDSN != expected.postgresDSN {
		return errors.New("compose contract PostgreSQL DSN override mismatch")
	}
	if expected.postgresDatabase != "" && postgresDatabase != expected.postgresDatabase {
		return errors.New("compose contract PostgreSQL database override mismatch")
	}
	if expected.postgresUser != "" && postgresUser != expected.postgresUser {
		return errors.New("compose contract PostgreSQL user override mismatch")
	}
	if expected.postgresPassword != "" && postgresPassword != expected.postgresPassword {
		return errors.New("compose contract PostgreSQL password override mismatch")
	}
	return nil
}

func requiredEnvironment(service composeService, path, key string) (string, error) {
	value, ok := service.Environment[key].(string)
	if !ok || value == "" {
		return "", fmt.Errorf("compose contract requires non-empty %s", path)
	}
	return value, nil
}

func main() {
	var configPath string
	expected := expectations{}
	flag.StringVar(&configPath, "config", "", "rendered Docker Compose JSON file")
	flag.StringVar(&expected.postgresDSN, "postgres-dsn", "", "expected API PostgreSQL DSN")
	flag.StringVar(&expected.postgresDatabase, "postgres-database", "", "expected PostgreSQL database")
	flag.StringVar(&expected.postgresUser, "postgres-user", "", "expected PostgreSQL user")
	flag.StringVar(&expected.postgresPassword, "postgres-password", "", "expected PostgreSQL password")
	flag.Parse()
	if configPath == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "compose verification failed: invalid arguments")
		os.Exit(2)
	}
	payload, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "compose verification failed: configuration is unreadable")
		os.Exit(1)
	}
	if err := verifyConfig(payload, expected); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

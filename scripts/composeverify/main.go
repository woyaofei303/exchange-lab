package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
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

	postgresDSN, err := requiredEnvironment(api, "services.api.environment.POSTGRES_DSN", "POSTGRES_DSN")
	if err != nil {
		return err
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

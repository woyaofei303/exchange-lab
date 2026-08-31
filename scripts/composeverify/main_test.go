package main

import (
	"strings"
	"testing"
)

const validComposeFixture = `{
  "services": {
    "api": {
      "depends_on": {
        "postgres": {"condition": "service_started"},
        "redis": {"condition": "service_started"},
        "nats": {"condition": "service_started"},
        "migrate": {"condition": "service_completed_successfully"}
      },
      "environment": {"POSTGRES_DSN": "postgres://app:encoded@postgres:5432/app"}
    },
    "migrate": {
      "depends_on": {"postgres": {"condition": "service_healthy"}},
      "environment": {"POSTGRES_DSN": "postgres://app:encoded@postgres:5432/app"}
    },
    "postgres": {
      "environment": {
        "POSTGRES_DB": "app",
        "POSTGRES_USER": "app",
        "POSTGRES_PASSWORD": "raw-password"
      }
    },
    "redis": {
      "command": ["redis-server", "--appendonly", "yes", "--maxmemory", "128mb", "--maxmemory-policy", "noeviction"]
    },
    "market-simulator": {},
    "market-worker": {
      "depends_on": {
        "market-simulator": {"condition": "service_healthy"},
        "candle-worker": {"condition": "service_healthy"},
        "price-worker": {"condition": "service_healthy"},
        "redis": {"condition": "service_healthy"},
        "nats": {"condition": "service_healthy"}
      },
      "environment": {
        "MARKET_WS_URL": "ws://market-simulator:8090/ws",
        "MARKET_MAX_RECONNECTS": "5"
      }
    },
    "candle-worker": {
      "depends_on": {
        "migrate": {"condition": "service_completed_successfully"},
        "nats": {"condition": "service_healthy"}
      },
      "environment": {
        "POSTGRES_DSN": "postgres://app:encoded@postgres:5432/app"
      }
    },
    "event-worker": {
      "depends_on": {
        "migrate": {"condition": "service_completed_successfully"},
        "nats": {"condition": "service_healthy"}
      },
      "environment": {
        "POSTGRES_DSN": "postgres://app:encoded@postgres:5432/app"
      }
    },
    "price-worker": {
      "depends_on": {
        "migrate": {"condition": "service_completed_successfully"},
        "nats": {"condition": "service_healthy"}
      },
      "environment": {
        "POSTGRES_DSN": "postgres://app:encoded@postgres:5432/app"
      }
    },
    "risk-worker": {
      "depends_on": {
        "migrate": {"condition": "service_completed_successfully"}
      },
      "environment": {
        "POSTGRES_DSN": "postgres://app:encoded@postgres:5432/app"
      }
    },
    "funding-worker": {
      "depends_on": {
        "migrate": {"condition": "service_completed_successfully"}
      },
      "environment": {
        "POSTGRES_DSN": "postgres://app:encoded@postgres:5432/app"
      }
    },
    "web": {
      "depends_on": {"api": {"condition": "service_healthy"}}
    }
  }
}`

func TestVerifyConfigAcceptsRequiredExactPathsAndExpectations(t *testing.T) {
	err := verifyConfig([]byte(validComposeFixture), expectations{
		postgresDSN:      "postgres://app:encoded@postgres:5432/app",
		postgresDatabase: "app",
		postgresUser:     "app",
		postgresPassword: "raw-password",
	})
	if err != nil {
		t.Fatalf("verifyConfig() error = %v", err)
	}
}

func TestVerifyConfigRejectsEvictingRedisSecurityKeys(t *testing.T) {
	fixture := strings.Replace(validComposeFixture, "noeviction", "allkeys-lru", 1)
	if err := verifyConfig([]byte(fixture), expectations{}); err == nil {
		t.Fatal("verifyConfig() error = nil")
	}
}

func TestVerifyConfigRejectsMissingBoundedMarketPipeline(t *testing.T) {
	fixture := strings.Replace(validComposeFixture, `"MARKET_MAX_RECONNECTS": "5"`, `"MARKET_MAX_RECONNECTS": "0"`, 1)
	if err := verifyConfig([]byte(fixture), expectations{}); err == nil {
		t.Fatal("verifyConfig() error = nil")
	}
}

func TestVerifyConfigRejectsCandleWorkerWithoutMigrationGate(t *testing.T) {
	fixture := strings.Replace(validComposeFixture, `"migrate": {"condition": "service_completed_successfully"}`, `"migrate": {"condition": "service_started"}`, 1)
	if err := verifyConfig([]byte(fixture), expectations{}); err == nil {
		t.Fatal("verifyConfig() error = nil")
	}
}

func TestVerifyConfigRejectsWebWithoutAPIReadinessGate(t *testing.T) {
	fixture := strings.Replace(validComposeFixture, `"api": {"condition": "service_healthy"}`, `"api": {"condition": "service_started"}`, 1)
	if err := verifyConfig([]byte(fixture), expectations{}); err == nil {
		t.Fatal("verifyConfig() error = nil")
	}
}

func TestVerifyConfigRejectsDerivativeWorkerWithoutMigrationGate(t *testing.T) {
	fixture := strings.Replace(validComposeFixture,
		`"funding-worker": {
      "depends_on": {
        "migrate": {"condition": "service_completed_successfully"}`,
		`"funding-worker": {
      "depends_on": {
        "migrate": {"condition": "service_started"}`,
		1,
	)
	if err := verifyConfig([]byte(fixture), expectations{}); err == nil {
		t.Fatal("verifyConfig() error = nil")
	}
}

func TestVerifyConfigRejectsMissingMigrationGate(t *testing.T) {
	fixture := `{
      "services": {
        "api": {
          "depends_on": {
            "postgres": {"condition": "service_started"},
            "redis": {"condition": "service_started"},
            "nats": {"condition": "service_started"}
          },
          "environment": {"POSTGRES_DSN": "postgres://app@postgres/app"}
        },
        "postgres": {
          "environment": {
            "POSTGRES_DB": "app",
            "POSTGRES_USER": "app",
            "POSTGRES_PASSWORD": "password"
          }
        }
      }
    }`
	if err := verifyConfig([]byte(fixture), expectations{}); err == nil {
		t.Fatal("verifyConfig() error = nil")
	}
}

func TestVerifyConfigRejectsConditionsFoundOnlyOnUnrelatedService(t *testing.T) {
	fixture := `{
      "services": {
        "api": {
          "depends_on": {
            "postgres": {"condition": "service_healthy"},
            "redis": {"condition": "service_healthy"},
            "nats": {"condition": "service_healthy"}
          },
          "environment": {"POSTGRES_DSN": "postgres://app@postgres/app"}
        },
        "decoy": {
          "depends_on": {
            "first": {"condition": "service_started"},
            "second": {"condition": "service_started"},
            "third": {"condition": "service_started"}
          }
        },
        "postgres": {
          "environment": {
            "POSTGRES_DB": "app",
            "POSTGRES_USER": "app",
            "POSTGRES_PASSWORD": "password"
          }
        }
      }
    }`
	if err := verifyConfig([]byte(fixture), expectations{}); err == nil {
		t.Fatal("verifyConfig() error = nil")
	}
}

func TestVerifyConfigRejectsMissingOrEmptyRequiredEnvironmentPaths(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
	}{
		{
			name:    "empty API DSN",
			fixture: `{"services":{"api":{"depends_on":{"postgres":{"condition":"service_started"},"redis":{"condition":"service_started"},"nats":{"condition":"service_started"}},"environment":{"POSTGRES_DSN":""}},"postgres":{"environment":{"POSTGRES_DB":"app","POSTGRES_USER":"app","POSTGRES_PASSWORD":"password"}}}}`,
		},
		{
			name:    "missing PostgreSQL password",
			fixture: `{"services":{"api":{"depends_on":{"postgres":{"condition":"service_started"},"redis":{"condition":"service_started"},"nats":{"condition":"service_started"}},"environment":{"POSTGRES_DSN":"postgres://app@postgres/app"}},"postgres":{"environment":{"POSTGRES_DB":"app","POSTGRES_USER":"app"}}}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := verifyConfig([]byte(tt.fixture), expectations{}); err == nil {
				t.Fatal("verifyConfig() error = nil")
			}
		})
	}
}

func TestVerifyConfigReturnsSanitizedExpectationMismatch(t *testing.T) {
	const secret = "compose-verifier-secret"
	err := verifyConfig([]byte(validComposeFixture), expectations{postgresPassword: secret})
	if err == nil {
		t.Fatal("verifyConfig() error = nil")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "raw-password") {
		t.Fatalf("verifyConfig() leaked configuration value: %v", err)
	}
}

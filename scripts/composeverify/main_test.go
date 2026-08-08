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
        "nats": {"condition": "service_started"}
      },
      "environment": {"POSTGRES_DSN": "postgres://app:encoded@postgres:5432/app"}
    },
    "postgres": {
      "environment": {
        "POSTGRES_DB": "app",
        "POSTGRES_USER": "app",
        "POSTGRES_PASSWORD": "raw-password"
      }
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

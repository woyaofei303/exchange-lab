package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRetrySerializationRetriesOnlySerializationFailures(t *testing.T) {
	attempts := 0
	result, err := retrySerialization(context.Background(), 3, func() (string, error) {
		attempts++
		if attempts < 3 {
			return "", &pgconn.PgError{Code: "40001"}
		}
		return "committed", nil
	})
	if err != nil || result != "committed" || attempts != 3 {
		t.Fatalf("retry result=%q attempts=%d err=%v", result, attempts, err)
	}

	nonRetryable := errors.New("validation failed")
	attempts = 0
	_, err = retrySerialization(context.Background(), 3, func() (string, error) {
		attempts++
		return "", nonRetryable
	})
	if !errors.Is(err, nonRetryable) || attempts != 1 {
		t.Fatalf("non-retryable attempts=%d err=%v", attempts, err)
	}
}

func TestRetrySerializationHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	attempts := 0
	_, err := retrySerialization(ctx, 3, func() (string, error) {
		attempts++
		return "", &pgconn.PgError{Code: "40001"}
	})
	if !errors.Is(err, context.Canceled) || attempts != 1 {
		t.Fatalf("canceled retry attempts=%d err=%v", attempts, err)
	}
}

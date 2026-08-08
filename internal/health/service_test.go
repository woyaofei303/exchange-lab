package health_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/woyaofei303/exchange-lab/internal/health"
)

type checker struct {
	name  string
	check func(context.Context) error
}

func (c checker) Name() string                    { return c.name }
func (c checker) Check(ctx context.Context) error { return c.check(ctx) }

func TestServiceReportsAllDependenciesReady(t *testing.T) {
	service := health.NewService(time.Second,
		checker{name: "postgres", check: func(context.Context) error { return nil }},
		checker{name: "redis", check: func(context.Context) error { return nil }},
	)

	report := service.Check(context.Background())
	if !report.Ready || report.Checks["postgres"] != health.StatusOK || report.Checks["redis"] != health.StatusOK {
		t.Fatalf("Check() report = %#v", report)
	}
}

func TestServiceKeepsEveryResultWhenOneDependencyFails(t *testing.T) {
	service := health.NewService(time.Second,
		checker{name: "postgres", check: func(context.Context) error { return nil }},
		checker{name: "redis", check: func(context.Context) error { return errors.New("redis secret detail") }},
	)

	report := service.Check(context.Background())
	if report.Ready || report.Checks["postgres"] != health.StatusOK || report.Checks["redis"] != health.StatusError {
		t.Fatalf("Check() report = %#v", report)
	}
	if report.Errors["redis"] == nil {
		t.Fatal("Check() did not retain internal error for logging")
	}
}

func TestServiceTimesOutSlowDependency(t *testing.T) {
	service := health.NewService(10*time.Millisecond,
		checker{name: "nats", check: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}},
	)

	started := time.Now()
	report := service.Check(context.Background())
	if report.Ready || report.Checks["nats"] != health.StatusError {
		t.Fatalf("Check() report = %#v", report)
	}
	if time.Since(started) > 200*time.Millisecond {
		t.Fatal("Check() exceeded timeout")
	}
}

func TestServiceChecksDependenciesConcurrently(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	service := health.NewService(time.Second,
		checker{name: "postgres", check: blockingCheck(started, release)},
		checker{name: "redis", check: blockingCheck(started, release)},
	)

	result := make(chan health.Report, 1)
	go func() { result <- service.Check(context.Background()) }()

	for range 2 {
		select {
		case <-started:
		case <-time.After(200 * time.Millisecond):
			close(release)
			t.Fatal("Check() did not start every dependency concurrently")
		}
	}
	close(release)

	report := <-result
	if !report.Ready || report.Checks["postgres"] != health.StatusOK || report.Checks["redis"] != health.StatusOK {
		t.Fatalf("Check() report = %#v", report)
	}
}

func blockingCheck(started chan<- struct{}, release <-chan struct{}) func(context.Context) error {
	return func(ctx context.Context) error {
		started <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

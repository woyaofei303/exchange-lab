// Package health reports liveness-independent dependency readiness.
package health

import (
	"context"
	"sync"
	"time"
)

// Stable dependency status values exposed by readiness responses.
const (
	StatusOK    = "ok"
	StatusError = "error"
)

// Checker verifies one named dependency within the supplied context.
type Checker interface {
	Name() string
	// Check verifies the dependency. Implementations MUST honor cancellation
	// and deadlines in ctx, returning promptly after ctx.Done is closed.
	// Service.Check's completion bound depends on this invariant.
	Check(ctx context.Context) error
}

// Report contains sanitized statuses plus internal errors for logging.
type Report struct {
	Ready  bool
	Checks map[string]string
	Errors map[string]error
}

// Service evaluates independent dependency checks concurrently with a timeout.
type Service struct {
	timeout  time.Duration
	checkers []Checker
}

// NewService copies the checker list so callers cannot mutate service configuration.
func NewService(timeout time.Duration, checkers ...Checker) *Service {
	return &Service{timeout: timeout, checkers: append([]Checker(nil), checkers...)}
}

// Check waits for every bounded dependency check and returns one readiness snapshot.
func (s *Service) Check(ctx context.Context) Report {
	report := Report{
		Ready:  true,
		Checks: make(map[string]string, len(s.checkers)),
		Errors: make(map[string]error),
	}

	var mutex sync.Mutex
	var wait sync.WaitGroup
	for _, current := range s.checkers {
		current := current
		wait.Add(1)
		go func() {
			defer wait.Done()

			checkContext, cancel := context.WithTimeout(ctx, s.timeout)
			defer cancel()

			err := current.Check(checkContext)
			mutex.Lock()
			defer mutex.Unlock()

			if err != nil {
				report.Ready = false
				report.Checks[current.Name()] = StatusError
				report.Errors[current.Name()] = err
				return
			}

			report.Checks[current.Name()] = StatusOK
		}()
	}

	wait.Wait()
	return report
}

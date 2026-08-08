package health

import (
	"context"
	"sync"
	"time"
)

const (
	StatusOK    = "ok"
	StatusError = "error"
)

type Checker interface {
	Name() string
	Check(context.Context) error
}

type Report struct {
	Ready  bool
	Checks map[string]string
	Errors map[string]error
}

type Service struct {
	timeout  time.Duration
	checkers []Checker
}

func NewService(timeout time.Duration, checkers ...Checker) *Service {
	return &Service{timeout: timeout, checkers: append([]Checker(nil), checkers...)}
}

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

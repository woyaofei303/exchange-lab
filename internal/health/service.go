// Package health 报告独立于存活状态的依赖就绪情况。
package health

import (
	"context"
	"sync"
	"time"
)

// 就绪响应公开的稳定依赖状态值。
const (
	StatusOK    = "ok"
	StatusError = "error"
)

// Checker 在提供的上下文中检查一个具名依赖。
type Checker interface {
	Name() string
	// Check 检查依赖。实现必须遵守 ctx 的取消信号和截止时间，
	// 并在 ctx.Done 关闭后立即返回；Service.Check 的完成时间上限依赖此约束。
	Check(ctx context.Context) error
}

// Report 包含脱敏后的状态，以及仅供日志记录的内部错误。
type Report struct {
	Ready  bool
	Checks map[string]string
	Errors map[string]error
}

// Service 并发执行相互独立且带超时的依赖检查。
type Service struct {
	timeout  time.Duration
	checkers []Checker
}

// NewService 复制检查器列表，防止调用方修改服务配置。
func NewService(timeout time.Duration, checkers ...Checker) *Service {
	return &Service{timeout: timeout, checkers: append([]Checker(nil), checkers...)}
}

// Check 等待所有有界依赖检查完成，并返回一份就绪状态快照。
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

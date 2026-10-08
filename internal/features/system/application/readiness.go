package application

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoHealthCheckers reports readiness without any registered dependency
// check. Readiness fails closed in that case.
var ErrNoHealthCheckers = errors.New("no health checkers registered")

// CheckError identifies the dependency whose health check failed.
type CheckError struct {
	Name string
	Err  error
}

func (e *CheckError) Error() string { return fmt.Sprintf("health check %q: %v", e.Name, e.Err) }

func (e *CheckError) Unwrap() error { return e.Err }

// Readiness reports whether every registered dependency is ready.
type Readiness struct {
	checkers []HealthChecker
}

// NewReadiness checks the given dependencies in order.
func NewReadiness(checkers ...HealthChecker) *Readiness {
	return &Readiness{checkers: checkers}
}

// Check returns nil only when at least one checker is registered and all
// checkers pass. It stops at the first failure and returns a *CheckError.
func (r *Readiness) Check(ctx context.Context) error {
	if len(r.checkers) == 0 {
		return ErrNoHealthCheckers
	}
	for _, checker := range r.checkers {
		if err := checker.Check(ctx); err != nil {
			return &CheckError{Name: checker.Name(), Err: err}
		}
	}
	return nil
}

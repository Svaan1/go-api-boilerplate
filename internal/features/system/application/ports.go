// Package application holds system use cases and the ports they depend on.
package application

import "context"

// HealthChecker reports whether one dependency can currently serve traffic.
// Implementations must bound their own work by ctx and must not put secrets
// such as connection strings in returned errors' public surface.
type HealthChecker interface {
	// Name is a stable, non-sensitive label used in logs.
	Name() string
	// Check returns nil when the dependency is ready.
	Check(ctx context.Context) error
}

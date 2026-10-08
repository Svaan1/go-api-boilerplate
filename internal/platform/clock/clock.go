// Package clock provides the process wall clock to use cases.
package clock

import "time"

// System reads the wall clock in UTC.
type System struct{}

// Now returns the current UTC time.
func (System) Now() time.Time { return time.Now().UTC() }

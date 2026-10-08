// Package domain holds the Example entity, its value objects, and domain
// errors. It imports the standard library only.
package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxNameLength bounds Name in characters (runes).
const MaxNameLength = 100

var (
	// ErrInvalidName rejects empty or overlong names.
	ErrInvalidName = errors.New("example name must contain 1 to 100 characters")
	// ErrNotFound reports a missing example.
	ErrNotFound = errors.New("example not found")
	// ErrDuplicateName reports a name already used by another example.
	ErrDuplicateName = errors.New("example name already exists")
)

// ID identifies an Example. Its format is owned by the ID generator.
type ID string

// Name is a validated, whitespace-trimmed example name.
type Name struct {
	value string
}

// NewName validates raw and returns ErrInvalidName when it is empty after
// trimming or longer than MaxNameLength.
func NewName(raw string) (Name, error) {
	trimmed := strings.TrimSpace(raw)
	if length := utf8.RuneCountInString(trimmed); length == 0 || length > MaxNameLength {
		return Name{}, ErrInvalidName
	}
	return Name{value: trimmed}, nil
}

func (n Name) String() string { return n.value }

// Example is the reference entity. Fields are unexported so every change
// goes through methods that keep timestamps consistent.
type Example struct {
	id        ID
	name      Name
	createdAt time.Time
	updatedAt time.Time
}

// New creates an example at time now.
func New(id ID, name Name, now time.Time) Example {
	return Example{id: id, name: name, createdAt: now, updatedAt: now}
}

// Restore rebuilds a persisted example without applying creation rules.
func Restore(id ID, name Name, createdAt, updatedAt time.Time) Example {
	return Example{id: id, name: name, createdAt: createdAt, updatedAt: updatedAt}
}

// Rename changes the name and records the change time.
func (e *Example) Rename(name Name, now time.Time) {
	e.name = name
	e.updatedAt = now
}

// ID returns the identifier.
func (e Example) ID() ID { return e.id }

// Name returns the current name.
func (e Example) Name() Name { return e.name }

// CreatedAt returns the creation time.
func (e Example) CreatedAt() time.Time { return e.createdAt }

// UpdatedAt returns the last modification time.
func (e Example) UpdatedAt() time.Time { return e.updatedAt }

package activeso

import (
	"errors"
	"strings"
)

var (
	// ErrNotFound reports that a query did not return a matching record.
	ErrNotFound = errors.New("activeso: record not found")

	// ErrUnboundRecord reports that a record was not created, loaded, or bound through a Model.
	ErrUnboundRecord = errors.New("activeso: record is not bound to a model")

	// ErrIDChanged reports that a bound record's primary key was modified.
	ErrIDChanged = errors.New("activeso: bound record ID cannot be changed")

	// ErrUnique reports that the database rejected a write for violating a unique index.
	ErrUnique = errors.New("activeso: unique constraint violated")

	// ErrSchemaMismatch reports that a table does not match the model's db tags and activeso hints.
	ErrSchemaMismatch = errors.New("activeso: table does not match model")
)

// SchemaError lists the ways a table differs from its model.
type SchemaError struct {
	Table    string
	Problems []string
}

// Error returns every mismatch in one human-readable message.
func (error *SchemaError) Error() string {
	// Initialize Variables
	message := "activeso: table " + error.Table + " does not match its model: " + strings.Join(error.Problems, "; ")

	return message
}

// Is allows errors.Is to match a SchemaError against ErrSchemaMismatch.
func (error *SchemaError) Is(target error) bool {
	// Initialize Variables
	schemaError := target == ErrSchemaMismatch

	return schemaError
}

// UniqueError identifies the model field that conflicts with a unique constraint.
type UniqueError struct {
	Field string
}

// Error returns a human-readable unique-constraint validation error.
func (error UniqueError) Error() string {
	// Initialize Variables
	message := "activeso: " + error.Field + " must be unique"

	return message
}

// Is allows errors.Is to match a UniqueError against ErrUnique.
func (error UniqueError) Is(target error) bool {
	// Initialize Variables
	uniqueError := target == ErrUnique

	return uniqueError
}

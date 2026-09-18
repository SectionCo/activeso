package activeso

import "errors"

var (
	// ErrNotFound reports that a query did not return a matching record.
	ErrNotFound = errors.New("activeso: record not found")

	// ErrUnboundRecord reports that a record was not created, loaded, or bound through a Model.
	ErrUnboundRecord = errors.New("activeso: record is not bound to a model")

	// ErrIDChanged reports that a bound record's primary key was modified.
	ErrIDChanged = errors.New("activeso: bound record ID cannot be changed")

	// ErrUnique reports that a value conflicts with a field marked activeso:"unique".
	ErrUnique = errors.New("activeso: unique constraint violated")
)

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

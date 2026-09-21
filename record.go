package activeso

import (
	"context"
	"reflect"
	"time"
)

type recordBinding interface {
	save(context.Context, any, any) error
	delete(context.Context, any, any) error
}

// Record embeds active-record persistence methods and managed timestamps into an application-defined model.
type Record struct {
	CreatedAt  time.Time
	UpdatedAt  time.Time
	binding    recordBinding
	owner      any
	originalID any
}

// Save persists the owning record's current field values through its bound model.
func (record *Record) Save(ctx context.Context) error {
	// Initialize Variables
	binding := record.binding
	owner := record.owner
	originalID := record.originalID

	// Reject manually-created or copied records that have no valid model binding.
	if binding == nil || !record.owns(owner) {
		return ErrUnboundRecord
	}

	return binding.save(ctx, owner, originalID)
}

// Delete removes the owning record from its bound model's table.
func (record *Record) Delete(ctx context.Context) error {
	// Initialize Variables
	binding := record.binding
	owner := record.owner
	originalID := record.originalID

	// Reject manually-created or copied records that have no valid model binding.
	if binding == nil || !record.owns(owner) {
		return ErrUnboundRecord
	}

	return binding.delete(ctx, owner, originalID)
}

// owns reports whether owner still contains this exact embedded Record instance.
func (record *Record) owns(owner any) bool {
	// Initialize Variables
	ownerValue := reflect.ValueOf(owner)

	// Confirm that the owner is a pointer to a struct with a direct Record field.
	if ownerValue.Kind() != reflect.Pointer || ownerValue.IsNil() || ownerValue.Elem().Kind() != reflect.Struct {
		return false
	}

	recordField := ownerValue.Elem().FieldByName("Record")
	if !recordField.IsValid() || !recordField.CanAddr() || recordField.Type() != reflect.TypeFor[Record]() {
		return false
	}

	return recordField.Addr().Interface().(*Record) == record
}

package activeso

import (
	"context"
	"database/sql"
	"reflect"
	"sync/atomic"
)

// Executor is the subset of database/sql that ActiveSo needs; *sql.DB, *sql.Tx, and *sql.Conn all satisfy it.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// readiness records whether a table has passed its structural check; every view of one model shares it.
// It is lock-free on purpose: a lock held during inspection would block a transaction's own operations behind
// a root-model call that is itself waiting for the connection that transaction holds.
type readiness struct {
	ready atomic.Bool
}

// nilExecutor reports whether exec is nil, including a typed nil pointer such as (*sql.DB)(nil).
func nilExecutor(exec Executor) bool {
	// Initialize Variables
	value := reflect.ValueOf(exec)

	// Catch both an empty interface and an interface holding a nil pointer.
	if exec == nil {
		return true
	}

	return value.Kind() == reflect.Pointer && value.IsNil()
}

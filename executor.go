package activeso

import (
	"context"
	"database/sql"
	"reflect"
	"sync"
)

// Executor is the subset of database/sql that ActiveSo needs; *sql.DB, *sql.Tx, and *sql.Conn all satisfy it.
type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// readiness records whether a table has passed its structural check; every view of one model shares it.
type readiness struct {
	mutex sync.Mutex
	ready bool
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

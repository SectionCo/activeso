package activeso

import (
	"context"
	"database/sql"
	"fmt"
)

const enableForeignKeys = "PRAGMA foreign_keys = ON"

// execWrite runs a row write with foreign-key enforcement turned on wherever ActiveSo owns the connection.
// A *sql.DB write runs on a pooled connection that gets the pragma first, and a *sql.Conn gets it before the
// write. A *sql.Tx runs as-is: SQLite ignores the pragma inside a transaction, so enforcement there depends
// on the connection the transaction began on.
func (model *model[T]) execWrite(ctx context.Context, statement string, arguments ...any) (sql.Result, error) {
	// Initialize Variables
	var connection *sql.Conn
	var result sql.Result
	var err error

	switch executor := model.exec.(type) {
	case *sql.DB:
		// Take one connection so the pragma and the write share it.
		connection, err = executor.Conn(ctx)
		if err != nil {
			return nil, err
		}
		// Release the connection before returning so follow-up reads cannot starve a one-connection pool.
		defer connection.Close()
	case *sql.Conn:
		connection = executor
	default:
		return model.exec.ExecContext(ctx, statement, arguments...)
	}

	// The pragma is per connection, so set it on the exact connection that performs the write.
	if _, err := connection.ExecContext(ctx, enableForeignKeys); err != nil {
		return nil, fmt.Errorf("activeso: enable foreign keys: %w", err)
	}
	result, err = connection.ExecContext(ctx, statement, arguments...)

	return result, err
}

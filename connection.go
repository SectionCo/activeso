package activeso

import (
	"context"
	"database/sql"
	"fmt"
)

const enableForeignKeys = "PRAGMA foreign_keys = ON"

// execWrite runs a row write on a pooled connection with foreign-key enforcement turned on, so
// belongs_to constraints hold for every ActiveSo write however the pool was opened.
func (model *model[T]) execWrite(ctx context.Context, statement string, arguments ...any) (sql.Result, error) {
	// Initialize Variables
	connection, err := model.db.Conn(ctx)
	var result sql.Result

	if err != nil {
		return nil, err
	}
	// Release the connection before returning so follow-up reads cannot starve a one-connection pool.
	defer connection.Close()

	// The pragma is per connection, so set it on the exact connection that performs the write.
	if _, err := connection.ExecContext(ctx, enableForeignKeys); err != nil {
		return nil, fmt.Errorf("activeso: enable foreign keys: %w", err)
	}
	result, err = connection.ExecContext(ctx, statement, arguments...)

	return result, err
}

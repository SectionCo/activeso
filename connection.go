package activeso

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
)

const enableForeignKeys = "PRAGMA foreign_keys = ON"

// foreignKeyConnector wraps a driver connector so every new physical connection enforces foreign keys.
type foreignKeyConnector struct {
	driver.Connector
}

// OpenDB opens a database/sql handle over connector with foreign-key enforcement enabled on every
// pooled connection. Use it instead of sql.OpenDB so belongs_to constraints hold for direct SQL too.
func OpenDB(connector driver.Connector) *sql.DB {
	// Initialize Variables
	wrapped := foreignKeyConnector{Connector: connector}

	return sql.OpenDB(wrapped)
}

// Connect opens a connection from the wrapped connector and enables foreign-key enforcement on it.
func (connector foreignKeyConnector) Connect(ctx context.Context) (driver.Conn, error) {
	// Initialize Variables
	connection, err := connector.Connector.Connect(ctx)
	var execer driver.ExecerContext
	var ok bool

	if err != nil {
		return nil, err
	}

	// Run the pragma on the new connection before the pool can hand it out.
	execer, ok = connection.(driver.ExecerContext)
	if !ok {
		connection.Close()
		return nil, errors.New("activeso: database driver connection cannot execute PRAGMA foreign_keys")
	}
	if _, err := execer.ExecContext(ctx, enableForeignKeys, nil); err != nil {
		connection.Close()
		return nil, fmt.Errorf("activeso: enable foreign keys: %w", err)
	}

	return connection, nil
}

// execWrite runs a row write on a pooled connection with foreign-key enforcement turned on, so
// belongs_to constraints hold even when the caller opened db without OpenDB.
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

package activeso

import (
	"context"
	"testing"
)

// TestWritesEnforceForeignKeysOnConnAndTransaction rejects orphaning writes through a *sql.Conn executor and through a transaction begun on a connection that already enforces foreign keys.
func TestWritesEnforceForeignKeysOnConnAndTransaction(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	regionModel := Model[Region](db)
	cityModel := Model[City](db)

	execStatements(t, db, `CREATE TABLE regions (id TEXT PRIMARY KEY, name TEXT);
CREATE TABLE cities (id TEXT PRIMARY KEY, name TEXT NOT NULL, region_id TEXT REFERENCES regions (id))`)
	if _, err := regionModel.Create(ctx, Region{ID: "oregon", Name: "Oregon"}); err != nil {
		t.Fatal(err)
	}
	connection, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	// A model built on a *sql.Conn gets the pragma before each write.
	connectionModel := Model[City](connection)
	if _, err := connectionModel.Create(ctx, City{ID: "kept", Name: "Portland", RegionID: "oregon"}); err != nil {
		t.Fatalf("Create() with a valid parent on a connection error = %v", err)
	}
	if _, err := connectionModel.Create(ctx, City{ID: "orphan", Name: "Nowhere", RegionID: "missing"}); err == nil {
		t.Fatal("Create() on a *sql.Conn accepted a missing belongs_to target")
	}

	// SQLite ignores the pragma inside a transaction, so begin on a connection that already enforces it.
	if _, err := connection.ExecContext(ctx, enableForeignKeys); err != nil {
		t.Fatal(err)
	}
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if _, err := cityModel.CreateTx(ctx, transaction, City{ID: "inside", Name: "Salem", RegionID: "oregon"}); err != nil {
		t.Fatalf("CreateTx() with a valid parent error = %v", err)
	}
	if _, err := cityModel.CreateTx(ctx, transaction, City{ID: "inside-orphan", Name: "Nowhere", RegionID: "missing"}); err == nil {
		t.Fatal("CreateTx() accepted a missing belongs_to target on a connection that enforces foreign keys")
	}
}

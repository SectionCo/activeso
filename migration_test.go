package activeso

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// TestTargetedMigrations preserves omitted columns, constraints, indexes, and row IDs across migrations.
func TestTargetedMigrations(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	integerEmail := Model[migrationUserIntegerEmail](db)
	requiredNickname := Model[migrationUserRequiredNickname](db)
	versionOne := Model[migrationUserV1](db)
	var emailType, emailDefault, nickname, extra, indexSQL string
	var rowID, count int

	// Include model-omitted fields, quoted commas, and a partial expression index.
	migrationExec(t, db, `CREATE TABLE migration_users (
		id TEXT PRIMARY KEY, email TEXT NOT NULL DEFAULT '7',
		nickname TEXT COLLATE NOCASE DEFAULT ('a,b'),
		extra TEXT NOT NULL DEFAULT 'keep' CHECK (length(extra) > 0),
		CONSTRAINT keep_unique UNIQUE (extra, nickname)
	)`)
	migrationExec(t, db, `CREATE INDEX custom_email ON migration_users (lower(email), extra) WHERE email IS NOT NULL`)
	migrationExec(t, db, `INSERT INTO migration_users (rowid, id, email, nickname, extra) VALUES (42, 'old', '123', 'Nick', 'data')`)
	if err := db.QueryRowContext(ctx, "SELECT sql FROM sqlite_schema WHERE name = 'custom_email'").Scan(&indexSQL); err != nil {
		t.Fatal(err)
	}

	if err := integerEmail.ChangeColumnType(ctx, "email"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT type, dflt_value FROM pragma_table_info('migration_users') WHERE name = 'email'").Scan(&emailType, &emailDefault); err != nil {
		t.Fatal(err)
	}
	if emailType != "INTEGER" || emailDefault != "'7'" {
		t.Fatalf("email schema: %s, default %s", emailType, emailDefault)
	}
	if err := db.QueryRowContext(ctx, "SELECT rowid, nickname, extra FROM migration_users WHERE id = 'old'").Scan(&rowID, &nickname, &extra); err != nil {
		t.Fatal(err)
	}
	if rowID != 42 || nickname != "Nick" || extra != "data" {
		t.Fatalf("lost row data: %d %q %q", rowID, nickname, extra)
	}

	// SetNotNull must not apply the different Email type from this model.
	if err := requiredNickname.SetNotNull(ctx, "nickname"); err != nil {
		t.Fatal(err)
	}
	if err := requiredNickname.SetNotNull(ctx, "nickname"); err != nil {
		t.Fatalf("repeat SetNotNull: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT type FROM pragma_table_info('migration_users') WHERE name = 'email'").Scan(&emailType); err != nil {
		t.Fatal(err)
	}
	if emailType != "INTEGER" {
		t.Fatalf("SetNotNull changed unrelated email type to %s", emailType)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO migration_users (id, email, nickname, extra) VALUES ('duplicate', 5, 'nick', 'data')"); err == nil {
		t.Fatal("lost unique constraint or collation")
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO migration_users (id, email, nickname, extra) VALUES ('invalid', 5, 'Other', '')"); err == nil {
		t.Fatal("lost CHECK constraint")
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name = 'custom_email' AND sql = ?", indexSQL).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("lost custom index definition")
	}

	// A typo must not apply any pending model differences.
	if err := versionOne.DropColumn(ctx, "typo"); err == nil {
		t.Fatal("accepted missing column")
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info('migration_users') WHERE name = 'extra'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("lost extra column: %d %v", count, err)
	}
}

// TestDropColumnPreservesOtherColumns verifies only the named field is removed.
func TestDropColumnPreservesOtherColumns(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[migrationUserV1](db)
	var extra string
	var count int

	migrationExec(t, db, "CREATE TABLE migration_users (id TEXT PRIMARY KEY, email TEXT, nickname TEXT, extra TEXT DEFAULT 'keep')")
	migrationExec(t, db, "CREATE INDEX keep_extra ON migration_users(extra)")
	migrationExec(t, db, "INSERT INTO migration_users VALUES ('a', 'email', 'remove', 'survive')")
	if err := model.DropColumn(ctx, "nickname"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT extra FROM migration_users").Scan(&extra); err != nil || extra != "survive" {
		t.Fatalf("extra = %q, error %v", extra, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info('migration_users') WHERE name = 'nickname'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("nickname remains: %d %v", count, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name = 'keep_extra'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("lost index: %d %v", count, err)
	}
}

// TestMigrationRejectsDependencies verifies unsupported rebuilds leave schema and rows intact.
func TestMigrationRejectsDependencies(t *testing.T) {
	// Initialize Variables
	cases := []struct{ name, extraSQL, operation string }{
		{"index", `CREATE INDEX dependent ON migration_users ("nickname")`, "drop"},
		{"view", `CREATE VIEW dependent AS SELECT * FROM migration_users`, "type"},
		{"foreign_key", `CREATE TABLE dependent (parent TEXT REFERENCES migration_users(id))`, "type"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			// Initialize Variables
			ctx := context.Background()
			db := openTestDatabase(t, ctx)
			var before, after, nickname string
			var err error

			migrationExec(t, db, "CREATE TABLE migration_users (id TEXT PRIMARY KEY, email TEXT, nickname TEXT)")
			migrationExec(t, db, "INSERT INTO migration_users VALUES ('a', '42', 'keep')")
			migrationExec(t, db, test.extraSQL)
			if err := db.QueryRowContext(ctx, "SELECT sql FROM sqlite_schema WHERE name = 'migration_users'").Scan(&before); err != nil {
				t.Fatal(err)
			}
			if test.operation == "drop" {
				err = Model[migrationUserV1](db).DropColumn(ctx, "nickname")
			} else {
				err = Model[migrationUserIntegerEmail](db).ChangeColumnType(ctx, "email")
			}
			if err == nil {
				t.Fatal("migration accepted an unsupported dependency")
			}
			if err := db.QueryRowContext(ctx, "SELECT sql FROM sqlite_schema WHERE name = 'migration_users'").Scan(&after); err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatal("failed migration changed schema")
			}
			if err := db.QueryRowContext(ctx, "SELECT nickname FROM migration_users").Scan(&nickname); err != nil || nickname != "keep" {
				t.Fatalf("lost data: %q %v", nickname, err)
			}
		})
	}
}

// TestMigrationRollback preserves the original schema when copying rows violates the new constraint.
func TestMigrationRollback(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[migrationUserRequiredNickname](db)
	var count int

	migrationExec(t, db, "CREATE TABLE migration_users (id TEXT PRIMARY KEY, email TEXT, nickname TEXT)")
	migrationExec(t, db, "INSERT INTO migration_users VALUES ('a', '42', NULL)")
	// Exercise the transactional copy check independently of the public preliminary NULL check.
	if err := model.rebuildTable(ctx, "nickname", "not_null", ""); err == nil {
		t.Fatal("accepted NULL rows")
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM migration_users WHERE nickname IS NULL").Scan(&count); err != nil || count != 1 {
		t.Fatalf("lost original row: %d %v", count, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name = 'activeso_migration_users_rebuild'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("left temporary table: %d %v", count, err)
	}
}

// TestMigrationDefinitionQuotes verifies SQL delimiters inside comments and quoted values stay intact.
func TestMigrationDefinitionQuotes(t *testing.T) {
	// Initialize Variables
	definition := "CREATE TABLE t ([id] TEXT PRIMARY KEY, `email` VARCHAR(20) DEFAULT 'x,''y)', /* , ) */ \"nick,name\" TEXT CHECK (length(\"nick,name\") > 0))"
	statement, columns, err := migrationDefinition(definition, "replacement", "email", "type", "INTEGER")

	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 3 || columns[2] != "nick,name" || !strings.Contains(statement, "`email` INTEGER DEFAULT 'x,''y)'") || !strings.Contains(statement, `CHECK (length("nick,name") > 0)`) {
		t.Fatalf("unexpected rewrite: %s; columns %v", statement, columns)
	}
}

// TestMigrationIndexRollback restores original rows and indexes if a type change creates duplicates.
func TestMigrationIndexRollback(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[migrationUserIntegerEmail](db)
	var count int
	var columnType string

	// The values are distinct as TEXT but collide after INTEGER affinity conversion.
	migrationExec(t, db, "CREATE TABLE migration_users (id TEXT PRIMARY KEY, email TEXT)")
	migrationExec(t, db, "CREATE UNIQUE INDEX original_unique ON migration_users(email)")
	migrationExec(t, db, "INSERT INTO migration_users VALUES ('a', '01'), ('b', '1')")
	if err := model.ChangeColumnType(ctx, "email"); err == nil {
		t.Fatal("accepted duplicate converted values")
	}
	if err := db.QueryRowContext(ctx, "SELECT type FROM pragma_table_info('migration_users') WHERE name = 'email'").Scan(&columnType); err != nil || columnType != "TEXT" {
		t.Fatalf("schema not restored: %s %v", columnType, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM migration_users WHERE email IN ('01', '1')").Scan(&count); err != nil || count != 2 {
		t.Fatalf("rows not restored: %d %v", count, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name = 'original_unique'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("index not restored: %d %v", count, err)
	}
}

// TestMigrationPreservesIntegerPrimaryKey keeps an existing integer ID despite a different model ID type.
func TestMigrationPreservesIntegerPrimaryKey(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[migrationUserIntegerEmail](db)
	var id, rowID int
	var columnType string

	migrationExec(t, db, "CREATE TABLE migration_users (id INTEGER PRIMARY KEY, email TEXT)")
	migrationExec(t, db, "INSERT INTO migration_users VALUES (42, '7')")
	if err := model.ChangeColumnType(ctx, "email"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT id, rowid FROM migration_users").Scan(&id, &rowID); err != nil || id != 42 || rowID != 42 {
		t.Fatalf("identity changed: %d %d %v", id, rowID, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT type FROM pragma_table_info('migration_users') WHERE name = 'id'").Scan(&columnType); err != nil || columnType != "INTEGER" {
		t.Fatalf("ID type changed: %s %v", columnType, err)
	}
}

// TestSetNotNullWithCheck distinguishes expression predicates from column constraints.
func TestSetNotNullWithCheck(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[migrationUserRequiredNickname](db)
	var required bool

	migrationExec(t, db, "CREATE TABLE migration_users (id TEXT PRIMARY KEY, email TEXT, nickname TEXT CHECK (email IS NOT NULL))")
	migrationExec(t, db, "INSERT INTO migration_users VALUES ('a', 'email', 'nick')")
	if err := model.SetNotNull(ctx, "nickname"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT "notnull" FROM pragma_table_info('migration_users') WHERE name = 'nickname'`).Scan(&required); err != nil || !required {
		t.Fatalf("NOT NULL not applied: %t %v", required, err)
	}
}

// migrationExec executes fixture SQL and reports failures at the caller.
func migrationExec(t *testing.T, db *sql.DB, statement string) {
	// Initialize Variables
	_, err := db.Exec(statement)

	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

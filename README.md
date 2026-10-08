# ActiveSo

ActiveSo is a small active record-like persistence layer for Go structs backed by Turso. It generates the SQL for creating, reading, updating, and deleting records. **It does not create or migrate your schema:** you own your tables and their migrations, and ActiveSo checks that they match your Go types.

## Table of contents

- [Quick start](#quick-start)
- [Bring your own schema](#bring-your-own-schema)
- [Hints](#hints)
- [Verifying your schema](#verifying-your-schema)
- [API](#api)
- [Upgrading from 1.x](#upgrading-from-1x)
- [Browser example](#browser-example)

## Quick start

```go
package main

import (
	"context"
	"database/sql"

	"github.com/sectionco/activeso/v2"
	turso "turso.tech/database/tursogo"
)

type User struct {
	activeso.Record `activeso:"timestamps"`

	ID        string            `db:"id"`
	Email     string            `db:"email" activeso:"not_null,unique"`
	Embedding activeso.Vector32 `db:"embedding"`
}

func example(ctx context.Context) error {

	// Create a Turso connector and open a database/sql handle over it.
	connector, err := turso.NewConnector("app.db")
	if err != nil {
		return err
	}
	db := sql.OpenDB(connector)
	defer db.Close()

	// Create the table with your own migrations; ActiveSo never creates or alters tables.
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		email TEXT NOT NULL,
		embedding BLOB,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "CREATE UNIQUE INDEX IF NOT EXISTS users_email ON users (email)"); err != nil {
		return err
	}

	// If using the belongs_to hint, enable foreign-key enforcement on this connection.
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return err
	}

	// Initialize the User model once and reuse it.
	userModel := activeso.Model[User](db)

	// Create a new User record.
	user, err := userModel.Create(ctx, User{Email: "hello@null.live"})
	if err != nil {
		return err
	}

	// Update the user's email.
	user.Email = "updated@null.live"
	if err := user.Save(ctx); err != nil {
		return err
	}

	// Delete the user.
	return user.Delete(ctx)
}
```

`Model` uses plural snake_case table names by default. English inflection handles common and irregular forms (`User` → `users`, `City` → `cities`, `Person` → `people`) and does not double-pluralize a type already named `Users`.

Implement `TableName() string` whenever your database uses a specific or domain-specific table name. For example:

```go
type Customer struct {
	activeso.Record

	ID string `db:"id"`
}

func (Customer) TableName() string {
	return "crm_customers"
}
```

## Bring your own schema

ActiveSo reads and writes tables that already exist. Create and change them with whatever migration tooling you prefer. The `User` model above maps to this table:

```sql
CREATE TABLE users (
	id TEXT PRIMARY KEY,
	email TEXT NOT NULL,
	embedding BLOB,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE UNIQUE INDEX users_email ON users (email);
```

**Every persisted field needs a `db` tag** naming its column. ActiveSo does not guess column names, so `activeso.Model[T](db)` panics at setup when an exported field has no `db` tag. Use `db:"-"` to exclude an exported field from persistence. Columns that exist in the table but not in the struct are ignored, so you can add columns ahead of a deploy or drop a field's column later.

`Record` holds ActiveSo's runtime persistence binding and is not a database column.

### Timestamps

`Record` promotes `CreatedAt` and `UpdatedAt` fields, but ActiveSo only manages them when you opt in with the `timestamps` hint on the embedded `Record`:

```go
type User struct {
	activeso.Record `activeso:"timestamps"`

	ID    string `db:"id"`
	Email string `db:"email"`
}
```

With the hint, ActiveSo **expects the table to have `created_at` and `updated_at` columns** holding UTC timestamp text. `Create` stamps both with `CURRENT_TIMESTAMP`, `Save` stamps `updated_at` and never rewrites `created_at`, and reads populate the Go fields. The columns need no defaults or triggers. If either column is missing, the first operation fails with an `activeso.ErrSchemaMismatch` error that names `created_at` and `updated_at`; a stored value that is not valid UTC timestamp text (`YYYY-MM-DD HH:MM:SS` or RFC3339) fails when read with an error that names the same columns. Without the hint, ActiveSo never touches those columns and `CreatedAt` and `UpdatedAt` stay zero. A struct field cannot map to `created_at` or `updated_at` when the hint is set.

Timestamps are metadata written by ActiveSo, not an authorization boundary. Writes made with raw SQL outside ActiveSo do not refresh `updated_at`, and `Bind` attaches persistence behavior without loading timestamps.

### NULL handling

When loading a plain Go `string`, ActiveSo reads SQL `NULL` as `""`. A plain string write always stores text, so `NULL` and an empty string become indistinguishable after the value is loaded or saved.

Plain numeric and boolean fields also read `NULL` as their Go zero values (`0`, `0.0`, and `false`), so rows that predate a nullable column remain readable. Use nullable `database/sql` value types when the distinction between `NULL` and a zero value matters.

ActiveSo supports `sql.NullString` as `TEXT`, `sql.NullInt64` as `INTEGER`, `sql.NullFloat64` as `REAL`, and `sql.NullBool` as `INTEGER`. ActiveSo retains `Valid` when loading and saving:

```go
type User struct {
	activeso.Record

	ID       string         `db:"id"`
	Nickname sql.NullString `db:"nickname"`
}
```

`sql.NullString{Valid: false}` represents SQL `NULL`; `sql.NullString{String: "", Valid: true}` represents an empty string.

## Hints

Hints describe what your table already enforces and how its records relate. Add comma-separated hints in an `activeso` struct tag:

```go
type City struct {
	activeso.Record

	ID       string `db:"id"`
	Name     string `db:"name" activeso:"not_null,index"`
	RegionID string `db:"region_id" activeso:"belongs_to=regions(id)"`
}
```

Hints never change the database. An unknown or malformed hint makes `activeso.Model[T](db)` panic, so mistakes surface during setup.

| Hint | Meaning | Runtime behavior |
| --- | --- | --- |
| `primary_key` | The field is the table's primary key. | Selects the identity used by `Find`, `Save`, `Delete`, and `Bind`. The field must use a supported immutable scalar type. |
| `unique` | A unique index covers exactly this column. | When Turso rejects a write for violating it, `Create` and `Save` return an error matching `activeso.ErrUnique` that is also an `activeso.UniqueError` naming the column. |
| `unique_with=column[+column...]` | A composite unique index begins with this column and continues with the named columns, in order. Do not repeat the tagged column in the list. | Turso rejects duplicate combinations with an error matching `activeso.ErrUnique`. |
| `index` | An index begins with this column. | `Verify` checks it. It makes `FindBy` lookups eligible to use the index. |
| `not_null` | The column is declared `NOT NULL`. | `Verify` checks it. Turso rejects `NULL` values. |
| `belongs_to=table(column)` | The column has a foreign key to `table(column)`. | `Verify` checks it. With foreign-key enforcement enabled, Turso rejects non-`NULL` values missing from the referenced column. |
| `on_delete=cascade` | The `belongs_to` foreign key uses `ON DELETE CASCADE`. Requires `belongs_to` on the same field. | `Verify` checks it. With enforcement enabled, deleting a referenced row also deletes its matching child rows. |

On the embedded `Record`, the only hint is [`timestamps`](#timestamps).

If no field has `primary_key`, ActiveSo uses the field mapped to `id`. Exactly one primary key is required. Supported primary-key types are strings, booleans, signed integers, `uint8`, `uint16`, `uint32`, and floats. `uint` and `uint64` are rejected because their full range cannot be represented by Turso's signed 64-bit `INTEGER`.

`belongs_to` targets must use simple identifiers in `table(column)` form. A primary key satisfies `not_null` and `index`.

Use `unique_with` when a combination of fields, rather than one field alone, must be unique:

```go
type OrganizationCustomer struct {
	activeso.Record

	ID             string `db:"id"`
	CustomerID     string `db:"customer_id" activeso:"not_null,index,belongs_to=customers(id),unique_with=organization_id"`
	OrganizationID string `db:"organization_id" activeso:"not_null,index,belongs_to=organizations(id)"`
}
```

```sql
CREATE UNIQUE INDEX memberships_customer_organization ON organization_customers (customer_id, organization_id);
```

Foreign-key enforcement is a connection setting in SQLite-compatible databases. Enable it after opening the database, and make sure every pooled connection is configured:

```go
db := sql.OpenDB(connector)
if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
	return err
}
```

## Verifying your schema

Because ActiveSo trusts your tables, it can check that they match your model. There are two levels:

- **Automatic structural check.** The first `Create`, `Find`, query, `Save`, or `Delete` on a model checks that the table and every mapped column exist, that the `timestamps` columns exist when that hint is set, and that the primary-key column is a `PRIMARY KEY` or has a single-column unique index. A failure returns an `*activeso.SchemaError` and is retried on the next call, so a table created later is picked up. A passing check is remembered for the model's lifetime, so create a model once and reuse it.
- **Full check with `Verify(ctx)`.** It adds everything the automatic check skips: column types against Go types (a field type ActiveSo cannot map to a column is reported unless it implements `driver.Valuer` and `sql.Scanner`), and every `not_null`, `unique`, `unique_with`, `index`, `belongs_to`, and `on_delete=cascade` hint. Call it at startup or in a test. It only reads schema metadata and never changes the database.

```go
if err := userModel.Verify(ctx); err != nil {
	var schemaError *activeso.SchemaError
	if errors.As(err, &schemaError) {
		for _, problem := range schemaError.Problems {
			log.Println(problem)
		}
	}
	return err
}
```

`SchemaError` matches `activeso.ErrSchemaMismatch` with `errors.Is` and lists every problem found, for example `hint unique on email, but no unique index covers exactly that column`.

## API

### Contents

- [Model](#model)
- [Verify](#verify)
- [Create](#create)
- [Find](#find)
- [FindBy](#findby)
- [All](#all)
- [Where](#where)
- [OrderBy](#orderby)
- [Limit](#limit)
- [Nearest](#nearest)
- [First](#first)
- [Bind](#bind)
- [Save](#save)
- [Delete](#delete)

### Model

`activeso.Model[T](db)` creates the persistence model for your Go struct type. In this example, `T` is `User`, so `activeso.Model[User](db)` maps `User` values to the `users` table.

```go
userModel := activeso.Model[User](db)
```

### Verify

`Verify(ctx)` compares the live table with the model's `db` tags and hints and returns an `*activeso.SchemaError` listing every mismatch. See [Verifying your schema](#verifying-your-schema).

```go
if err := userModel.Verify(ctx); err != nil {
	return err
}
```

### Create

`Create(ctx, value)` inserts `value`, generates an ID when its string ID field is empty, and returns the bound record.

```go
user, err := userModel.Create(ctx, User{Email: "hello@null.live"})
```

### Find

`Find(ctx, id)` loads the record with `id`. It returns `activeso.ErrNotFound` when no record exists.

```go
user, err := userModel.Find(ctx, "8b291a21-e69b-47ed-a3e0-f43e7609b26d")
```

### FindBy

`FindBy(ctx, column, value)` loads every record whose mapped `column` equals `value`. The column name must exist on the model, so ActiveSo quotes it safely rather than accepting arbitrary SQL. Index frequently searched non-unique columns in your schema and mark them with the `index` hint:

```go
type City struct {
	activeso.Record

	ID   string `db:"id"`
	Name string `db:"name" activeso:"not_null,index"`
}

cities, err := cityModel.FindBy(ctx, "name", "Portland")
```

### All

`All(ctx)` loads every record from the model's table.

```go
users, err := userModel.All(ctx)
```

### Where

`Where(predicate, arguments...)` starts a parameterized query. Pass one argument for each `?` placeholder.

```go
user, err := userModel.Where("email = ?", "hello@null.live").First(ctx)
```

Use multiple arguments for predicates with multiple placeholders:

```go
excludedID := "8b291a21-e69b-47ed-a3e0-f43e7609b26d"
users, err := userModel.Where("email LIKE ? AND id <> ?", "%@null.live", excludedID).All(ctx)
```

### OrderBy

`OrderBy(expression)` applies an `ORDER BY` expression to a query. Calling it after `Nearest` replaces the vector-distance ordering and discards that ordering's embedding argument.

```go
users, err := userModel.Where("email LIKE ?", "%@null.live").
	OrderBy("email ASC").
	All(ctx)
```

### Limit

`Limit(limit)` applies a positive maximum row count to a query.

```go
users, err := userModel.Where("email LIKE ?", "%@null.live").
	Limit(10).
	All(ctx)
```

### Nearest

`Nearest(column, embedding)` orders records by cosine distance from a `Vector32` embedding in the specified Turso `vector32` column. Lower distance is more similar; combine it with `Limit` to perform a nearest-neighbor search.

```go
queryEmbedding := activeso.Vector32{0.12, -0.08, 0.63}
users, err := userModel.Nearest("embedding", queryEmbedding).Limit(10).All(ctx)
```

You can also use a vector as a `Where` argument when the predicate explicitly calls Turso's `vector32(?)` function:

```go
queryEmbedding := activeso.Vector32{0.12, -0.08, 0.63}
users, err := userModel.Where(
	"vector_distance_cos(embedding, vector32(?)) < ?",
	queryEmbedding,
	0.2,
).All(ctx)
```

### First

`First(ctx)` loads the first matching query result. It returns `activeso.ErrNotFound` when no record matches.

```go
user, err := userModel.Where("email LIKE ?", "%@null.live").
	OrderBy("email ASC").
	First(ctx)
```

### Bind

`Bind(value)` attaches persistence behavior to an existing record pointer, such as one loaded outside ActiveSo. It does not load timestamps.

```go
externalUser := &User{ID: "8b291a21-e69b-47ed-a3e0-f43e7609b26d", Email: "hello@null.live"}
user, err := userModel.Bind(externalUser)
```

### Save

`Save(ctx)` updates all persisted fields except the primary key. It is available on records returned by `Create`, `Find`, query methods, or `Bind`. A record loaded outside ActiveSo must be attached with `Bind` first; an unbound record's `Save` and `Delete` return `activeso.ErrUnboundRecord`.

A record's ID is captured when it becomes bound and cannot be changed. If you modify it, `Save` and `Delete` return `activeso.ErrIDChanged`; create a new record when you need a new identity.

```go
user.Email = "updated@null.live"
err := user.Save(ctx)
```

### Delete

`Delete(ctx)` deletes a bound record by primary key.

```go
err := user.Delete(ctx)
```

## Upgrading from 1.x

ActiveSo 2.0 removes all schema management. Plan these changes together:

1. **Import path.** The module is now `github.com/sectionco/activeso/v2`.
2. **Remove migration calls.** `AutoMigrate`, `DropUnique`, `DropUniqueWith`, `DropColumn`, `ChangeColumnType`, and `SetNotNull` no longer exist. Existing tables keep working; changes to them are yours to write.
3. **Tag every field with `db`.** Fields that relied on derived snake_case names now panic at `Model` setup until they are tagged.
4. **Rename constraints to hints.** The tag syntax is unchanged, but hints no longer create anything. `unique`, `unique_with`, `index`, `not_null`, `belongs_to`, and `on_delete=cascade` are checked by `Verify`.
5. **Unique errors come from the database.** `Create` and `Save` no longer run a check-then-write query first. Keep the unique index in your schema; a missing index means duplicates are accepted. `Verify` reports it.
6. **Timestamps are opt-in and renamed.** Add `activeso:"timestamps"` to the embedded `Record` to keep `CreatedAt` and `UpdatedAt`. The columns are now `created_at` and `updated_at`, and ActiveSo stamps them in its own SQL instead of using triggers.

To move a 1.x database to the new timestamp columns, drop ActiveSo's triggers first (renaming a column rewrites trigger bodies, and the old triggers would then refer to columns that no longer exist), then rename the columns. Trigger names are `activeso_<hex of the lowercase table name>_timestamps_` followed by `insert`, `update`, `protect_created_at`, or `protect_updated_at`; list them with `SELECT name FROM sqlite_schema WHERE type = 'trigger'`.

```sql
DROP TRIGGER IF EXISTS activeso_7573657273_timestamps_insert;
DROP TRIGGER IF EXISTS activeso_7573657273_timestamps_update;
DROP TRIGGER IF EXISTS activeso_7573657273_timestamps_protect_created_at;
DROP TRIGGER IF EXISTS activeso_7573657273_timestamps_protect_updated_at;
ALTER TABLE users RENAME COLUMN activeso_created_at TO created_at;
ALTER TABLE users RENAME COLUMN activeso_updated_at TO updated_at;
```

Indexes that 1.x created (named like `activeso_<hex>_<hex>_unique`) keep working and need no changes. Back up production data before running any migration, and never open a live synced replica with a plain non-sync connection or the SQLite CLI; use the sync SDK or an offline copy.

## References

- [Turso Go SDK](https://docs.turso.tech/sdk/go)
- [SQLite foreign-key support](https://www.sqlite.org/foreignkeys.html)

## Browser example

The `example/` directory contains a small Echo v5 server with a single HTML page for creating, editing, and deleting users. It creates its own schema in a local `activeso-example.db` file and listens on [http://localhost:8080](http://localhost:8080).

```sh
go -C example run .
```

### Inspect the local database

After stopping the browser example so it no longer has the database file open, start a local Turso server in one terminal:

```sh
turso dev --db-file activeso-example.db --port 8100
```

In a second terminal, connect its SQL shell:

```sh
turso db shell http://127.0.0.1:8100
```

Then inspect the database from the shell:

```sql
.tables
.schema users
SELECT * FROM users;
```

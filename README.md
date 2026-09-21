# ActiveSo

ActiveSo is a small active record-like persistence layer for Go structs backed by Turso.

## Table of contents

- [Quick start](#quick-start)
- [Mapping a model to SQL](#mapping-a-model-to-sql)
- [Constraint options](#constraint-options)
- [API](#api)
- [Browser example](#browser-example)

## Quick start

```go
package main

import (
	"context"
	"database/sql"

	"github.com/sectionco/activeso"
	turso "turso.tech/database/tursogo"
)

type User struct {
	activeso.Record

	ID        string             `db:"id"`
	Email     string             `db:"email" activeso:"not_null,unique"`
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

	// Initialize the User model.
	userModel := activeso.Model[User](db)

	// Optional: run during application setup to create or update the schema.
	if err := userModel.AutoMigrate(ctx); err != nil {
		return err
	}

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

`Model` uses plural snake_case table names by default (`User` maps to `users`). 

Implement `TableName() string` on a model for an explicit table name. 

Run `AutoMigrate(ctx)` separately during application setup or deployment when you want ActiveSo to create the table, add missing nullable columns, and create tagged unique indexes; it is not required for normal model initialization or record operations.

## Mapping a model to SQL

For example, the `User` model maps to the following Turso table.

```go
type User struct {
	activeso.Record

	ID    string `db:"id"`
	Email string `db:"email" activeso:"not_null,unique"`
}
```

```sql
CREATE TABLE users (
	id TEXT PRIMARY KEY,
	email TEXT NOT NULL
);

CREATE UNIQUE INDEX activeso_7573657273_656d61696c_unique ON users (email);
```

`Record` holds ActiveSo's runtime persistence binding and is not a database column. The `db` tags select column names; without them, ActiveSo derives snake_case names from exported field names. The `activeso:"not_null,unique"` tag adds `NOT NULL` and a unique index when `AutoMigrate(ctx)` runs.

When loading a plain Go `string`, ActiveSo reads SQL `NULL` as `""`. This is the default: it lets a nullable string column added by `AutoMigrate` remain readable for pre-existing rows. A plain string write always stores text, so `NULL` and an empty string become indistinguishable after the value is loaded or saved.

Plain numeric and boolean fields also read `NULL` as their Go zero values (`0`, `0.0`, and `false`) so rows created before an additive nullable migration remain readable. Use nullable `database/sql` value types when the distinction between `NULL` and a zero value matters.

Use `sql.NullString` when your application needs to preserve that distinction. Import `database/sql` and use it directly in the model; ActiveSo stores it as `TEXT` and retains `Valid` when loading and saving:

```go
type User struct {
	activeso.Record

	ID       string         `db:"id"`
	Nickname sql.NullString `db:"nickname"`
}
```

`sql.NullString{Valid: false}` represents SQL `NULL`; `sql.NullString{String: "", Valid: true}` represents an empty string.

## Constraint options

Add comma-separated constraint options in an `activeso` struct tag:

```go
type User struct {
	activeso.Record

	ID    string `db:"id"`
	Email string `db:"email" activeso:"not_null,unique"`
}
```

| Option | `AutoMigrate(ctx)` behavior | Write behavior |
| --- | --- | --- |
| `not_null` | Adds `NOT NULL` when creating a table. ActiveSo refuses to add a new required column to an existing table automatically. | Turso rejects `NULL` values. |
| `unique` | Creates a stable, unambiguous unique index for the table and column. | `Create` and `Save` check for an existing value first and return an error matching `activeso.ErrUnique`; the Turso index remains the concurrency-safe authority. |
| `primary_key` | Declares the field as the table primary key. | Selects the identity used by `Find`, `Save`, and `Delete`; the field must use an immutable scalar type. |

If no field has `primary_key`, ActiveSo uses the field mapped to `id`. Exactly one primary key is required. 

An unknown `activeso` constraint causes `activeso.Model[T](db)` to panic so schema mistakes are caught during setup.

## API

### Contents

- [Model](#model)
- [AutoMigrate](#automigrate)
- [Explicit migrations](#explicit-migrations)
- [Create](#create)
- [Find](#find)
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

### AutoMigrate

`AutoMigrate(ctx)` creates the table, adds missing nullable columns, and creates tagged unique indexes. Call it once during application setup or deployment, separately from normal record operations.

```go
if err := userModel.AutoMigrate(ctx); err != nil {
	return err
}
```

### Explicit migrations

`AutoMigrate(ctx)` is intentionally additive and safe: it never drops data, removes indexes, changes column types, or tightens existing constraints. Make destructive schema changes deliberately by first updating the model definition, then calling the matching operation during deployment.

| Operation | Required model change | Effect |
| --- | --- | --- |
| `DropUnique(ctx, column)` | Remove `unique` from the column's `activeso` tag. | Drops ActiveSo's named unique index. |
| `DropColumn(ctx, column)` | Remove the field from the model. | Rebuilds the table without the column. |
| `ChangeColumnType(ctx, column)` | Change the Go field's type. | Rebuilds the table using the newly inferred Turso column type. |
| `SetNotNull(ctx, column)` | Add `not_null` to the field's `activeso` tag. | Rebuilds the table with `NOT NULL`; it fails if any existing row contains `NULL`. |

For example, to stop requiring unique emails, remove `unique` from the tag and run an explicit migration:

```go
type User struct {
	activeso.Record

	ID    string `db:"id"`
	Email string `db:"email" activeso:"not_null"`
}

userModel := activeso.Model[User](db)
if err := userModel.DropUnique(ctx, "email"); err != nil {
	return err
}
```

Each column migration changes only its named target. Existing columns omitted from your Go struct, their data, and unrelated defaults, constraints, and indexes are preserved. For example, `ChangeColumnType(ctx, "email")` leaves an existing `nickname` column intact even if the struct no longer contains it. Removing a struct field alone never drops its database column; call `DropColumn` explicitly.

These operations rebuild the existing database schema inside a transaction. Unsupported dependencies cause an error, and failed migrations roll back. Remove indexes or constraints referencing a column before dropping it. Rebuilds currently reject dependent views/triggers, foreign keys involving the table, generated columns, autoincrement columns, and primary-key type changes; those require a dedicated migration. Explicit NULL expressions and fully shadowed row IDs can also require a dedicated migration. Run destructive migrations during a controlled deployment and back up production data first.

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

`Bind(value)` attaches persistence behavior to an existing record pointer, such as one loaded outside ActiveSo.

```go
externalUser := &User{ID: "8b291a21-e69b-47ed-a3e0-f43e7609b26d", Email: "hello@null.live"}
user, err := userModel.Bind(externalUser)
```

### Save

`Save(ctx)` updates all persisted fields except the primary key. It is available on records returned by `Create`, `Find`, query methods, or `Bind`. 

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

## Browser example

The `example/` directory contains a small Echo v5 server with a single HTML page for creating, editing, and deleting users. It creates a local `activeso-example.db` file and listens on [http://localhost:8080](http://localhost:8080).

```sh
go -C example run .
```

# Active Record Binding

## Public lifecycle

`activeso.Model[T](db)` is the entry point for a model type that directly embeds `activeso.Record`. `Create`, `Find`, `First`, and `All` return `*T` values (or `[]*T`) so the embedded record can retain an exact pointer to its owning model instance.

A returned record supports `record.Save(ctx)` and `record.Delete(ctx)`. A manually built or copied record must be attached first with `model.Bind(&record)`; otherwise those methods return `activeso.ErrUnboundRecord`. `Bind` only attaches behavior and does not load database values.


## Naming and schema

The default table name is the pluralized snake_case model type name (`User` -> `users`). Implement `TableName() string` for an override. Column names are derived from `db` tags or snake_case field names. Each model needs an `ID` field or a field tagged `db:"id"`.

`model.AutoMigrate(ctx)` explicitly creates missing tables, adds missing nullable columns, creates managed timestamps, and creates unique indexes. Every ActiveSo table has implicit `activeso_created_at` and `activeso_updated_at` TEXT columns with `CURRENT_TIMESTAMP` defaults when newly created. Existing tables are backfilled and protected with an insert trigger because SQLite cannot add a dynamic timestamp default through `ALTER TABLE`. A managed update trigger refreshes `activeso_updated_at` when any non-timestamp column changes, including direct SQL updates. Protection triggers reject replacement of an established creation timestamp and arbitrary replacement of the update timestamp. `Record` promotes the timestamp values as `time.Time` fields named `CreatedAt` and `UpdatedAt`; `Create`, reads, and successful `Save` calls populate or refresh them. It is deliberately additive and safe: it never removes columns or indexes, changes types, or tightens constraints. Supported `activeso` field constraints are `not_null` and `unique`; `unique` is checked before writes for a friendly `ErrUnique` result and enforced by Turso with a unique index. If a concurrent write passes preflight and Turso rejects it, `Create` and `Save` translate the authoritative unique-constraint violation to `UniqueError` (which matches `ErrUnique`).

When scanning SQL data into a plain Go string field, ActiveSo uses `sql.NullString` internally and maps SQL `NULL` to `""`. This keeps rows readable after adding a nullable string column. Plain Go strings cannot represent the difference between NULL and an empty string; subsequent saves write `""`. Models may use `sql.NullString` directly when they need to preserve the distinction; it maps to TEXT and its `Valid` flag is retained.

Binding captures a record's primary-key value. Changing a bound record's ID makes `Save` and `Delete` return `ErrIDChanged`, preventing the operation from targeting another row.

Destructive changes require an explicit operation after the Go model has been updated: `DropUnique(ctx, column)`, `DropColumn(ctx, column)`, `ChangeColumnType(ctx, column)`, or `SetNotNull(ctx, column)`. Column removal, type changes, and `NOT NULL` tightening rebuild the existing database schema inside a transaction, changing only the named column. Fields omitted from the model and unrelated model changes do not affect that rebuild. `SetNotNull` first rejects the migration when existing rows contain `NULL` values. See `../targeted-migrations/SKILL.md` for preservation guarantees and rejected schema features.

## Turso vectors

`Vector32` fields are persisted with Turso's `vector32(?)` function and read with `vector_extract(...)`. The Go value is a `[]float32`-based type; its transport representation is JSON.

`model.Nearest(column, embedding)` orders records by Turso's `vector_distance_cos` in ascending order, so lower distance results are returned first. `Vector32` values supplied to `Where` are JSON-encoded automatically; callers must use them with a `vector32(?)` SQL expression, such as `Where("vector_distance_cos(embedding, vector32(?)) < ?", embedding, 0.2)`.

`OrderBy(expression)` replaces the full previous ordering, including any vector argument installed by `Nearest`.

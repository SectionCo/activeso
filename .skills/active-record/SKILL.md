# Active Record Binding

## Public lifecycle

`activeso.Model[T](db)` is the entry point for a model type that directly embeds `activeso.Record`. `Create`, `Find`, `First`, and `All` return `*T` values (or `[]*T`) so the embedded record can retain an exact pointer to its owning model instance.

A returned record supports `record.Save(ctx)` and `record.Delete(ctx)`. A manually built or copied record must be attached first with `model.Bind(&record)`; otherwise those methods return `activeso.ErrUnboundRecord`. `Bind` only attaches behavior and does not load database values.


## Naming and schema

The default table name is the English-pluralized snake_case model type name (`User` -> `users`, `Person` -> `people`). The pluralizer recognizes already-plural type names (`Users` -> `users`) without double-pluralizing. Implement `TableName() string` for an explicit override. Every persisted field needs a `db` tag; each model needs an `ID` field or a `primary_key` hint. See `../model-mapping/SKILL.md`.

ActiveSo 2.0 does not create or migrate schemas: there is no `AutoMigrate`, `DropColumn`, `ChangeColumnType`, `SetNotNull`, or `DropUnique*`. The application owns its DDL. `model.Verify(ctx)` and a one-time structural check before the first operation compare the table with the model (see `../schema-verification/SKILL.md`). Managed `created_at`/`updated_at` timestamps are opt-in through ``activeso.Record `activeso:"timestamps"` `` (see `../timestamp-replay/SKILL.md`).

Supported hints are `primary_key`, `unique`, `unique_with`, `index`, `not_null`, `belongs_to`, and `on_delete=cascade`. Uniqueness is enforced only by the user's database index: `Create` and `Save` translate Turso's unique-constraint failure to `UniqueError` (which matches `ErrUnique`) and do no preflight query.

When scanning SQL data into a plain Go string field, ActiveSo uses `sql.NullString` internally and maps SQL `NULL` to `""`. Plain Go strings cannot represent the difference between NULL and an empty string; subsequent saves write `""`. Models may use `sql.NullString` directly when they need to preserve the distinction.

Models have `CreateTx` and `FindTx`, and records have `SaveTx` and `DeleteTx`, for a single call on a transaction; `model.Using(tx)` returns a transaction-scoped view for queries and multi-step work (see `../transactions/SKILL.md`).

Binding captures a record's primary-key value. Changing a bound record's ID makes `Save` and `Delete` return `ErrIDChanged`, preventing the operation from targeting another row.

## Turso vectors

`Vector32` fields are persisted with Turso's `vector32(?)` function and read with `vector_extract(...)`. The Go value is a `[]float32`-based type; its transport representation is JSON.

`model.Nearest(column, embedding)` orders records by Turso's `vector_distance_cos` in ascending order, so lower distance results are returned first. `Vector32` values supplied to `Where` are JSON-encoded automatically; callers must use them with a `vector32(?)` SQL expression, such as `Where("vector_distance_cos(embedding, vector32(?)) < ?", embedding, 0.2)`. `FindBy(ctx, vectorColumn, embedding)` recognizes mapped `Vector32` columns and adds that conversion automatically for equality predicates.

`OrderBy(expression)` replaces the full previous ordering, including any vector argument installed by `Nearest`.

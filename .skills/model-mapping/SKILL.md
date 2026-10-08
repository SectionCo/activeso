# ActiveSo model mapping

## Fields and columns

- Every exported field other than the embedded `activeso.Record` needs a `db` tag naming its column; `Model[T]` panics for an untagged field. `db:"-"` excludes a field. Column names are never derived from field names. Two fields cannot map to the same column (compared case-insensitively).
- Table names default to the English-pluralized snake_case type name (`User` -> `users`, `Person` -> `people`, `Users` -> `users`); implement `TableName() string` to override it.

## Primary keys

- A model embeds `activeso.Record` and has exactly one primary key.
- The `primary_key` hint selects a primary-key field. Without it, the field mapped to `id` is the primary key.
- Primary keys must be immutable scalar Go types: strings, booleans, signed integers, `uint8`/`uint16`/`uint32`, and floats. `uint` and `uint64` are rejected because their full range exceeds Turso's signed 64-bit INTEGER; mutable values such as `[]byte` are also rejected.
- The table must protect the primary-key column with a sole `PRIMARY KEY` or a single-column unique index. This is checked structurally before the first operation; composite primary keys do not uniquely identify one modeled ID. See `../schema-verification/SKILL.md`.

## Hints

Hints (the `activeso` struct tag) describe the schema the application owns and never change it: `primary_key`, `unique`, `unique_with=column[+column...]`, `index`, `not_null`, `belongs_to=table(column)`, and `on_delete=cascade` (requires `belongs_to`). Unknown or malformed hints panic at `Model` setup. `unique` and `unique_with` cannot be combined on one field; `unique_with` cannot be on the primary key, cannot list its own column, and must reference mapped columns. `belongs_to` targets must be simple identifiers. On the embedded `Record` the only hint is `timestamps` (see `../timestamp-replay/SKILL.md`).

Enforcement of foreign keys requires `PRAGMA foreign_keys = ON` on the writing connection; `Verify` only checks that the foreign key exists.

## NULL handling and nullable wrappers

- Reads map `NULL` to zero values for plain strings, numbers, and booleans, so rows written before a column was filled stay readable. Use `database/sql` nullable types when `NULL` must stay distinguishable: `sql.NullString`, `sql.NullInt64`, `sql.NullFloat64`, and `sql.NullBool` map to TEXT, INTEGER, REAL, and INTEGER columns respectively (`sqlColumnType`, used by `Verify`).
- Reading a `Vector32` column that is NULL fails with `Conversion error: Expected blob value` through the `vector_extract` projection. Tests supply a real vector where a vector column is read.

## Indexes

The `index` hint asserts an index begins with the column; `FindBy(ctx, column, value)` safely queries any mapped column and returns all matches. ActiveSo no longer generates or names indexes.

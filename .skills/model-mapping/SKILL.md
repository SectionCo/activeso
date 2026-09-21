# ActiveSo model mapping

## Primary keys

- A model embeds `activeso.Record` and has exactly one primary key.
- `activeso:"primary_key"` explicitly selects a primary-key field. Without that tag, the field mapped to `id` is the primary key.
- Primary keys must be immutable scalar Go types: strings, booleans, signed integers, `uint8`/`uint16`/`uint32`, and floats. `uint` and `uint64` are rejected during model construction because their full range exceeds Turso's signed 64-bit INTEGER representation; mutable values such as `[]byte` are also rejected.

## Additive migrations and nullable reads

- `AutoMigrate` compares SQLite column names case-insensitively.
- New nullable scalar columns leave existing rows with SQL `NULL`.
- Reads map `NULL` to zero values for plain strings, numbers, and booleans; use `database/sql` nullable types when `NULL` must remain distinguishable. `sql.NullString`, `sql.NullInt64`, `sql.NullFloat64`, and `sql.NullBool` map to TEXT, INTEGER, REAL, and INTEGER columns respectively.

## Unique indexes

- ActiveSo-generated unique-index names hex-encode table and column names, making SQLite-global index names unambiguous for every table/column pair.

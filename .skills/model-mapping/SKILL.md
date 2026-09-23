# ActiveSo model mapping

## Primary keys

- A model embeds `activeso.Record` and has exactly one primary key.
- `activeso:"primary_key"` explicitly selects a primary-key field. Without that tag, the field mapped to `id` is the primary key.
- `AutoMigrate` rejects an existing table unless the modeled primary-key column is protected by a sole primary key or a single-column unique index. Composite primary keys do not uniquely identify one modeled ID. It also rejects a modeled primary-key storage-affinity change; primary-key rebuilds require a dedicated manual migration.
- Primary keys must be immutable scalar Go types: strings, booleans, signed integers, `uint8`/`uint16`/`uint32`, and floats. `uint` and `uint64` are rejected during model construction because their full range exceeds Turso's signed 64-bit INTEGER representation; mutable values such as `[]byte` are also rejected.

## Foreign keys

- `activeso:"belongs_to=table(column)"` on a scalar field, including the primary-key field, adds an inline `REFERENCES table(column)` foreign key. Target table and column names must be simple identifiers, preventing tag content from becoming arbitrary SQL.
- `AutoMigrate` creates the foreign key for new tables and for missing nullable linked columns. It intentionally does not retrofit a new foreign key onto an existing column; use a dedicated migration for that destructive schema change.
- Combine `belongs_to` with `not_null` on the foreign-key field when the relationship is required. Foreign-key enforcement remains the database's responsibility.

## Additive migrations and nullable reads

- `AutoMigrate` compares SQLite column names case-insensitively. It directs nullable-to-`not_null` transitions on existing columns to `SetNotNull` rather than silently accepting them.
- New nullable scalar columns leave existing rows with SQL `NULL`.
- Reads map `NULL` to zero values for plain strings, numbers, and booleans; use `database/sql` nullable types when `NULL` must remain distinguishable. `sql.NullString`, `sql.NullInt64`, `sql.NullFloat64`, and `sql.NullBool` map to TEXT, INTEGER, REAL, and INTEGER columns respectively.

## Indexes

- `activeso:"index"` creates a non-unique single-column index during `AutoMigrate`; `FindBy(ctx, column, value)` safely queries any mapped column and returns all matches.

- `activeso:"unique_with=column[+column...]"` creates an ordered composite unique index beginning with the tagged field. Each referenced column must be mapped, distinct, and different from the tagged field. `AutoMigrate` creates these indexes, but requires `DropUniqueWith(ctx, columns...)` after the tag is removed rather than dropping them implicitly.
- ActiveSo-generated ordinary and unique index names lowercase then hex-encode table and column names, making SQLite-global index names unambiguous and stable across case-only mapping changes. Managed-index lookup also compares SQLite table names case-insensitively. Single-column unique index names end in `_unique`, composite unique index names end in `_unique_with`, and ordinary index names end in `_index`.
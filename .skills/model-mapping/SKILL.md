# ActiveSo model mapping

## Primary keys

- A model embeds `activeso.Record` and has exactly one primary key.
- `activeso:"primary_key"` explicitly selects a primary-key field. Without that tag, the field mapped to `id` is the primary key.
- Primary keys must be immutable scalar Go types: strings, booleans, signed or unsigned integers, and floats. Mutable values such as `[]byte` are rejected during model construction.

## Additive migrations and nullable reads

- `AutoMigrate` compares SQLite column names case-insensitively.
- New nullable scalar columns leave existing rows with SQL `NULL`.
- Reads map `NULL` to zero values for plain strings, numbers, and booleans; use nullable `database/sql` types when `NULL` must remain distinguishable.

## Unique indexes

- ActiveSo-generated unique-index names hex-encode table and column names, making SQLite-global index names unambiguous for every table/column pair.

# Schema Verification and Bring-Your-Own Schema

ActiveSo 2.0 never creates or alters tables. `db` tags and `activeso` hints describe a schema the application owns; `verify.go` checks them.

## Two levels of checking

- `ensureReady` runs the structural check once per model before the first `Create`, query, `Save`, or `Delete`. It reports a missing table, a missing mapped column, missing `created_at`/`updated_at` when the `timestamps` hint is set, and a primary-key column that is neither a sole `PRIMARY KEY` nor covered by a single-column unique index (otherwise `Save`/`Delete` could touch several rows). Only a passing result is cached (`ready` under `readyMutex`), so a table created later is picked up on the next call. A `Verify` that returns no problems through the root executor also sets `ready`. Models should be created once and reused.
- `Verify(ctx)` adds hint drift: Go type affinity versus column type (a field type with no SQL mapping, such as a map, is reported as a problem unless it implements `driver.Valuer` and a pointer `sql.Scanner`), `not_null`, `unique` (a complete unique index over exactly that column), `unique_with` (a unique index over exactly the owner column then the listed columns, in order), `index` (any complete index starting with the column), `belongs_to` (a foreign key to that table and column; `REFERENCES parent` with no column is resolved to the parent's sole primary-key column, and a composite or missing parent key matches nothing), and `on_delete=cascade`. Primary keys satisfy `not_null` and `index`. Partial indexes never count.

Both return `*SchemaError`, which matches `ErrSchemaMismatch` with `errors.Is` and lists every problem in `Problems`. Column and table comparisons are case-insensitive.

## Verified Turso behavior

Verified with the pinned Turso SDK (see `../turso-upgrades/SKILL.md`):

- `PRAGMA table_info`, `index_list`, `index_info`, and `foreign_key_list` work through a plain `*sql.DB`. `table_info` returns no rows for a missing table, which is how absence is detected. A `foreign_key_list` row has eight columns: id, seq, table, from, to, on_update, on_delete, match. `to` can be NULL when the key references the parent's primary key, so it is scanned as `sql.NullString` and then resolved with `PRAGMA table_info` on the parent (`primaryKeyColumn`).
- Close one PRAGMA result set before issuing the next query on the same handle; `inspectIndexes` collects index names first, then reads each index's columns.
- `ALTER TABLE ... RENAME COLUMN` works, and `DROP TRIGGER IF EXISTS` is accepted. This is the documented 1.x upgrade path for the timestamp columns.
- A `CREATE TRIGGER` statement contains semicolons, so a test helper that splits DDL on `;` (`execStatements` in `model_test.go`) cannot create triggers; use `db.ExecContext` directly.

## Hints versus behavior

Hints never change the database. The only hints with write-time effect are `primary_key` (identity) and `unique`/`unique_with` (they determine how a Turso unique violation is reported). `uniqueWriteError` matches Turso's constraint error and attributes it to a single-column `unique` field when the column name appears in the message; composite violations return a bare `ErrUnique`. There is no check-then-write preflight, so the unique index must exist in the schema.

`db` tags are mandatory for every exported non-`Record` field (`db:"-"` skips). Columns present in the table but absent from the struct are ignored.

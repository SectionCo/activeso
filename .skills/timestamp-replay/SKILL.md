# Opt-In Timestamps

## Behavior

- A model gets managed timestamps only when its embedded record carries the hint: ``activeso.Record `activeso:"timestamps"` ``. Without it ActiveSo never selects, writes, or parses `created_at`/`updated_at`, and `Record.CreatedAt`/`UpdatedAt` stay zero.
- The columns are `created_at` and `updated_at` (renamed from 1.x `activeso_created_at`/`activeso_updated_at`) holding UTC text. `timestampValue` parses SQL date-time (`YYYY-MM-DD HH:MM:SS`) or RFC3339Nano.
- ActiveSo writes timestamps in its own SQL: `Create` inserts `CURRENT_TIMESTAMP` for both columns; `Save` adds `updated_at = CURRENT_TIMESTAMP` and never touches `created_at`. The user's table needs no defaults and no triggers, and there are no ActiveSo-managed triggers anywhere. Because `Save` always has the `updated_at` assignment, even an ID-only model issues a real `UPDATE` (so `Save` returns `ErrNotFound` for a deleted row through `RowsAffected`).
- After each write `refreshTimestamps` reads both columns back so the Go fields match the database. Reads project the columns after the model fields (`selectColumns`) and `scan` fills them.
- Assignments to `Record.CreatedAt`/`UpdatedAt` are never persisted. `Bind` does not load timestamps.
- A struct field mapped to `created_at` or `updated_at` is rejected at `Model` setup when the hint is set.
- A missing timestamp column is a structural `SchemaError` naming both column names; an unparseable stored value returns a parse error that names them too. `Verify` also expects TEXT affinity.

## Replay and sync

Writes outside ActiveSo (raw SQL, replayed row images) are not intercepted: they do not bump `updated_at`, and a replayed row image carries whatever timestamps it was captured with. Timestamps are metadata, not an authorization boundary or ordering guarantee. Local tests do not validate a remote Push/Pull cycle; validate that through the consumer's sync SDK. Never inspect a live synced replica through SQLite, a plain non-sync Turso connection, or the CLI.

## Upgrading a 1.x database

1.x installed insert/update triggers (and, before 1.0.5, protection triggers) named `activeso_<hex of lowercase table>_timestamps_{insert,update,protect_created_at,protect_updated_at}`. Drop them before renaming the columns, because a column rename rewrites trigger bodies and would leave them pointing at missing columns. See the README's "Upgrading from 1.x".

## Coverage

`model_test.go`: opt-in versus no-hint behavior, missing-column errors, `Save` refreshing `updated_at` while `created_at` stays fixed (the test backdates both columns because `CURRENT_TIMESTAMP` has one-second resolution), and an ID-only timestamped model. Tests use isolated local databases.

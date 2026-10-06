# Timestamp Replay Policy

## Replay boundary

Timestamp comparisons cannot distinguish a replayed row image from a direct application write. The trigger policy is value-based, not replay detection. Do not invent a native replay-context API or treat timestamp equality as authentication of a write's origin.

Local timestamp behavior does not establish end-to-end remote Push/Pull correctness; validate that separately through the consumer's sync SDK.

## Current guarantees and tradeoffs

- Fresh tables retain TEXT NOT NULL timestamp columns with `CURRENT_TIMESTAMP` defaults. Legacy tables retain nullable columns, migration backfill, and an insert trigger that fills omitted/NULL timestamps.
- The data-column AFTER UPDATE trigger refreshes `activeso_updated_at` only when `NEW.activeso_updated_at IS OLD.activeso_updated_at`. This includes no-op data assignments such as `SET email = email`.
- If a statement changes the update timestamp, the supplied value is preserved even when data columns are also updated. Supplied timestamps are not required to equal the destination's current time.
- A supplied update timestamp equal to the old value cannot be distinguished from an omitted timestamp; it refreshes on data-column writes. This policy does not guarantee exact original timestamps for every possible replay.
- Timestamp-only updates do not invoke the data-column refresh trigger. `IS` provides NULL-safe comparison on legacy columns. Direct SQL can still set legacy nullable timestamps to NULL; fresh NOT NULL constraints remain authoritative.
- Neither timestamp is SQL-protected against replacement. Creation time remains unchanged on ordinary ActiveSo/data-only updates, but direct SQL and row images can replace it. This explicitly gives up SQL-level immutability to avoid a second potential row-image rejection path.
- ActiveSo `Create`/`Save` do not persist assignments to `Record.CreatedAt` or `Record.UpdatedAt`; reads and successful writes populate/refresh those fields from database values.
- Callers supplying timestamps must use valid UTC timestamp text accepted by `timestampValue` (SQL date/time or RFC3339Nano). These timestamps are metadata, not an authorization boundary or a global ordering guarantee.

## Upgrade and schema rebuilds

A package update alone cannot alter existing database triggers. Run `AutoMigrate` for each affected model. `createTimestampTriggers` drops the deterministic old protection-trigger names as well as the insert/update triggers, then reinstalls only insert and conditional-update behavior in the migration transaction. Repeated `AutoMigrate` retains this policy. Update every database that may execute replayed writes; mixed old/new trigger policies can still reject writes.

`managedTimestampTrigger` in `migration.go` intentionally continues recognizing the two old protection names so supported targeted table rebuilds can accept old schemas and replace their managed triggers using the new policy.

Never inspect a live synced replica through SQLite, a plain non-sync Turso connection, or the CLI. Use the sync SDK or an offline copy. Tests here use isolated local databases, not live replicas.

## Current coverage

- `model_test.go` covers timestamp defaults, legacy-table backfill and insert behavior, timestamp-only replacement, ordinary automatic refresh with unchanged creation time, and Go timestamp loading/Save behavior.
- Tests use isolated local databases, not live replicas. Local coverage does not verify a complete remote sync cycle.
- Reading a `testUser` with a NULL embedding fails with `Conversion error: Expected blob value` through the `vector_extract` projection. Timestamp Find/Save coverage supplies a real vector to isolate timestamp behavior.
- See `../turso-upgrades/SKILL.md` for the current dependency versions and validation workflow.

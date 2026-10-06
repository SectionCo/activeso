# Timestamp Replay Policy

## Root cause and SDK boundary

The former `protect_updated_at` trigger rejected any changed update timestamp that was not equal to `CURRENT_TIMESTAMP`. A delayed row-image update carrying an earlier trigger-generated timestamp therefore aborted the entire data update. `TestTimestampReplay` reproduced that failure against the actual Turso engine on both newly created and legacy tables before the fix.

Read-only inspection of installed `tursogo` v0.7.2 and v0.8.1 found byte-identical `driver_sync.go` and `bindings_sync.go`. `Pull` describes rebasing local changes and calls native wait/apply operations, but these Go interfaces expose no supported SQL replay predicate, mutation-origin hook, or replay trigger-bypass control. Native replay symbols support logical-replay implementation evidence, not an application API. Do not invent replay detection or treat timestamp equality as authentication of a write's origin.

The reported consumer background Pull failure is consistent with the reproduced guard conflict. The exact failing native statement and a complete remote Push/Pull reproduction remain unverified.

## Current guarantees and tradeoffs

- Fresh tables retain TEXT NOT NULL timestamp columns with `CURRENT_TIMESTAMP` defaults. Legacy tables retain nullable columns, migration backfill, and an insert trigger that fills omitted/NULL timestamps.
- The data-column AFTER UPDATE trigger refreshes `activeso_updated_at` only when `NEW.activeso_updated_at IS OLD.activeso_updated_at`. This includes no-op data assignments such as `SET email = email`.
- If a statement changes the update timestamp, the supplied value is preserved even when data columns are also updated. Historical row images no longer fail the old current-time guard.
- A supplied update timestamp equal to the old value cannot be distinguished from an omitted timestamp; it refreshes on data-column writes. This policy does not guarantee exact original timestamps for every possible replay.
- Timestamp-only updates do not invoke the data-column refresh trigger. `IS` provides NULL-safe comparison on legacy columns. Direct SQL can still set legacy nullable timestamps to NULL; fresh NOT NULL constraints remain authoritative.
- Neither timestamp is SQL-protected against replacement. Creation time remains unchanged on ordinary ActiveSo/data-only updates, but direct SQL and row images can replace it. This explicitly gives up SQL-level immutability to avoid a second potential row-image rejection path.
- ActiveSo `Create`/`Save` do not persist assignments to `Record.CreatedAt` or `Record.UpdatedAt`; reads and successful writes populate/refresh those fields from database values.
- Callers supplying timestamps must use valid UTC timestamp text accepted by `timestampValue` (SQL date/time or RFC3339Nano). These timestamps are metadata, not an authorization boundary or a global ordering guarantee.

## Upgrade and schema rebuilds

A package update alone cannot alter existing database triggers. Run `AutoMigrate` for each affected model. `createTimestampTriggers` drops the deterministic old protection-trigger names as well as the insert/update triggers, then reinstalls only insert and conditional-update behavior in the migration transaction. Repeated `AutoMigrate` retains this policy. Update every database that may execute replayed writes; mixed old/new trigger policies can still reject writes.

`managedTimestampTrigger` in `migration.go` intentionally continues recognizing the two old protection names so supported targeted table rebuilds can accept old schemas and replace their managed triggers using the new policy.

Never inspect a live synced replica through SQLite, a plain non-sync Turso connection, or the CLI. Use the sync SDK or an offline copy. Tests here use isolated local databases, not live replicas.

## Regression coverage

- `timestamp_replay_test.go`: captures a real automatically generated timestamp, crosses the SQL clock's second boundary, and replays it with data and a different creation timestamp; verifies exact row values on fresh and legacy tables across repeated migrations. Also tests equal supplied timestamps and no-op data refresh.
- Upgrade coverage installs the old rejecting guards and unconditional refresh trigger, verifies the former rejection, and confirms repeated migration removes guards and preserves changed replay timestamps.
- `model_test.go`: verifies defaults, timestamp-only replacement, ordinary automatic refresh with unchanged creation time, and Go timestamp loading/Save behavior.
- Initial validation passed on the then-pinned Turso v0.7.2 and an isolated alternate modfile selecting v0.8.1. The repository now pins v0.8.2; the full suite, including these timestamp regressions, also passes on that release. See `../turso-upgrades/SKILL.md` for the dependency upgrade workflow. No remote credentials or databases were used; end-to-end remote sync is not established by these tests.
- Fixture constraint verified while adding Find/Save coverage on both SDK versions: reading a `testUser` with a NULL embedding fails with `Conversion error: Expected blob value` through the existing `vector_extract` projection. The timestamp test supplies a real vector to isolate timestamp behavior; this unrelated nullable-vector read limitation was not changed.

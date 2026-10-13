# Turso Cloud Sync (`OpenSync`)

`activeso.OpenSync(ctx, SyncConfig)` in `sync.go` is the only way to open an ActiveSo database. It wraps `turso.NewTursoSyncDb` (tursogo v0.8.2) and returns a `*Sync` whose `DB()` feeds `activeso.Model`. `Model` itself still takes any `*sql.DB`, which is why internal tests may use a plain local `sql.Open("turso", path)`.

## Config and semantics

- `SyncConfig` is ActiveSo's own struct (field `RemoteURL`, not the SDK's `RemoteUrl`). `Path` and `RemoteURL` are required; `OpenSync` rejects empty values before touching the SDK. `SkipBootstrap` maps to the SDK's `BootstrapIfEmpty=false`.
- `RemoteURL` docs and examples use the `turso://<db>-<org>.<region>.turso.io` form. The SDK (`normalizeUrl` in `driver_sync.go`, v0.8.2) rewrites `turso://` and `libsql://` to `https://` and leaves `http://` alone, so older `libsql://` URLs still work. Use `turso://` in new docs, not `libsql://` or `https://`.
- `Pull` fetches remote changes and rebases local ones on top; it sends nothing. `Push` sends local changes; it fetches nothing. `Sync` is `Pull` then `Push`. `Checkpoint` compacts the local WAL (SDK `Checkpoint`); callers should run it periodically on long-lived replicas. `Stats` returns the SDK's `TursoSyncDbStats` (aliased as `SyncStats`); `CdcOperations` counts local operations written since the last `Pull` (per the SDK); it is not a count of unpushed writes and does not reset on `Push`.
- ActiveSo never pushes on its own. Model writes stay local until the caller pushes.
- Users own the schema on the cloud database as well; ActiveSo never creates or alters tables. Use `Verify` to catch drift, including after a `Pull`.

## Close

`sql.DB.Close` alone does NOT release the files: with tursogo v0.8.2 and v0.8.3, `lsof` still shows the database, `-wal`, and `-changes` files open after it, because `TursoSyncDb` has no public close and never frees its native handle. `Sync.Close` (`sync.go`) closes the pool, then calls `releaseRemote` (`sync_release.go`), which uses an exported `Close() error` on `TursoSyncDb` if a future SDK adds one, and otherwise links the SDK's own `turso_sync_database_deinit` with `//go:linkname` (an empty `sync_release.s` makes the bodyless declaration legal) and passes it the handle read by reflection from `TursoSyncDb.db`. Verified: three open files before, none after, and a new `OpenSync` on the same path still reads the unpushed rows. A renamed symbol fails the build; a changed field layout makes `Close` return an error instead of crashing.

Freeing the handle while a connection still uses it would crash, so `Close` marks the handle closed (`Pull`/`Push`/`Stats` return `ErrSyncClosed`), closes the pool, and returns `ErrSyncBusy` without releasing while `db.Stats().OpenConnections > 0`; calling `Close` again after returning the connections finishes the release. A read-write mutex makes `Close` wait for in-flight sync calls; `Sync` holds one read lock across its pull and push so `Close` cannot land between them (`TestSyncCloseWaitsForWholeSync`). Connections checked out from `DB()` keep working until returned, which is why `Close` reports `ErrSyncBusy`. Prefer an upstream `Close` on `TursoSyncDb` and drop the linkname once it exists; recheck on every SDK upgrade.

## Foreign keys

`PRAGMA foreign_keys` is per connection and the sync connector is built inside the SDK, so ActiveSo cannot wrap it. Model writes are safe because `model.execWrite` (`connection.go`) sets the pragma on the exact `*sql.Conn` that runs each `Create`/`Save`/`Delete`. Direct SQL on `Sync.DB()` is not enforced unless the caller sets the pragma on its own `*sql.Conn`.

## Verified locally (no Turso Cloud account)

Tests in `sync_test.go` use `SkipBootstrap` and an `httptest` server that answers 503:

- Opening with `SkipBootstrap` makes no network request; models create, find, save, and delete normally on the synced handle.
- `Pull`, `Push`, and `Sync` against the failing remote return errors and leave the local rows intact.
- `Stats().CdcOperations` increases after a model `Create`. Its reset-on-`Pull` meaning comes from the SDK documentation, not from a test.
- Orphaning `Create`/`Save` are rejected across a held pool of four connections on the synced handle.
- `Close` frees every database file (`TestSyncCloseReleasesFiles`, via `/proc/self/fd` or `lsof`) and returns `ErrSyncBusy` while a connection is checked out (`TestSyncCloseWaitsForBusyConnections`).

Not verified: a successful `Pull`/`Push` round trip, rebase-conflict behavior, and whether schema changes replicate. The SDK's own test server is in a `_test.go` file and cannot be imported. `TestSyncAgainstTursoCloud` runs a real round trip when `ACTIVESO_SYNC_URL` (and `ACTIVESO_SYNC_TOKEN`) are set; it is skipped otherwise. Never open a live synced replica with a plain non-sync connection or SQLite.

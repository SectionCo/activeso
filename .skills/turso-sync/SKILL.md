# Turso Cloud Sync (`OpenSync`)

`activeso.OpenSync(ctx, SyncConfig)` in `sync.go` is the only way to open an ActiveSo database. It wraps `turso.NewTursoSyncDb` (tursogo v0.8.2) and returns a `*Sync` whose `DB()` feeds `activeso.Model`. `Model` itself still takes any `*sql.DB`, which is why internal tests may use a plain local `sql.Open("turso", path)`.

## Config and semantics

- `SyncConfig` is ActiveSo's own struct (field `RemoteURL`, not the SDK's `RemoteUrl`). `Path` and `RemoteURL` are required; `OpenSync` rejects empty values before touching the SDK. `SkipBootstrap` maps to the SDK's `BootstrapIfEmpty=false`.
- `Pull` fetches remote changes and rebases local ones on top; it sends nothing. `Push` sends local changes; it fetches nothing. `Sync` is `Pull` then `Push`. `Stats` returns the SDK's `TursoSyncDbStats` (aliased as `SyncStats`); `CdcOperations` counts local writes not yet pulled/pushed.
- ActiveSo never pushes on its own. Model writes stay local until the caller pushes.
- Users own the schema on the cloud database as well; ActiveSo never creates or alters tables. Use `Verify` to catch drift, including after a `Pull`.

## Close

The SDK has no public close for `TursoSyncDb` (its native sync handle is never deinitialized by the public API) and its HTTP client is unexported. `Sync.Close` therefore closes the `*sql.DB`, which closes every pooled connection, marks the handle closed, and makes `Pull`/`Push`/`Stats` return `ErrSyncClosed`. A read-write mutex makes `Close` wait for in-flight sync calls. A second `Close` is a no-op. Releasing the SDK handle needs an upstream change.

## Foreign keys

`PRAGMA foreign_keys` is per connection and the sync connector is built inside the SDK, so ActiveSo cannot wrap it. Model writes are safe because `model.execWrite` (`connection.go`) sets the pragma on the exact `*sql.Conn` that runs each `Create`/`Save`/`Delete`. Direct SQL on `Sync.DB()` is not enforced unless the caller sets the pragma on its own `*sql.Conn`.

## Verified locally (no Turso Cloud account)

Tests in `sync_test.go` use `SkipBootstrap` and an `httptest` server that answers 503:

- Opening with `SkipBootstrap` makes no network request; models create, find, save, and delete normally on the synced handle.
- `Pull`, `Push`, and `Sync` against the failing remote return errors and leave the local rows intact.
- `Stats().CdcOperations` increases after a model `Create`.
- Orphaning `Create`/`Save` are rejected across a held pool of four connections on the synced handle.

Not verified: a successful `Pull`/`Push` round trip, rebase-conflict behavior, and whether schema changes replicate. The SDK's own test server is in a `_test.go` file and cannot be imported. `TestSyncAgainstTursoCloud` runs a real round trip when `ACTIVESO_SYNC_URL` (and `ACTIVESO_SYNC_TOKEN`) are set; it is skipped otherwise. Never open a live synced replica with a plain non-sync connection or SQLite.

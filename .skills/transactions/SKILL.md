# Transactions and Executors

## Public surface

- `activeso.Executor` is the interface ActiveSo needs (`ExecContext`, `QueryContext`, `QueryRowContext`). `*sql.DB`, `*sql.Tx`, and `*sql.Conn` satisfy it. `Model[T](db Executor)` accepts it, so existing `*sql.DB` callers compile unchanged. Nil and typed-nil executors (`(*sql.DB)(nil)`) are rejected by `nilExecutor`.
- `model.Using(exec)` returns a shallow copy of the model bound to `exec`. Metadata is copied; `readiness` is shared by pointer. Records and queries produced by the view keep that executor, so `Save`, `Delete`, and `Where(...).All` stay in the transaction. `Using(tx).Bind(record)` moves an existing record into one.
- Per-call twins: `model.CreateTx(ctx, tx, value)`, `model.FindTx(ctx, tx, id)`, `Record.SaveTx(ctx, tx)`, and `Record.DeleteTx(ctx, tx)`. Each borrows an executor for one call via `viewOn`/`recordBinding.using`. Records returned by `CreateTx` and `FindTx` are rebound to the original (unscoped) model, and `SaveTx`/`DeleteTx` never rebind, so a later plain `Save` uses the model's own executor. A nil or typed-nil `tx` returns an error from these methods; only `Using` panics. There is deliberately no `FindByTx`, `AllTx`, or query-level Tx variants: queries and multi-step work in a transaction go through `Using`, documented in the README as the route for more complex transactions.
- ActiveSo never begins, commits, or rolls back; the caller owns the transaction. A finished transaction surfaces `sql.ErrTxDone`.

## Readiness caching

`ensureReady` caches a passing structural check in the shared `readiness`, but only when run through the root executor (`scoped == false`). A scoped view that passes still re-inspects next time, because a transaction can see uncommitted DDL. `TestScopedViewDoesNotCacheReadiness` covers this. A model whose first and only operations are `Using`/`SaveTx` calls therefore pays the PRAGMA inspection on each call; run one non-transaction operation or `Verify` first to warm it.

## Verified Turso behavior

Verified with tursogo v0.8.2 (see `../turso-upgrades/SKILL.md`):

- The driver implements `driver.ConnBeginTx` and issues a plain `BEGIN` (snapshot isolation). It ignores `driver.TxOptions`, so isolation level and read-only requests have no effect.
- `CREATE TABLE` inside a transaction is visible to PRAGMA inspection on the same transaction and disappears on `Rollback`.
- `PRAGMA` inspection, unique-violation mapping (`UniqueError`/`ErrUnique`), and `RowsAffected` all work through `*sql.Tx`.
- Reading a NULL `Vector32` column still fails inside a transaction (see `../model-mapping/SKILL.md`); transaction tests give models an embedding.

## Connection pools and locking

- `readiness` is a lone `atomic.Bool`; `ensureReady` never holds a lock while inspecting. A mutex held across the PRAGMA queries deadlocked a one-connection pool (`SetMaxOpenConns(1)`): a root-model call took the lock and waited for the connection a transaction held, and that transaction's own `Using` call then waited on the lock and could never `Commit`. The cost is that concurrent first calls may each inspect, which is harmless because the check only reads. `TestScopedViewDoesNotWaitBehindRootInspection` covers it.
- Plain `database/sql` behavior still applies: on a one-connection pool, any call through the root model (`Create`, `Find`, `All`, queries, `Verify`, or `Save`/`Delete` on a record bound to the root model) blocks until the open transaction ends. Inside a transaction use `Using(tx)` or the `*Tx` methods. Records from `CreateTx` and `FindTx` are bound to the root model, so call `SaveTx`, not `Save`, on them while the transaction is open.

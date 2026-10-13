# ActiveSo Echo example

A small [Echo v5](https://echo.labstack.com/) web application that demonstrates ActiveSo’s active-record workflow with a single user-management page.

The example and parent ActiveSo module pin `turso.tech/database/tursogo` and its native platform libraries to **v0.8.2**. The example uses the parent checkout through its local `replace` directive, so no published ActiveSo release is required.

## Run it

From the repository root:

```sh
TURSO_DATABASE_URL=libsql://my-db-org.turso.io TURSO_AUTH_TOKEN=... go -C example run .
```

Open [http://localhost:8080](http://localhost:8080).

The example uses `activeso.OpenSync` to sync `example/activeso-example.db` with the Turso Cloud database you name, syncs after each create, update, or delete, and shows the user any sync failure straight away. The change stays saved locally, and the user decides when to retry with the "Sync now" button; the example never retries on its own. Delete the local file to re-bootstrap it from the remote.

## What it demonstrates

- `activeso.OpenSync(ctx, config)` opens a local database synced with Turso Cloud.
- `activeso.Model[User](db)` binds the Go model to the `users` table.
- Creating a user with `userModel.Create(ctx, User{...})`.
- Finding a user with `userModel.Find(ctx, id)` via the find form.
- Updating a bound record with `user.Save(ctx)`.
- Deleting a bound record with `user.Delete(ctx)`.
- Creating several users in one transaction with `userModel.CreateTx(ctx, tx, User{...})`: the example begins and commits the transaction itself, and a duplicate email rolls back the whole batch.

## Routes

| Method | Path | Behavior |
| --- | --- | --- |
| `GET` | `/` | Renders the create form, find form, and existing users. |
| `GET` | `/users/find` | Finds a user by ID from the `id` query parameter and renders the page with the result or a message. |
| `POST` | `/users` | Creates a user from an email address. |
| `POST` | `/sync` | Pulls remote changes and pushes local ones, then redirects home, or shows the failure on the page. |
| `POST` | `/users/batch` | Creates one user per line of the `emails` field in a single transaction; if any fails, none are saved. |
| `POST` | `/users/:id` | Updates a user's email address. |
| `POST` | `/users/:id/delete` | Deletes a user. |

## Verify

Run the example module’s tests from the repository root. They include a Turso-backed create/find/save/delete lifecycle test using a temporary database; they do not open `activeso-example.db`:

```sh
go -C example test ./...
go -C example vet ./...
go -C example build -o /dev/null .
```

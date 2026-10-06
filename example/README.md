# ActiveSo Echo example

A small [Echo v5](https://echo.labstack.com/) web application that demonstrates ActiveSo’s active-record workflow with a single user-management page.

The example and parent ActiveSo module pin `turso.tech/database/tursogo` and its native platform libraries to **v0.8.2**. The existing `turso.NewConnector` / `sql.OpenDB` setup remains compatible. The example uses the parent checkout through its local `replace` directive, so no published ActiveSo release is required.

## Run it

From the repository root:

```sh
go -C example run .
```

Open [http://localhost:8080](http://localhost:8080).

The application creates `example/activeso-example.db` if it does not already exist. Delete that file to reset the example data.

## What it demonstrates

- `activeso.Model[User](db)` binds the Go model to the `users` table.
- Creating a user with `userModel.Create(ctx, User{...})`.
- Finding a user with `userModel.Find(ctx, id)` via the find form.
- Updating a bound record with `user.Save(ctx)`.
- Deleting a bound record with `user.Delete(ctx)`.

## Routes

| Method | Path | Behavior |
| --- | --- | --- |
| `GET` | `/` | Renders the create form, find form, and existing users. |
| `GET` | `/users/find` | Finds a user by ID from the `id` query parameter and renders the page with the result or a message. |
| `POST` | `/users` | Creates a user from an email address. |
| `POST` | `/users/:id` | Updates a user's email address. |
| `POST` | `/users/:id/delete` | Deletes a user. |

## Verify

Run the example module’s tests from the repository root. They include a Turso-backed create/find/save/delete lifecycle test using a temporary database; they do not open `activeso-example.db`:

```sh
go -C example test ./...
go -C example vet ./...
go -C example build -o /dev/null .
```

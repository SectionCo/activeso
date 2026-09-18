# ActiveSo Echo example

A small [Echo v5](https://echo.labstack.com/) web application that demonstrates ActiveSo’s active-record workflow with a single user-management page.

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
- Finding a user with `userModel.Find(ctx, id)`.
- Updating a bound record with `user.Save(ctx)`.
- Deleting a bound record with `user.Delete(ctx)`.

## Routes

| Method | Path | Behavior |
| --- | --- | --- |
| `GET` | `/` | Renders the create form and existing users. |
| `POST` | `/users` | Creates a user from an email address. |
| `POST` | `/users/:id` | Updates a user's email address. |
| `POST` | `/users/:id/delete` | Deletes a user. |

## Verify

Run the example module’s tests from the repository root:

```sh
go -C example test ./...
go -C example vet ./...
```

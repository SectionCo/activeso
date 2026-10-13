# Example user lookup

The Echo example's find form submits `GET /users/find?email=...`. `registerFindUserRoute` in `example/main.go` trims surrounding whitespace and queries `userModel.FindBy(ctx, "email", email)` using exact equality; it does not perform partial or case-insensitive searches.

`FindBy` returns a slice, with an empty slice for a missing email. The application's `users_email` unique index guarantees at most one match. Empty input and missing matches render explanatory messages; a match renders the found user above the existing users list.

Save and delete routes remain ID-based and use `Find(ctx, id)`. `TestFindUserByEmail` exercises the registered HTTP route against a temporary Turso database without touching the running example's database.

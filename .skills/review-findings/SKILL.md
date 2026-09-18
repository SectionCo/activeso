# Verified Persistence Edge Cases

Verified against the working tree and pinned Turso v0.7.2 on 2026-09-18 using focused Go tests in a temporary repository copy. These describe current limitations, not desired behavior. Update or remove each entry when fixed.

## Table rebuild scope — resolved

Column migrations now edit the existing database definition only at the named target and preserve unrelated columns and indexes. See `../targeted-migrations/SKILL.md` for the supported behavior and limitations.

## Nullable additions and reads — resolved for plain strings

Plain Go string fields scan through `sql.NullString`; SQL `NULL` becomes `""`. Existing rows remain readable after `AutoMigrate` adds a nullable string field. This intentionally collapses the distinction between NULL and an empty string, and saving the record writes an empty string. Models can instead use `sql.NullString`, which maps to TEXT and preserves its `Valid` flag. Pointer fields remain unsupported for automatic schema generation.

## Bound record identity — resolved

Record binding captures the original primary-key value. Save and Delete reject a record whose public ID no longer matches with `ErrIDChanged`, preventing writes or deletion of another record.

## Replacing vector ordering — resolved

`OrderBy` clears `orderByArguments` when replacing a previous ordering, so it can safely replace `Nearest` without leaving an obsolete vector parameter.

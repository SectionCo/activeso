# Verified Persistence Edge Cases

Verified against the working tree and pinned Turso v0.7.2 on 2026-09-18 using focused Go tests in a temporary repository copy. These describe current limitations, not desired behavior. Update or remove each entry when fixed.

## Table rebuilds and nullable additions — removed in 2.0

ActiveSo 2.0 no longer migrates schemas, so the table-rebuild and additive-migration findings no longer apply. Plain Go strings and numbers still read SQL `NULL` as zero values, and `sql.NullString` still preserves `Valid`.

## Bound record identity — resolved

Record binding captures the original primary-key value. Save and Delete reject a record whose public ID no longer matches with `ErrIDChanged`, preventing writes or deletion of another record.

## Replacing vector ordering — resolved

`OrderBy` clears `orderByArguments` when replacing a previous ordering, so it can safely replace `Nearest` without leaving an obsolete vector parameter.

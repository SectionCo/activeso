# Targeted Column Migrations

`DropColumn`, `ChangeColumnType`, and `SetNotNull` use the existing CREATE TABLE SQL from `sqlite_schema`, not a replacement schema inferred from all model fields. Only the named column changes. Model-omitted columns, unrelated types, defaults, collations, CHECK/UNIQUE constraints, and existing explicit index definitions survive supported rebuilds. Hidden row IDs are copied for ordinary tables; `WITHOUT ROWID` tables copy only their declared columns. Model tags do not implicitly add or remove unrelated constraints during these operations.

`migration.go` tokenizes quotes, comments, and nested parentheses to preserve SQL outside the requested edit. Original indexes are recreated after replacement. Creation, copying, replacement, and index restoration share a transaction, so any failure rolls back. Missing target columns are errors.

Safety limits: dependent views/triggers, incoming/outgoing foreign keys, generated columns, autoincrement columns, primary-key type changes, dropping inline primary-key columns, tables with table-level primary keys, and fully shadowed row IDs require dedicated migrations. Explicit NULL expressions can prevent SetNotNull. Indexes and constraints referencing a dropped column must be removed explicitly; migration refuses to discard them silently. Detection is conservative and may reject SQL whose identifiers or literals resemble a dependency.

Regression tests in `migration_test.go` verify preservation, targeted drops, dependency rejection, rollback, and quoted/nested SQL against pinned Turso v0.7.2. The nullable-field read limitation is separate and remains unchanged.

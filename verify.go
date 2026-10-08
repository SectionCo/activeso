package activeso

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type columnInfo struct {
	columnType string
	notNull    bool
	primaryKey bool
}

type indexInfo struct {
	unique  bool
	partial bool
	columns []string
}

type foreignKeyInfo struct {
	table    string
	from     string
	to       string
	onDelete string
}

type tableSchema struct {
	columns     map[string]columnInfo
	indexes     []indexInfo
	foreignKeys []foreignKeyInfo
}

// Verify compares the live table with the model's hints and reports every mismatch without changing the database.
// Missing tables, columns, timestamp columns, and primary-key protection are always reported; hint drift is reported here only.
func (model *model[T]) Verify(ctx context.Context) error {
	// Initialize Variables
	problems, err := model.schemaProblems(ctx, true)

	if err != nil {
		return err
	}
	if len(problems) > 0 {
		return &SchemaError{Table: model.tableName, Problems: problems}
	}

	return nil
}

// ensureReady runs the structural part of Verify once per model before its first database operation.
// A failed check is not cached, so a table created afterwards is picked up on the next call.
func (model *model[T]) ensureReady(ctx context.Context) error {
	// Initialize Variables
	var problems []string
	var err error

	model.readyMutex.Lock()
	defer model.readyMutex.Unlock()

	// Skip the inspection once the table has matched the model.
	if model.ready {
		return nil
	}
	problems, err = model.schemaProblems(ctx, false)
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		return &SchemaError{Table: model.tableName, Problems: problems}
	}
	model.ready = true

	return nil
}

// schemaProblems lists structural mismatches and, when requested, hint drift between the model and its table.
func (model *model[T]) schemaProblems(ctx context.Context, includeHints bool) ([]string, error) {
	// Initialize Variables
	schema, found, err := model.inspectTable(ctx)
	var problems []string

	if err != nil {
		return nil, err
	}
	if !found {
		return []string{fmt.Sprintf("table %s does not exist", model.tableName)}, nil
	}

	// Structural problems would make ordinary queries fail, so every call reports them.
	problems = append(problems, model.structuralProblems(schema)...)
	if includeHints {
		problems = append(problems, model.hintProblems(schema)...)
	}

	return problems, nil
}

// structuralProblems reports missing columns, missing timestamp columns, and an unprotected primary key.
func (model *model[T]) structuralProblems(schema tableSchema) []string {
	// Initialize Variables
	var problems []string
	idColumn, idFound := schema.columns[strings.ToLower(model.idField.column)]

	for _, field := range model.fields {
		if _, found := schema.columns[strings.ToLower(field.column)]; !found {
			problems = append(problems, fmt.Sprintf("column %s is missing", field.column))
		}
	}
	// Name both timestamp columns together so the failure explains what the hint expects.
	if model.timestamps {
		for _, column := range []string{createdAtColumn, updatedAtColumn} {
			if _, found := schema.columns[column]; !found {
				problems = append(problems, fmt.Sprintf("column %s is missing; the timestamps hint expects columns %s and %s", column, createdAtColumn, updatedAtColumn))
			}
		}
	}
	// Save and Delete target one row by primary key, so the column must uniquely identify rows.
	if idFound && !schema.uniqueColumn(model.idField.column) && !idColumn.primaryKey {
		problems = append(problems, fmt.Sprintf("primary key column %s needs a PRIMARY KEY or single-column unique index", model.idField.column))
	}

	return problems
}

// hintProblems reports where the table disagrees with the model's type, not_null, unique, unique_with, index, and belongs_to hints.
func (model *model[T]) hintProblems(schema tableSchema) []string {
	// Initialize Variables
	var problems []string

	for _, field := range model.fields {
		column, found := schema.columns[strings.ToLower(field.column)]
		if !found {
			continue
		}

		// A Go type that maps to a different storage affinity reads and writes unpredictably.
		if columnType, err := sqlColumnType(field); err == nil && sqliteTypeAffinity(column.columnType) != sqliteTypeAffinity(columnType) {
			problems = append(problems, fmt.Sprintf("column %s has type %q but its Go type expects %s", field.column, column.columnType, columnType))
		}
		if field.notNull && !column.notNull && !column.primaryKey {
			problems = append(problems, fmt.Sprintf("hint not_null on %s, but the column allows NULL", field.column))
		}
		if field.unique && !field.isID && !schema.uniqueColumn(field.column) {
			problems = append(problems, fmt.Sprintf("hint unique on %s, but no unique index covers exactly that column", field.column))
		}
		if field.uniqueWith != nil && !schema.hasUniqueIndex(append([]string{field.column}, field.uniqueWith...)) {
			problems = append(problems, fmt.Sprintf("hint unique_with on %s, but no unique index covers (%s)", field.column, strings.Join(append([]string{field.column}, field.uniqueWith...), ", ")))
		}
		if field.indexed && !field.isID && !schema.indexedColumn(field.column) {
			problems = append(problems, fmt.Sprintf("hint index on %s, but no index starts with that column", field.column))
		}
		if field.belongsToTable != "" {
			problems = append(problems, model.foreignKeyProblems(schema, field)...)
		}
	}
	// Timestamps are stored as UTC text, which other affinities may not round-trip.
	if model.timestamps {
		for _, name := range []string{createdAtColumn, updatedAtColumn} {
			if column, found := schema.columns[name]; found && sqliteTypeAffinity(column.columnType) != "TEXT" {
				problems = append(problems, fmt.Sprintf("column %s has type %q but the timestamps hint expects TEXT", name, column.columnType))
			}
		}
	}

	return problems
}

// foreignKeyProblems checks one belongs_to field, and its on_delete=cascade hint, against the table's foreign keys.
func (model *model[T]) foreignKeyProblems(schema tableSchema, field field) []string {
	// Initialize Variables
	var problems []string
	matched := false

	for _, key := range schema.foreignKeys {
		if !strings.EqualFold(key.from, field.column) || !strings.EqualFold(key.table, field.belongsToTable) {
			continue
		}
		// An empty target means the foreign key references the parent's primary key.
		if key.to != "" && !strings.EqualFold(key.to, field.belongsToColumn) {
			continue
		}
		matched = true
		if field.onDeleteCascade && !strings.EqualFold(key.onDelete, "CASCADE") {
			problems = append(problems, fmt.Sprintf("hint on_delete=cascade on %s, but the foreign key uses ON DELETE %s", field.column, key.onDelete))
		}
	}
	if !matched {
		problems = append(problems, fmt.Sprintf("hint belongs_to=%s(%s) on %s, but no matching foreign key exists", field.belongsToTable, field.belongsToColumn, field.column))
	}

	return problems
}

// uniqueColumn reports whether column alone is protected by a primary key or a complete unique index.
func (schema tableSchema) uniqueColumn(column string) bool {
	// Initialize Variables
	primaryKeyColumns := 0
	target := schema.columns[strings.ToLower(column)]

	// A sole primary-key column is unique; composite primary keys are not.
	for _, info := range schema.columns {
		if info.primaryKey {
			primaryKeyColumns++
		}
	}
	if target.primaryKey && primaryKeyColumns == 1 {
		return true
	}

	return schema.hasUniqueIndex([]string{column})
}

// hasUniqueIndex reports whether a complete unique index covers exactly the ordered columns.
func (schema tableSchema) hasUniqueIndex(columns []string) bool {
	// Initialize Variables
	matches := false

	for _, index := range schema.indexes {
		if index.unique && !index.partial && sameColumns(index.columns, columns) {
			matches = true
		}
	}

	return matches
}

// indexedColumn reports whether any complete index, or a sole primary key, begins with column.
func (schema tableSchema) indexedColumn(column string) bool {
	// Initialize Variables
	indexed := schema.uniqueColumn(column)

	for _, index := range schema.indexes {
		if !index.partial && len(index.columns) > 0 && strings.EqualFold(index.columns[0], column) {
			indexed = true
		}
	}

	return indexed
}

// sameColumns compares ordered column lists using SQLite's case-insensitive identifiers.
func sameColumns(left, right []string) bool {
	// Initialize Variables
	same := len(left) == len(right)

	for index := 0; same && index < len(left); index++ {
		same = strings.EqualFold(left[index], right[index])
	}

	return same
}

// inspectTable reads the live table's columns, indexes, and foreign keys; found is false when the table is absent.
func (model *model[T]) inspectTable(ctx context.Context) (tableSchema, bool, error) {
	// Initialize Variables
	schema := tableSchema{columns: make(map[string]columnInfo)}
	rows, err := model.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(model.tableName)))

	if err != nil {
		return schema, false, fmt.Errorf("activeso: inspect table %s: %w", model.tableName, err)
	}
	defer rows.Close()

	// PRAGMA table_info returns no rows for a table that does not exist.
	for rows.Next() {
		var index int
		var name, columnType string
		var notNull bool
		var defaultValue any
		var primaryKeyPosition int
		if err := rows.Scan(&index, &name, &columnType, &notNull, &defaultValue, &primaryKeyPosition); err != nil {
			return schema, false, fmt.Errorf("activeso: inspect columns for %s: %w", model.tableName, err)
		}
		schema.columns[strings.ToLower(name)] = columnInfo{columnType: columnType, notNull: notNull, primaryKey: primaryKeyPosition > 0}
	}
	if err := rows.Err(); err != nil {
		return schema, false, fmt.Errorf("activeso: iterate columns for %s: %w", model.tableName, err)
	}
	if err := rows.Close(); err != nil {
		return schema, false, fmt.Errorf("activeso: close column inspection for %s: %w", model.tableName, err)
	}
	if len(schema.columns) == 0 {
		return schema, false, nil
	}

	schema.indexes, err = model.inspectIndexes(ctx)
	if err != nil {
		return schema, false, err
	}
	schema.foreignKeys, err = model.inspectForeignKeys(ctx)
	if err != nil {
		return schema, false, err
	}

	return schema, true, nil
}

// inspectIndexes returns every index on the model's table with its ordered columns.
func (model *model[T]) inspectIndexes(ctx context.Context) ([]indexInfo, error) {
	// Initialize Variables
	rows, err := model.db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_list(%s)", quoteIdentifier(model.tableName)))
	var names []string
	var indexes []indexInfo

	if err != nil {
		return nil, fmt.Errorf("activeso: inspect indexes for %s: %w", model.tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var sequence int
		var name, origin string
		var unique, partial bool
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			return nil, fmt.Errorf("activeso: inspect indexes for %s: %w", model.tableName, err)
		}
		names = append(names, name)
		indexes = append(indexes, indexInfo{unique: unique, partial: partial})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activeso: iterate indexes for %s: %w", model.tableName, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("activeso: close index inspection for %s: %w", model.tableName, err)
	}

	// Read each index's columns after closing the listing so the connection is free.
	for position, name := range names {
		columns, err := model.indexColumns(ctx, name)
		if err != nil {
			return nil, err
		}
		indexes[position].columns = columns
	}

	return indexes, nil
}

// indexColumns returns the ordered columns of one index, skipping expression entries.
func (model *model[T]) indexColumns(ctx context.Context, name string) ([]string, error) {
	// Initialize Variables
	rows, err := model.db.QueryContext(ctx, fmt.Sprintf("PRAGMA index_info(%s)", quoteIdentifier(name)))
	var columns []string

	if err != nil {
		return nil, fmt.Errorf("activeso: inspect index %s for %s: %w", name, model.tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var sequence, columnIndex int
		var column sql.NullString
		if err := rows.Scan(&sequence, &columnIndex, &column); err != nil {
			return nil, fmt.Errorf("activeso: inspect index %s for %s: %w", name, model.tableName, err)
		}
		columns = append(columns, column.String)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activeso: iterate index %s for %s: %w", name, model.tableName, err)
	}

	return columns, nil
}

// inspectForeignKeys returns the foreign keys declared on the model's table.
func (model *model[T]) inspectForeignKeys(ctx context.Context) ([]foreignKeyInfo, error) {
	// Initialize Variables
	rows, err := model.db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", quoteIdentifier(model.tableName)))
	var keys []foreignKeyInfo

	if err != nil {
		return nil, fmt.Errorf("activeso: inspect foreign keys for %s: %w", model.tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, sequence int
		var table, from, onUpdate, onDelete, match string
		var to sql.NullString
		if err := rows.Scan(&id, &sequence, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return nil, fmt.Errorf("activeso: inspect foreign keys for %s: %w", model.tableName, err)
		}
		keys = append(keys, foreignKeyInfo{table: table, from: from, to: to.String, onDelete: onDelete})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activeso: iterate foreign keys for %s: %w", model.tableName, err)
	}

	return keys, nil
}

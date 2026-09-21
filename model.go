package activeso

import (
	"context"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	turso "turso.tech/database/tursogo"
)

type tableNamer interface {
	TableName() string
}

type field struct {
	index           int
	column          string
	goType          reflect.Type
	belongsToTable  string
	belongsToColumn string
	isID            bool
	isVector        bool
	notNull         bool
	unique          bool
	indexed         bool
	primaryKey      bool
}

const (
	createdAtColumn = "activeso_created_at"
	updatedAtColumn = "activeso_updated_at"
)

type columnInfo struct {
	primaryKey bool
	unique     bool
}

type model[T any] struct {
	db        *sql.DB
	tableName string
	fields    []field
	idField   field
}

type query[T any] struct {
	model            *model[T]
	conditions       []string
	arguments        []any
	orderBy          string
	orderByArguments []any
	limit            int
}

// Model binds an application-defined struct type to its Turso table using db.
// T must embed activeso.Record and expose one primary_key field or an id column.
func Model[T any](db *sql.DB) *model[T] {
	// Initialize Variables
	model, err := newModel[T](db)

	if err != nil {
		panic(err)
	}

	return model
}

// Create inserts value, generates an empty string ID, and returns a bound record.
func (model *model[T]) Create(ctx context.Context, value T) (*T, error) {
	// Initialize Variables
	record := &value
	var columns, expressions []string
	var arguments []any
	var statement string
	var err error

	// Generate an ID and validate constrained fields before inserting the record.
	if err := model.generateID(record); err != nil {
		return nil, err
	}
	if err := model.bind(record); err != nil {
		return nil, err
	}
	if err := model.validateUnique(ctx, record); err != nil {
		return nil, err
	}

	columns, expressions, arguments, err = model.insertValues(record)
	if err != nil {
		return nil, err
	}

	statement = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", quoteIdentifier(model.tableName), strings.Join(columns, ", "), strings.Join(expressions, ", "))
	if _, err := model.db.ExecContext(ctx, statement, arguments...); err != nil {
		return nil, fmt.Errorf("activeso: create %s: %w", model.tableName, model.uniqueWriteError(err))
	}
	if err := model.refreshTimestamps(ctx, record); err != nil {
		return nil, err
	}

	return record, nil
}

// Find loads and binds the record with id, returning ErrNotFound when it does not exist.
func (model *model[T]) Find(ctx context.Context, id any) (*T, error) {
	// Initialize Variables
	query := model.Where(fmt.Sprintf("%s = ?", quoteIdentifier(model.idField.column)), id)

	return query.First(ctx)
}

// FindBy loads and binds every record whose mapped column equals value.
func (model *model[T]) FindBy(ctx context.Context, column string, value any) ([]*T, error) {
	// Initialize Variables
	field, found := model.fieldForColumn(column)

	if !found {
		return nil, fmt.Errorf("activeso: column %s is not defined on %s", column, model.tableName)
	}

	return model.Where(fmt.Sprintf("%s = ?", quoteIdentifier(field.column)), value).All(ctx)
}

// All loads and binds every row in the model's table.
func (model *model[T]) All(ctx context.Context) ([]*T, error) {
	// Initialize Variables
	query := &query[T]{model: model}

	return query.All(ctx)
}

// Where returns a query constrained by a parameterized SQL predicate.
func (model *model[T]) Where(predicate string, arguments ...any) *query[T] {
	// Initialize Variables
	conditions := []string{predicate}
	values := append([]any(nil), arguments...)

	return &query[T]{model: model, conditions: conditions, arguments: values}
}

// Nearest orders records by cosine distance from embedding in the specified vector column.
func (model *model[T]) Nearest(column string, embedding Vector32) *query[T] {
	// Initialize Variables
	vectorColumn := quoteIdentifier(column)
	orderBy := fmt.Sprintf("vector_distance_cos(%s, vector32(?)) ASC", vectorColumn)
	arguments := []any{embedding}

	return &query[T]{model: model, orderBy: orderBy, orderByArguments: arguments}
}

// Bind attaches model persistence behavior to value and returns the same pointer.
func (model *model[T]) Bind(value *T) (*T, error) {
	// Initialize Variables
	err := model.bind(value)

	if err != nil {
		return nil, err
	}

	return value, nil
}

// AutoMigrate creates the model table, adds missing nullable columns, and creates unique indexes.
func (model *model[T]) AutoMigrate(ctx context.Context) error {
	// Initialize Variables
	transaction, err := model.db.BeginTx(ctx, nil)
	var createStatement string
	var columns map[string]columnInfo
	var idColumn columnInfo
	var found bool

	if err != nil {
		return fmt.Errorf("activeso: begin migration for %s: %w", model.tableName, err)
	}
	defer transaction.Rollback()

	// Create the table before checking its existing columns.
	createStatement, err = model.createTableStatement()
	if err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, createStatement); err != nil {
		return fmt.Errorf("activeso: create table %s: %w", model.tableName, err)
	}

	columns, err = model.existingColumns(ctx, transaction)
	if err != nil {
		return err
	}
	idColumn, found = columns[strings.ToLower(model.idField.column)]
	if !found || !idColumn.unique {
		return fmt.Errorf("activeso: existing table %s must define a primary key or unique constraint on %s", model.tableName, model.idField.column)
	}
	for _, field := range model.fields {
		if _, found := columns[strings.ToLower(field.column)]; found {
			continue
		}
		if field.notNull {
			return fmt.Errorf("activeso: cannot automatically add required column %s to %s", field.column, model.tableName)
		}

		definition, err := model.columnDefinition(field, false)
		if err != nil {
			return err
		}
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", quoteIdentifier(model.tableName), definition)
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("activeso: add column %s to %s: %w", field.column, model.tableName, err)
		}
	}

	if err := model.createTimestampColumns(ctx, transaction, columns); err != nil {
		return err
	}
	if err := model.createUniqueIndexes(ctx, transaction); err != nil {
		return err
	}
	if err := model.createIndexes(ctx, transaction); err != nil {
		return err
	}
	if err := model.createTimestampTriggers(ctx, transaction); err != nil {
		return err
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("activeso: commit migration for %s: %w", model.tableName, err)
	}

	return nil
}

// DropUnique removes the unique index for column after its unique tag has been removed.
func (model *model[T]) DropUnique(ctx context.Context, column string) error {
	// Initialize Variables
	field, found := model.fieldForColumn(column)

	if !found {
		return fmt.Errorf("activeso: column %s is not defined on %s", column, model.tableName)
	}
	if field.unique {
		return fmt.Errorf("activeso: remove the unique constraint from %s before dropping its index", column)
	}

	statement := fmt.Sprintf("DROP INDEX IF EXISTS %s", quoteIdentifier(model.uniqueIndexName(field)))
	if _, err := model.db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("activeso: drop unique index for %s.%s: %w", model.tableName, column, err)
	}

	return nil
}

// DropColumn rebuilds the table without column after its Go field has been removed.
func (model *model[T]) DropColumn(ctx context.Context, column string) error {
	// Initialize Variables
	_, found := model.fieldForColumn(column)

	if found {
		return fmt.Errorf("activeso: remove field %s from the model before dropping its column", column)
	}

	return model.rebuildTable(ctx, column, "drop", "")
}

// ChangeColumnType rebuilds the table using the current Go type for column.
func (model *model[T]) ChangeColumnType(ctx context.Context, column string) error {
	// Initialize Variables
	field, found := model.fieldForColumn(column)
	var columnType string
	var err error

	if !found {
		return fmt.Errorf("activeso: column %s is not defined on %s", column, model.tableName)
	}

	columnType, err = sqlColumnType(field)
	if err != nil {
		return err
	}
	return model.rebuildTable(ctx, column, "type", columnType)
}

// SetNotNull rebuilds the table with a NOT NULL constraint declared on column.
func (model *model[T]) SetNotNull(ctx context.Context, column string) error {
	// Initialize Variables
	field, found := model.fieldForColumn(column)
	statement := fmt.Sprintf("SELECT 1 FROM %s WHERE %s IS NULL LIMIT 1", quoteIdentifier(model.tableName), quoteIdentifier(column))
	var nullValue int

	if !found {
		return fmt.Errorf("activeso: column %s is not defined on %s", column, model.tableName)
	}
	if !field.notNull {
		return fmt.Errorf("activeso: add the not_null constraint to %s before tightening it", column)
	}
	if err := model.db.QueryRowContext(ctx, statement).Scan(&nullValue); err == nil {
		return fmt.Errorf("activeso: cannot set %s.%s to NOT NULL while NULL values exist", model.tableName, column)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("activeso: check NULL values for %s.%s: %w", model.tableName, column, err)
	}

	return model.rebuildTable(ctx, column, "not_null", "")
}

// save updates every persisted field except the primary key for an already-bound record.
func (model *model[T]) save(ctx context.Context, entity, originalID any) error {
	// Initialize Variables
	record, ok := entity.(*T)
	var currentID any
	var err error

	if !ok {
		return ErrUnboundRecord
	}
	currentID, err = model.idValue(record)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(currentID, originalID) {
		return ErrIDChanged
	}

	assignments, arguments, err := model.updateValues(record)
	if err != nil {
		return err
	}

	if err := model.validateUnique(ctx, record); err != nil {
		return err
	}
	if len(assignments) == 0 {
		return model.recordExists(ctx, originalID)
	}

	statement := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", quoteIdentifier(model.tableName), strings.Join(assignments, ", "), quoteIdentifier(model.idField.column))
	result, err := model.db.ExecContext(ctx, statement, append(arguments, originalID)...)
	if err != nil {
		return fmt.Errorf("activeso: save %s: %w", model.tableName, model.uniqueWriteError(err))
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("activeso: save %s rows affected: %w", model.tableName, err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	if err := model.refreshTimestamps(ctx, record); err != nil {
		return err
	}

	return nil
}

// recordExists confirms that a record remains present without issuing an empty update.
func (model *model[T]) recordExists(ctx context.Context, id any) error {
	// Initialize Variables
	statement := fmt.Sprintf("SELECT 1 FROM %s WHERE %s = ? LIMIT 1", quoteIdentifier(model.tableName), quoteIdentifier(model.idField.column))
	var match int
	var err error

	// Preserve Save's ErrNotFound behavior for ID-only models.
	err = model.db.QueryRowContext(ctx, statement, id).Scan(&match)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("activeso: check %s existence: %w", model.tableName, err)
	}

	return nil
}

// delete removes an already-bound record using its primary key.
func (model *model[T]) delete(ctx context.Context, entity, originalID any) error {
	// Initialize Variables
	record, ok := entity.(*T)
	var currentID any
	var err error

	if !ok {
		return ErrUnboundRecord
	}
	currentID, err = model.idValue(record)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(currentID, originalID) {
		return ErrIDChanged
	}

	statement := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", quoteIdentifier(model.tableName), quoteIdentifier(model.idField.column))
	result, err := model.db.ExecContext(ctx, statement, originalID)
	if err != nil {
		return fmt.Errorf("activeso: delete %s: %w", model.tableName, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("activeso: delete %s rows affected: %w", model.tableName, err)
	}
	if rows == 0 {
		return ErrNotFound
	}

	return nil
}

// OrderBy applies an ORDER BY expression to the query.
func (query *query[T]) OrderBy(expression string) *query[T] {
	// Initialize Variables
	orderBy := expression
	orderByArguments := []any(nil)

	// Replace both the previous ordering expression and its bound arguments.
	query.orderByArguments = orderByArguments
	query.orderBy = orderBy
	return query
}

// Limit applies a positive row limit to the query.
func (query *query[T]) Limit(limit int) *query[T] {
	// Initialize Variables
	rowLimit := limit

	query.limit = rowLimit
	return query
}

// First loads and binds the first matching record, returning ErrNotFound when absent.
func (query *query[T]) First(ctx context.Context) (*T, error) {
	// Initialize Variables
	limit := query.limit

	if limit == 0 || limit > 1 {
		limit = 1
	}

	records, err := query.withLimit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, ErrNotFound
	}

	return records[0], nil
}

// All loads and binds all records matching the query.
func (query *query[T]) All(ctx context.Context) ([]*T, error) {
	// Initialize Variables
	statement := query.selectStatement()
	arguments, err := query.queryArguments()

	if err != nil {
		return nil, err
	}

	rows, err := query.model.db.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, fmt.Errorf("activeso: query %s: %w", query.model.tableName, err)
	}
	defer rows.Close()

	var records []*T
	for rows.Next() {
		record := new(T)
		if err := query.model.scan(rows, record); err != nil {
			return nil, err
		}
		if err := query.model.bind(record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activeso: iterate %s: %w", query.model.tableName, err)
	}

	return records, nil
}

// withLimit returns a copy of query with a replacement row limit.
func (query *query[T]) withLimit(limit int) *query[T] {
	// Initialize Variables
	copy := *query

	copy.limit = limit
	return &copy
}

// queryArguments encodes Vector32 arguments for Turso's vector32 SQL function.
func (query *query[T]) queryArguments() ([]any, error) {
	// Initialize Variables
	arguments := make([]any, 0, len(query.arguments)+len(query.orderByArguments))
	values := append(append([]any(nil), query.arguments...), query.orderByArguments...)

	// Serialize vector arguments so they can be passed to vector32(?) expressions.
	for _, value := range values {
		if vector, ok := value.(Vector32); ok {
			encoded, err := vectorJSON(vector)
			if err != nil {
				return nil, err
			}
			arguments = append(arguments, encoded)
			continue
		}
		arguments = append(arguments, value)
	}

	return arguments, nil
}

// selectStatement builds the SELECT statement for this query.
func (query *query[T]) selectStatement() string {
	// Initialize Variables
	columns := query.model.selectColumns()
	parts := []string{fmt.Sprintf("SELECT %s FROM %s", strings.Join(columns, ", "), quoteIdentifier(query.model.tableName))}

	// Add caller-supplied parameterized predicates and query modifiers.
	if len(query.conditions) > 0 {
		parts = append(parts, "WHERE "+strings.Join(query.conditions, " AND "))
	}
	if query.orderBy != "" {
		parts = append(parts, "ORDER BY "+query.orderBy)
	}
	if query.limit > 0 {
		parts = append(parts, fmt.Sprintf("LIMIT %d", query.limit))
	}

	return strings.Join(parts, " ")
}

// newModel inspects T and constructs its immutable persistence metadata.
func newModel[T any](db *sql.DB) (*model[T], error) {
	// Initialize Variables
	typeOfT := reflect.TypeFor[T]()
	model := &model[T]{db: db}
	fields := make([]field, 0, typeOfT.NumField())
	columns := make(map[string]struct{}, typeOfT.NumField())
	explicitPrimaryKeyCount := 0

	// Validate the model shape before extracting database fields.
	if db == nil {
		return nil, errors.New("activeso: model database is nil")
	}
	if typeOfT.Kind() != reflect.Struct {
		return nil, fmt.Errorf("activeso: model type %s is not a struct", typeOfT)
	}

	for index := range typeOfT.NumField() {
		structField := typeOfT.Field(index)
		if structField.Anonymous && structField.Type == reflect.TypeFor[Record]() {
			continue
		}
		if !structField.IsExported() {
			continue
		}

		column := columnName(structField)
		if column == "" {
			continue
		}
		notNull, unique, indexed, primaryKey, belongsToTable, belongsToColumn, err := fieldConstraints(structField)
		if err != nil {
			return nil, err
		}
		normalizedColumn := strings.ToLower(column)
		if _, exists := columns[normalizedColumn]; exists {
			return nil, fmt.Errorf("activeso: model type %s maps more than one field to column %s", typeOfT, column)
		}
		columns[normalizedColumn] = struct{}{}

		field := field{index: index, column: column, goType: structField.Type, belongsToTable: belongsToTable, belongsToColumn: belongsToColumn, isVector: structField.Type == reflect.TypeFor[Vector32](), notNull: notNull, unique: unique, indexed: indexed, primaryKey: primaryKey}
		fields = append(fields, field)
		if primaryKey {
			explicitPrimaryKeyCount++
		}
	}

	if !hasDirectRecord(typeOfT) {
		return nil, fmt.Errorf("activeso: model type %s must embed activeso.Record", typeOfT)
	}
	if explicitPrimaryKeyCount > 1 {
		return nil, fmt.Errorf("activeso: model type %s declares more than one primary_key field", typeOfT)
	}
	for index := range fields {
		if fields[index].primaryKey || (explicitPrimaryKeyCount == 0 && strings.EqualFold(fields[index].column, "id")) {
			if model.idField.goType != nil {
				return nil, fmt.Errorf("activeso: model type %s must expose exactly one primary key", typeOfT)
			}
			if !validPrimaryKeyType(fields[index].goType) {
				return nil, fmt.Errorf("activeso: primary key field %s must use an immutable scalar type", fields[index].column)
			}
			fields[index].isID = true
			model.idField = fields[index]
		}
	}
	if model.idField.goType == nil {
		return nil, fmt.Errorf("activeso: model type %s must expose an id column or primary_key field", typeOfT)
	}

	model.tableName = tableName[T](typeOfT)
	model.fields = fields
	return model, nil
}

// validPrimaryKeyType reports whether typeOfT is a mutable-safe SQL scalar representable by Turso.
func validPrimaryKeyType(typeOfT reflect.Type) bool {
	// Initialize Variables
	kind := typeOfT.Kind()

	// Reject values that can alias, encode NULL, or exceed Turso's signed integer range.
	switch kind {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// hasDirectRecord reports whether typeOfT anonymously embeds Record.
func hasDirectRecord(typeOfT reflect.Type) bool {
	// Initialize Variables
	recordType := reflect.TypeFor[Record]()

	for index := range typeOfT.NumField() {
		structField := typeOfT.Field(index)
		if structField.Anonymous && structField.Type == recordType {
			return true
		}
	}

	return false
}

// tableName resolves a custom TableName method or a plural snake_case type name.
func tableName[T any](typeOfT reflect.Type) string {
	// Initialize Variables
	var zero T
	name := snakeCase(typeOfT.Name())

	if named, ok := any(zero).(tableNamer); ok {
		if customName := named.TableName(); customName != "" {
			return customName
		}
	}
	if named, ok := any(&zero).(tableNamer); ok {
		if customName := named.TableName(); customName != "" {
			return customName
		}
	}

	return pluralize(name)
}

// columnName resolves the db tag or derives a snake_case field name.
func columnName(structField reflect.StructField) string {
	// Initialize Variables
	tag := structField.Tag.Get("db")
	parts := strings.Split(tag, ",")

	if tag == "-" {
		return ""
	}
	if parts[0] != "" {
		return parts[0]
	}

	return snakeCase(structField.Name)
}

// fieldConstraints parses supported ActiveSo schema constraints from a struct field.
func fieldConstraints(structField reflect.StructField) (bool, bool, bool, bool, string, string, error) {
	// Initialize Variables
	tag := structField.Tag.Get("activeso")
	constraints := strings.Split(tag, ",")
	notNull := false
	unique := false
	indexed := false
	primaryKey := false
	belongsToTable := ""
	belongsToColumn := ""

	for _, constraint := range constraints {
		switch constraint {
		case "", "-":
			continue
		case "not_null":
			notNull = true
		case "unique":
			unique = true
		case "index":
			indexed = true
		case "primary_key":
			primaryKey = true
		default:
			if !strings.HasPrefix(constraint, "belongs_to=") {
				return false, false, false, false, "", "", fmt.Errorf("activeso: unsupported constraint %q on field %s", constraint, structField.Name)
			}
			if belongsToTable != "" {
				return false, false, false, false, "", "", fmt.Errorf("activeso: duplicate belongs_to constraint on field %s", structField.Name)
			}

			var err error
			belongsToTable, belongsToColumn, err = belongsToTarget(strings.TrimPrefix(constraint, "belongs_to="))
			if err != nil {
				return false, false, false, false, "", "", fmt.Errorf("activeso: invalid belongs_to constraint on field %s: %w", structField.Name, err)
			}
		}
	}

	return notNull, unique, indexed, primaryKey, belongsToTable, belongsToColumn, nil
}

// belongsToTarget validates and splits a table(column) foreign-key target.
func belongsToTarget(value string) (string, string, error) {
	// Initialize Variables
	openParenthesis := strings.IndexByte(value, '(')
	table := ""
	column := ""

	if openParenthesis <= 0 || !strings.HasSuffix(value, ")") || strings.Count(value, "(") != 1 || strings.Count(value, ")") != 1 {
		return "", "", fmt.Errorf("expected table(column)")
	}
	table = value[:openParenthesis]
	column = value[openParenthesis+1 : len(value)-1]
	if !schemaIdentifier(table) || !schemaIdentifier(column) {
		return "", "", fmt.Errorf("target identifiers must contain only letters, digits, and underscores")
	}

	return table, column, nil
}

// schemaIdentifier reports whether value is a non-empty identifier safe for generated schema SQL.
func schemaIdentifier(value string) bool {
	// Initialize Variables
	runes := []rune(value)

	if len(runes) == 0 || !(unicode.IsLetter(runes[0]) || runes[0] == '_') {
		return false
	}
	for _, character := range runes[1:] {
		if !(unicode.IsLetter(character) || unicode.IsDigit(character) || character == '_') {
			return false
		}
	}

	return true
}

// snakeCase converts an exported Go identifier into a lower snake_case identifier.
func snakeCase(value string) string {
	// Initialize Variables
	runes := []rune(value)
	var builder strings.Builder

	for index, character := range runes {
		previous := rune(0)
		next := rune(0)
		isUpper := unicode.IsUpper(character)
		if index > 0 {
			previous = runes[index-1]
		}
		if index+1 < len(runes) {
			next = runes[index+1]
		}

		// Separate word boundaries and the final capital in an initialism.
		if index > 0 && isUpper && (unicode.IsLower(previous) || unicode.IsDigit(previous) || unicode.IsLower(next)) {
			builder.WriteByte('_')
		}
		builder.WriteRune(unicode.ToLower(character))
	}

	return builder.String()
}

// pluralize converts the common singular table identifier forms used by Model defaults.
func pluralize(value string) string {
	// Initialize Variables
	last := value[len(value)-1:]

	if strings.HasSuffix(value, "ch") || strings.HasSuffix(value, "sh") || strings.ContainsAny(last, "sxz") {
		return value + "es"
	}
	if strings.HasSuffix(value, "y") && len(value) > 1 && !strings.ContainsAny(value[len(value)-2:len(value)-1], "aeiou") {
		return value[:len(value)-1] + "ies"
	}

	return value + "s"
}

// quoteIdentifier quotes a trusted schema identifier for use in generated SQL.
func quoteIdentifier(identifier string) string {
	// Initialize Variables
	escaped := strings.ReplaceAll(identifier, "\"", "\"\"")

	return "\"" + escaped + "\""
}

// bind records the model and exact owning pointer in value's embedded Record.
func (model *model[T]) bind(value *T) error {
	// Initialize Variables
	var valueOfT reflect.Value
	var recordField reflect.Value
	var originalID any
	var err error

	if value == nil {
		return ErrUnboundRecord
	}
	valueOfT = reflect.ValueOf(value).Elem()
	recordField = valueOfT.FieldByName("Record")
	originalID, err = model.idValue(value)

	if !recordField.IsValid() || !recordField.CanAddr() || recordField.Type() != reflect.TypeFor[Record]() {
		return ErrUnboundRecord
	}
	if err != nil {
		return err
	}

	record := recordField.Addr().Interface().(*Record)
	record.binding = model
	record.owner = value
	record.originalID = originalID
	return nil
}

// generateID assigns a random UUID to an empty string primary key.
func (model *model[T]) generateID(record *T) error {
	// Initialize Variables
	valueOfT := reflect.ValueOf(record).Elem()
	idField := valueOfT.Field(model.idField.index)

	if idField.Kind() != reflect.String || idField.String() != "" {
		return nil
	}

	id, err := randomUUID()
	if err != nil {
		return err
	}
	idField.SetString(id)
	return nil
}

// randomUUID creates a RFC 4122 version 4 UUID without adding a dependency.
func randomUUID() (string, error) {
	// Initialize Variables
	bytes := make([]byte, 16)

	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("activeso: generate UUID: %w", err)
	}

	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return hex.EncodeToString(bytes[0:4]) + "-" + hex.EncodeToString(bytes[4:6]) + "-" + hex.EncodeToString(bytes[6:8]) + "-" + hex.EncodeToString(bytes[8:10]) + "-" + hex.EncodeToString(bytes[10:16]), nil
}

// insertValues returns the generated INSERT columns, expressions, and bound values.
func (model *model[T]) insertValues(record *T) ([]string, []string, []any, error) {
	// Initialize Variables
	columns := make([]string, 0, len(model.fields))
	expressions := make([]string, 0, len(model.fields))
	arguments := make([]any, 0, len(model.fields))
	valueOfT := reflect.ValueOf(record).Elem()

	for _, field := range model.fields {
		value, expression, err := databaseValue(valueOfT.Field(field.index), field.isVector)
		if err != nil {
			return nil, nil, nil, err
		}
		columns = append(columns, quoteIdentifier(field.column))
		expressions = append(expressions, expression)
		arguments = append(arguments, value)
	}

	return columns, expressions, arguments, nil
}

// updateValues returns SQL assignments and bound values for all non-ID fields.
func (model *model[T]) updateValues(record *T) ([]string, []any, error) {
	// Initialize Variables
	assignments := make([]string, 0, len(model.fields)-1)
	arguments := make([]any, 0, len(model.fields)-1)
	valueOfT := reflect.ValueOf(record).Elem()

	for _, field := range model.fields {
		if field.isID {
			continue
		}
		value, expression, err := databaseValue(valueOfT.Field(field.index), field.isVector)
		if err != nil {
			return nil, nil, err
		}
		assignments = append(assignments, quoteIdentifier(field.column)+" = "+expression)
		arguments = append(arguments, value)
	}

	return assignments, arguments, nil
}

// fieldForColumn returns the persisted field that maps to column.
func (model *model[T]) fieldForColumn(column string) (field, bool) {
	// Initialize Variables
	var zero field

	for _, field := range model.fields {
		if field.column == column {
			return field, true
		}
	}

	return zero, false
}

// createUniqueIndexes creates every unique index declared by the current model.
func (model *model[T]) createUniqueIndexes(ctx context.Context, transaction *sql.Tx) error {
	// Initialize Variables
	fields := model.fields

	for _, field := range fields {
		if !field.unique || field.isID {
			continue
		}

		statement := fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s (%s)", quoteIdentifier(model.uniqueIndexName(field)), quoteIdentifier(model.tableName), quoteIdentifier(field.column))
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("activeso: create unique index for %s.%s: %w", model.tableName, field.column, err)
		}
	}

	return nil
}

// createIndexes creates every non-unique index declared by the current model.
func (model *model[T]) createIndexes(ctx context.Context, transaction *sql.Tx) error {
	// Initialize Variables
	fields := model.fields

	for _, field := range fields {
		if !field.indexed || field.unique || field.isID {
			continue
		}

		statement := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s)", quoteIdentifier(model.indexName(field)), quoteIdentifier(model.tableName), quoteIdentifier(field.column))
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("activeso: create index for %s.%s: %w", model.tableName, field.column, err)
		}
	}

	return nil
}

// createTableStatement builds the idempotent CREATE TABLE statement for the model.
func (model *model[T]) createTableStatement() (string, error) {
	// Initialize Variables
	name := model.tableName
	statement, err := model.createTableStatementFor(name)

	if err != nil {
		return "", err
	}

	return strings.Replace(statement, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1), nil
}

// createTableStatementFor builds a CREATE TABLE statement for a supplied table name.
func (model *model[T]) createTableStatementFor(tableName string) (string, error) {
	// Initialize Variables
	definitions := make([]string, 0, len(model.fields)+2)

	for _, field := range model.fields {
		definition, err := model.columnDefinition(field, true)
		if err != nil {
			return "", err
		}
		definitions = append(definitions, definition)
	}
	// Store UTC timestamps automatically without requiring fields on application models.
	definitions = append(definitions,
		quoteIdentifier(createdAtColumn)+" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP",
		quoteIdentifier(updatedAtColumn)+" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP",
	)

	return fmt.Sprintf("CREATE TABLE %s (%s)", quoteIdentifier(tableName), strings.Join(definitions, ", ")), nil
}

// createTimestampColumns adds and initializes the managed timestamp columns on an existing table.
func (model *model[T]) createTimestampColumns(ctx context.Context, transaction *sql.Tx, columns map[string]columnInfo) error {
	// Initialize Variables
	missingCreatedAt := false
	missingUpdatedAt := false

	// SQLite cannot add a column with a non-constant timestamp default, so legacy tables use triggers.
	if _, found := columns[createdAtColumn]; !found {
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT", quoteIdentifier(model.tableName), quoteIdentifier(createdAtColumn))
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("activeso: add timestamp column %s to %s: %w", createdAtColumn, model.tableName, err)
		}
		missingCreatedAt = true
	}
	if _, found := columns[updatedAtColumn]; !found {
		statement := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT", quoteIdentifier(model.tableName), quoteIdentifier(updatedAtColumn))
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("activeso: add timestamp column %s to %s: %w", updatedAtColumn, model.tableName, err)
		}
		missingUpdatedAt = true
	}
	if !missingCreatedAt && !missingUpdatedAt {
		return nil
	}

	// Assign creation-time values to rows that existed before timestamp support.
	statement := fmt.Sprintf("UPDATE %s SET %s = COALESCE(%s, CURRENT_TIMESTAMP), %s = COALESCE(%s, CURRENT_TIMESTAMP)", quoteIdentifier(model.tableName), quoteIdentifier(createdAtColumn), quoteIdentifier(createdAtColumn), quoteIdentifier(updatedAtColumn), quoteIdentifier(updatedAtColumn))
	if _, err := transaction.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("activeso: initialize timestamps for %s: %w", model.tableName, err)
	}

	return nil
}

// createTimestampTriggers installs the managed triggers that fill and refresh table timestamps.
func (model *model[T]) createTimestampTriggers(ctx context.Context, transaction *sql.Tx) error {
	// Initialize Variables
	columns, err := model.existingColumns(ctx, transaction)
	updatedColumns := make([]string, 0, len(columns)-2)
	insertTrigger := model.timestampTriggerName("insert")
	updateTrigger := model.timestampTriggerName("update")
	createdAtTrigger := model.timestampTriggerName("protect_created_at")
	updatedAtTrigger := model.timestampTriggerName("protect_updated_at")

	if err != nil {
		return err
	}
	for column := range columns {
		if strings.EqualFold(column, createdAtColumn) || strings.EqualFold(column, updatedAtColumn) {
			continue
		}
		updatedColumns = append(updatedColumns, quoteIdentifier(column))
	}
	if len(updatedColumns) == 0 {
		return nil
	}
	for _, trigger := range []string{insertTrigger, updateTrigger, createdAtTrigger, updatedAtTrigger} {
		statement := "DROP TRIGGER IF EXISTS " + quoteIdentifier(trigger)
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("activeso: replace timestamp trigger for %s: %w", model.tableName, err)
		}
	}

	// Reject updates that attempt to replace an established creation timestamp.
	createdAtStatement := fmt.Sprintf("CREATE TRIGGER %s BEFORE UPDATE OF %s ON %s WHEN OLD.%s IS NOT NULL AND NEW.%s IS NOT OLD.%s BEGIN SELECT RAISE(ABORT, 'activeso: activeso_created_at is immutable'); END", quoteIdentifier(createdAtTrigger), quoteIdentifier(createdAtColumn), quoteIdentifier(model.tableName), quoteIdentifier(createdAtColumn), quoteIdentifier(createdAtColumn), quoteIdentifier(createdAtColumn))
	if _, err := transaction.ExecContext(ctx, createdAtStatement); err != nil {
		return fmt.Errorf("activeso: create creation timestamp trigger for %s: %w", model.tableName, err)
	}

	// Reject direct timestamp replacements while allowing the managed trigger's current timestamp.
	updatedAtStatement := fmt.Sprintf("CREATE TRIGGER %s BEFORE UPDATE OF %s ON %s WHEN NEW.%s IS NOT OLD.%s AND NEW.%s IS NOT CURRENT_TIMESTAMP BEGIN SELECT RAISE(ABORT, 'activeso: activeso_updated_at is managed'); END", quoteIdentifier(updatedAtTrigger), quoteIdentifier(updatedAtColumn), quoteIdentifier(model.tableName), quoteIdentifier(updatedAtColumn), quoteIdentifier(updatedAtColumn), quoteIdentifier(updatedAtColumn))
	if _, err := transaction.ExecContext(ctx, updatedAtStatement); err != nil {
		return fmt.Errorf("activeso: create update timestamp trigger for %s: %w", model.tableName, err)
	}

	// Fill nullable legacy timestamp columns when callers omit them during inserts.
	insertStatement := fmt.Sprintf("CREATE TRIGGER %s AFTER INSERT ON %s WHEN NEW.%s IS NULL OR NEW.%s IS NULL BEGIN UPDATE %s SET %s = COALESCE(%s, CURRENT_TIMESTAMP), %s = COALESCE(%s, CURRENT_TIMESTAMP) WHERE %s IS NEW.%s; END", quoteIdentifier(insertTrigger), quoteIdentifier(model.tableName), quoteIdentifier(createdAtColumn), quoteIdentifier(updatedAtColumn), quoteIdentifier(model.tableName), quoteIdentifier(createdAtColumn), quoteIdentifier(createdAtColumn), quoteIdentifier(updatedAtColumn), quoteIdentifier(updatedAtColumn), quoteIdentifier(model.idField.column), quoteIdentifier(model.idField.column))
	if _, err := transaction.ExecContext(ctx, insertStatement); err != nil {
		return fmt.Errorf("activeso: create timestamp insert trigger for %s: %w", model.tableName, err)
	}

	// Restrict the trigger to data columns so its timestamp write cannot recursively trigger itself.
	updateStatement := fmt.Sprintf("CREATE TRIGGER %s AFTER UPDATE OF %s ON %s BEGIN UPDATE %s SET %s = CURRENT_TIMESTAMP WHERE %s IS NEW.%s; END", quoteIdentifier(updateTrigger), strings.Join(updatedColumns, ", "), quoteIdentifier(model.tableName), quoteIdentifier(model.tableName), quoteIdentifier(updatedAtColumn), quoteIdentifier(model.idField.column), quoteIdentifier(model.idField.column))
	if _, err := transaction.ExecContext(ctx, updateStatement); err != nil {
		return fmt.Errorf("activeso: create timestamp update trigger for %s: %w", model.tableName, err)
	}

	return nil
}

// timestampTriggerName returns a database-wide stable name for one managed table trigger.
func (model *model[T]) timestampTriggerName(kind string) string {
	// Initialize Variables
	name := strings.ToLower(model.tableName)

	return "activeso_" + hex.EncodeToString([]byte(name)) + "_timestamps_" + kind
}

// columnDefinition derives a Turso column definition for one model field.
func (model *model[T]) columnDefinition(field field, includeRequired bool) (string, error) {
	// Initialize Variables
	columnType, err := sqlColumnType(field)
	definition := quoteIdentifier(field.column) + " " + columnType

	if err != nil {
		return "", err
	}
	if field.isID {
		definition += " PRIMARY KEY"
		if includeRequired && field.notNull {
			definition += " NOT NULL"
		}

		return definition, nil
	}
	if includeRequired && field.notNull {
		definition += " NOT NULL"
	}
	if field.belongsToTable != "" {
		definition += " REFERENCES " + quoteIdentifier(field.belongsToTable) + "(" + quoteIdentifier(field.belongsToColumn) + ")"
	}

	return definition, nil
}

// sqlColumnType maps supported Go field types to Turso SQL storage types.
func sqlColumnType(field field) (string, error) {
	// Initialize Variables
	kind := field.goType.Kind()
	nullStringType := reflect.TypeFor[sql.NullString]()
	nullInt64Type := reflect.TypeFor[sql.NullInt64]()
	nullFloat64Type := reflect.TypeFor[sql.NullFloat64]()
	nullBoolType := reflect.TypeFor[sql.NullBool]()

	if field.isVector {
		return "BLOB", nil
	}
	switch field.goType {
	case nullStringType:
		return "TEXT", nil
	case nullInt64Type, nullBoolType:
		return "INTEGER", nil
	case nullFloat64Type:
		return "REAL", nil
	}

	switch kind {
	case reflect.String:
		return "TEXT", nil
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "INTEGER", nil
	case reflect.Float32, reflect.Float64:
		return "REAL", nil
	case reflect.Slice:
		if field.goType.Elem().Kind() == reflect.Uint8 {
			return "BLOB", nil
		}
	}

	return "", fmt.Errorf("activeso: cannot infer a SQL type for %s", field.goType)
}

// existingColumns returns each column's identity metadata from the model's existing table.
func (model *model[T]) existingColumns(ctx context.Context, transaction *sql.Tx) (map[string]columnInfo, error) {
	// Initialize Variables
	statement := fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(model.tableName))
	rows, err := transaction.QueryContext(ctx, statement)
	columns := make(map[string]columnInfo)
	primaryKeyColumns := 0

	if err != nil {
		return nil, fmt.Errorf("activeso: inspect table %s: %w", model.tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var index int
		var name string
		var columnType string
		var notNull bool
		var defaultValue any
		var primaryKeyPosition int
		if err := rows.Scan(&index, &name, &columnType, &notNull, &defaultValue, &primaryKeyPosition); err != nil {
			return nil, fmt.Errorf("activeso: inspect columns for %s: %w", model.tableName, err)
		}
		primaryKey := primaryKeyPosition > 0
		if primaryKey {
			primaryKeyColumns++
		}
		columns[strings.ToLower(name)] = columnInfo{primaryKey: primaryKey}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activeso: iterate columns for %s: %w", model.tableName, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("activeso: close column inspection for %s: %w", model.tableName, err)
	}
	if primaryKeyColumns == 1 {
		for name, column := range columns {
			if column.primaryKey {
				column.unique = true
				columns[name] = column
			}
		}
	}

	uniqueColumns, err := model.uniqueColumns(ctx, transaction)
	if err != nil {
		return nil, err
	}
	for name := range uniqueColumns {
		column, found := columns[name]
		if found {
			column.unique = true
			columns[name] = column
		}
	}

	return columns, nil
}

// uniqueColumns returns columns protected by a single-column unique index on the model's table.
func (model *model[T]) uniqueColumns(ctx context.Context, transaction *sql.Tx) (map[string]bool, error) {
	// Initialize Variables
	statement := fmt.Sprintf("PRAGMA index_list(%s)", quoteIdentifier(model.tableName))
	rows, err := transaction.QueryContext(ctx, statement)
	indexNames := make([]string, 0)
	columns := make(map[string]bool)

	if err != nil {
		return nil, fmt.Errorf("activeso: inspect indexes for %s: %w", model.tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var sequence int
		var name string
		var unique bool
		var origin string
		var partial bool
		if err := rows.Scan(&sequence, &name, &unique, &origin, &partial); err != nil {
			return nil, fmt.Errorf("activeso: inspect indexes for %s: %w", model.tableName, err)
		}
		if unique {
			indexNames = append(indexNames, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activeso: iterate indexes for %s: %w", model.tableName, err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("activeso: close index inspection for %s: %w", model.tableName, err)
	}

	for _, name := range indexNames {
		statement = fmt.Sprintf("PRAGMA index_info(%s)", quoteIdentifier(name))
		rows, err = transaction.QueryContext(ctx, statement)
		if err != nil {
			return nil, fmt.Errorf("activeso: inspect index %s for %s: %w", name, model.tableName, err)
		}

		var indexedColumns []string
		for rows.Next() {
			var sequence, columnIndex int
			var columnName string
			if err := rows.Scan(&sequence, &columnIndex, &columnName); err != nil {
				rows.Close()
				return nil, fmt.Errorf("activeso: inspect index %s for %s: %w", name, model.tableName, err)
			}
			indexedColumns = append(indexedColumns, strings.ToLower(columnName))
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("activeso: iterate index %s for %s: %w", name, model.tableName, err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("activeso: close index %s for %s: %w", name, model.tableName, err)
		}
		if len(indexedColumns) == 1 {
			columns[indexedColumns[0]] = true
		}
	}

	return columns, nil
}

// uniqueIndexName returns an unambiguous stable name used for one field's unique index.
func (model *model[T]) uniqueIndexName(field field) string {
	// Initialize Variables
	tableName := strings.ToLower(model.tableName)
	columnName := strings.ToLower(field.column)
	table := hex.EncodeToString([]byte(tableName))
	column := hex.EncodeToString([]byte(columnName))
	name := "activeso_" + table + "_" + column + "_unique"

	return name
}

// indexName returns an unambiguous stable name used for one field's ordinary index.
func (model *model[T]) indexName(field field) string {
	// Initialize Variables
	tableName := strings.ToLower(model.tableName)
	columnName := strings.ToLower(field.column)
	table := hex.EncodeToString([]byte(tableName))
	column := hex.EncodeToString([]byte(columnName))
	name := "activeso_" + table + "_" + column + "_index"

	return name
}

// uniqueWriteError translates a Turso unique violation to ActiveSo's public uniqueness errors.
func (model *model[T]) uniqueWriteError(err error) error {
	// Initialize Variables
	message := strings.ToLower(err.Error())
	uniqueViolation := errors.Is(err, turso.ErrTursoConstraint) && strings.Contains(message, "unique constraint")

	if !uniqueViolation {
		return err
	}
	for _, field := range model.fields {
		if field.unique && strings.Contains(message, strings.ToLower(field.column)) {
			return UniqueError{Field: field.column}
		}
	}

	return ErrUnique
}

// validateUnique rejects duplicate values before a write while the database index remains authoritative.
func (model *model[T]) validateUnique(ctx context.Context, record *T) error {
	// Initialize Variables
	valueOfT := reflect.ValueOf(record).Elem()
	id, err := model.idValue(record)

	if err != nil {
		return err
	}

	for _, field := range model.fields {
		if !field.unique || field.isID {
			continue
		}

		value, expression, err := databaseValue(valueOfT.Field(field.index), field.isVector)
		if err != nil {
			return err
		}
		if value == nil {
			continue
		}

		statement := fmt.Sprintf("SELECT 1 FROM %s WHERE %s = %s AND %s <> ? LIMIT 1", quoteIdentifier(model.tableName), quoteIdentifier(field.column), expression, quoteIdentifier(model.idField.column))
		var match int
		err = model.db.QueryRowContext(ctx, statement, value, id).Scan(&match)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("activeso: validate %s.%s uniqueness: %w", model.tableName, field.column, err)
		}

		return UniqueError{Field: field.column}
	}

	return nil
}

// databaseValue converts a reflected field into a database/sql argument and SQL expression.
func databaseValue(value reflect.Value, isVector bool) (any, string, error) {
	// Initialize Variables
	interfaceValue := value.Interface()

	if !isVector {
		if value.CanInterface() {
			if valuer, ok := interfaceValue.(driver.Valuer); ok {
				converted, err := valuer.Value()
				if err != nil {
					return nil, "", err
				}
				return converted, "?", nil
			}
		}
		return interfaceValue, "?", nil
	}

	vector := interfaceValue.(Vector32)
	if vector == nil {
		return nil, "?", nil
	}
	encoded, err := vectorJSON(vector)
	if err != nil {
		return nil, "", err
	}
	return encoded, "vector32(?)", nil
}

// idValue obtains the primary key value from record.
func (model *model[T]) idValue(record *T) (any, error) {
	// Initialize Variables
	value := reflect.ValueOf(record).Elem().Field(model.idField.index)

	if value.Kind() == reflect.String && value.String() == "" {
		return nil, errors.New("activeso: record ID is empty")
	}

	return value.Interface(), nil
}

// selectColumns returns scan-friendly SELECT expressions for the model's fields.
func (model *model[T]) selectColumns() []string {
	// Initialize Variables
	columns := make([]string, 0, len(model.fields)+2)

	for _, field := range model.fields {
		column := quoteIdentifier(field.column)
		if field.isVector {
			columns = append(columns, "vector_extract("+column+") AS "+column)
			continue
		}
		if nullableScalar(field.goType) {
			columns = append(columns, "COALESCE("+column+", 0) AS "+column)
			continue
		}
		columns = append(columns, column)
	}
	columns = append(columns, quoteIdentifier(createdAtColumn), quoteIdentifier(updatedAtColumn))

	return columns
}

// nullableScalar reports whether a nullable scalar should read SQL NULL as its Go zero value.
func nullableScalar(typeOfT reflect.Type) bool {
	// Initialize Variables
	kind := typeOfT.Kind()

	switch kind {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// scan maps the current SQL row into record and leaves Record's private binding untouched.
func (model *model[T]) scan(rows *sql.Rows, record *T) error {
	// Initialize Variables
	values := make([]any, len(model.fields))
	destinations := make([]any, len(model.fields)+2)
	nullableStrings := make([]sql.NullString, len(model.fields))
	createdAt := sql.NullString{}
	updatedAt := sql.NullString{}
	valueOfT := reflect.ValueOf(record).Elem()
	recordValue := valueOfT.FieldByName("Record").Addr().Interface().(*Record)

	for index, field := range model.fields {
		if field.isVector {
			destinations[index] = &values[index]
			continue
		}
		if field.goType.Kind() == reflect.String {
			destinations[index] = &nullableStrings[index]
			continue
		}
		destinations[index] = valueOfT.Field(field.index).Addr().Interface()
	}
	destinations[len(model.fields)] = &createdAt
	destinations[len(model.fields)+1] = &updatedAt

	if err := rows.Scan(destinations...); err != nil {
		return fmt.Errorf("activeso: scan %s: %w", model.tableName, err)
	}

	for index, field := range model.fields {
		if field.isVector {
			vector, err := parseVector32(values[index])
			if err != nil {
				return err
			}
			valueOfT.Field(field.index).Set(reflect.ValueOf(vector))
			continue
		}
		if field.goType.Kind() == reflect.String {
			// Normalize database NULL values to Go's zero string.
			valueOfT.Field(field.index).SetString(nullableStrings[index].String)
		}
	}
	if err := assignRecordTimestamps(recordValue, createdAt, updatedAt); err != nil {
		return err
	}

	return nil
}

// refreshTimestamps loads the managed timestamp values after an ActiveSo write.
func (model *model[T]) refreshTimestamps(ctx context.Context, record *T) error {
	// Initialize Variables
	createdAt := sql.NullString{}
	updatedAt := sql.NullString{}
	id, err := model.idValue(record)
	statement := fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s = ?", quoteIdentifier(createdAtColumn), quoteIdentifier(updatedAtColumn), quoteIdentifier(model.tableName), quoteIdentifier(model.idField.column))
	recordValue := reflect.ValueOf(record).Elem().FieldByName("Record").Addr().Interface().(*Record)

	if err != nil {
		return err
	}
	if err := model.db.QueryRowContext(ctx, statement, id).Scan(&createdAt, &updatedAt); err != nil {
		return fmt.Errorf("activeso: load timestamps for %s: %w", model.tableName, err)
	}

	return assignRecordTimestamps(recordValue, createdAt, updatedAt)
}

// assignRecordTimestamps decodes database timestamps into the embedded Record fields.
func assignRecordTimestamps(record *Record, createdAt, updatedAt sql.NullString) error {
	// Initialize Variables
	createdValue, err := timestampValue(createdAt)
	updatedValue := time.Time{}

	if err != nil {
		return err
	}
	updatedValue, err = timestampValue(updatedAt)
	if err != nil {
		return err
	}

	record.CreatedAt = createdValue
	record.UpdatedAt = updatedValue
	return nil
}

// timestampValue parses Turso's UTC timestamp text, preserving SQL NULL as time.Time's zero value.
func timestampValue(value sql.NullString) (time.Time, error) {
	// Initialize Variables
	layouts := []string{time.DateTime, time.RFC3339Nano}
	zero := time.Time{}

	if !value.Valid {
		return zero, nil
	}
	for _, layout := range layouts {
		parsed, err := time.ParseInLocation(layout, value.String, time.UTC)
		if err == nil {
			return parsed, nil
		}
	}

	return zero, fmt.Errorf("activeso: parse timestamp %q", value.String)
}

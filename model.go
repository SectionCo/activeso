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
	"sync"
	"time"
	"unicode"

	"github.com/gobuffalo/flect"
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
	onDeleteCascade bool
	isID            bool
	isVector        bool
	notNull         bool
	unique          bool
	uniqueWith      []string
	indexed         bool
	primaryKey      bool
}

const (
	createdAtColumn = "created_at"
	updatedAtColumn = "updated_at"
)

type model[T any] struct {
	db         *sql.DB
	tableName  string
	fields     []field
	idField    field
	timestamps bool

	readyMutex sync.Mutex
	ready      bool
}

type query[T any] struct {
	model            *model[T]
	conditions       []string
	arguments        []any
	orderBy          string
	orderByArguments []any
	limit            int
}

// Model binds an application-defined struct type to an existing Turso table using db.
// T must embed activeso.Record, tag every persisted field with db, and expose one primary_key field or an id column.
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

	// Confirm the table matches the model, then generate an ID before inserting the record.
	if err := model.ensureReady(ctx); err != nil {
		return nil, err
	}
	if err := model.generateID(record); err != nil {
		return nil, err
	}
	if err := model.bind(record); err != nil {
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
	expression := "?"

	if !found {
		return nil, fmt.Errorf("activeso: column %s is not defined on %s", column, model.tableName)
	}

	// Compare vector BLOBs using Turso's vector32 conversion.
	if field.isVector {
		expression = "vector32(?)"
	}

	return model.Where(fmt.Sprintf("%s = %s", quoteIdentifier(field.column), expression), value).All(ctx)
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
	if err := model.ensureReady(ctx); err != nil {
		return err
	}

	assignments, arguments, err := model.updateValues(record)
	if err != nil {
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
	if err := model.ensureReady(ctx); err != nil {
		return err
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
	if err := query.model.ensureReady(ctx); err != nil {
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
			// The embedded Record opts into managed timestamp columns with an activeso hint.
			timestamps, err := recordHints(structField)
			if err != nil {
				return nil, err
			}
			model.timestamps = timestamps
			continue
		}
		if !structField.IsExported() {
			continue
		}

		column, err := columnName(typeOfT, structField)
		if err != nil {
			return nil, err
		}
		if column == "" {
			continue
		}
		notNull, unique, uniqueWith, indexed, primaryKey, belongsToTable, belongsToColumn, onDeleteCascade, err := fieldHints(structField)
		if err != nil {
			return nil, err
		}
		normalizedColumn := strings.ToLower(column)
		if _, exists := columns[normalizedColumn]; exists {
			return nil, fmt.Errorf("activeso: model type %s maps more than one field to column %s", typeOfT, column)
		}
		columns[normalizedColumn] = struct{}{}

		field := field{index: index, column: column, goType: structField.Type, belongsToTable: belongsToTable, belongsToColumn: belongsToColumn, onDeleteCascade: onDeleteCascade, isVector: structField.Type == reflect.TypeFor[Vector32](), notNull: notNull, unique: unique, uniqueWith: uniqueWith, indexed: indexed, primaryKey: primaryKey}
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
	if err := validateUniqueWith(fields); err != nil {
		return nil, err
	}
	// Reserve the timestamp columns so a field cannot collide with the managed values.
	if model.timestamps {
		for _, reserved := range []string{createdAtColumn, updatedAtColumn} {
			if _, exists := columns[reserved]; exists {
				return nil, fmt.Errorf("activeso: model type %s maps a field to %s, which the timestamps hint reserves", typeOfT, reserved)
			}
		}
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

// columnName returns the column named by a field's db tag; every persisted field must declare one.
func columnName(typeOfT reflect.Type, structField reflect.StructField) (string, error) {
	// Initialize Variables
	tag := structField.Tag.Get("db")
	parts := strings.Split(tag, ",")

	if tag == "-" {
		return "", nil
	}
	if parts[0] == "" {
		return "", fmt.Errorf("activeso: field %s on %s requires a db tag naming its column (use db:\"-\" to skip it)", structField.Name, typeOfT)
	}

	return parts[0], nil
}

// recordHints parses the activeso hints declared on the embedded Record field.
func recordHints(structField reflect.StructField) (bool, error) {
	// Initialize Variables
	hints := strings.Split(structField.Tag.Get("activeso"), ",")
	timestamps := false

	for _, hint := range hints {
		switch hint {
		case "", "-":
			continue
		case "timestamps":
			timestamps = true
		default:
			return false, fmt.Errorf("activeso: unsupported hint %q on Record; the only supported hint is timestamps", hint)
		}
	}

	return timestamps, nil
}

// fieldHints parses the ActiveSo schema hints declared on a struct field.
func fieldHints(structField reflect.StructField) (bool, bool, []string, bool, bool, string, string, bool, error) {
	// Initialize Variables
	tag := structField.Tag.Get("activeso")
	hints := strings.Split(tag, ",")
	notNull := false
	unique := false
	uniqueWith := []string(nil)
	indexed := false
	primaryKey := false
	belongsToTable := ""
	belongsToColumn := ""
	onDeleteCascade := false
	onDeleteSeen := false

	for _, hint := range hints {
		switch hint {
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
		case "on_delete=cascade":
			if onDeleteSeen {
				return false, false, nil, false, false, "", "", false, fmt.Errorf("activeso: duplicate on_delete hint on field %s", structField.Name)
			}
			onDeleteCascade = true
			onDeleteSeen = true
		default:
			if strings.HasPrefix(hint, "unique_with=") {
				if uniqueWith != nil {
					return false, false, nil, false, false, "", "", false, fmt.Errorf("activeso: duplicate unique_with hint on field %s", structField.Name)
				}
				columns, err := uniqueWithColumns(strings.TrimPrefix(hint, "unique_with="))
				if err != nil {
					return false, false, nil, false, false, "", "", false, fmt.Errorf("activeso: invalid unique_with hint on field %s: %w", structField.Name, err)
				}
				uniqueWith = columns
				continue
			}
			if !strings.HasPrefix(hint, "belongs_to=") {
				return false, false, nil, false, false, "", "", false, fmt.Errorf("activeso: unsupported hint %q on field %s", hint, structField.Name)
			}
			if belongsToTable != "" {
				return false, false, nil, false, false, "", "", false, fmt.Errorf("activeso: duplicate belongs_to hint on field %s", structField.Name)
			}

			var err error
			belongsToTable, belongsToColumn, err = belongsToTarget(strings.TrimPrefix(hint, "belongs_to="))
			if err != nil {
				return false, false, nil, false, false, "", "", false, fmt.Errorf("activeso: invalid belongs_to hint on field %s: %w", structField.Name, err)
			}
		}
	}
	if unique && uniqueWith != nil {
		return false, false, nil, false, false, "", "", false, fmt.Errorf("activeso: field %s cannot combine unique and unique_with", structField.Name)
	}
	if onDeleteCascade && belongsToTable == "" {
		return false, false, nil, false, false, "", "", false, fmt.Errorf("activeso: on_delete requires belongs_to on field %s", structField.Name)
	}

	return notNull, unique, uniqueWith, indexed, primaryKey, belongsToTable, belongsToColumn, onDeleteCascade, nil
}

// uniqueWithColumns validates the ordered columns used by a composite unique index.
func uniqueWithColumns(value string) ([]string, error) {
	// Initialize Variables
	parts := strings.Split(value, "+")
	columns := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	// Require simple, distinct column identifiers to keep tags out of generated SQL.
	for _, column := range parts {
		if !schemaIdentifier(column) {
			return nil, fmt.Errorf("column %q must be a simple identifier", column)
		}
		key := strings.ToLower(column)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("column %q is listed more than once", column)
		}
		seen[key] = struct{}{}
		columns = append(columns, column)
	}

	return columns, nil
}

// validateUniqueWith confirms every composite unique declaration references mapped columns.
func validateUniqueWith(fields []field) error {
	// Initialize Variables
	columns := make(map[string]struct{}, len(fields))

	// Build a case-insensitive set because SQLite column names are case-insensitive.
	for _, field := range fields {
		columns[strings.ToLower(field.column)] = struct{}{}
	}
	for _, field := range fields {
		if field.uniqueWith == nil {
			continue
		}
		if field.isID {
			return fmt.Errorf("activeso: primary key field %s cannot declare unique_with", field.column)
		}
		for _, column := range field.uniqueWith {
			if strings.EqualFold(field.column, column) {
				return fmt.Errorf("activeso: unique_with on field %s cannot include itself", field.column)
			}
			if _, found := columns[strings.ToLower(column)]; !found {
				return fmt.Errorf("activeso: unique_with on field %s references undefined column %s", field.column, column)
			}
		}
	}

	return nil
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

// pluralize converts a table identifier to its English plural without duplicating plural names.
func pluralize(value string) string {
	// Initialize Variables
	plural := flect.Pluralize(value)

	return plural
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
	// Stamp both managed timestamps in SQL so the table needs no defaults or triggers.
	if model.timestamps {
		columns = append(columns, quoteIdentifier(createdAtColumn), quoteIdentifier(updatedAtColumn))
		expressions = append(expressions, "CURRENT_TIMESTAMP", "CURRENT_TIMESTAMP")
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
	// Refresh the update timestamp on every Save; creation time is never rewritten.
	if model.timestamps {
		assignments = append(assignments, quoteIdentifier(updatedAtColumn)+" = CURRENT_TIMESTAMP")
	}

	return assignments, arguments, nil
}

// fieldForColumn returns the persisted field that maps to column.
func (model *model[T]) fieldForColumn(column string) (field, bool) {
	// Initialize Variables
	var zero field

	for _, field := range model.fields {
		if strings.EqualFold(field.column, column) {
			return field, true
		}
	}

	return zero, false
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

// sqliteTypeAffinity returns SQLite's comparison affinity for a declared column type.
func sqliteTypeAffinity(columnType string) string {
	// Initialize Variables
	normalized := strings.ToUpper(strings.TrimSpace(columnType))

	// Treat SQLite type synonyms as equivalent so legacy schemas remain compatible.
	switch {
	case strings.Contains(normalized, "INT"):
		return "INTEGER"
	case strings.Contains(normalized, "CHAR"), strings.Contains(normalized, "CLOB"), strings.Contains(normalized, "TEXT"):
		return "TEXT"
	case strings.Contains(normalized, "BLOB") || normalized == "":
		return "BLOB"
	case strings.Contains(normalized, "REAL"), strings.Contains(normalized, "FLOA"), strings.Contains(normalized, "DOUB"):
		return "REAL"
	default:
		return "NUMERIC"
	}
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
	if model.timestamps {
		columns = append(columns, quoteIdentifier(createdAtColumn), quoteIdentifier(updatedAtColumn))
	}

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
	destinations := make([]any, len(model.fields), len(model.fields)+2)
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
	if model.timestamps {
		destinations = append(destinations, &createdAt, &updatedAt)
	}

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
	if model.timestamps {
		if err := assignRecordTimestamps(recordValue, createdAt, updatedAt); err != nil {
			return err
		}
	}

	return nil
}

// refreshTimestamps loads the managed timestamp values after an ActiveSo write when the timestamps hint is set.
func (model *model[T]) refreshTimestamps(ctx context.Context, record *T) error {
	// Initialize Variables
	createdAt := sql.NullString{}
	updatedAt := sql.NullString{}
	id, err := model.idValue(record)
	statement := fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s = ?", quoteIdentifier(createdAtColumn), quoteIdentifier(updatedAtColumn), quoteIdentifier(model.tableName), quoteIdentifier(model.idField.column))
	recordValue := reflect.ValueOf(record).Elem().FieldByName("Record").Addr().Interface().(*Record)

	if !model.timestamps {
		return nil
	}
	if err != nil {
		return err
	}
	if err := model.db.QueryRowContext(ctx, statement, id).Scan(&createdAt, &updatedAt); err != nil {
		return fmt.Errorf("activeso: load timestamps for %s; the timestamps hint expects columns %s and %s: %w", model.tableName, createdAtColumn, updatedAtColumn, err)
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

	return zero, fmt.Errorf("activeso: parse timestamp %q; the timestamps hint expects UTC text in columns %s and %s", value.String, createdAtColumn, updatedAtColumn)
}

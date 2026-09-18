package activeso

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type schemaToken struct {
	text   string
	start  int
	end    int
	quoted bool
}

// schemaTokens locates SQL tokens without interpreting quoted text or comments as syntax.
func schemaTokens(statement string) ([]schemaToken, error) {
	// Initialize Variables
	var tokens []schemaToken

	// Keep byte offsets so edits preserve the original SQL outside the target.
	for index := 0; index < len(statement); {
		start := index
		character := statement[index]
		if strings.ContainsRune(" \t\r\n", rune(character)) {
			index++
			continue
		}
		if strings.HasPrefix(statement[index:], "--") {
			for index < len(statement) && statement[index] != '\n' {
				index++
			}
			continue
		}
		if strings.HasPrefix(statement[index:], "/*") {
			end := strings.Index(statement[index+2:], "*/")
			if end < 0 {
				return nil, fmt.Errorf("activeso: unterminated schema comment")
			}
			index += end + 4
			continue
		}
		if strings.ContainsRune("\"'`[", rune(character)) {
			closing := character
			if closing == '[' {
				closing = ']'
			}
			index++
			var value strings.Builder
			closed := false
			for index < len(statement) {
				if statement[index] == closing {
					index++
					if closing != ']' && index < len(statement) && statement[index] == closing {
						value.WriteByte(closing)
						index++
						continue
					}
					closed = true
					break
				}
				value.WriteByte(statement[index])
				index++
			}
			if !closed {
				return nil, fmt.Errorf("activeso: unterminated schema quote")
			}
			tokens = append(tokens, schemaToken{value.String(), start, index, true})
			continue
		}
		if strings.ContainsRune("(),;", rune(character)) {
			index++
		} else {
			for index < len(statement) && !strings.ContainsRune(" \t\r\n(),;\"'`[", rune(statement[index])) && !strings.HasPrefix(statement[index:], "/*") && !strings.HasPrefix(statement[index:], "--") {
				index++
			}
		}
		tokens = append(tokens, schemaToken{statement[start:index], start, index, false})
	}
	return tokens, nil
}

// schemaKeyword matches unquoted SQL syntax independently of case.
func schemaKeyword(token schemaToken, word string) bool {
	// Initialize Variables
	matches := !token.quoted && strings.EqualFold(token.text, word)

	return matches
}

// migrationDefinition edits only the requested column in an existing CREATE TABLE statement.
func migrationDefinition(statement, temporaryTable, column, operation, columnType string) (string, []string, error) {
	// Initialize Variables
	tokens, err := schemaTokens(statement)
	open := -1
	close := -1
	depth := 0
	start := 0
	found := false
	var definitions []string
	var columns []string

	if err != nil {
		return "", nil, err
	}
	if len(tokens) < 4 || !schemaKeyword(tokens[0], "CREATE") || !schemaKeyword(tokens[1], "TABLE") {
		return "", nil, fmt.Errorf("activeso: migration requires an ordinary CREATE TABLE definition")
	}

	// Split definitions only at top-level commas, respecting expressions and quotes.
	for index, token := range tokens {
		if schemaKeyword(token, "(") {
			depth++
			if open < 0 {
				open = index
				start = index + 1
			}
			continue
		}
		if schemaKeyword(token, ")") {
			depth--
			if depth == 0 {
				close = index
			}
		}
		if open < 0 || !(depth == 1 && schemaKeyword(token, ",") || close == index) {
			continue
		}
		if start == index {
			return "", nil, fmt.Errorf("activeso: empty table definition")
		}
		definition := tokens[start:index]
		first := definition[0]
		raw := statement[first.start:token.start]
		constraint := schemaKeyword(first, "CONSTRAINT") || schemaKeyword(first, "PRIMARY") || schemaKeyword(first, "UNIQUE") || schemaKeyword(first, "CHECK") || schemaKeyword(first, "FOREIGN")
		// Do not let quoted references to a dropped column become string literals.
		if operation == "drop" && (constraint || !strings.EqualFold(first.text, column)) {
			for _, part := range definition[1:] {
				if strings.EqualFold(part.text, column) {
					return "", nil, fmt.Errorf("activeso: remove constraints referencing %s before dropping it", column)
				}
			}
		}
		if !constraint {
			// Generated columns and rowid allocation rules need a specialized migration.
			for _, part := range definition[1:] {
				if schemaKeyword(part, "AS") || schemaKeyword(part, "AUTOINCREMENT") || schemaKeyword(part, "REFERENCES") {
					return "", nil, fmt.Errorf("activeso: cannot safely rebuild generated, autoincrement, or foreign-key columns")
				}
			}
			if strings.EqualFold(first.text, column) {
				found = true
				if operation == "drop" {
					start = index + 1
					continue
				}
				if operation == "type" {
					endType := token.start
					for _, part := range definition[1:] {
						if !part.quoted && strings.Contains("|CONSTRAINT|PRIMARY|NOT|NULL|UNIQUE|CHECK|DEFAULT|COLLATE|REFERENCES|GENERATED|AS|", "|"+strings.ToUpper(part.text)+"|") {
							endType = part.start
							break
						}
					}
					// Changing primary-key storage can change the rowid identity.
					for _, part := range definition[1:] {
						if schemaKeyword(part, "PRIMARY") {
							return "", nil, fmt.Errorf("activeso: cannot safely change a primary-key type")
						}
					}
					raw = statement[first.start:first.end] + " " + columnType + " " + statement[endType:token.start]
				} else if operation == "not_null" {
					// Explicit NULL and conflict clauses require more than appending a constraint.
					alreadyRequired := false
					expressionDepth := 0
					for position, part := range definition {
						if schemaKeyword(part, "(") {
							expressionDepth++
						}
						if schemaKeyword(part, ")") {
							expressionDepth--
						}
						if expressionDepth == 0 && schemaKeyword(part, "NULL") {
							if position > 0 && schemaKeyword(definition[position-1], "NOT") {
								alreadyRequired = true
							} else {
								return "", nil, fmt.Errorf("activeso: cannot safely tighten an explicit NULL expression on %s", column)
							}
						}
					}
					if !alreadyRequired {
						raw += "\nNOT NULL"
					}
				}
			}
			columns = append(columns, first.text)
		} else if operation == "type" {
			for _, part := range definition {
				if schemaKeyword(part, "PRIMARY") {
					return "", nil, fmt.Errorf("activeso: cannot safely change types with a table-level primary key")
				}
			}
		}
		definitions = append(definitions, raw)
		start = index + 1
		if close >= 0 {
			break
		}
	}
	if !found || close < 0 || len(columns) == 0 {
		return "", nil, fmt.Errorf("activeso: column %s is absent or cannot be removed", column)
	}
	return "CREATE TABLE " + quoteIdentifier(temporaryTable) + " (" + strings.Join(definitions, "\n,") + "\n)" + statement[tokens[close].end:], columns, nil
}

// migrationSchema reads the original SQL and indexes, rejecting dependencies unsafe to rebuild.
func migrationSchema(ctx context.Context, transaction *sql.Tx, table string) (string, []string, error) {
	// Initialize Variables
	rows, err := transaction.QueryContext(ctx, "SELECT type, name, tbl_name, sql FROM sqlite_schema WHERE sql IS NOT NULL")
	var definition string
	var indexes []string

	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	// Inspect references before issuing any schema writes.
	for rows.Next() {
		var kind, name, owner, statement string
		if err := rows.Scan(&kind, &name, &owner, &statement); err != nil {
			return "", nil, err
		}
		if kind == "table" && strings.EqualFold(name, table) {
			definition = statement
		}
		if kind == "index" && strings.EqualFold(owner, table) {
			indexes = append(indexes, statement)
		}
		if kind == "index" {
			continue
		}
		tokens, err := schemaTokens(statement)
		if err != nil {
			return "", nil, err
		}
		for index, token := range tokens {
			if (kind == "trigger" || kind == "view") && (strings.EqualFold(owner, table) || strings.EqualFold(token.text, table)) {
				return "", nil, fmt.Errorf("activeso: cannot safely rebuild %s with dependent %s %s", table, kind, name)
			}
			if kind == "table" && schemaKeyword(token, "REFERENCES") && index+1 < len(tokens) && (strings.EqualFold(name, table) || strings.EqualFold(tokens[index+1].text, table)) {
				return "", nil, fmt.Errorf("activeso: cannot safely rebuild %s with foreign-key dependencies", table)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	if definition == "" {
		return "", nil, fmt.Errorf("activeso: table %s does not exist", table)
	}
	return definition, indexes, nil
}

// rebuildTable changes one existing column while preserving the remaining stored schema and rows.
func (model *model[T]) rebuildTable(ctx context.Context, column, operation, columnType string) error {
	// Initialize Variables
	transaction, err := model.db.BeginTx(ctx, nil)
	temporaryTable := "activeso_" + model.tableName + "_rebuild"
	var copyColumns []string
	rowID := ""
	var definition, statement string
	var indexes, columns []string

	if err != nil {
		return fmt.Errorf("activeso: begin migration: %w", err)
	}
	defer transaction.Rollback()
	definition, indexes, err = migrationSchema(ctx, transaction, model.tableName)
	if err != nil {
		return err
	}
	// Require explicit removal of indexes that reference a dropped column.
	if operation == "drop" {
		for _, index := range indexes {
			tokens, err := schemaTokens(index)
			if err != nil {
				return err
			}
			for _, token := range tokens {
				if strings.EqualFold(token.text, column) {
					return fmt.Errorf("activeso: remove indexes referencing %s before dropping it", column)
				}
			}
		}
	}
	statement, columns, err = migrationDefinition(definition, temporaryTable, column, operation, columnType)
	if err != nil {
		return err
	}

	// Preserve hidden row IDs, including tables with an INTEGER PRIMARY KEY alias.
	for _, candidate := range []string{"rowid", "_rowid_", "oid"} {
		shadowed := false
		for _, name := range columns {
			if strings.EqualFold(name, candidate) {
				shadowed = true
			}
		}
		if strings.EqualFold(column, candidate) {
			shadowed = true
		}
		if !shadowed {
			rowID = candidate
			break
		}
	}
	if rowID == "" {
		return fmt.Errorf("activeso: cannot safely preserve shadowed row IDs")
	}
	copyColumns = append(copyColumns, quoteIdentifier(rowID))
	for _, name := range columns {
		copyColumns = append(copyColumns, quoteIdentifier(name))
	}

	// Build and populate the replacement before dropping the original table.
	if _, err := transaction.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("activeso: create replacement table: %w", err)
	}
	statement = fmt.Sprintf("INSERT INTO %s (%s) SELECT %s FROM %s", quoteIdentifier(temporaryTable), strings.Join(copyColumns, ", "), strings.Join(copyColumns, ", "), quoteIdentifier(model.tableName))
	if _, err := transaction.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("activeso: copy migration rows: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "DROP TABLE "+quoteIdentifier(model.tableName)); err != nil {
		return fmt.Errorf("activeso: drop original table: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "ALTER TABLE "+quoteIdentifier(temporaryTable)+" RENAME TO "+quoteIdentifier(model.tableName)); err != nil {
		return fmt.Errorf("activeso: rename replacement table: %w", err)
	}
	// Recreate original indexes; an incompatible dependency rolls the entire migration back.
	for _, index := range indexes {
		if _, err := transaction.ExecContext(ctx, index); err != nil {
			return fmt.Errorf("activeso: restore index; remove dependent indexes explicitly before migrating: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("activeso: commit migration: %w", err)
	}
	return nil
}

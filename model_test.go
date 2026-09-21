package activeso

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	_ "turso.tech/database/tursogo"
)

type testUser struct {
	Record

	ID        string   `db:"id"`
	Email     string   `db:"email" activeso:"not_null,unique"`
	Embedding Vector32 `db:"embedding"`
}

type migrationUserV1 struct {
	Record

	ID    string `db:"id"`
	Email string `db:"email"`
}

type migrationUserV2 struct {
	Record

	ID       string `db:"id"`
	Email    string `db:"email"`
	Nickname string `db:"nickname"`
}

type migrationUserV3 struct {
	Record

	ID       string `db:"id"`
	Email    string `db:"email"`
	Nickname string `db:"nickname"`
	Required string `db:"required" activeso:"not_null"`
}

type migrationUserIntegerEmail struct {
	Record

	ID    string `db:"id"`
	Email int    `db:"email"`
}

type migrationUserRequiredNickname struct {
	Record

	ID       string `db:"id"`
	Email    string `db:"email"`
	Nickname string `db:"nickname" activeso:"not_null"`
}

type nullableStringUser struct {
	Record

	ID       string         `db:"id"`
	Email    string         `db:"email"`
	Nickname sql.NullString `db:"nickname"`
}

type idOnlyUser struct {
	Record

	ID string `db:"id"`
}

type numericMigrationV1 struct {
	Record

	ID string `db:"id"`
}

type numericMigrationV2 struct {
	Record

	ID      string  `db:"id"`
	Count   int     `db:"count"`
	Score   float64 `db:"score"`
	Enabled bool    `db:"enabled"`
}

type explicitPrimaryKeyUser struct {
	Record

	ExternalID string `db:"external_id" activeso:"primary_key"`
	Email      string `db:"email"`
}

type requiredPrimaryKeyUser struct {
	Record

	ExternalID string `db:"external_id" activeso:"not_null,primary_key"`
}

type duplicateColumnUser struct {
	Record

	ID    string `db:"id"`
	Email string `db:"email"`
	Alias string `db:"EMAIL"`
}

type multiplePrimaryKeyUser struct {
	Record

	ID         string `db:"id" activeso:"primary_key"`
	ExternalID string `db:"external_id" activeso:"primary_key"`
}

type mutablePrimaryKeyUser struct {
	Record

	ID []byte `db:"id"`
}

type uniqueCollisionFirst struct {
	Record

	ID  string `db:"id"`
	Baz string `db:"baz" activeso:"unique"`
}

type uniqueCollisionSecond struct {
	Record

	ID     string `db:"id"`
	BarBaz string `db:"bar_baz" activeso:"unique"`
}

type caseColumnUser struct {
	Record

	ID    string `db:"id"`
	Email string `db:"email"`
}

// TableName maps testUser to the temporary users table.
func (testUser) TableName() string {
	// Initialize Variables
	name := "users"

	return name
}

// TableName maps migrationUserV1 to the shared migration test table.
func (migrationUserV1) TableName() string {
	// Initialize Variables
	name := "migration_users"

	return name
}

// TableName maps migrationUserV2 to the shared migration test table.
func (migrationUserV2) TableName() string {
	// Initialize Variables
	name := "migration_users"

	return name
}

// TableName maps migrationUserV3 to the shared migration test table.
func (migrationUserV3) TableName() string {
	// Initialize Variables
	name := "migration_users"

	return name
}

// TableName maps migrationUserIntegerEmail to the shared migration test table.
func (migrationUserIntegerEmail) TableName() string {
	// Initialize Variables
	name := "migration_users"

	return name
}

// TableName maps migrationUserRequiredNickname to the shared migration test table.
func (migrationUserRequiredNickname) TableName() string {
	// Initialize Variables
	name := "migration_users"

	return name
}

// TableName maps nullableStringUser to the shared migration test table.
func (nullableStringUser) TableName() string {
	// Initialize Variables
	name := "migration_users"

	return name
}

// TableName maps idOnlyUser to the temporary ID-only users table.
func (idOnlyUser) TableName() string {
	// Initialize Variables
	name := "id_only_users"

	return name
}

// TableName maps numeric migration models to their shared temporary table.
func (numericMigrationV1) TableName() string {
	// Initialize Variables
	name := "numeric_migration_users"

	return name
}

// TableName maps numeric migration models to their shared temporary table.
func (numericMigrationV2) TableName() string {
	// Initialize Variables
	name := "numeric_migration_users"

	return name
}

// TableName maps explicitPrimaryKeyUser to its temporary users table.
func (explicitPrimaryKeyUser) TableName() string {
	// Initialize Variables
	name := "explicit_primary_key_users"

	return name
}

// TableName maps requiredPrimaryKeyUser to its temporary users table.
func (requiredPrimaryKeyUser) TableName() string {
	// Initialize Variables
	name := "required_primary_key_users"

	return name
}

// TableName maps the first collision model to its deliberately ambiguous legacy table name.
func (uniqueCollisionFirst) TableName() string {
	// Initialize Variables
	name := "foo_bar"

	return name
}

// TableName maps the second collision model to its deliberately ambiguous legacy table name.
func (uniqueCollisionSecond) TableName() string {
	// Initialize Variables
	name := "foo"

	return name
}

// TableName maps caseColumnUser to its temporary case-sensitive spelling test table.
func (caseColumnUser) TableName() string {
	// Initialize Variables
	name := "case_column_users"

	return name
}

// TestModelLifecycle verifies create, query, update, delete, and vector persistence with Turso.
func TestModelLifecycle(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	userModel := Model[testUser](db)
	inputVector := Vector32{0.12, -0.08, 0.63}

	if err := userModel.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}

	// Create binds a generated-ID record that supports instance persistence methods.
	created, err := userModel.Create(ctx, testUser{Email: "hello@null.live", Embedding: inputVector})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create() did not generate an ID")
	}

	// Find decodes vectors and returns another bound instance.
	found, err := userModel.Find(ctx, created.ID)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if found.Email != created.Email {
		t.Fatalf("Find() email = %q, want %q", found.Email, created.Email)
	}
	if !reflect.DeepEqual(found.Embedding, inputVector) {
		t.Fatalf("Find() embedding = %#v, want %#v", found.Embedding, inputVector)
	}

	// Vector predicates and nearest-neighbor ordering use encoded Vector32 query arguments.
	matchingVectors, err := userModel.Where("vector_distance_cos(\"embedding\", vector32(?)) < ?", inputVector, 0.001).All(ctx)
	if err != nil {
		t.Fatalf("Where() vector search error = %v", err)
	}
	if len(matchingVectors) != 1 || matchingVectors[0].ID != created.ID {
		t.Fatalf("Where() vector search = %#v, want created user", matchingVectors)
	}

	nearest, err := userModel.Nearest("embedding", inputVector).First(ctx)
	if err != nil {
		t.Fatalf("Nearest().First() error = %v", err)
	}
	if nearest.ID != created.ID {
		t.Fatalf("Nearest().First() ID = %q, want %q", nearest.ID, created.ID)
	}
	ordered, err := userModel.Nearest("embedding", inputVector).OrderBy("email ASC").All(ctx)
	if err != nil {
		t.Fatalf("Nearest().OrderBy().All() error = %v", err)
	}
	if len(ordered) != 1 || ordered[0].ID != created.ID {
		t.Fatalf("Nearest().OrderBy().All() = %#v, want created user", ordered)
	}

	// Save persists changes without requiring the model handle again.
	found.Email = "updated@null.live"
	if err := found.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	matching, err := userModel.Where("email = ?", "updated@null.live").All(ctx)
	if err != nil {
		t.Fatalf("Where().All() error = %v", err)
	}
	if len(matching) != 1 || matching[0].ID != found.ID {
		t.Fatalf("Where().All() = %#v, want one matching user", matching)
	}

	allUsers, err := userModel.All(ctx)
	if err != nil {
		t.Fatalf("All() error = %v", err)
	}
	if len(allUsers) != 1 {
		t.Fatalf("All() returned %d users, want 1", len(allUsers))
	}

	// Delete removes the bound row and makes subsequent lookups report absence.
	if err := found.Delete(ctx); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	_, err = userModel.Find(ctx, found.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find() after Delete() error = %v, want ErrNotFound", err)
	}
}

// TestAutoMigrateConstraints verifies automatic schema creation and unique-value validation.
func TestAutoMigrateConstraints(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	userModel := Model[testUser](db)
	first := testUser{Email: "unique@null.live"}
	second := testUser{Email: "unique@null.live"}

	if err := userModel.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if _, err := userModel.Create(ctx, first); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if _, err := userModel.Create(ctx, second); !errors.Is(err, ErrUnique) {
		t.Fatalf("second Create() error = %v, want ErrUnique", err)
	}

	_, err := db.ExecContext(ctx, "INSERT INTO users (id, email) VALUES (?, ?)", "database-duplicate", first.Email)
	if err == nil {
		t.Fatal("Turso accepted a duplicate value without enforcing the unique index")
	}
}

// TestAutoMigrateEvolvesSafeSchemaChanges verifies nullable additions and rejects unsafe required ones.
func TestAutoMigrateEvolvesSafeSchemaChanges(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	versionOne := Model[migrationUserV1](db)
	versionTwo := Model[migrationUserV2](db)
	versionThree := Model[migrationUserV3](db)
	var migrated *migrationUserV2

	if err := versionOne.AutoMigrate(ctx); err != nil {
		t.Fatalf("v1 AutoMigrate() error = %v", err)
	}
	if _, err := versionOne.Create(ctx, migrationUserV1{ID: "existing-user", Email: "existing@null.live"}); err != nil {
		t.Fatalf("create existing user: %v", err)
	}
	if err := versionTwo.AutoMigrate(ctx); err != nil {
		t.Fatalf("v2 AutoMigrate() error = %v", err)
	}
	migrated, err := versionTwo.Find(ctx, "existing-user")
	if err != nil {
		t.Fatalf("load existing migrated user: %v", err)
	}
	if migrated.Nickname != "" {
		t.Fatalf("migrated nickname = %q, want empty string", migrated.Nickname)
	}
	if _, err := versionTwo.Create(ctx, migrationUserV2{ID: "empty-nickname", Email: "empty@null.live", Nickname: ""}); err != nil {
		t.Fatalf("create empty nickname user: %v", err)
	}
	migrated, err = versionTwo.Find(ctx, "empty-nickname")
	if err != nil {
		t.Fatalf("load empty nickname user: %v", err)
	}
	if migrated.Nickname != "" {
		t.Fatalf("empty nickname = %q, want empty string", migrated.Nickname)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO migration_users (id, email, nickname) VALUES (?, ?, ?)", "migrated-user", "migrated@null.live", "Migrate"); err != nil {
		t.Fatalf("insert with migrated column: %v", err)
	}
	if err := versionThree.AutoMigrate(ctx); err == nil {
		t.Fatal("v3 AutoMigrate() succeeded while adding a required column")
	}
}

// TestExplicitMigrations verifies destructive schema changes require explicit model operations.
func TestExplicitMigrations(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	versionOne := Model[migrationUserV1](db)
	integerEmail := Model[migrationUserIntegerEmail](db)
	versionTwo := Model[migrationUserV2](db)
	requiredNickname := Model[migrationUserRequiredNickname](db)
	var columnType string

	if err := versionOne.AutoMigrate(ctx); err != nil {
		t.Fatalf("v1 AutoMigrate() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO migration_users (id, email) VALUES (?, ?)", "migration-user", "42"); err != nil {
		t.Fatalf("insert initial migration row: %v", err)
	}
	if err := integerEmail.ChangeColumnType(ctx, "email"); err != nil {
		t.Fatalf("ChangeColumnType() error = %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT type FROM pragma_table_info('migration_users') WHERE name = 'email'").Scan(&columnType); err != nil {
		t.Fatalf("inspect changed column type: %v", err)
	}
	if columnType != "INTEGER" {
		t.Fatalf("email type = %q, want INTEGER", columnType)
	}

	if err := versionTwo.AutoMigrate(ctx); err != nil {
		t.Fatalf("v2 AutoMigrate() error = %v", err)
	}
	if err := requiredNickname.SetNotNull(ctx, "nickname"); err == nil {
		t.Fatal("SetNotNull() succeeded while nickname contains NULL")
	}
	if _, err := db.ExecContext(ctx, "UPDATE migration_users SET nickname = ?", "Migrate"); err != nil {
		t.Fatalf("fill nickname: %v", err)
	}
	if err := requiredNickname.SetNotNull(ctx, "nickname"); err != nil {
		t.Fatalf("SetNotNull() error = %v", err)
	}
	if err := versionOne.DropColumn(ctx, "nickname"); err != nil {
		t.Fatalf("DropColumn() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "SELECT nickname FROM migration_users"); err == nil {
		t.Fatal("DropColumn() left nickname queryable")
	}

	if _, err := db.ExecContext(ctx, "CREATE UNIQUE INDEX "+quoteIdentifier(versionOne.uniqueIndexName(field{column: "email"}))+" ON migration_users (email)"); err != nil {
		t.Fatalf("create explicit unique index: %v", err)
	}
	if err := versionOne.DropUnique(ctx, "email"); err != nil {
		t.Fatalf("DropUnique() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO migration_users (id, email) VALUES (?, ?)", "duplicate-email", "42"); err != nil {
		t.Fatalf("insert after DropUnique(): %v", err)
	}
}

// TestDefaultIdentifierNames verifies default table names preserve initialism word boundaries.
func TestDefaultIdentifierNames(t *testing.T) {
	// Initialize Variables
	apiKey := snakeCase("APIKey")
	blogPost := pluralize(snakeCase("BlogPost"))

	if apiKey != "api_key" {
		t.Fatalf("snakeCase(APIKey) = %q, want api_key", apiKey)
	}
	if blogPost != "blog_posts" {
		t.Fatalf("default BlogPost table = %q, want blog_posts", blogPost)
	}
}

// TestRecordRequiresBinding verifies standalone records cannot issue database writes accidentally.
func TestRecordRequiresBinding(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	user := testUser{ID: "unbound", Email: "hello@null.live"}
	err := user.Save(ctx)

	if !errors.Is(err, ErrUnboundRecord) {
		t.Fatalf("Save() error = %v, want ErrUnboundRecord", err)
	}
}

// TestModelBind verifies a manually-created record can opt into instance persistence methods.
func TestModelBind(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	userModel := Model[testUser](db)
	user := testUser{ID: "manual-user", Email: "manual@null.live"}

	if err := userModel.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if _, err := userModel.Bind(&user); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO users (id, email) VALUES (?, ?)", user.ID, user.Email); err != nil {
		t.Fatalf("insert bound user: %v", err)
	}

	user.Email = "bound@null.live"
	if err := user.Save(ctx); err != nil {
		t.Fatalf("Save() on bound user error = %v", err)
	}
}

// TestBoundRecordIDIsImmutable rejects writes after a bound record's ID changes.
func TestBoundRecordIDIsImmutable(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	userModel := Model[testUser](db)
	alice := testUser{ID: "alice", Email: "alice@null.live"}
	bob := testUser{ID: "bob", Email: "bob@null.live"}
	var email string

	if err := userModel.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	boundAlice, err := userModel.Create(ctx, alice)
	if err != nil {
		t.Fatalf("Create(alice) error = %v", err)
	}
	if _, err := userModel.Create(ctx, bob); err != nil {
		t.Fatalf("Create(bob) error = %v", err)
	}

	// Reject identity changes before they can target another row.
	boundAlice.ID = "bob"
	boundAlice.Email = "overwritten@null.live"
	if err := boundAlice.Save(ctx); !errors.Is(err, ErrIDChanged) {
		t.Fatalf("Save() error = %v, want ErrIDChanged", err)
	}
	if err := boundAlice.Delete(ctx); !errors.Is(err, ErrIDChanged) {
		t.Fatalf("Delete() error = %v, want ErrIDChanged", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT email FROM users WHERE id = ?", "bob").Scan(&email); err != nil {
		t.Fatalf("load bob: %v", err)
	}
	if email != "bob@null.live" {
		t.Fatalf("bob email = %q, want original value", email)
	}
}

// TestNullStringPreservesNULL verifies sql.NullString distinguishes NULL from an empty string.
func TestNullStringPreservesNULL(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	userModel := Model[nullableStringUser](db)
	missingNickname := nullableStringUser{ID: "missing", Email: "missing@null.live"}
	emptyNickname := nullableStringUser{ID: "empty", Email: "empty@null.live", Nickname: sql.NullString{String: "", Valid: true}}

	if err := userModel.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if _, err := userModel.Create(ctx, missingNickname); err != nil {
		t.Fatalf("Create(NULL nickname) error = %v", err)
	}
	if _, err := userModel.Create(ctx, emptyNickname); err != nil {
		t.Fatalf("Create(empty nickname) error = %v", err)
	}

	missing, err := userModel.Find(ctx, "missing")
	if err != nil {
		t.Fatalf("Find(NULL nickname) error = %v", err)
	}
	empty, err := userModel.Find(ctx, "empty")
	if err != nil {
		t.Fatalf("Find(empty nickname) error = %v", err)
	}
	if missing.Nickname.Valid {
		t.Fatalf("NULL nickname Valid = true, want false")
	}
	if !empty.Nickname.Valid || empty.Nickname.String != "" {
		t.Fatalf("empty nickname = %#v, want valid empty string", empty.Nickname)
	}
}

// TestModelEdgeCases verifies validation and persistence behavior for reviewed boundary cases.
func TestModelEdgeCases(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	idOnlyModel := Model[idOnlyUser](db)
	explicitKeyModel := Model[explicitPrimaryKeyUser](db)
	var nilUser *testUser

	if err := idOnlyModel.AutoMigrate(ctx); err != nil {
		t.Fatalf("ID-only AutoMigrate() error = %v", err)
	}
	idOnly, err := idOnlyModel.Create(ctx, idOnlyUser{ID: "only"})
	if err != nil {
		t.Fatalf("ID-only Create() error = %v", err)
	}
	if err := idOnly.Save(ctx); err != nil {
		t.Fatalf("ID-only Save() error = %v", err)
	}
	if _, err := Model[testUser](db).Bind(nilUser); !errors.Is(err, ErrUnboundRecord) {
		t.Fatalf("Bind(nil) error = %v, want ErrUnboundRecord", err)
	}

	if err := explicitKeyModel.AutoMigrate(ctx); err != nil {
		t.Fatalf("explicit primary key AutoMigrate() error = %v", err)
	}
	created, err := explicitKeyModel.Create(ctx, explicitPrimaryKeyUser{ExternalID: "external", Email: "key@null.live"})
	if err != nil {
		t.Fatalf("explicit primary key Create() error = %v", err)
	}
	found, err := explicitKeyModel.Find(ctx, "external")
	if err != nil || found.Email != created.Email {
		t.Fatalf("explicit primary key Find() = %#v, %v", found, err)
	}
}

// TestAutoMigrateEnforcesRequiredPrimaryKeys rejects NULL primary-key values in direct writes.
func TestAutoMigrateEnforcesRequiredPrimaryKeys(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[requiredPrimaryKeyUser](db)
	var required bool

	if err := model.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT "notnull" FROM pragma_table_info('required_primary_key_users') WHERE name = 'external_id'`).Scan(&required); err != nil || !required {
		t.Fatalf("external_id NOT NULL = %t, error = %v, want true", required, err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO required_primary_key_users (external_id) VALUES (NULL)"); err == nil {
		t.Fatal("direct NULL primary-key insert succeeded")
	}
}

// TestAutoMigrateHandlesNullableScalars keeps legacy rows readable after scalar additions.
func TestAutoMigrateHandlesNullableScalars(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	versionOne := Model[numericMigrationV1](db)
	versionTwo := Model[numericMigrationV2](db)

	if err := versionOne.AutoMigrate(ctx); err != nil {
		t.Fatalf("v1 AutoMigrate() error = %v", err)
	}
	if _, err := versionOne.Create(ctx, numericMigrationV1{ID: "existing"}); err != nil {
		t.Fatalf("v1 Create() error = %v", err)
	}
	if err := versionTwo.AutoMigrate(ctx); err != nil {
		t.Fatalf("v2 AutoMigrate() error = %v", err)
	}
	migrated, err := versionTwo.Find(ctx, "existing")
	if err != nil {
		t.Fatalf("v2 Find() error = %v", err)
	}
	if migrated.Count != 0 || migrated.Score != 0 || migrated.Enabled {
		t.Fatalf("nullable scalar defaults = %#v, want zero values", migrated)
	}
}

// TestAutoMigrateMatchesColumnsCaseInsensitively accepts SQLite's case-insensitive identifiers.
func TestAutoMigrateMatchesColumnsCaseInsensitively(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[caseColumnUser](db)
	var columns int

	if _, err := db.ExecContext(ctx, "CREATE TABLE case_column_users (id TEXT PRIMARY KEY, Email TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := model.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info('case_column_users')").Scan(&columns); err != nil || columns != 2 {
		t.Fatalf("column count = %d, error = %v, want 2 columns", columns, err)
	}
}

// TestUniqueIndexNamesAreUnambiguous creates indexes for formerly colliding table-column pairs.
func TestUniqueIndexNamesAreUnambiguous(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	first := Model[uniqueCollisionFirst](db)
	second := Model[uniqueCollisionSecond](db)

	if first.uniqueIndexName(first.fields[1]) == second.uniqueIndexName(second.fields[1]) {
		t.Fatal("unique index names collide")
	}
	if err := first.AutoMigrate(ctx); err != nil {
		t.Fatalf("first AutoMigrate() error = %v", err)
	}
	if err := second.AutoMigrate(ctx); err != nil {
		t.Fatalf("second AutoMigrate() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO foo_bar (id, baz) VALUES ('one', 'duplicate'), ('two', 'duplicate')"); err == nil {
		t.Fatal("first unique index was not enforced")
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO foo (id, bar_baz) VALUES ('one', 'duplicate'), ('two', 'duplicate')"); err == nil {
		t.Fatal("second unique index was not enforced")
	}
}

// TestModelRejectsInvalidPrimaryKeyMappings rejects ambiguous and mutable model identities.
func TestModelRejectsInvalidPrimaryKeyMappings(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)

	assertModelPanics(t, func() { Model[duplicateColumnUser](db) })
	assertModelPanics(t, func() { Model[multiplePrimaryKeyUser](db) })
	assertModelPanics(t, func() { Model[mutablePrimaryKeyUser](db) })
}

// assertModelPanics confirms invalid model declarations fail before touching the database.
func assertModelPanics(t *testing.T, create func()) {
	// Initialize Variables
	recovered := any(nil)

	t.Helper()
	defer func() {
		// Initialize Variables
		recovered = recover()

		if recovered == nil {
			t.Fatal("Model() did not panic")
		}
	}()
	create()
}

// openTestDatabase opens a temporary Turso database for an AutoMigrate call.
func openTestDatabase(t *testing.T, ctx context.Context) *sql.DB {
	// Initialize Variables
	path := filepath.Join(t.TempDir(), "activeso.db")
	db, err := sql.Open("turso", path)

	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() {
		// Initialize Variables
		closeError := db.Close()

		if closeError != nil {
			t.Errorf("db.Close() error = %v", closeError)
		}
	})

	return db
}

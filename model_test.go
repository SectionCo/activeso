package activeso

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "turso.tech/database/tursogo"
)

type testUser struct {
	Record

	ID        string   `db:"id"`
	Email     string   `db:"email" activeso:"not_null,unique"`
	Embedding Vector32 `db:"embedding"`
}

type foreignKeyRegion struct {
	Record

	ID   string `db:"id"`
	Name string `db:"name"`
}

type foreignKeyCity struct {
	Record

	ID       string `db:"id"`
	Name     string `db:"name"`
	RegionID string `db:"region_id" activeso:"belongs_to=regions(id)"`
}

type foreignKeySharedPrimaryKeyCity struct {
	Record

	RegionID string `db:"region_id" activeso:"primary_key,belongs_to=regions(id)"`
	Name     string `db:"name"`
}

type indexedCity struct {
	Record

	ID   string `db:"id"`
	Name string `db:"name" activeso:"not_null,index"`
}

type compositeMembershipV1 struct {
	Record

	ID             string `db:"id"`
	CustomerID     string `db:"customer_id" activeso:"not_null,unique_with=organization_id"`
	OrganizationID string `db:"organization_id" activeso:"not_null"`
}

type compositeMembershipV2 struct {
	Record

	ID             string `db:"id"`
	CustomerID     string `db:"customer_id" activeso:"not_null"`
	OrganizationID string `db:"organization_id" activeso:"not_null"`
}

type invalidForeignKeyTarget struct {
	Record

	ID       string `db:"id"`
	RegionID string `db:"region_id" activeso:"belongs_to=regions(id); DROP TABLE users"`
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

type migrationUserIntegerEmailV2 struct {
	Record

	ID       string `db:"id"`
	Email    int    `db:"email"`
	Nickname string `db:"nickname"`
}

type migrationUserMovedID struct {
	Record

	NewID string `db:"new_id" activeso:"primary_key"`
	Email string `db:"email"`
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

type nullableWrapperUser struct {
	Record

	ID      string          `db:"id"`
	Count   sql.NullInt64   `db:"count"`
	Score   sql.NullFloat64 `db:"score"`
	Enabled sql.NullBool    `db:"enabled"`
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

type uintPrimaryKeyUser struct {
	Record

	ID uint `db:"id"`
}

type uint64PrimaryKeyUser struct {
	Record

	ID uint64 `db:"id"`
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

type caseUniqueUserV1 struct {
	Record

	ID    string `db:"id"`
	Email string `db:"email" activeso:"unique"`
}

type caseUniqueUserV2 struct {
	Record

	ID    string `db:"ID"`
	Email string `db:"EMAIL" activeso:"unique"`
}

type caseUniqueUserV3 struct {
	Record

	ID    string `db:"id"`
	Email string `db:"EMAIL"`
}

// TableName maps testUser to the temporary users table.
func (testUser) TableName() string {
	// Initialize Variables
	name := "users"

	return name
}

// TableName maps foreignKeyRegion to the regions table used by foreign-key tests.
func (foreignKeyRegion) TableName() string {
	// Initialize Variables
	name := "regions"

	return name
}

// TableName maps foreignKeyCity to the cities table used by foreign-key tests.
func (foreignKeyCity) TableName() string {
	// Initialize Variables
	name := "cities"

	return name
}

// TableName maps indexedCity to the cities table used by ordinary-index tests.
func (indexedCity) TableName() string {
	// Initialize Variables
	name := "indexed_cities"

	return name
}

// TableName maps compositeMembershipV1 to the shared membership test table.
func (compositeMembershipV1) TableName() string {
	// Initialize Variables
	name := "composite_memberships"

	return name
}

// TableName maps compositeMembershipV2 to the shared membership test table.
func (compositeMembershipV2) TableName() string {
	// Initialize Variables
	name := "composite_memberships"

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

// TableName maps migrationUserIntegerEmailV2 to the shared migration test table.
func (migrationUserIntegerEmailV2) TableName() string {
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

// TableName maps migrationUserMovedID to the shared migration test table.
func (migrationUserMovedID) TableName() string {
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

// TableName maps nullableWrapperUser to its temporary users table.
func (nullableWrapperUser) TableName() string {
	// Initialize Variables
	name := "nullable_wrapper_users"

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

// TableName maps the first unique-index spelling to its temporary users table.
func (caseUniqueUserV1) TableName() string {
	// Initialize Variables
	name := "case_unique_users"

	return name
}

// TableName maps the second unique-index spelling to its temporary users table.
func (caseUniqueUserV2) TableName() string {
	// Initialize Variables
	name := "CASE_UNIQUE_USERS"

	return name
}

// TableName maps the unique-index removal spelling to its temporary users table.
func (caseUniqueUserV3) TableName() string {
	// Initialize Variables
	name := "case_unique_users"

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
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("Create() timestamps = %v, %v", created.CreatedAt, created.UpdatedAt)
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
	matchingByVector, err := userModel.FindBy(ctx, "embedding", inputVector)
	if err != nil {
		t.Fatalf("FindBy() vector error = %v", err)
	}
	if len(matchingByVector) != 1 || matchingByVector[0].ID != created.ID {
		t.Fatalf("FindBy() vector = %#v, want created user", matchingByVector)
	}
	if found.CreatedAt.IsZero() || found.UpdatedAt.IsZero() {
		t.Fatalf("Find() timestamps = %v, %v", found.CreatedAt, found.UpdatedAt)
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
	if found.UpdatedAt.IsZero() {
		t.Fatal("Save() did not refresh UpdatedAt")
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

// TestAutoMigrateIndexAndFindBy creates ordinary indexes and queries mapped columns safely.
func TestAutoMigrateIndexAndFindBy(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[indexedCity](db)
	var indexCount int

	if err := model.AutoMigrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type = 'index' AND name = ?", model.indexName(model.fields[1])).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 1 {
		t.Fatalf("managed index count = %d, want 1", indexCount)
	}
	if _, err := model.Create(ctx, indexedCity{ID: "portland", Name: "Portland"}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Create(ctx, indexedCity{ID: "portland-maine", Name: "Portland"}); err != nil {
		t.Fatal(err)
	}
	cities, err := model.FindBy(ctx, "name", "Portland")
	if err != nil {
		t.Fatal(err)
	}
	if len(cities) != 2 {
		t.Fatalf("FindBy() returned %d cities, want 2", len(cities))
	}
	if _, err := model.FindBy(ctx, "missing", "Portland"); err == nil {
		t.Fatal("FindBy() accepted an undefined column")
	}
}

// TestCompositeUniqueWith creates, enforces, and explicitly removes composite unique indexes.
func TestCompositeUniqueWith(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[compositeMembershipV1](db)
	removedModel := Model[compositeMembershipV2](db)
	fields := []field{model.fields[1], model.fields[2]}
	indexName := model.uniqueWithIndexName(fields)
	var indexCount int

	if err := model.AutoMigrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type = 'index' AND name = ?", indexName).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 1 {
		t.Fatalf("composite unique index count = %d, want 1", indexCount)
	}
	if _, err := model.Create(ctx, compositeMembershipV1{ID: "one", CustomerID: "customer-a", OrganizationID: "organization-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Create(ctx, compositeMembershipV1{ID: "two", CustomerID: "customer-a", OrganizationID: "organization-b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Create(ctx, compositeMembershipV1{ID: "three", CustomerID: "customer-b", OrganizationID: "organization-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := model.Create(ctx, compositeMembershipV1{ID: "four", CustomerID: "customer-a", OrganizationID: "organization-a"}); !errors.Is(err, ErrUnique) {
		t.Fatalf("duplicate composite membership error = %v, want ErrUnique", err)
	}

	// Removing the tag requires an explicit index migration before AutoMigrate can proceed.
	if err := removedModel.AutoMigrate(ctx); err == nil || !strings.Contains(err.Error(), "DropUniqueWith") {
		t.Fatalf("AutoMigrate() after unique_with removal error = %v, want DropUniqueWith guidance", err)
	}
	if err := removedModel.DropUniqueWith(ctx, "customer_id", "organization_id"); err != nil {
		t.Fatal(err)
	}
	if err := removedModel.AutoMigrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type = 'index' AND name = ?", indexName).Scan(&indexCount); err != nil {
		t.Fatal(err)
	}
	if indexCount != 0 {
		t.Fatalf("composite unique index count after removal = %d, want 0", indexCount)
	}
}

// TestAutoMigrateForeignKey creates and enforces belongs_to foreign-key constraints.
func TestAutoMigrateForeignKey(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	regionModel := Model[foreignKeyRegion](db)
	cityModel := Model[foreignKeyCity](db)
	var table, from, to string

	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if err := regionModel.AutoMigrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cityModel.AutoMigrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT \"table\", \"from\", \"to\" FROM pragma_foreign_key_list('cities')").Scan(&table, &from, &to); err != nil {
		t.Fatal(err)
	}
	if table != "regions" || from != "region_id" || to != "id" {
		t.Fatalf("foreign key = %s(%s) -> %s, want region_id -> regions(id)", table, from, to)
	}
	if _, err := regionModel.Create(ctx, foreignKeyRegion{ID: "oregon", Name: "Oregon"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cityModel.Create(ctx, foreignKeyCity{ID: "portland", Name: "Portland", RegionID: "oregon"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cityModel.Create(ctx, foreignKeyCity{ID: "unknown", Name: "Unknown", RegionID: "missing"}); err == nil {
		t.Fatal("Create() accepted a missing belongs_to target")
	}
}

// TestAutoMigrateSharedPrimaryKeyForeignKey enforces belongs_to constraints on explicitly tagged primary keys.
func TestAutoMigrateSharedPrimaryKeyForeignKey(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	regionModel := Model[foreignKeyRegion](db)
	cityModel := Model[foreignKeySharedPrimaryKeyCity](db)
	var table, from, to string

	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if err := regionModel.AutoMigrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cityModel.AutoMigrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT \"table\", \"from\", \"to\" FROM pragma_foreign_key_list(?)", cityModel.tableName).Scan(&table, &from, &to); err != nil {
		t.Fatal(err)
	}
	if table != "regions" || from != "region_id" || to != "id" {
		t.Fatalf("foreign key = %s(%s) -> %s, want region_id -> regions(id)", table, from, to)
	}
	if _, err := regionModel.Create(ctx, foreignKeyRegion{ID: "oregon", Name: "Oregon"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cityModel.Create(ctx, foreignKeySharedPrimaryKeyCity{RegionID: "oregon", Name: "Portland"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cityModel.Create(ctx, foreignKeySharedPrimaryKeyCity{RegionID: "missing", Name: "Unknown"}); err == nil {
		t.Fatal("Create() accepted a missing shared primary-key belongs_to target")
	}
}

// TestModelRejectsInvalidForeignKeyTarget rejects malformed belongs_to targets before migration.
func TestModelRejectsInvalidForeignKeyTarget(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)

	assertModelPanics(t, func() { Model[invalidForeignKeyTarget](db) })
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

// TestAutoMigrateTimestamps verifies fresh tables receive timestamp defaults and refresh triggers.
func TestAutoMigrateTimestamps(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[testUser](db)
	var createdAt, updatedAt, createdDefault, updatedDefault string

	if err := model.AutoMigrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT dflt_value FROM pragma_table_info('users') WHERE name = 'activeso_created_at'`).Scan(&createdDefault); err != nil || createdDefault != "CURRENT_TIMESTAMP" {
		t.Fatalf("created timestamp default = %q, error = %v", createdDefault, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT dflt_value FROM pragma_table_info('users') WHERE name = 'activeso_updated_at'`).Scan(&updatedDefault); err != nil || updatedDefault != "CURRENT_TIMESTAMP" {
		t.Fatalf("updated timestamp default = %q, error = %v", updatedDefault, err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO users (id, email) VALUES ('timestamped', 'timestamped@example.com')"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT activeso_created_at, activeso_updated_at FROM users WHERE id = 'timestamped'").Scan(&createdAt, &updatedAt); err != nil || createdAt == "" || updatedAt == "" {
		t.Fatalf("insert timestamps = %q, %q; error = %v", createdAt, updatedAt, err)
	}

	// Timestamp columns reject direct replacement while ordinary data updates remain allowed.
	if _, err := db.ExecContext(ctx, "UPDATE users SET activeso_created_at = 'before' WHERE id = 'timestamped'"); err == nil {
		t.Fatal("accepted a direct creation timestamp update")
	}
	if _, err := db.ExecContext(ctx, "UPDATE users SET activeso_updated_at = 'before' WHERE id = 'timestamped'"); err == nil {
		t.Fatal("accepted a direct update timestamp replacement")
	}
	if _, err := db.ExecContext(ctx, "UPDATE users SET email = 'changed@example.com' WHERE id = 'timestamped'"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT activeso_updated_at FROM users WHERE id = 'timestamped'").Scan(&updatedAt); err != nil || updatedAt == "" {
		t.Fatalf("updated timestamp = %q, error = %v", updatedAt, err)
	}
}

// TestAutoMigrateLegacyTimestamps verifies old tables are populated and protected by timestamp triggers.
func TestAutoMigrateLegacyTimestamps(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[testUser](db)
	var createdAt, updatedAt string

	if _, err := db.ExecContext(ctx, "CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO users (id, email) VALUES ('existing', 'existing@example.com')"); err != nil {
		t.Fatal(err)
	}
	if err := model.AutoMigrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT activeso_created_at, activeso_updated_at FROM users WHERE id = 'existing'").Scan(&createdAt, &updatedAt); err != nil || createdAt == "" || updatedAt == "" {
		t.Fatalf("backfilled timestamps = %q, %q; error = %v", createdAt, updatedAt, err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO users (id, email) VALUES ('new', 'new@example.com')"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT activeso_created_at, activeso_updated_at FROM users WHERE id = 'new'").Scan(&createdAt, &updatedAt); err != nil || createdAt == "" || updatedAt == "" {
		t.Fatalf("triggered timestamps = %q, %q; error = %v", createdAt, updatedAt, err)
	}
}

// TestCreateTranslatesAuthoritativeUniqueViolation reports database-enforced races as UniqueError.
func TestCreateTranslatesAuthoritativeUniqueViolation(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[testUser](db)
	var err error
	var uniqueError UniqueError

	if err := model.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TRIGGER force_create_unique BEFORE INSERT ON users WHEN NEW.id <> 'competing' BEGIN INSERT INTO users (id, email) VALUES ('competing', NEW.email); END"); err != nil {
		t.Fatalf("create unique trigger error = %v", err)
	}

	_, err = model.Create(ctx, testUser{Email: "race@null.live"})
	if !errors.Is(err, ErrUnique) {
		t.Fatalf("Create() error = %v, want ErrUnique", err)
	}
	if !errors.As(err, &uniqueError) || uniqueError.Field != "email" {
		t.Fatalf("Create() unique error = %#v, want email", uniqueError)
	}
}

// TestSaveTranslatesAuthoritativeUniqueViolation reports database-enforced races as UniqueError.
func TestSaveTranslatesAuthoritativeUniqueViolation(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[testUser](db)
	var record *testUser
	var err error
	var uniqueError UniqueError

	if err := model.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	record, err = model.Create(ctx, testUser{Email: "original@null.live"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, "CREATE TRIGGER force_save_unique BEFORE UPDATE ON users BEGIN INSERT INTO users (id, email) VALUES ('competing', NEW.email); END"); err != nil {
		t.Fatalf("create unique trigger error = %v", err)
	}

	record.Email = "race@null.live"
	err = record.Save(ctx)
	if !errors.Is(err, ErrUnique) {
		t.Fatalf("Save() error = %v, want ErrUnique", err)
	}
	if !errors.As(err, &uniqueError) || uniqueError.Field != "email" {
		t.Fatalf("Save() unique error = %#v, want email", uniqueError)
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

// TestAutoMigrateHintsExplicitMigrations directs destructive schema changes to the matching API.
func TestAutoMigrateHintsExplicitMigrations(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	notNullDB := openTestDatabase(t, ctx)
	typeDB := openTestDatabase(t, ctx)
	uniqueDB := openTestDatabase(t, ctx)
	versionOne := Model[migrationUserV1](notNullDB)
	versionThree := Model[migrationUserV3](notNullDB)
	typeVersionOne := Model[migrationUserV1](typeDB)
	integerEmail := Model[migrationUserIntegerEmail](typeDB)
	uniqueVersionOne := Model[caseUniqueUserV1](uniqueDB)
	uniqueRemoval := Model[caseUniqueUserV3](uniqueDB)
	var err error

	// Adding a required column requires a nullable addition and a backfill first.
	if err := versionOne.AutoMigrate(ctx); err != nil {
		t.Fatalf("not-null v1 AutoMigrate() error = %v", err)
	}
	err = versionThree.AutoMigrate(ctx)
	if err == nil || !strings.Contains(err.Error(), `SetNotNull(ctx, "required")`) {
		t.Fatalf("not-null AutoMigrate() error = %v, want SetNotNull hint", err)
	}

	// Changing an existing column's storage type requires an explicit rebuild.
	if err := typeVersionOne.AutoMigrate(ctx); err != nil {
		t.Fatalf("type v1 AutoMigrate() error = %v", err)
	}
	err = integerEmail.AutoMigrate(ctx)
	if err == nil || !strings.Contains(err.Error(), `ChangeColumnType(ctx, "email")`) {
		t.Fatalf("type AutoMigrate() error = %v, want ChangeColumnType hint", err)
	}

	// Removing ActiveSo's managed unique index also requires an explicit operation.
	if err := uniqueVersionOne.AutoMigrate(ctx); err != nil {
		t.Fatalf("unique v1 AutoMigrate() error = %v", err)
	}
	err = uniqueRemoval.AutoMigrate(ctx)
	if err == nil || !strings.Contains(err.Error(), `DropUnique(ctx, "EMAIL")`) {
		t.Fatalf("unique AutoMigrate() error = %v, want DropUnique hint", err)
	}
}

// TestExplicitMigrations verifies destructive schema changes require explicit model operations.
func TestExplicitMigrations(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	versionOne := Model[migrationUserV1](db)
	integerEmail := Model[migrationUserIntegerEmail](db)
	versionTwo := Model[migrationUserIntegerEmailV2](db)
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
	if err := requiredNickname.SetNotNull(ctx, "nickname"); err == nil || !strings.Contains(err.Error(), `SetNotNull(ctx, "nickname")`) {
		t.Fatalf("SetNotNull() error = %v, want backfill and retry hint", err)
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

// TestDefaultIdentifierNames verifies default table names use English plurals without double-pluralizing names.
func TestDefaultIdentifierNames(t *testing.T) {
	// Initialize Variables
	apiKey := snakeCase("APIKey")
	blogPost := pluralize(snakeCase("BlogPost"))
	city := pluralize(snakeCase("City"))
	person := pluralize(snakeCase("Person"))
	users := pluralize(snakeCase("Users"))

	if apiKey != "api_key" {
		t.Fatalf("snakeCase(APIKey) = %q, want api_key", apiKey)
	}
	if blogPost != "blog_posts" {
		t.Fatalf("default BlogPost table = %q, want blog_posts", blogPost)
	}
	if city != "cities" {
		t.Fatalf("default City table = %q, want cities", city)
	}
	if person != "people" {
		t.Fatalf("default Person table = %q, want people", person)
	}
	if users != "users" {
		t.Fatalf("default Users table = %q, want users", users)
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
	if !user.CreatedAt.IsZero() || !user.UpdatedAt.IsZero() {
		t.Fatalf("Bind() loaded timestamps = %v, %v", user.CreatedAt, user.UpdatedAt)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO users (id, email) VALUES (?, ?)", user.ID, user.Email); err != nil {
		t.Fatalf("insert bound user: %v", err)
	}

	user.Email = "bound@null.live"
	if err := user.Save(ctx); err != nil {
		t.Fatalf("Save() on bound user error = %v", err)
	}
	if user.CreatedAt.IsZero() || user.UpdatedAt.IsZero() {
		t.Fatalf("Save() on bound user timestamps = %v, %v", user.CreatedAt, user.UpdatedAt)
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

// TestNullableWrappersMapAndPreserveValues maps database/sql nullable scalar wrappers correctly.
func TestNullableWrappersMapAndPreserveValues(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[nullableWrapperUser](db)
	present := nullableWrapperUser{
		ID:      "present",
		Count:   sql.NullInt64{Int64: 42, Valid: true},
		Score:   sql.NullFloat64{Float64: 3.5, Valid: true},
		Enabled: sql.NullBool{Bool: true, Valid: true},
	}
	var countType, scoreType, enabledType string
	var missing, found *nullableWrapperUser
	var err error

	if err := model.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT type FROM pragma_table_info('nullable_wrapper_users') WHERE name = 'count'").Scan(&countType); err != nil || countType != "INTEGER" {
		t.Fatalf("count schema = %q, error = %v, want INTEGER", countType, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT type FROM pragma_table_info('nullable_wrapper_users') WHERE name = 'score'").Scan(&scoreType); err != nil || scoreType != "REAL" {
		t.Fatalf("score schema = %q, error = %v, want REAL", scoreType, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT type FROM pragma_table_info('nullable_wrapper_users') WHERE name = 'enabled'").Scan(&enabledType); err != nil || enabledType != "INTEGER" {
		t.Fatalf("enabled schema = %q, error = %v, want INTEGER", enabledType, err)
	}
	if _, err := model.Create(ctx, nullableWrapperUser{ID: "missing"}); err != nil {
		t.Fatalf("Create(NULL wrappers) error = %v", err)
	}
	if _, err := model.Create(ctx, present); err != nil {
		t.Fatalf("Create(present wrappers) error = %v", err)
	}

	missing, err = model.Find(ctx, "missing")
	if err != nil || missing.Count.Valid || missing.Score.Valid || missing.Enabled.Valid {
		t.Fatalf("Find(NULL wrappers) = %#v, error = %v", missing, err)
	}
	found, err = model.Find(ctx, "present")
	if err != nil || found.Count != present.Count || found.Score != present.Score || found.Enabled != present.Enabled {
		t.Fatalf("Find(present wrappers) = %#v, error = %v", found, err)
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

// TestAutoMigrateRejectsUnconstrainedLegacyID rejects tables whose modeled identity is not unique.
func TestAutoMigrateRejectsUnconstrainedLegacyID(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[testUser](db)
	var err error

	if _, err := db.ExecContext(ctx, "CREATE TABLE users (id TEXT, email TEXT, embedding BLOB)"); err != nil {
		t.Fatal(err)
	}
	err = model.AutoMigrate(ctx)
	if err == nil {
		t.Fatal("AutoMigrate() accepted an unconstrained legacy ID")
	}
}

// TestAutoMigrateRejectsCompositeLegacyID rejects composite keys that do not uniquely identify the modeled ID.
func TestAutoMigrateRejectsCompositeLegacyID(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[testUser](db)
	var err error

	if _, err := db.ExecContext(ctx, "CREATE TABLE users (id TEXT, tenant TEXT, email TEXT, embedding BLOB, PRIMARY KEY (id, tenant))"); err != nil {
		t.Fatal(err)
	}
	err = model.AutoMigrate(ctx)
	if err == nil {
		t.Fatal("AutoMigrate() accepted a composite legacy ID")
	}
}

// TestAutoMigrateAcceptsUniqueLegacyID accepts an existing identity protected by a unique constraint.
func TestAutoMigrateAcceptsUniqueLegacyID(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	model := Model[testUser](db)

	if _, err := db.ExecContext(ctx, "CREATE TABLE users (id TEXT UNIQUE, email TEXT, embedding BLOB)"); err != nil {
		t.Fatal(err)
	}
	if err := model.AutoMigrate(ctx); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
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
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info('case_column_users')").Scan(&columns); err != nil || columns != 4 {
		t.Fatalf("column count = %d, error = %v, want 4 columns", columns, err)
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

// TestUniqueIndexNamesIgnoreIdentifierCase reuses and removes indexes across case-only mapping changes.
func TestUniqueIndexNamesIgnoreIdentifierCase(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)
	first := Model[caseUniqueUserV1](db)
	second := Model[caseUniqueUserV2](db)
	removal := Model[caseUniqueUserV3](db)
	indexName := first.uniqueIndexName(first.fields[1])
	var indexes int

	if err := first.AutoMigrate(ctx); err != nil {
		t.Fatalf("first AutoMigrate() error = %v", err)
	}
	if err := second.AutoMigrate(ctx); err != nil {
		t.Fatalf("second AutoMigrate() error = %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type = 'index' AND name = ?", indexName).Scan(&indexes); err != nil || indexes != 1 {
		t.Fatalf("unique index count = %d, error = %v, want 1", indexes, err)
	}
	if err := removal.DropUnique(ctx, "EMAIL"); err != nil {
		t.Fatalf("DropUnique() error = %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE type = 'index' AND name = ?", indexName).Scan(&indexes); err != nil || indexes != 0 {
		t.Fatalf("unique index count after DropUnique() = %d, error = %v, want 0", indexes, err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO case_unique_users (id, email) VALUES ('one', 'duplicate'), ('two', 'duplicate')"); err != nil {
		t.Fatalf("duplicate insert after DropUnique() error = %v", err)
	}
}

// TestModelRejectsInvalidPrimaryKeyMappings rejects ambiguous and unrepresentable model identities.
func TestModelRejectsInvalidPrimaryKeyMappings(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t, ctx)

	assertModelPanics(t, func() { Model[duplicateColumnUser](db) })
	assertModelPanics(t, func() { Model[multiplePrimaryKeyUser](db) })
	assertModelPanics(t, func() { Model[mutablePrimaryKeyUser](db) })
	assertModelPanics(t, func() { Model[uintPrimaryKeyUser](db) })
	assertModelPanics(t, func() { Model[uint64PrimaryKeyUser](db) })
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

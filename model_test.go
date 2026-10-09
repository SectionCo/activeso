package activeso

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "turso.tech/database/tursogo"
)

type User struct {
	Record `activeso:"timestamps"`

	ID        string   `db:"id"`
	Email     string   `db:"email" activeso:"not_null,unique"`
	Embedding Vector32 `db:"embedding"`
}

type Region struct {
	Record

	ID   string `db:"id"`
	Name string `db:"name"`
}

type City struct {
	Record

	ID       string `db:"id"`
	Name     string `db:"name" activeso:"not_null,index"`
	RegionID string `db:"region_id" activeso:"belongs_to=regions(id)"`
}

type CodeCity struct {
	Record

	ID       string `db:"id"`
	RegionID string `db:"region_id" activeso:"belongs_to=regions(code)"`
}

// Gadget has a field type ActiveSo cannot map to a SQL column.
type Gadget struct {
	Record

	ID   string            `db:"id"`
	Meta map[string]string `db:"meta"`
}

// Label converts itself to and from TEXT, so it needs no built-in mapping.
type Label struct {
	Text string
}

// Badge stores a self-converting Label.
type Badge struct {
	Record

	ID    string `db:"id"`
	Label Label  `db:"label"`
}

type CascadeCity struct {
	Record

	ID       string `db:"id"`
	RegionID string `db:"region_id" activeso:"belongs_to=regions(id),on_delete=cascade"`
}

type Membership struct {
	Record

	ID             string `db:"id"`
	CustomerID     string `db:"customer_id" activeso:"not_null,unique_with=organization_id"`
	OrganizationID string `db:"organization_id" activeso:"not_null"`
}

type Note struct {
	Record `activeso:"timestamps"`

	ID string `db:"id"`
}

type Plain struct {
	Record

	ID   string `db:"id"`
	Name string `db:"name"`
}

type Stamped struct {
	Record `activeso:"timestamps"`

	ID   string `db:"id"`
	Name string `db:"name"`
}

type Profile struct {
	Record

	ID       string         `db:"id"`
	Nickname sql.NullString `db:"nickname"`
}

type Account struct {
	Record

	ID      string          `db:"id"`
	Count   sql.NullInt64   `db:"count"`
	Score   sql.NullFloat64 `db:"score"`
	Enabled sql.NullBool    `db:"enabled"`
	Visits  int             `db:"visits"`
}

type Device struct {
	Record

	ExternalID string `db:"external_id" activeso:"primary_key"`
	Label      string `db:"label"`
}

type untaggedField struct {
	Record

	ID    string `db:"id"`
	Email string
}

type skippedField struct {
	Record

	ID    string `db:"id"`
	Cache string `db:"-"`
}

type unsupportedRecordHint struct {
	Record `activeso:"audit"`

	ID string `db:"id"`
}

type unsupportedFieldHint struct {
	Record

	ID    string `db:"id"`
	Email string `db:"email" activeso:"fast"`
}

type reservedTimestampColumn struct {
	Record `activeso:"timestamps"`

	ID        string `db:"id"`
	CreatedOn string `db:"created_at"`
}

type invalidCascadeWithoutForeignKey struct {
	Record

	ID       string `db:"id"`
	RegionID string `db:"region_id" activeso:"on_delete=cascade"`
}

type invalidForeignKeyTarget struct {
	Record

	ID       string `db:"id"`
	RegionID string `db:"region_id" activeso:"belongs_to=regions(id); DROP TABLE users"`
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

type primaryKeyUniqueWithUser struct {
	Record

	ID    string `db:"id" activeso:"unique_with=email"`
	Email string `db:"email"`
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

type noPrimaryKeyUser struct {
	Record

	Email string `db:"email"`
}

// TableName maps CascadeCity to the shared cities table used by foreign-key tests.
// Value converts the label to its TEXT representation.
func (label Label) Value() (driver.Value, error) {
	// Initialize Variables
	text := label.Text

	return text, nil
}

// Scan reads a TEXT value into the label.
func (label *Label) Scan(value any) error {
	// Initialize Variables
	text, _ := value.(string)

	label.Text = text
	return nil
}

func (CascadeCity) TableName() string {
	// Initialize Variables
	name := "cities"

	return name
}

// TableName maps Stamped to the same table as Plain so one table serves both timestamp modes.
func (Stamped) TableName() string {
	// Initialize Variables
	name := "plains"

	return name
}

const usersSchema = `CREATE TABLE users (
	id TEXT PRIMARY KEY,
	email TEXT NOT NULL,
	embedding BLOB,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX users_email_unique ON users (email);`

// TestModelLifecycle verifies create, query, update, delete, and vector persistence with Turso.
func TestModelLifecycle(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)
	inputVector := Vector32{0.12, -0.08, 0.63}

	execStatements(t, db, usersSchema)

	// Create binds a generated-ID record that supports instance persistence methods.
	created, err := userModel.Create(ctx, User{Email: "hello@null.live", Embedding: inputVector})
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
	if found.CreatedAt.IsZero() || found.UpdatedAt.IsZero() {
		t.Fatalf("Find() timestamps = %v, %v", found.CreatedAt, found.UpdatedAt)
	}
	matchingByVector, err := userModel.FindBy(ctx, "embedding", inputVector)
	if err != nil {
		t.Fatalf("FindBy() vector error = %v", err)
	}
	if len(matchingByVector) != 1 || matchingByVector[0].ID != created.ID {
		t.Fatalf("FindBy() vector = %#v, want created user", matchingByVector)
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
	matching, err := userModel.Where("email = ?", "updated@null.live").Limit(5).All(ctx)
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
	if _, err = userModel.Find(ctx, found.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find() after Delete() error = %v, want ErrNotFound", err)
	}
	if _, err = userModel.Where("email = ?", "missing").First(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("First() without matches error = %v, want ErrNotFound", err)
	}
}

// TestTimestampsAreOptIn verifies models without the timestamps hint never touch created_at or updated_at.
func TestTimestampsAreOptIn(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	plainModel := Model[Plain](db)

	execStatements(t, db, `CREATE TABLE plains (id TEXT PRIMARY KEY, name TEXT)`)

	created, err := plainModel.Create(ctx, Plain{Name: "plain"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	created.Name = "renamed"
	if err := created.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	found, err := plainModel.Find(ctx, created.ID)
	if err != nil || found.Name != "renamed" {
		t.Fatalf("Find() = %#v, error = %v", found, err)
	}
	if !found.CreatedAt.IsZero() || !found.UpdatedAt.IsZero() {
		t.Fatalf("timestamps = %v, %v, want zero without the hint", found.CreatedAt, found.UpdatedAt)
	}
}

// TestTimestampsNameMissingColumns verifies a timestamps hint explains the columns it expects.
func TestTimestampsNameMissingColumns(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	stampedModel := Model[Stamped](db)

	execStatements(t, db, `CREATE TABLE plains (id TEXT PRIMARY KEY, name TEXT)`)

	_, err := stampedModel.Create(ctx, Stamped{Name: "stamped"})
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("Create() error = %v, want ErrSchemaMismatch", err)
	}
	if !strings.Contains(err.Error(), "created_at") || !strings.Contains(err.Error(), "updated_at") {
		t.Fatalf("Create() error = %q, want both timestamp column names", err)
	}
}

// TestTimestampsRefreshOnSave verifies Save advances updated_at while created_at stays fixed.
func TestTimestampsRefreshOnSave(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)

	execStatements(t, db, usersSchema)

	created, err := userModel.Create(ctx, User{Email: "hello@null.live"})
	if err != nil {
		t.Fatal(err)
	}
	// Backdate both columns so the refreshed value is distinguishable within one second.
	if _, err := db.ExecContext(ctx, "UPDATE users SET created_at = '2000-01-01 00:00:00', updated_at = '2000-01-01 00:00:00'"); err != nil {
		t.Fatal(err)
	}

	created.Email = "updated@null.live"
	if err := created.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if created.CreatedAt.Year() != 2000 {
		t.Fatalf("CreatedAt = %v, want the stored creation time", created.CreatedAt)
	}
	if created.UpdatedAt.Year() == 2000 {
		t.Fatalf("UpdatedAt = %v, want a refreshed time", created.UpdatedAt)
	}
}

// TestTimestampsOnIDOnlyModel verifies a model with only an ID still updates its timestamp on Save.
func TestTimestampsOnIDOnlyModel(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	noteModel := Model[Note](db)

	execStatements(t, db, `CREATE TABLE notes (id TEXT PRIMARY KEY, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`)

	note, err := noteModel.Create(ctx, Note{})
	if err != nil {
		t.Fatal(err)
	}
	if err := note.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := note.Delete(ctx); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := note.Save(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Save() after Delete() error = %v, want ErrNotFound", err)
	}
}

// TestFindByUsesMappedColumnsOnly verifies FindBy rejects columns the model does not map.
func TestFindByUsesMappedColumnsOnly(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	cityModel := Model[City](db)

	execStatements(t, db, `CREATE TABLE cities (id TEXT PRIMARY KEY, name TEXT NOT NULL, region_id TEXT);
CREATE INDEX cities_name ON cities (name);`)

	for _, id := range []string{"portland", "portland-maine"} {
		if _, err := cityModel.Create(ctx, City{ID: id, Name: "Portland"}); err != nil {
			t.Fatal(err)
		}
	}
	cities, err := cityModel.FindBy(ctx, "name", "Portland")
	if err != nil || len(cities) != 2 {
		t.Fatalf("FindBy() = %d cities, error = %v, want 2", len(cities), err)
	}
	if _, err := cityModel.FindBy(ctx, "missing", "Portland"); err == nil {
		t.Fatal("FindBy() accepted an undefined column")
	}
}

// TestUniqueViolationsMapToErrUnique verifies database unique-index failures become UniqueError.
func TestUniqueViolationsMapToErrUnique(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)
	var uniqueError UniqueError

	execStatements(t, db, usersSchema)

	if _, err := userModel.Create(ctx, User{Email: "taken@null.live"}); err != nil {
		t.Fatal(err)
	}

	// Create reports the conflicting column.
	_, err := userModel.Create(ctx, User{Email: "taken@null.live"})
	if !errors.Is(err, ErrUnique) {
		t.Fatalf("Create() error = %v, want ErrUnique", err)
	}
	if !errors.As(err, &uniqueError) || uniqueError.Field != "email" {
		t.Fatalf("Create() unique error = %#v, want email", uniqueError)
	}

	// Save reports the same failure.
	other, err := userModel.Create(ctx, User{Email: "other@null.live"})
	if err != nil {
		t.Fatal(err)
	}
	other.Email = "taken@null.live"
	err = other.Save(ctx)
	if !errors.Is(err, ErrUnique) {
		t.Fatalf("Save() error = %v, want ErrUnique", err)
	}
	if !errors.As(err, &uniqueError) || uniqueError.Field != "email" {
		t.Fatalf("Save() unique error = %#v, want email", uniqueError)
	}
}

// TestCompositeUniqueViolationsMapToErrUnique verifies unique_with indexes surface as ErrUnique.
func TestCompositeUniqueViolationsMapToErrUnique(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	membershipModel := Model[Membership](db)

	execStatements(t, db, `CREATE TABLE memberships (id TEXT PRIMARY KEY, customer_id TEXT NOT NULL, organization_id TEXT NOT NULL);
CREATE UNIQUE INDEX memberships_customer_organization ON memberships (customer_id, organization_id);`)

	for _, record := range []Membership{
		{ID: "one", CustomerID: "customer-a", OrganizationID: "organization-a"},
		{ID: "two", CustomerID: "customer-a", OrganizationID: "organization-b"},
		{ID: "three", CustomerID: "customer-b", OrganizationID: "organization-a"},
	} {
		if _, err := membershipModel.Create(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := membershipModel.Create(ctx, Membership{ID: "four", CustomerID: "customer-a", OrganizationID: "organization-a"}); !errors.Is(err, ErrUnique) {
		t.Fatalf("duplicate membership error = %v, want ErrUnique", err)
	}
}

// TestVerifyAcceptsMatchingSchema verifies a table that honors every hint passes Verify.
func TestVerifyAcceptsMatchingSchema(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)

	execStatements(t, db, usersSchema)
	execStatements(t, db, `CREATE TABLE regions (id TEXT PRIMARY KEY, name TEXT);
CREATE TABLE cities (id TEXT PRIMARY KEY, name TEXT NOT NULL, region_id TEXT REFERENCES regions (id) ON DELETE CASCADE);
CREATE INDEX cities_name ON cities (name);
CREATE TABLE memberships (id TEXT PRIMARY KEY, customer_id TEXT NOT NULL, organization_id TEXT NOT NULL, UNIQUE (customer_id, organization_id));`)

	if err := Model[User](db).Verify(ctx); err != nil {
		t.Fatalf("User Verify() error = %v", err)
	}
	if err := Model[City](db).Verify(ctx); err != nil {
		t.Fatalf("City Verify() error = %v", err)
	}
	if err := Model[CascadeCity](db).Verify(ctx); err != nil {
		t.Fatalf("CascadeCity Verify() error = %v", err)
	}
	if err := Model[Membership](db).Verify(ctx); err != nil {
		t.Fatalf("Membership Verify() error = %v", err)
	}
}

// TestVerifyReportsDrift verifies Verify lists every hint the table fails to honor.
func TestVerifyReportsDrift(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	var schemaError *SchemaError

	execStatements(t, db, `CREATE TABLE users (id TEXT PRIMARY KEY, email INTEGER, embedding BLOB, created_at INTEGER, updated_at TEXT NOT NULL);
CREATE TABLE regions (id TEXT PRIMARY KEY, name TEXT);
CREATE TABLE cities (id TEXT PRIMARY KEY, name TEXT, region_id TEXT REFERENCES regions (id));
CREATE TABLE memberships (id TEXT PRIMARY KEY, customer_id TEXT NOT NULL, organization_id TEXT NOT NULL);`)

	err := Model[User](db).Verify(ctx)
	if !errors.Is(err, ErrSchemaMismatch) || !errors.As(err, &schemaError) {
		t.Fatalf("User Verify() error = %v, want SchemaError", err)
	}
	for _, expected := range []string{
		`column email has type "INTEGER"`,
		"hint not_null on email",
		"hint unique on email",
		"timestamps hint expects TEXT",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("User Verify() error = %q, missing %q", err, expected)
		}
	}

	// A missing index and a foreign key without cascade are advisory hint drift.
	if err := Model[City](db).Verify(ctx); err == nil || !strings.Contains(err.Error(), "hint index on name") {
		t.Fatalf("City Verify() error = %v, want index drift", err)
	}
	if err := Model[CascadeCity](db).Verify(ctx); err == nil || !strings.Contains(err.Error(), "on_delete=cascade") {
		t.Fatalf("CascadeCity Verify() error = %v, want cascade drift", err)
	}
	if err := Model[Membership](db).Verify(ctx); err == nil || !strings.Contains(err.Error(), "hint unique_with on customer_id") {
		t.Fatalf("Membership Verify() error = %v, want unique_with drift", err)
	}

	// A missing foreign key is reported against its belongs_to hint.
	execStatements(t, db, `DROP TABLE cities; CREATE TABLE cities (id TEXT PRIMARY KEY, name TEXT NOT NULL, region_id TEXT);`)
	if err := Model[CascadeCity](db).Verify(ctx); err == nil || !strings.Contains(err.Error(), "hint belongs_to=regions(id)") {
		t.Fatalf("CascadeCity Verify() error = %v, want missing foreign key", err)
	}
}

// TestVerifyReportsMissingTable verifies a missing table is a schema mismatch naming the table.
func TestVerifyReportsMissingTable(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)

	err := Model[User](db).Verify(ctx)
	if !errors.Is(err, ErrSchemaMismatch) || !strings.Contains(err.Error(), "table users does not exist") {
		t.Fatalf("Verify() error = %v, want missing table", err)
	}
}

// TestFirstUseChecksStructureWithoutCachingFailures verifies operations fail clearly until the table exists.
func TestFirstUseChecksStructureWithoutCachingFailures(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)

	if _, err := userModel.All(ctx); !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("All() before the table exists error = %v, want ErrSchemaMismatch", err)
	}
	if _, err := userModel.Create(ctx, User{Email: "early@null.live"}); !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("Create() before the table exists error = %v, want ErrSchemaMismatch", err)
	}

	// The failed check is retried, so creating the table afterward makes the model usable.
	execStatements(t, db, usersSchema)
	if _, err := userModel.Create(ctx, User{Email: "later@null.live"}); err != nil {
		t.Fatalf("Create() after the table exists error = %v", err)
	}

	// Missing mapped columns are structural and reported together.
	execStatements(t, db, `CREATE TABLE plains (id TEXT PRIMARY KEY)`)
	if _, err := Model[Plain](db).All(ctx); err == nil || !strings.Contains(err.Error(), "column name is missing") {
		t.Fatalf("All() with a missing column error = %v, want column name is missing", err)
	}
}

// TestFirstUseRequiresProtectedPrimaryKey verifies Save and Delete cannot target an unprotected identity column.
func TestFirstUseRequiresProtectedPrimaryKey(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)

	execStatements(t, db, `CREATE TABLE plains (id TEXT, name TEXT)`)
	if _, err := Model[Plain](db).All(ctx); err == nil || !strings.Contains(err.Error(), "primary key column id") {
		t.Fatalf("All() error = %v, want primary key guidance", err)
	}

	// A single-column unique index protects identity just like a PRIMARY KEY.
	execStatements(t, db, `CREATE UNIQUE INDEX plains_id ON plains (id)`)
	if _, err := Model[Plain](db).All(ctx); err != nil {
		t.Fatalf("All() with a unique index error = %v", err)
	}
}

// TestFirstUseRejectsCompositePrimaryKey verifies an ID that is only part of a composite key is not accepted as unique.
func TestFirstUseRejectsCompositePrimaryKey(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)

	// Rows may share an id across tenants, so Save and Delete by id could touch several rows.
	execStatements(t, db, `CREATE TABLE plains (id TEXT, tenant_id TEXT, name TEXT, PRIMARY KEY (id, tenant_id))`)
	if _, err := Model[Plain](db).All(ctx); err == nil || !strings.Contains(err.Error(), "primary key column id") {
		t.Fatalf("All() error = %v, want primary key guidance", err)
	}

	// A single-column unique index on the modeled ID restores identity.
	execStatements(t, db, `CREATE UNIQUE INDEX plains_id ON plains (id)`)
	if _, err := Model[Plain](db).All(ctx); err != nil {
		t.Fatalf("All() with a unique index error = %v", err)
	}
}

// TestVerifyResolvesOmittedForeignKeyTargets verifies REFERENCES without a column only matches the parent's primary key.
func TestVerifyResolvesOmittedForeignKeyTargets(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)

	// The foreign keys omit their target column, so they reference regions' primary key, id.
	execStatements(t, db, `CREATE TABLE regions (id TEXT PRIMARY KEY, code TEXT UNIQUE, name TEXT);
CREATE TABLE cities (id TEXT PRIMARY KEY, name TEXT NOT NULL, region_id TEXT REFERENCES regions);
CREATE INDEX cities_name ON cities (name);
CREATE TABLE code_cities (id TEXT PRIMARY KEY, region_id TEXT REFERENCES regions)`)

	// A hint naming the primary key matches the omitted target.
	if err := Model[City](db).Verify(ctx); err != nil {
		t.Fatalf("City Verify() error = %v", err)
	}

	// A hint naming another column must not be accepted just because the target was omitted.
	if err := Model[CodeCity](db).Verify(ctx); err == nil || !strings.Contains(err.Error(), "hint belongs_to=regions(code)") {
		t.Fatalf("CodeCity Verify() error = %v, want missing foreign key to regions(code)", err)
	}
}

// TestVerifyReportsUnmappableFieldTypes verifies Verify flags types with no SQL mapping unless they convert themselves.
func TestVerifyReportsUnmappableFieldTypes(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)

	execStatements(t, db, `CREATE TABLE gadgets (id TEXT PRIMARY KEY, meta TEXT);
CREATE TABLE badges (id TEXT PRIMARY KEY, label TEXT)`)

	// A map has no column mapping and no Valuer, so the model cannot persist it.
	err := Model[Gadget](db).Verify(ctx)
	if !errors.Is(err, ErrSchemaMismatch) || !strings.Contains(err.Error(), "Go type map[string]string has no SQL column mapping") {
		t.Fatalf("Gadget Verify() error = %v, want unmappable type problem", err)
	}

	// A type that implements driver.Valuer and sql.Scanner is storable without a built-in mapping.
	if err := Model[Badge](db).Verify(ctx); err != nil {
		t.Fatalf("Badge Verify() error = %v", err)
	}
}

// TestVerifyMatchesColumnsCaseInsensitively verifies identifier case does not cause false drift.
func TestVerifyMatchesColumnsCaseInsensitively(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)

	execStatements(t, db, `CREATE TABLE users (ID TEXT PRIMARY KEY, EMAIL TEXT NOT NULL, Embedding BLOB, CREATED_AT TEXT NOT NULL, Updated_At TEXT NOT NULL);
CREATE UNIQUE INDEX users_email ON users (Email);`)

	if err := Model[User](db).Verify(ctx); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

// TestExplicitPrimaryKey verifies the primary_key hint selects the identity column.
func TestExplicitPrimaryKey(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	deviceModel := Model[Device](db)

	execStatements(t, db, `CREATE TABLE devices (external_id TEXT PRIMARY KEY, label TEXT)`)

	created, err := deviceModel.Create(ctx, Device{ExternalID: "external", Label: "phone"})
	if err != nil {
		t.Fatal(err)
	}
	found, err := deviceModel.Find(ctx, "external")
	if err != nil || found.Label != created.Label {
		t.Fatalf("Find() = %#v, error = %v", found, err)
	}
}

// TestDefaultIdentifierNames verifies table names derive from type names.
func TestDefaultIdentifierNames(t *testing.T) {
	// Initialize Variables
	blogPost := pluralize(snakeCase("BlogPost"))
	city := pluralize(snakeCase("City"))
	person := pluralize(snakeCase("Person"))
	users := pluralize(snakeCase("Users"))

	if apiKey := snakeCase("APIKey"); apiKey != "api_key" {
		t.Fatalf("snakeCase(APIKey) = %q, want api_key", apiKey)
	}
	if blogPost != "blog_posts" || city != "cities" || person != "people" || users != "users" {
		t.Fatalf("table names = %q, %q, %q, %q", blogPost, city, person, users)
	}
}

// TestRecordRequiresBinding verifies standalone records cannot issue database writes accidentally.
func TestRecordRequiresBinding(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	user := User{ID: "unbound", Email: "hello@null.live"}

	if err := user.Save(ctx); !errors.Is(err, ErrUnboundRecord) {
		t.Fatalf("Save() error = %v, want ErrUnboundRecord", err)
	}
	if err := user.Delete(ctx); !errors.Is(err, ErrUnboundRecord) {
		t.Fatalf("Delete() error = %v, want ErrUnboundRecord", err)
	}
}

// TestModelBind verifies a manually-created record can opt into instance persistence methods.
func TestModelBind(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)
	user := User{ID: "manual-user", Email: "manual@null.live"}
	var nilUser *User

	execStatements(t, db, usersSchema)

	if _, err := userModel.Bind(nilUser); !errors.Is(err, ErrUnboundRecord) {
		t.Fatalf("Bind(nil) error = %v, want ErrUnboundRecord", err)
	}
	if _, err := userModel.Bind(&user); err != nil {
		t.Fatalf("Bind() error = %v", err)
	}
	if !user.CreatedAt.IsZero() || !user.UpdatedAt.IsZero() {
		t.Fatalf("Bind() loaded timestamps = %v, %v", user.CreatedAt, user.UpdatedAt)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO users (id, email, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", user.ID, user.Email); err != nil {
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
	db := openTestDatabase(t)
	userModel := Model[User](db)
	var email string

	execStatements(t, db, usersSchema)

	alice, err := userModel.Create(ctx, User{ID: "alice", Email: "alice@null.live"})
	if err != nil {
		t.Fatalf("Create(alice) error = %v", err)
	}
	if _, err := userModel.Create(ctx, User{ID: "bob", Email: "bob@null.live"}); err != nil {
		t.Fatalf("Create(bob) error = %v", err)
	}

	// Reject identity changes before they can target another row.
	alice.ID = "bob"
	alice.Email = "overwritten@null.live"
	if err := alice.Save(ctx); !errors.Is(err, ErrIDChanged) {
		t.Fatalf("Save() error = %v, want ErrIDChanged", err)
	}
	if err := alice.Delete(ctx); !errors.Is(err, ErrIDChanged) {
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
	db := openTestDatabase(t)
	profileModel := Model[Profile](db)

	execStatements(t, db, `CREATE TABLE profiles (id TEXT PRIMARY KEY, nickname TEXT)`)

	if _, err := profileModel.Create(ctx, Profile{ID: "missing"}); err != nil {
		t.Fatalf("Create(NULL nickname) error = %v", err)
	}
	if _, err := profileModel.Create(ctx, Profile{ID: "empty", Nickname: sql.NullString{Valid: true}}); err != nil {
		t.Fatalf("Create(empty nickname) error = %v", err)
	}
	missing, err := profileModel.Find(ctx, "missing")
	if err != nil || missing.Nickname.Valid {
		t.Fatalf("Find(NULL nickname) = %#v, error = %v", missing, err)
	}
	empty, err := profileModel.Find(ctx, "empty")
	if err != nil || !empty.Nickname.Valid || empty.Nickname.String != "" {
		t.Fatalf("Find(empty nickname) = %#v, error = %v", empty, err)
	}
}

// TestNullableScalarsReadAsZeroValues verifies NULL columns remain readable by plain Go fields.
func TestNullableScalarsReadAsZeroValues(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	accountModel := Model[Account](db)
	present := Account{
		ID:      "present",
		Count:   sql.NullInt64{Int64: 42, Valid: true},
		Score:   sql.NullFloat64{Float64: 3.5, Valid: true},
		Enabled: sql.NullBool{Bool: true, Valid: true},
		Visits:  7,
	}

	execStatements(t, db, `CREATE TABLE accounts (id TEXT PRIMARY KEY, count INTEGER, score REAL, enabled INTEGER, visits INTEGER);
INSERT INTO accounts (id) VALUES ('legacy');`)

	// Rows with NULL scalars read as zero values; nullable wrappers keep NULL distinguishable.
	legacy, err := accountModel.Find(ctx, "legacy")
	if err != nil || legacy.Visits != 0 || legacy.Count.Valid || legacy.Score.Valid || legacy.Enabled.Valid {
		t.Fatalf("Find(legacy) = %#v, error = %v", legacy, err)
	}
	if _, err := accountModel.Create(ctx, present); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	found, err := accountModel.Find(ctx, "present")
	if err != nil || found.Count != present.Count || found.Score != present.Score || found.Enabled != present.Enabled || found.Visits != 7 {
		t.Fatalf("Find(present) = %#v, error = %v", found, err)
	}
}

// TestModelRejectsInvalidDeclarations verifies bad tags and shapes panic before touching the database.
func TestModelRejectsInvalidDeclarations(t *testing.T) {
	// Initialize Variables
	db := openTestDatabase(t)

	assertModelPanics(t, func() { Model[untaggedField](db) })
	assertModelPanics(t, func() { Model[unsupportedRecordHint](db) })
	assertModelPanics(t, func() { Model[unsupportedFieldHint](db) })
	assertModelPanics(t, func() { Model[reservedTimestampColumn](db) })
	assertModelPanics(t, func() { Model[invalidCascadeWithoutForeignKey](db) })
	assertModelPanics(t, func() { Model[invalidForeignKeyTarget](db) })
	assertModelPanics(t, func() { Model[duplicateColumnUser](db) })
	assertModelPanics(t, func() { Model[multiplePrimaryKeyUser](db) })
	assertModelPanics(t, func() { Model[primaryKeyUniqueWithUser](db) })
	assertModelPanics(t, func() { Model[mutablePrimaryKeyUser](db) })
	assertModelPanics(t, func() { Model[uintPrimaryKeyUser](db) })
	assertModelPanics(t, func() { Model[uint64PrimaryKeyUser](db) })
	assertModelPanics(t, func() { Model[noPrimaryKeyUser](db) })
}

// TestModelAcceptsSkippedFields verifies db:"-" excludes an exported field from persistence.
func TestModelAcceptsSkippedFields(t *testing.T) {
	// Initialize Variables
	model := Model[skippedField](openTestDatabase(t))

	if len(model.fields) != 1 || model.fields[0].column != "id" {
		t.Fatalf("fields = %#v, want only id", model.fields)
	}
}

// TestUntaggedFieldPanicNamesTheField verifies the db-tag requirement explains how to fix the model.
func TestUntaggedFieldPanicNamesTheField(t *testing.T) {
	// Initialize Variables
	db := openTestDatabase(t)
	recovered := any(nil)

	func() {
		defer func() { recovered = recover() }()
		Model[untaggedField](db)
	}()
	err, ok := recovered.(error)
	if !ok || !strings.Contains(err.Error(), "Email") || !strings.Contains(err.Error(), "db tag") {
		t.Fatalf("panic = %v, want db tag guidance naming Email", recovered)
	}
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

// execStatements runs semicolon-separated DDL statements against db.
func execStatements(t *testing.T, db *sql.DB, statements string) {
	// Initialize Variables
	ctx := context.Background()

	t.Helper()
	for _, statement := range strings.Split(statements, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
}

// openTestDatabase opens an isolated temporary Turso database.
func openTestDatabase(t *testing.T) *sql.DB {
	// Initialize Variables
	path := filepath.Join(t.TempDir(), "activeso.db")
	db, err := sql.Open("turso", path)

	t.Helper()
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

// TestSaveTxCommitsAndRollsBack verifies SaveTx and DeleteTx join a caller's transaction without rebinding the record.
func TestSaveTxCommitsAndRollsBack(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)

	execStatements(t, db, usersSchema)

	created, err := userModel.Create(ctx, User{Email: "before@null.live", Embedding: Vector32{1, 2, 3}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// A rolled-back SaveTx and DeleteTx leave the row untouched.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	created.Email = "rolled-back@null.live"
	if err := created.SaveTx(ctx, tx); err != nil {
		t.Fatalf("SaveTx() error = %v", err)
	}
	if err := created.DeleteTx(ctx, tx); err != nil {
		t.Fatalf("DeleteTx() error = %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	found, err := userModel.Find(ctx, created.ID)
	if err != nil {
		t.Fatalf("Find() after rollback error = %v", err)
	}
	if found.Email != "before@null.live" {
		t.Fatalf("Find() after rollback email = %q, want before@null.live", found.Email)
	}

	// A committed SaveTx persists, and the record still saves through the model's own executor afterwards.
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	created.Email = "committed@null.live"
	if err := created.SaveTx(ctx, tx); err != nil {
		t.Fatalf("SaveTx() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	created.Email = "plain-save@null.live"
	if err := created.Save(ctx); err != nil {
		t.Fatalf("Save() after SaveTx() error = %v", err)
	}
	found, err = userModel.Find(ctx, created.ID)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if found.Email != "plain-save@null.live" {
		t.Fatalf("Find() email = %q, want plain-save@null.live", found.Email)
	}

	// A committed DeleteTx removes the row.
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := found.DeleteTx(ctx, tx); err != nil {
		t.Fatalf("DeleteTx() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if _, err := userModel.Find(ctx, found.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find() after committed DeleteTx() error = %v, want ErrNotFound", err)
	}
}

// TestSaveTxRequiresBinding verifies unbound records and ended transactions fail clearly.
func TestSaveTxRequiresBinding(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)
	unbound := User{ID: "unbound", Email: "unbound@null.live"}

	execStatements(t, db, usersSchema)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := unbound.SaveTx(ctx, tx); !errors.Is(err, ErrUnboundRecord) {
		t.Fatalf("SaveTx() unbound error = %v, want ErrUnboundRecord", err)
	}
	if err := unbound.DeleteTx(ctx, tx); !errors.Is(err, ErrUnboundRecord) {
		t.Fatalf("DeleteTx() unbound error = %v, want ErrUnboundRecord", err)
	}
	created, err := userModel.Create(ctx, User{Email: "ended@null.live", Embedding: Vector32{1, 2, 3}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	// Using a finished transaction reports database/sql's ErrTxDone.
	if err := created.SaveTx(ctx, tx); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("SaveTx() after Commit() error = %v, want sql.ErrTxDone", err)
	}
}

// TestUsingViewSpansModelsAtomically verifies a Using view makes creates across models commit or roll back together.
func TestUsingViewSpansModelsAtomically(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)
	regionModel := Model[Region](db)

	execStatements(t, db, usersSchema+";CREATE TABLE regions (id TEXT PRIMARY KEY, name TEXT)")

	// Writes through both views are invisible after a rollback.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	txUsers, txRegions := userModel.Using(tx), regionModel.Using(tx)
	user, err := txUsers.Create(ctx, User{Email: "tx@null.live", Embedding: Vector32{1, 2, 3}})
	if err != nil {
		t.Fatalf("Using().Create() error = %v", err)
	}
	if _, err := txRegions.Create(ctx, Region{Name: "North"}); err != nil {
		t.Fatalf("Using().Create() region error = %v", err)
	}
	inside, err := txUsers.Find(ctx, user.ID)
	if err != nil || inside.Email != "tx@null.live" {
		t.Fatalf("Using().Find() = %#v, %v; want the uncommitted user", inside, err)
	}

	// A record created through the view saves on the transaction with a plain Save.
	user.Email = "tx-saved@null.live"
	if err := user.Save(ctx); err != nil {
		t.Fatalf("Save() on a Using record error = %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if users, err := userModel.All(ctx); err != nil || len(users) != 0 {
		t.Fatalf("All() users after rollback = %d, %v; want none", len(users), err)
	}
	if regions, err := regionModel.All(ctx); err != nil || len(regions) != 0 {
		t.Fatalf("All() regions after rollback = %d, %v; want none", len(regions), err)
	}

	// Bind moves an existing record into a new transaction.
	existing, err := userModel.Create(ctx, User{Email: "existing@null.live", Embedding: Vector32{1, 2, 3}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if _, err := userModel.Using(tx).Bind(existing); err != nil {
		t.Fatalf("Using().Bind() error = %v", err)
	}
	existing.Email = "bound-to-tx@null.live"
	if err := existing.Save(ctx); err != nil {
		t.Fatalf("Save() after Bind() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	found, err := userModel.Find(ctx, existing.ID)
	if err != nil || found.Email != "bound-to-tx@null.live" {
		t.Fatalf("Find() after commit = %#v, %v; want bound-to-tx@null.live", found, err)
	}
}

// TestUsingViewMapsUniqueViolations verifies unique errors keep their public type inside a transaction.
func TestUsingViewMapsUniqueViolations(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)

	execStatements(t, db, usersSchema)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	defer tx.Rollback()
	txUsers := userModel.Using(tx)
	if _, err := txUsers.Create(ctx, User{Email: "dup@null.live"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = txUsers.Create(ctx, User{Email: "dup@null.live"})
	if !errors.Is(err, ErrUnique) {
		t.Fatalf("Create() duplicate error = %v, want ErrUnique", err)
	}
}

// TestScopedViewDoesNotCacheReadiness verifies DDL rolled back inside a transaction never marks the model ready.
func TestScopedViewDoesNotCacheReadiness(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	regionModel := Model[Region](db)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if _, err := tx.ExecContext(ctx, "CREATE TABLE regions (id TEXT PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatalf("CREATE TABLE error = %v", err)
	}
	if _, err := regionModel.Using(tx).All(ctx); err != nil {
		t.Fatalf("Using().All() error = %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	// The root model must still see the missing table.
	if _, err := regionModel.All(ctx); !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("All() after rolled-back DDL error = %v, want ErrSchemaMismatch", err)
	}
}

// TestModelRejectsNilExecutors verifies nil and typed-nil executors fail at setup.
func TestModelRejectsNilExecutors(t *testing.T) {
	// Initialize Variables
	var nilDB *sql.DB
	db := openTestDatabase(t)

	assertModelPanics(t, func() { Model[User](nil) })
	assertModelPanics(t, func() { Model[User](nilDB) })
	assertModelPanics(t, func() { Model[User](db).Using(nilDB) })
}

// TestCreateTxAndFindTxJoinTransactions verifies the per-call variants commit, roll back, and rebind to the original model.
func TestCreateTxAndFindTxJoinTransactions(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)
	vector := Vector32{1, 2, 3}

	execStatements(t, db, usersSchema)

	// A rolled-back CreateTx leaves no row, and FindTx sees the uncommitted one inside.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	created, err := userModel.CreateTx(ctx, tx, User{Email: "tx@null.live", Embedding: vector})
	if err != nil {
		t.Fatalf("CreateTx() error = %v", err)
	}
	inside, err := userModel.FindTx(ctx, tx, created.ID)
	if err != nil || inside.Email != "tx@null.live" {
		t.Fatalf("FindTx() = %#v, %v; want the uncommitted user", inside, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if _, err := userModel.Find(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find() after rollback error = %v, want ErrNotFound", err)
	}

	// A committed CreateTx persists, and the record then saves on the model's own executor without the finished transaction.
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	created, err = userModel.CreateTx(ctx, tx, User{Email: "kept@null.live", Embedding: vector})
	if err != nil {
		t.Fatalf("CreateTx() error = %v", err)
	}
	found, err := userModel.FindTx(ctx, tx, created.ID)
	if err != nil {
		t.Fatalf("FindTx() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	created.Email = "saved-after-commit@null.live"
	if err := created.Save(ctx); err != nil {
		t.Fatalf("Save() after CreateTx() error = %v", err)
	}
	found.Email = "found-saved@null.live"
	if err := found.Save(ctx); err != nil {
		t.Fatalf("Save() after FindTx() error = %v", err)
	}
}

// TestTxMethodsRejectNilExecutors verifies per-call methods return an error instead of panicking.
func TestTxMethodsRejectNilExecutors(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)
	var nilTx *sql.Tx

	execStatements(t, db, usersSchema)

	created, err := userModel.Create(ctx, User{Email: "nil@null.live", Embedding: Vector32{1, 2, 3}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := userModel.CreateTx(ctx, nilTx, User{Email: "x@null.live"}); err == nil {
		t.Fatal("CreateTx() with a nil transaction returned no error")
	}
	if _, err := userModel.FindTx(ctx, nil, created.ID); err == nil {
		t.Fatal("FindTx() with a nil transaction returned no error")
	}
	if err := created.SaveTx(ctx, nilTx); err == nil {
		t.Fatal("SaveTx() with a nil transaction returned no error")
	}
	if err := created.DeleteTx(ctx, nil); err == nil {
		t.Fatal("DeleteTx() with a nil transaction returned no error")
	}
}

// TestScopedViewDoesNotWaitBehindRootInspection verifies a transaction can finish while a root-model call waits for its connection.
func TestScopedViewDoesNotWaitBehindRootInspection(t *testing.T) {
	// Initialize Variables
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	db := openTestDatabase(t)
	userModel := Model[User](db)
	rootDone := make(chan error, 1)
	txDone := make(chan error, 1)

	defer cancel()
	db.SetMaxOpenConns(1)
	execStatements(t, db, usersSchema)

	// The transaction holds the pool's only connection.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}

	// A root-model call starts its first inspection and waits for a connection.
	go func() {
		_, err := userModel.All(ctx)
		rootDone <- err
	}()
	time.Sleep(200 * time.Millisecond)

	// The transaction's own work must still complete, or it can never release the connection.
	go func() {
		if _, err := userModel.Using(tx).Create(ctx, User{Email: "tx@null.live", Embedding: Vector32{1, 2, 3}}); err != nil {
			txDone <- err
			return
		}
		txDone <- tx.Commit()
	}()
	select {
	case err := <-txDone:
		if err != nil {
			t.Fatalf("transaction error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("transaction blocked behind a root-model call waiting for its connection")
	}

	// The waiting root call proceeds once the connection is released.
	if err := <-rootDone; err != nil {
		t.Fatalf("root All() error = %v", err)
	}
}

// TestVerifyWarmsReadinessOnlyForRootPasses verifies a clean root Verify skips later inspections and nothing else does.
func TestVerifyWarmsReadinessOnlyForRootPasses(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db := openTestDatabase(t)
	userModel := Model[User](db)

	execStatements(t, db, usersSchema)

	// A clean Verify through a transaction view must not warm the model.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := userModel.Using(tx).Verify(ctx); err != nil {
		t.Fatalf("Using().Verify() error = %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if userModel.readiness.ready.Load() {
		t.Fatal("Using().Verify() marked the model ready")
	}

	// Hint drift fails Verify, so it must not warm the model either.
	driftModel := Model[Membership](db)
	execStatements(t, db, `CREATE TABLE memberships (id TEXT PRIMARY KEY, customer_id TEXT NOT NULL, organization_id TEXT NOT NULL)`)
	if err := driftModel.Verify(ctx); err == nil {
		t.Fatal("Verify() with drift returned no error")
	}
	if driftModel.readiness.ready.Load() {
		t.Fatal("a failed Verify() marked the model ready")
	}

	// A clean root Verify warms the model.
	if err := userModel.Verify(ctx); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !userModel.readiness.ready.Load() {
		t.Fatal("Verify() did not mark the model ready")
	}
}

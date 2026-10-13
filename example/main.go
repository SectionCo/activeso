// Package main demonstrates ActiveSo in a small Echo v5 user-management web application.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/sectionco/activeso/v2"
)

// usersSchema is the application-owned DDL for the users table.
// ActiveSo never creates or alters tables; the hints on User describe what this schema enforces.
const usersSchema = `CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	email TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS users_email ON users (email);`

// User maps an application user to the default users table.
// The timestamps hint tells ActiveSo the table has created_at and updated_at columns.
type User struct {
	activeso.Record `activeso:"timestamps"`

	ID    string `db:"id"`
	Email string `db:"email" activeso:"not_null,unique"`
}

// pageData holds the data for the HTML template.
type pageData struct {
	Users   []*User
	Found   *User
	Message string
}

// page holds the HTML template for the users page.
var page = template.Must(template.New("users").Parse(`<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>ActiveSo Users</title>
	<style>
		body { font-family: system-ui, sans-serif; max-width: 44rem; margin: 3rem auto; padding: 0 1rem; }
		form { display: flex; gap: .5rem; align-items: center; margin: .75rem 0; }
		input, textarea { flex: 1; padding: .5rem; font: inherit; }
		button { padding: .5rem .75rem; cursor: pointer; }
		.delete { background: #b42318; color: white; border: 0; }
		.message { color: #b42318; }
		.id { color: #667085; font-size: .8rem; }
	</style>
</head>
<body>
	<h1>Users</h1>
	<form action="/users" method="post">
		<input name="email" type="email" placeholder="hello@null.live" required>
		<button type="submit">Create user</button>
	</form>
	<form action="/sync" method="post">
		<button type="submit">Sync now</button>
	</form>
	<form action="/users/batch" method="post">
		<textarea name="emails" rows="3" placeholder="One email per line; all are created together or none are" required></textarea>
		<button type="submit">Create all</button>
	</form>
	<form action="/users/find" method="get">
		<input name="id" type="text" placeholder="User ID" required>
		<button type="submit">Find user</button>
	</form>

	{{if .Message}}
		<p class="message">{{.Message}}</p>
	{{end}}

	{{if .Found}}
		<h2>Found user</h2>
		<p>{{.Found.Email}} <span class="id">{{.Found.ID}}</span></p>
	{{end}}

	{{if .Users}}
		<h2>Existing users</h2>
		{{range .Users}}
			<form action="/users/{{.ID}}" method="post">
				<input name="email" type="email" value="{{.Email}}" required>
				<button type="submit">Save</button>
				<button class="delete" formaction="/users/{{.ID}}/delete" type="submit">Delete</button>
			</form>
			<div class="id">User ID: {{.ID}}</div>
		{{end}}
	{{else}}
		<p>No users yet.</p>
	{{end}}
</body>
</html>`))

// userCreator creates a user on a caller-owned transaction; the model returned by activeso.Model satisfies it.
type userCreator interface {
	CreateTx(ctx context.Context, tx activeso.Executor, value User) (*User, error)
}

// createUsers creates every email in one transaction, so either all users are saved or none are.
// ActiveSo never begins or ends a transaction: this function owns Begin, Commit, and Rollback.
func createUsers(ctx context.Context, db *sql.DB, users userCreator, emails []string) error {
	// Initialize Variables
	tx, err := db.BeginTx(ctx, nil)

	if err != nil {
		return err
	}

	// Roll back on any early return; after a successful Commit this is a harmless no-op.
	defer tx.Rollback()

	// CreateTx runs each insert on the transaction, so a duplicate email undoes the earlier ones too.
	for _, email := range emails {
		if _, err := users.CreateTx(ctx, tx, User{Email: email}); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// syncWithCloud pulls remote changes and pushes local ones, and reports a failure to the caller instead of retrying it.
func syncWithCloud(ctx context.Context, database *activeso.Sync) error {
	// Initialize Variables
	err := database.Sync(ctx)

	if err != nil {
		return err
	}

	return nil
}

// syncFailureMessage tells the user a sync failed and that their changes are safe locally, so they can decide when to retry.
func syncFailureMessage(err error) string {
	// Initialize Variables
	message := "Syncing with Turso Cloud failed: " + err.Error() + ". Your changes are saved locally; use Sync now to retry."

	return message
}

// main opens a local database synced with Turso Cloud and starts the Echo example server.
func main() {
	// Initialize Variables
	ctx := context.Background()
	remoteURL := os.Getenv("TURSO_DATABASE_URL")
	authToken := os.Getenv("TURSO_AUTH_TOKEN")

	// The example syncs with a Turso Cloud database you own, so its location is required.
	if remoteURL == "" {
		log.Fatal("set TURSO_DATABASE_URL (and TURSO_AUTH_TOKEN) to a Turso Cloud database")
	}

	// Open the local database file and bootstrap it from the remote.
	database, err := activeso.OpenSync(ctx, activeso.SyncConfig{
		Path:      "activeso-example.db",
		RemoteURL: remoteURL,
		AuthToken: authToken,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	db := database.DB()

	// Initialize ActiveSo Model
	userModel := activeso.Model[User](db)

	// Create the table with the application's own schema; ActiveSo does not manage migrations.
	for _, statement := range strings.Split(usersSchema, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			log.Fatal(err)
		}
	}

	// Verify reports any drift between the table and the User hints without changing the database.
	if err := userModel.Verify(ctx); err != nil {
		log.Fatal(err)
	}

	// finishWrite syncs after a write and redirects home, or shows the failure so the user decides whether to retry.
	finishWrite := func(c *echo.Context) error {
		// Initialize Variables
		ctx := c.Request().Context()
		syncErr := syncWithCloud(ctx, database)

		if syncErr == nil {
			return c.Redirect(http.StatusSeeOther, "/")
		}

		// The write already succeeded locally, so show the page with the sync error rather than an error status.
		users, listErr := userModel.All(ctx)
		if listErr != nil {
			return listErr
		}

		return renderPage(c, users, nil, syncFailureMessage(syncErr))
	}

	// Initialize the Echo server.
	// This will set up the necessary routes and middleware for the server.
	e := echo.New()
	e.GET("/", func(c *echo.Context) error {

		// Find all users.
		users, err := userModel.All(ctx)
		if err != nil {
			return err
		}

		// Render the page with the list of users.
		return renderPage(c, users, nil, "")
	})
	e.GET("/users/find", func(c *echo.Context) error {
		// Initialize Variables
		ctx := c.Request().Context()
		id := strings.TrimSpace(c.QueryParam("id"))
		users, err := userModel.All(ctx)
		if err != nil {
			return err
		}

		if id == "" {
			return renderPage(c, users, nil, "Enter a user ID to find a user.")
		}

		// Find the user by ID.
		user, err := userModel.Find(ctx, id)
		if errors.Is(err, activeso.ErrNotFound) {
			return renderPage(c, users, nil, "No user found for that ID.")
		}
		if err != nil {
			return err
		}

		return renderPage(c, users, user, "")
	})

	e.POST("/users", func(c *echo.Context) error {
		// Initialize Variables
		ctx := c.Request().Context()
		email := strings.TrimSpace(c.FormValue("email"))
		if email == "" {
			return c.String(http.StatusBadRequest, "email is required")
		}

		// Create a new user.
		if _, err := userModel.Create(ctx, User{Email: email}); err != nil {
			return err
		}
		return finishWrite(c)
	})
	e.POST("/sync", func(c *echo.Context) error {
		// Sync on demand so the user decides when to retry after a failure.
		return finishWrite(c)
	})
	e.POST("/users/batch", func(c *echo.Context) error {
		// Initialize Variables
		ctx := c.Request().Context()
		emails := []string{}

		// Read one email per line, skipping blanks.
		for _, line := range strings.Split(c.FormValue("emails"), "\n") {
			if email := strings.TrimSpace(line); email != "" {
				emails = append(emails, email)
			}
		}
		if len(emails) == 0 {
			return c.String(http.StatusBadRequest, "at least one email is required")
		}

		// Create every user in a single transaction.
		if err := createUsers(ctx, db, userModel, emails); err != nil {
			if errors.Is(err, activeso.ErrUnique) {
				users, listErr := userModel.All(ctx)
				if listErr != nil {
					return listErr
				}
				return renderPage(c, users, nil, "No users were added: one of those emails already exists.")
			}
			return err
		}
		return finishWrite(c)
	})
	e.POST("/users/:id", func(c *echo.Context) error {
		// Initialize Variables
		ctx := c.Request().Context()
		id := c.Param("id")
		email := strings.TrimSpace(c.FormValue("email"))
		if email == "" {
			return c.String(http.StatusBadRequest, "email is required")
		}

		// Find the user by ID.
		user, err := userModel.Find(ctx, id)
		if errors.Is(err, activeso.ErrNotFound) {
			return c.String(http.StatusNotFound, "user not found")
		}
		if err != nil {
			return err
		}

		// Update the user's email.
		user.Email = email
		if err := user.Save(ctx); err != nil {
			return err
		}
		return finishWrite(c)
	})
	e.POST("/users/:id/delete", func(c *echo.Context) error {
		// Initialize Variables
		ctx := c.Request().Context()
		id := c.Param("id")
		user, err := userModel.Find(ctx, id)
		if errors.Is(err, activeso.ErrNotFound) {
			return c.String(http.StatusNotFound, "user not found")
		}
		if err != nil {
			return err
		}

		// Delete the user.
		if err := user.Delete(ctx); err != nil {
			return err
		}

		// Sync, then redirect home.
		return finishWrite(c)
	})

	// Start the Echo server on port 8080.
	if err := e.Start(":8080"); err != nil {
		log.Fatal(err)
	}
	log.Println("Open http://localhost:8080")
}

// renderPage applies the user list, found user, and message to the HTML template and returns the result.
func renderPage(c *echo.Context, users []*User, found *User, message string) error {
	// Initialize Variables
	var output bytes.Buffer
	data := pageData{Users: users, Found: found, Message: message}
	err := page.Execute(&output, data)

	if err != nil {
		return err
	}

	return c.HTML(http.StatusOK, output.String())
}

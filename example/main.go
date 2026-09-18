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
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/sectionco/activeso"
	turso "turso.tech/database/tursogo"
)

// User maps an application user to the default users table.
type User struct {
	activeso.Record

	ID    string `db:"id"`
	Email string `db:"email" activeso:"not_null,unique"`
}

type pageData struct {
	Users []*User
}

var page = template.Must(template.New("users").Parse(`<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>ActiveSo Users</title>
	<style>
		body { font-family: system-ui, sans-serif; max-width: 44rem; margin: 3rem auto; padding: 0 1rem; }
		form { display: flex; gap: .5rem; align-items: center; margin: .75rem 0; }
		input { flex: 1; padding: .5rem; }
		button { padding: .5rem .75rem; cursor: pointer; }
		.delete { background: #b42318; color: white; border: 0; }
		.id { color: #667085; font-size: .8rem; }
	</style>
</head>
<body>
	<h1>Users</h1>
	<form action="/users" method="post">
		<input name="email" type="email" placeholder="hello@null.live" required>
		<button type="submit">Create user</button>
	</form>

	{{if .Users}}
		<h2>Existing users</h2>
		{{range .Users}}
			<form action="/users/{{.ID}}" method="post">
				<input name="email" type="email" value="{{.Email}}" required>
				<button type="submit">Save</button>
				<button class="delete" formaction="/users/{{.ID}}/delete" type="submit">Delete</button>
			</form>
			<div class="id">{{.ID}}</div>
		{{end}}
	{{else}}
		<p>No users yet.</p>
	{{end}}
</body>
</html>`))

// main creates a local Turso database and starts the Echo example server.
func main() {
	// Initialize Variables
	ctx := context.Background()

	// Open the database connection.
	db, err := openDatabase("activeso-example.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	echoServer, err := newServer(ctx, db)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Open http://localhost:8080")
	if err := echoServer.Start(":8080"); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// openDatabase creates a local Turso database connection for the supplied file path.
func openDatabase(path string) (*sql.DB, error) {
	// Initialize Variables
	connector, err := turso.NewConnector(path)

	if err != nil {
		return nil, err
	}

	return sql.OpenDB(connector), nil
}

// newServer migrates the User schema and configures its browser routes.
func newServer(ctx context.Context, db *sql.DB) (*echo.Echo, error) {
	// Initialize Variables
	userModel := activeso.Model[User](db)
	echoServer := echo.New()

	if err := userModel.AutoMigrate(ctx); err != nil {
		return nil, err
	}

	echoServer.GET("/", func(c *echo.Context) error {
		// Initialize Variables
		ctx := c.Request().Context()
		users, err := userModel.All(ctx)

		if err != nil {
			return err
		}

		return renderPage(c, users)
	})
	echoServer.POST("/users", func(c *echo.Context) error {
		// Initialize Variables
		ctx := c.Request().Context()
		email := strings.TrimSpace(c.FormValue("email"))

		if email == "" {
			return c.String(http.StatusBadRequest, "email is required")
		}

		if _, err := userModel.Create(ctx, User{Email: email}); err != nil {
			return err
		}

		return c.Redirect(http.StatusSeeOther, "/")
	})
	echoServer.POST("/users/:id", func(c *echo.Context) error {
		// Initialize Variables
		ctx := c.Request().Context()
		id := c.Param("id")
		email := strings.TrimSpace(c.FormValue("email"))

		if email == "" {
			return c.String(http.StatusBadRequest, "email is required")
		}

		user, err := userModel.Find(ctx, id)
		if errors.Is(err, activeso.ErrNotFound) {
			return c.String(http.StatusNotFound, "user not found")
		}
		if err != nil {
			return err
		}

		user.Email = email
		if err := user.Save(ctx); err != nil {
			return err
		}

		return c.Redirect(http.StatusSeeOther, "/")
	})
	echoServer.POST("/users/:id/delete", func(c *echo.Context) error {
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

		if err := user.Delete(ctx); err != nil {
			return err
		}

		return c.Redirect(http.StatusSeeOther, "/")
	})
	return echoServer, nil
}

// renderPage applies the user list to the HTML template and returns the result.
func renderPage(c *echo.Context, users []*User) error {
	// Initialize Variables
	var output bytes.Buffer
	data := pageData{Users: users}
	err := page.Execute(&output, data)

	if err != nil {
		return err
	}

	return c.HTML(http.StatusOK, output.String())
}

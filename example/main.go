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
		input { flex: 1; padding: .5rem; }
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

// main creates a local Turso database and starts the Echo example server.
func main() {
	// Initialize Variables
	ctx := context.Background()

	// Open the database connection to a local Turso database.
	connector, err := turso.NewConnector("activeso-example.db")
	if err != nil {
		log.Fatal(err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()

	// Initialize ActiveSo Model
	userModel := activeso.Model[User](db)

	// AutoMigrate the User model.
	// This will create the necessary database schema for the User model.
	// If the schema already exists, it will be migrated to the latest version.
	// We recommend calling this once during application startup.
	if err := userModel.AutoMigrate(ctx); err != nil {
		log.Fatal(err)
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

		// Redirect to the home page.
		return c.Redirect(http.StatusSeeOther, "/")
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

		// Redirect to the home page.
		return c.Redirect(http.StatusSeeOther, "/")
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

		return c.Redirect(http.StatusSeeOther, "/")
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

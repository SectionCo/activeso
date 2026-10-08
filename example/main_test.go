package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
	"github.com/sectionco/activeso/v2"
)

// TestExampleDatabaseLifecycle verifies the example model's persistence workflow with the Turso engine.
func TestExampleDatabaseLifecycle(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	db, err := sql.Open("turso", filepath.Join(t.TempDir(), "example.db"))
	userModel := activeso.Model[User](db)
	var user *User
	var found *User

	// Use an isolated local database without opening the example's existing data files.
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(usersSchema, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := userModel.Verify(ctx); err != nil {
		t.Fatal(err)
	}

	// Create and read a user with ActiveSo-managed timestamps.
	user, err = userModel.Create(ctx, User{Email: "example@null.live"})
	if err != nil {
		t.Fatal(err)
	}
	if user.ID == "" || user.CreatedAt.IsZero() || user.UpdatedAt.IsZero() {
		t.Fatalf("created user = %+v", user)
	}
	found, err = userModel.Find(ctx, user.ID)
	if err != nil || found.Email != user.Email {
		t.Fatalf("Find() = %+v; error = %v", found, err)
	}

	// Save through the bound record and confirm the database contains the change.
	found.Email = "updated@null.live"
	if err := found.Save(ctx); err != nil {
		t.Fatal(err)
	}
	found, err = userModel.Find(ctx, user.ID)
	if err != nil || found.Email != "updated@null.live" || !found.CreatedAt.Equal(user.CreatedAt) {
		t.Fatalf("saved user = %+v; error = %v", found, err)
	}

	// Delete through the bound record and verify the row is gone.
	if err := found.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := userModel.Find(ctx, user.ID); !errors.Is(err, activeso.ErrNotFound) {
		t.Fatalf("Find() after Delete() error = %v, want ErrNotFound", err)
	}
}

// TestRenderPage verifies the page renders existing users and their management controls.
func TestRenderPage(t *testing.T) {
	// Initialize Variables
	echoServer := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	context := echoServer.NewContext(request, response)
	users := []*User{{ID: "user-1", Email: "hello@null.live"}}

	if err := renderPage(context, users, nil, ""); err != nil {
		t.Fatalf("renderPage() error = %v", err)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("render status = %d, want %d", response.Code, http.StatusOK)
	}

	body := response.Body.String()
	for _, expected := range []string{
		`action="/users" method="post"`,
		`action="/users/find" method="get"`,
		"hello@null.live",
		"User ID: user-1",
		`formaction="/users/user-1/delete"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("rendered page does not contain %q: %s", expected, body)
		}
	}
}

// TestRenderPageFindStates verifies the page displays a found user and a search message.
func TestRenderPageFindStates(t *testing.T) {
	// Initialize Variables
	echoServer := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/users/find", nil)
	response := httptest.NewRecorder()
	context := echoServer.NewContext(request, response)
	foundUser := &User{ID: "user-2", Email: "found@null.live"}
	message := "No user found for that ID."

	if err := renderPage(context, nil, foundUser, message); err != nil {
		t.Fatalf("renderPage() error = %v", err)
	}

	body := response.Body.String()
	for _, expected := range []string{
		"Found user",
		"found@null.live",
		"user-2",
		message,
		"No users yet.",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("rendered find page does not contain %q: %s", expected, body)
		}
	}
}

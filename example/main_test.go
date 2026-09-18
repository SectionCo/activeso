package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

// TestUserForms verifies the page creates, updates, displays, and deletes users.
func TestUserForms(t *testing.T) {
	// Initialize Variables
	echoServer, db := openTestServer(t)
	createResponse := performForm(t, echoServer, "/users", url.Values{"email": {"hello@null.live"}})
	var id string

	if createResponse.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d, want %d", createResponse.Code, http.StatusSeeOther)
	}
	if err := db.QueryRow("SELECT id FROM users WHERE email = ?", "hello@null.live").Scan(&id); err != nil {
		t.Fatalf("load created user: %v", err)
	}

	updateResponse := performForm(t, echoServer, "/users/"+id, url.Values{"email": {"updated@null.live"}})
	if updateResponse.Code != http.StatusSeeOther {
		t.Fatalf("update status = %d, want %d", updateResponse.Code, http.StatusSeeOther)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	listResponse := httptest.NewRecorder()
	echoServer.ServeHTTP(listResponse, listRequest)
	if !strings.Contains(listResponse.Body.String(), "updated@null.live") {
		t.Fatalf("list page does not contain the updated email: %s", listResponse.Body.String())
	}

	deleteResponse := performForm(t, echoServer, "/users/"+id+"/delete", url.Values{})
	if deleteResponse.Code != http.StatusSeeOther {
		t.Fatalf("delete status = %d, want %d", deleteResponse.Code, http.StatusSeeOther)
	}
	if err := db.QueryRow("SELECT id FROM users WHERE id = ?", id).Scan(&id); err != sql.ErrNoRows {
		t.Fatalf("deleted user query error = %v, want sql.ErrNoRows", err)
	}
}

// openTestServer creates a temporary Turso database and a configured Echo application.
func openTestServer(t *testing.T) (*echo.Echo, *sql.DB) {
	// Initialize Variables
	ctx := context.Background()
	db, err := openDatabase(":memory:")

	if err != nil {
		t.Fatalf("openDatabase() error = %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		// Initialize Variables
		closeError := db.Close()

		if closeError != nil {
			t.Errorf("db.Close() error = %v", closeError)
		}
	})

	echoServer, err := newServer(ctx, db)
	if err != nil {
		t.Fatalf("newServer() error = %v", err)
	}

	return echoServer, db
}

// performForm submits values as an application/x-www-form-urlencoded POST request.
func performForm(t *testing.T, echoServer *echo.Echo, path string, values url.Values) *httptest.ResponseRecorder {
	// Initialize Variables
	request, err := http.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	response := httptest.NewRecorder()

	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	echoServer.ServeHTTP(response, request)
	return response
}

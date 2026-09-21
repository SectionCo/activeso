package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

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

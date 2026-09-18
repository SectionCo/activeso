module github.com/sectionco/activeso/example

go 1.25.0

require (
	github.com/labstack/echo/v5 v5.3.1
	github.com/sectionco/activeso v0.0.0
	turso.tech/database/tursogo v0.7.2
)

require (
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/tursodatabase/turso-go-platform-libs v0.7.2 // indirect
	golang.org/x/sys v0.38.0 // indirect
)

replace github.com/sectionco/activeso => ..

# Turso Dependency Upgrades

## Module layout and current release

- The root library and `example/` are separate Go modules. Root `go test ./...` does not cover the example module.
- Both modules now pin `turso.tech/database/tursogo v0.8.2` and `github.com/tursodatabase/turso-go-platform-libs v0.8.2`.
- `go list -m -json <module>@latest` verified v0.8.2 as the latest published release for both packages on 2026-10-06. Recheck rather than assuming this remains the latest.
- The example's `replace github.com/sectionco/activeso => ..` uses the parent working tree, regardless of the ActiveSo version in its require directive. Updating Turso does not require publishing or incrementing ActiveSo's version.

## Verified compatibility

The existing example setup using `turso.NewConnector("activeso-example.db")` and `sql.OpenDB(connector)` builds unchanged with v0.8.2. No application API migration was necessary. The root library keeps its Go 1.24.0 directive and the example keeps Go 1.25.0; this dependency upgrade did not require changing either.

Root timestamp coverage lives in `model_test.go`, and targeted migration coverage lives in `migration_test.go`. See `../timestamp-replay/SKILL.md` for the current timestamp policy and coverage. In the example module, `TestExampleDatabaseLifecycle` uses a temporary local Turso database to verify AutoMigrate, Create, Find, bound Save/Delete, and timestamp population/preservation. Rendering tests cover the user-management page.

These checks verify local engine compatibility on the current macOS host, not a remote Push/Pull cycle or every supported native platform.

## Upgrade workflow

1. Verify the published SDK release with `go list -m -json turso.tech/database/tursogo@latest` and inspect its required native platform-library version. Keep the platform binaries aligned with the SDK, rather than independently upgrading mismatched versions.
2. Run `go get turso.tech/database/tursogo@<verified-version>` and `go mod tidy` in the root module first, then separately in `example/`.
3. Review both modules' `go.mod` and `go.sum` changes for unrelated upgrades.
4. Run `go test ./...`, `go vet ./...`, and `go mod verify` in each module; build the example without launching its server (for example `go -C example build -o /dev/null .` on macOS).
5. Use temporary test databases. Do not run the example against its existing database files merely to check an SDK upgrade. Never open a live synced replica with a plain non-sync connection or SQLite.

No protected `types` files, shell scripts, or `AGENTS.md` files need modification for this workflow.

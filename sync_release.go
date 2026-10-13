package activeso

import (
	"errors"
	"fmt"
	"reflect"
	"unsafe"

	turso "turso.tech/database/tursogo"
)

// syncDatabaseDeinit is the SDK's own routine that closes a sync database's files and frees its native
// handle. tursogo v0.8.3 never calls it for a TursoSyncDb and offers no exported way to do so, so it
// is linked here by name. If the SDK renames it, the build fails instead of silently leaking files.
//
//go:linkname syncDatabaseDeinit turso.tech/database/tursogo.turso_sync_database_deinit
func syncDatabaseDeinit(self turso.TursoSyncDatabase)

// releaseRemote frees the SDK's native sync handle, which closes the database, WAL, and changes files.
// Every connection must already be closed and no sync call may be in flight.
func releaseRemote(remote *turso.TursoSyncDb) (err error) {
	// Initialize Variables
	var closer interface{ Close() error }
	var field reflect.Value
	var ok bool

	// Prefer an exported Close if a later SDK release adds one.
	closer, ok = any(remote).(interface{ Close() error })
	if ok {
		return closer.Close()
	}

	// Otherwise read the SDK's native handle and free it. Refuse if the SDK's layout has changed.
	field = reflect.ValueOf(remote).Elem().FieldByName("db")
	if !field.IsValid() || field.Type() != reflect.TypeFor[turso.TursoSyncDatabase]() {
		return errors.New("activeso: cannot release the sync database: unsupported tursogo version")
	}
	defer func() {
		// Initialize Variables
		recovered := recover()

		if recovered != nil {
			err = fmt.Errorf("activeso: release sync database: %v", recovered)
		}
	}()
	syncDatabaseDeinit(*(*turso.TursoSyncDatabase)(unsafe.Pointer(field.UnsafeAddr())))

	return nil
}

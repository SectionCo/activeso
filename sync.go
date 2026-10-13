package activeso

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	turso "turso.tech/database/tursogo"
)

// ErrSyncClosed is returned by Sync methods after Close has been called.
var ErrSyncClosed = errors.New("activeso: sync handle is closed")

// ErrSyncBusy is returned by Close while connections from DB() are still checked out. Release them,
// then call Close again to finish releasing the database files.
var ErrSyncBusy = errors.New("activeso: sync handle has connections in use")

// SyncConfig describes the local database file and the Turso Cloud database it syncs with.
type SyncConfig struct {
	// Path is the local database file that models read and write.
	Path string
	// RemoteURL is the Turso Cloud database URL, for example "libsql://my-db-org.turso.io".
	RemoteURL string
	// AuthToken authenticates against the remote. Pass the bare token, without a "Bearer " prefix.
	AuthToken string
	// Namespace selects a remote namespace on multi-tenant deployments. Optional.
	Namespace string
	// ClientName identifies this client to the remote. Defaults to the SDK's name.
	ClientName string
	// SkipBootstrap leaves an empty local database empty on open instead of downloading the remote
	// state first; call Pull to fetch it later.
	SkipBootstrap bool
	// BusyTimeoutMs is how long a connection waits on a locked database. Zero uses the SDK default.
	BusyTimeoutMs int
}

// SyncStats reports how much local work is waiting to be pushed, along with sync timing and traffic.
type SyncStats = turso.TursoSyncDbStats

// Sync is a local Turso database synchronized with Turso Cloud. Sync is database-wide, so Pull and
// Push live here rather than on models.
type Sync struct {
	remote *turso.TursoSyncDb
	db     *sql.DB
	mu     sync.RWMutex
	closed bool
	// released is set once the SDK's native handle and its files have been freed.
	released bool
}

// OpenSync opens the local database at config.Path, bootstraps it from the remote when it is empty,
// and returns a handle whose DB method feeds Model.
func OpenSync(ctx context.Context, config SyncConfig) (*Sync, error) {
	// Initialize Variables
	bootstrap := !config.SkipBootstrap
	var remote *turso.TursoSyncDb
	var db *sql.DB
	var err error

	// Every ActiveSo database syncs with Turso Cloud, so both locations are required.
	if strings.TrimSpace(config.Path) == "" {
		return nil, errors.New("activeso: sync config requires a Path")
	}
	if strings.TrimSpace(config.RemoteURL) == "" {
		return nil, errors.New("activeso: sync config requires a RemoteURL")
	}

	// Open the synced database and its database/sql handle.
	remote, err = turso.NewTursoSyncDb(ctx, turso.TursoSyncDbConfig{
		Path:             config.Path,
		RemoteUrl:        config.RemoteURL,
		AuthToken:        config.AuthToken,
		Namespace:        config.Namespace,
		ClientName:       config.ClientName,
		BootstrapIfEmpty: &bootstrap,
		BusyTimeout:      config.BusyTimeoutMs,
	})
	if err != nil {
		return nil, fmt.Errorf("activeso: open sync database: %w", err)
	}
	db, err = remote.Connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("activeso: connect sync database: %w", err)
	}

	return &Sync{remote: remote, db: db}, nil
}

// DB returns the database/sql handle for Model. Direct SQL on it is not foreign-key enforced unless
// the caller runs PRAGMA foreign_keys = ON on the connection it uses.
func (handle *Sync) DB() *sql.DB {
	// Initialize Variables
	db := handle.db

	return db
}

// Pull fetches remote changes and rebases local changes on top of them. It sends nothing. It reports
// whether new remote changes were applied locally.
func (handle *Sync) Pull(ctx context.Context) (bool, error) {
	// Hold the read lock so Close cannot release the database mid-operation.
	handle.mu.RLock()
	defer handle.mu.RUnlock()
	if handle.closed {
		return false, ErrSyncClosed
	}

	return handle.pull(ctx)
}

// Push sends local changes to the remote. It fetches nothing.
func (handle *Sync) Push(ctx context.Context) error {
	// Hold the read lock so Close cannot release the database mid-operation.
	handle.mu.RLock()
	defer handle.mu.RUnlock()
	if handle.closed {
		return ErrSyncClosed
	}

	return handle.push(ctx)
}

// Sync pulls remote changes and then pushes local ones, so local writes rebase onto the latest
// remote state before they are published. Close waits for the whole sequence, not just one half.
func (handle *Sync) Sync(ctx context.Context) error {
	// Initialize Variables
	var err error

	// Hold one read lock across both halves so Close cannot land between them.
	handle.mu.RLock()
	defer handle.mu.RUnlock()
	if handle.closed {
		return ErrSyncClosed
	}

	// Pull first so local writes rebase onto the latest remote state, then publish them.
	_, err = handle.pull(ctx)
	if err != nil {
		return err
	}

	return handle.push(ctx)
}

// Checkpoint compacts the local write-ahead log after many writes so a long-lived replica does not
// keep growing on disk.
func (handle *Sync) Checkpoint(ctx context.Context) error {
	// Initialize Variables
	var err error

	// Hold the read lock so Close cannot release the database mid-operation.
	handle.mu.RLock()
	defer handle.mu.RUnlock()
	if handle.closed {
		return ErrSyncClosed
	}

	err = handle.remote.Checkpoint(ctx)
	if err != nil {
		return fmt.Errorf("activeso: checkpoint: %w", err)
	}

	return nil
}

// pull runs a pull without taking the close lock; callers must hold it.
func (handle *Sync) pull(ctx context.Context) (bool, error) {
	// Initialize Variables
	changed, err := handle.remote.Pull(ctx)

	if err != nil {
		return false, fmt.Errorf("activeso: pull: %w", err)
	}

	return changed, nil
}

// push runs a push without taking the close lock; callers must hold it.
func (handle *Sync) push(ctx context.Context) error {
	// Initialize Variables
	err := handle.remote.Push(ctx)

	if err != nil {
		return fmt.Errorf("activeso: push: %w", err)
	}

	return nil
}

// Stats reports pending local operations, last pull and push times, and network traffic.
func (handle *Sync) Stats(ctx context.Context) (SyncStats, error) {
	// Initialize Variables
	var stats SyncStats
	var err error

	// Hold the read lock so Close cannot release the database mid-operation.
	handle.mu.RLock()
	defer handle.mu.RUnlock()
	if handle.closed {
		return stats, ErrSyncClosed
	}

	stats, err = handle.remote.Stats(ctx)
	if err != nil {
		return stats, fmt.Errorf("activeso: sync stats: %w", err)
	}

	return stats, nil
}

// Close stops sync calls, closes every pooled connection, and releases the database, WAL, and changes
// files. Later Pull, Push, and Stats calls return ErrSyncClosed. Writes that were never pushed stay in
// the local file. Connections obtained from DB() and still checked out keep working until they are
// returned, so return them first: while any remain, Close returns ErrSyncBusy without releasing the
// files, and calling Close again after they are returned finishes the release. Calling Close after a
// full release is a no-op.
func (handle *Sync) Close() error {
	// Initialize Variables
	var err error

	// Take the write lock so Close waits for in-flight sync operations to finish.
	handle.mu.Lock()
	defer handle.mu.Unlock()
	if handle.released {
		return nil
	}

	// Close the pool once; later calls only retry the release.
	if !handle.closed {
		handle.closed = true
		err = handle.db.Close()
		if err != nil {
			return fmt.Errorf("activeso: close sync database: %w", err)
		}
	}

	// Freeing the native handle while a connection still uses it would crash, so wait for them all.
	if open := handle.db.Stats().OpenConnections; open > 0 {
		return fmt.Errorf("%w: %d still open", ErrSyncBusy, open)
	}
	err = releaseRemote(handle.remote)
	if err != nil {
		return err
	}
	handle.released = true

	return nil
}

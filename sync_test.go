package activeso

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// openLocalSync opens a Sync handle whose remote refuses every request, so tests exercise the local
// database and the error paths without a Turso Cloud account.
func openLocalSync(t *testing.T) *Sync {
	// Initialize Variables
	ctx := context.Background()
	remote := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Initialize Variables
		status := http.StatusServiceUnavailable

		http.Error(writer, "remote unavailable", status)
	}))
	handle, err := OpenSync(ctx, SyncConfig{
		Path:          filepath.Join(t.TempDir(), "activeso.db"),
		RemoteURL:     remote.URL,
		AuthToken:     "test-token",
		SkipBootstrap: true,
	})

	t.Helper()
	t.Cleanup(remote.Close)
	if err != nil {
		t.Fatalf("OpenSync() error = %v", err)
	}
	t.Cleanup(func() {
		// Initialize Variables
		closeError := handle.Close()

		if closeError != nil {
			t.Errorf("Close() error = %v", closeError)
		}
	})

	return handle
}

// TestOpenSyncRequiresPathAndRemote rejects configs that omit either location.
func TestOpenSyncRequiresPathAndRemote(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()

	if _, err := OpenSync(ctx, SyncConfig{RemoteURL: "libsql://example.turso.io"}); err == nil {
		t.Fatal("OpenSync() accepted a config without Path")
	}
	if _, err := OpenSync(ctx, SyncConfig{Path: filepath.Join(t.TempDir(), "activeso.db")}); err == nil {
		t.Fatal("OpenSync() accepted a config without RemoteURL")
	}
}

// TestSyncModelLifecycle runs Create, Find, Save, and Delete on the synced handle.
func TestSyncModelLifecycle(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	handle := openLocalSync(t)
	userModel := Model[User](handle.DB())

	execStatements(t, handle.DB(), usersSchema)
	if err := userModel.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	user, err := userModel.Create(ctx, User{Email: "hello@null.live", Embedding: Vector32{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	user.Email = "updated@null.live"
	if err := user.Save(ctx); err != nil {
		t.Fatal(err)
	}
	found, err := userModel.Find(ctx, user.ID)
	if err != nil || found.Email != "updated@null.live" {
		t.Fatalf("Find() = %+v; error = %v", found, err)
	}
	if err := found.Delete(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestSyncStatsCountsLocalWrites reports unpushed local operations after a model write.
func TestSyncStatsCountsLocalWrites(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	handle := openLocalSync(t)
	userModel := Model[User](handle.DB())

	execStatements(t, handle.DB(), usersSchema)
	before, err := handle.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := userModel.Create(ctx, User{Email: "stats@null.live", Embedding: Vector32{1, 2}}); err != nil {
		t.Fatal(err)
	}
	after, err := handle.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.CdcOperations <= before.CdcOperations {
		t.Fatalf("CdcOperations = %d after a write, want more than %d", after.CdcOperations, before.CdcOperations)
	}
}

// TestSyncPullAndPushReportRemoteFailure surfaces remote errors and leaves local writes intact.
func TestSyncPullAndPushReportRemoteFailure(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	handle := openLocalSync(t)
	userModel := Model[User](handle.DB())

	execStatements(t, handle.DB(), usersSchema)
	user, err := userModel.Create(ctx, User{Email: "offline@null.live", Embedding: Vector32{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Pull(ctx); err == nil {
		t.Fatal("Pull() succeeded against an unavailable remote")
	}
	if err := handle.Push(ctx); err == nil {
		t.Fatal("Push() succeeded against an unavailable remote")
	}
	if err := handle.Sync(ctx); err == nil {
		t.Fatal("Sync() succeeded against an unavailable remote")
	}
	if _, err := userModel.Find(ctx, user.ID); err != nil {
		t.Fatalf("local record lost after failed sync: %v", err)
	}
}

// TestSyncModelWritesEnforceForeignKeysAcrossPool rejects orphaning writes on every pooled connection of the synced handle.
func TestSyncModelWritesEnforceForeignKeysAcrossPool(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	handle := openLocalSync(t)
	regionModel := Model[Region](handle.DB())
	cityModel := Model[City](handle.DB())

	execStatements(t, handle.DB(), `CREATE TABLE regions (id TEXT PRIMARY KEY, name TEXT);
CREATE TABLE cities (id TEXT PRIMARY KEY, name TEXT NOT NULL, region_id TEXT REFERENCES regions (id))`)
	if _, err := regionModel.Create(ctx, Region{ID: "oregon", Name: "Oregon"}); err != nil {
		t.Fatal(err)
	}
	holdPooledConnections(t, ctx, handle.DB(), 4)

	// Repeat so writes are serviced by different pooled connections.
	for index := range 8 {
		id := fmt.Sprintf("city-%d", index)
		city, err := cityModel.Create(ctx, City{ID: id, Name: id, RegionID: "oregon"})

		if err != nil {
			t.Fatal(err)
		}
		if _, err := cityModel.Create(ctx, City{ID: "orphan-" + id, Name: id, RegionID: "missing"}); err == nil {
			t.Fatalf("Create() accepted a missing belongs_to target on attempt %d", index)
		}
		city.RegionID = "missing"
		if err := city.Save(ctx); err == nil {
			t.Fatalf("Save() accepted a missing belongs_to target on attempt %d", index)
		}
	}
}

// TestSyncCloseReleasesHandle stops later sync calls and database use, and tolerates a second Close.
func TestSyncCloseReleasesHandle(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	handle := openLocalSync(t)

	if err := handle.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if _, err := handle.Pull(ctx); !errors.Is(err, ErrSyncClosed) {
		t.Fatalf("Pull() after Close error = %v, want ErrSyncClosed", err)
	}
	if err := handle.Push(ctx); !errors.Is(err, ErrSyncClosed) {
		t.Fatalf("Push() after Close error = %v, want ErrSyncClosed", err)
	}
	if _, err := handle.Stats(ctx); !errors.Is(err, ErrSyncClosed) {
		t.Fatalf("Stats() after Close error = %v, want ErrSyncClosed", err)
	}
	if err := handle.DB().PingContext(ctx); err == nil {
		t.Fatal("DB() still usable after Close")
	}
}

// openFilesUnder lists this process's open files inside dir, skipping the test when the platform offers no way to list them.
func openFilesUnder(t *testing.T, dir string) []string {
	// Initialize Variables
	var open []string
	var output []byte
	var err error

	t.Helper()

	// Linux exposes descriptors under /proc; other systems fall back to lsof.
	if entries, readError := os.ReadDir("/proc/self/fd"); readError == nil {
		for _, entry := range entries {
			target, linkError := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
			if linkError == nil && strings.HasPrefix(target, dir) {
				open = append(open, target)
			}
		}
		return open
	}
	output, err = exec.Command("lsof", "-p", fmt.Sprint(os.Getpid())).Output()
	if err != nil {
		t.Skipf("cannot list open files on this platform: %v", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		if index := strings.Index(line, dir); index >= 0 {
			open = append(open, line[index:])
		}
	}

	return open
}

// TestSyncCloseReleasesFiles frees the database, WAL, and changes files, and leaves local data readable by a later handle.
func TestSyncCloseReleasesFiles(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	remote := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Initialize Variables
		status := http.StatusServiceUnavailable

		http.Error(writer, "remote unavailable", status)
	}))
	dir, err := filepath.EvalSymlinks(t.TempDir())
	config := SyncConfig{Path: filepath.Join(dir, "activeso.db"), RemoteURL: remote.URL, SkipBootstrap: true}
	var handle *Sync

	t.Cleanup(remote.Close)
	if err != nil {
		t.Fatal(err)
	}
	handle, err = OpenSync(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	execStatements(t, handle.DB(), usersSchema)
	if _, err := Model[User](handle.DB()).Create(ctx, User{Email: "kept@null.live"}); err != nil {
		t.Fatal(err)
	}
	if len(openFilesUnder(t, dir)) == 0 {
		t.Fatal("expected the open sync database to hold files open")
	}

	if err := handle.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if open := openFilesUnder(t, dir); len(open) != 0 {
		t.Fatalf("files still open after Close(): %v", open)
	}

	// A new handle on the same path sees the unpushed row.
	handle, err = OpenSync(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	users, err := Model[User](handle.DB()).Where("email = ?", "kept@null.live").All(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("reopened users = %v; error = %v", users, err)
	}
}

// TestSyncCloseWaitsForBusyConnections refuses to free the database while a connection is checked out, then completes once it returns.
func TestSyncCloseWaitsForBusyConnections(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	handle := openLocalSync(t)
	connection, err := handle.DB().Conn(ctx)

	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); !errors.Is(err, ErrSyncBusy) {
		t.Fatalf("Close() with a connection in use error = %v, want ErrSyncBusy", err)
	}
	if _, err := handle.Pull(ctx); !errors.Is(err, ErrSyncClosed) {
		t.Fatalf("Pull() after a busy Close error = %v, want ErrSyncClosed", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("Close() after releasing the connection error = %v", err)
	}
}

// TestSyncCheckpointCompactsLocalLog runs a checkpoint after local writes and rejects it once the handle is closed.
func TestSyncCheckpointCompactsLocalLog(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	handle := openLocalSync(t)
	userModel := Model[User](handle.DB())

	execStatements(t, handle.DB(), usersSchema)
	if _, err := userModel.Create(ctx, User{Email: "checkpoint@null.live"}); err != nil {
		t.Fatal(err)
	}
	if err := handle.Checkpoint(ctx); err != nil {
		t.Fatalf("Checkpoint() error = %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Checkpoint(ctx); !errors.Is(err, ErrSyncClosed) {
		t.Fatalf("Checkpoint() after Close error = %v, want ErrSyncClosed", err)
	}
}

// TestSyncCloseWaitsForWholeSync keeps Close from landing between the pull and push halves of Sync.
func TestSyncCloseWaitsForWholeSync(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	requests := make(chan struct{}, 16)
	release := make(chan struct{})
	remote := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// Initialize Variables
		status := http.StatusServiceUnavailable

		// Announce the request, then hold the first one so the Sync call stays in flight.
		requests <- struct{}{}
		<-release
		http.Error(writer, "remote unavailable", status)
	}))
	handle, err := OpenSync(ctx, SyncConfig{Path: filepath.Join(t.TempDir(), "activeso.db"), RemoteURL: remote.URL, SkipBootstrap: true})
	syncDone := make(chan error, 1)
	closeDone := make(chan error, 1)

	t.Cleanup(remote.Close)
	if err != nil {
		t.Fatal(err)
	}
	go func() { syncDone <- handle.Sync(ctx) }()
	<-requests

	// Close must block while Sync is in flight.
	go func() { closeDone <- handle.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Close() returned %v while Sync() was still running", err)
	case <-time.After(200 * time.Millisecond):
	}

	// Once Sync finishes, Close completes.
	close(release)
	if err := <-syncDone; err == nil || errors.Is(err, ErrSyncClosed) {
		t.Fatalf("Sync() error = %v, want the remote failure and not ErrSyncClosed", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// TestSyncAgainstTursoCloud pushes and pulls against a real remote when ACTIVESO_SYNC_URL is set.
func TestSyncAgainstTursoCloud(t *testing.T) {
	// Initialize Variables
	ctx := context.Background()
	remoteURL := os.Getenv("ACTIVESO_SYNC_URL")
	token := os.Getenv("ACTIVESO_SYNC_TOKEN")

	if remoteURL == "" {
		t.Skip("set ACTIVESO_SYNC_URL (and ACTIVESO_SYNC_TOKEN) to run against a real Turso Cloud database")
	}
	handle, err := OpenSync(ctx, SyncConfig{Path: filepath.Join(t.TempDir(), "activeso.db"), RemoteURL: remoteURL, AuthToken: token})
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	if _, err := handle.Pull(ctx); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	if err := handle.Push(ctx); err != nil {
		t.Fatalf("Push() error = %v", err)
	}
	if _, err := handle.Stats(ctx); err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
}

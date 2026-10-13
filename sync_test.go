package activeso

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// openLocalSync opens a Sync handle whose remote refuses every request, so tests exercise the local
// database and the error paths without a Turso Cloud account.
func openLocalSync(t *testing.T) *Sync {
	// Initialize Variables
	ctx := context.Background()
	remote := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "remote unavailable", http.StatusServiceUnavailable)
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

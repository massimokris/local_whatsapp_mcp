package engine_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

func TestSyncFreshnessSurvivesRestartAndIgnoresLifecycleEvents(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "store"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "store", "wacli.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE retrieval_state (id INTEGER PRIMARY KEY, last_sync_at INTEGER); INSERT INTO retrieval_state VALUES (1, 1700000100)`); err != nil {
		t.Fatal(err)
	}
	want := time.Unix(1700000100, 0).UTC().Format(time.RFC3339)
	bin := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
printf '%s\n' '{"event":"policy_loaded","data":{"revision":2}}' '{"event":"connected"}' '{"event":"history_sync"}' '{"event":"progress","data":{"messages_synced":25}}' '{"event":"qr_code","data":{"code":"fixture-qr"}}' >&2
exec /bin/cat >/dev/null
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e := engine.New(bin, dir)
	if got := e.Status().LastSyncedAt; got != want {
		t.Fatalf("persisted freshness on startup = %q, want %q", got, want)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 2, Mode: "all"}, false); err != nil {
		t.Fatal(err)
	}
	defer e.Stop(ctx)
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && e.Status().QR == ""; time.Sleep(time.Millisecond) {
	}
	status := e.Status()
	if status.QR != "fixture-qr" || status.LastEventAt == "" {
		t.Fatalf("fixture lifecycle events were not received: %+v", status)
	}
	if status.LastSyncedAt != want {
		t.Fatalf("lifecycle events advanced successful sync: got %q, want %q", status.LastSyncedAt, want)
	}
	if _, err := db.Exec(`UPDATE retrieval_state SET last_sync_at=1700000200 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	want = time.Unix(1700000200, 0).UTC().Format(time.RFC3339)
	if got := e.Status().LastSyncedAt; got != want {
		t.Fatalf("new persisted sync was not observed: got %q, want %q", got, want)
	}
	if got := engine.New(bin, dir).Status().LastSyncedAt; got != want {
		t.Fatalf("restart lost successful sync: got %q, want %q", got, want)
	}
}

func TestSyncFreshnessUnavailableDoesNotCreateOrChangeArchive(t *testing.T) {
	dir := t.TempDir()
	e := engine.New("unused", dir)
	if got := e.Status().LastSyncedAt; got != "" {
		t.Fatalf("missing archive has freshness %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "store")); !os.IsNotExist(err) {
		t.Fatalf("status created an archive directory: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "store"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "store", "wacli.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE retrieval_state (id INTEGER PRIMARY KEY, last_sync_at INTEGER); INSERT INTO retrieval_state VALUES (1, NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{nil, int64(0), int64(-1), "invalid"} {
		if _, err := db.Exec(`UPDATE retrieval_state SET last_sync_at=? WHERE id=1`, value); err != nil {
			t.Fatal(err)
		}
		if got := e.Status().LastSyncedAt; got != "" {
			t.Fatalf("unavailable timestamp %v yielded freshness %q", value, got)
		}
		var saved any
		if err := db.QueryRow(`SELECT last_sync_at FROM retrieval_state WHERE id=1`).Scan(&saved); err != nil {
			t.Fatal(err)
		}
		if saved != value {
			t.Fatalf("status modified stored timestamp: got %v, want %v", saved, value)
		}
	}
}

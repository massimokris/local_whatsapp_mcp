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

func TestCurrentLinkEventsReplaceRestoredSessionTruth(t *testing.T) {
	for _, event := range []string{
		`{"event":"qr_code","data":{"code":"new-synthetic-code"}}`,
		`{"event":"logged_out"}`,
	} {
		t.Run(event, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "wacli")
			script := `#!/bin/sh
case "$*" in
  *"--read-only auth status")
    printf '%s\n' '{"success":true,"data":{"authenticated":true}}'
    exit 0
    ;;
esac
printf '%s\n' '{"event":"policy_loaded","data":{"revision":2}}' '` + event + `' >&2
exec /bin/cat >/dev/null
`
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			e := engine.New(bin, dir)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := e.RestoreSession(ctx); err != nil || !e.Status().Linked {
				t.Fatal("saved session was not restored", err)
			}
			if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 2, Mode: "initial"}, true); err != nil {
				t.Fatal(err)
			}
			defer e.Stop(ctx)
			deadline := time.Now().Add(time.Second)
			for e.Status().Linked && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if status := e.Status(); status.Linked {
				t.Fatal("current QR/logout retained obsolete session truth", status)
			}
		})
	}
}

func TestPinnedEngineRestoresSavedSessionWithoutWriting(t *testing.T) {
	binary := os.Getenv("WACLI_TEST_BINARY")
	if binary == "" {
		t.Skip("set WACLI_TEST_BINARY to exercise saved session restoration")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "content-policy.json"), []byte(`{"version":1,"revision":1,"mode":"initial","chat_ids":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	e := engine.New(binary, dir)
	if err := e.RestoreSession(context.Background()); err != nil || e.Status().Linked {
		t.Fatal("absent session was reported linked", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "store")); !os.IsNotExist(err) {
		t.Fatal("read-only restoration created a store", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "store"), 0700); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(dir, "store", "session.db")
	db, err := sql.Open("sqlite3", sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	// A synthetic JID is sufficient for the upstream read-only status contract.
	// No session keys or WhatsApp network connection are created.
	if _, err := db.Exec(`CREATE TABLE whatsmeow_device (jid TEXT); INSERT INTO whatsmeow_device VALUES ('123:1@s.whatsapp.net')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		e = engine.New(binary, dir)
		if err := e.RestoreSession(context.Background()); err != nil {
			t.Fatal(err)
		}
		if status := e.Status(); !status.Linked || status.Status != "connecting" || status.QR != "" {
			t.Fatal("restart failed to restore saved connection", status)
		}
	}
	after, err := os.ReadFile(sessionPath)
	if err != nil || string(before) != string(after) {
		t.Fatal("restoring saved connection modified the session", err)
	}
	entries, err := os.ReadDir(filepath.Dir(sessionPath))
	if err != nil || len(entries) != 1 || entries[0].Name() != "session.db" {
		t.Fatal("restoration wrote session sidecars or archive files", entries, err)
	}
}

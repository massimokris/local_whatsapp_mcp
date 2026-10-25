package engine_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

func TestLogoutUsesFixedPolicyAndConfirmsSessionInvalidation(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		authStatus string
		logout     string
		wantError  bool
		wantLogout bool
	}{
		{"authenticated", `{"authenticated":true}`, `{"logged_out":true}`, false, true},
		{"already signed out", `{"authenticated":false}`, `{"logged_out":true}`, false, false},
		{"logout refused", `{"authenticated":true}`, `{"logged_out":false}`, true, true},
		{"logout unknown", `{"authenticated":true}`, `{}`, true, true},
		{"session status missing", `{}`, `{"logged_out":true}`, true, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "wacli")
			// Each fake response uses the real command envelope. No account store or
			// network is used, and the fixed policy/store boundary is checked here.
			script := `#!/bin/sh
set -eu
root="$(dirname "$0")"
[ "$1" = --store ] && [ "$2" = "$root/store" ] || exit 81
[ "$3" = --content-policy ] && [ "$4" = "$root/content-policy.json" ] || exit 82
case "$*" in
  *"--read-only auth status")
    printf '%s\n' status >> "$root/calls"
    cat "$root/status.json"
    ;;
  *"auth logout")
    case "$*" in *"--read-only"*) exit 83;; esac
    printf '%s\n' logout >> "$root/calls"
    cat "$root/logout.json"
    ;;
  *) exit 84;;
esac
`
			for name, data := range map[string]string{
				"wacli":       script,
				"status.json": `{"success":true,"data":` + scenario.authStatus + `}`,
				"logout.json": `{"success":true,"data":` + scenario.logout + `}`,
			} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0700); err != nil {
					t.Fatal(err)
				}
			}
			e := engine.New(bin, dir)
			err := e.Logout(context.Background())
			if (err != nil) != scenario.wantError {
				t.Fatal("logout result", err)
			}
			calls, err := os.ReadFile(filepath.Join(dir, "calls"))
			if err != nil || strings.Contains(string(calls), "logout") != scenario.wantLogout {
				t.Fatal("wrong logout dispatch", string(calls), err)
			}
			if !scenario.wantError {
				status := e.Status()
				if status.Linked || status.QR != "" || status.Status != "disconnected" {
					t.Fatal("successful logout retained connection state", status)
				}
			}
		})
	}
}

func TestPinnedEngineLogoutWithoutSessionRetainsArchive(t *testing.T) {
	binary := os.Getenv("WACLI_TEST_BINARY")
	if binary == "" {
		t.Skip("set WACLI_TEST_BINARY to check idempotent logout against the pinned engine")
	}
	dir := t.TempDir()
	policy := `{"version":1,"revision":2,"mode":"selected","chat_ids":["123@s.whatsapp.net"]}`
	if err := os.WriteFile(filepath.Join(dir, "content-policy.json"), []byte(policy), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "store"), 0700); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "store", "wacli.db")
	if err := os.WriteFile(archive, []byte("untouched archive marker"), 0600); err != nil {
		t.Fatal(err)
	}
	e := engine.New(binary, dir)
	for range 2 {
		if err := e.Logout(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if data, err := os.ReadFile(archive); err != nil || string(data) != "untouched archive marker" {
		t.Fatal("idempotent logout modified the archive", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "store", "session.db")); !os.IsNotExist(err) {
		t.Fatal("read-only logout check created a session", err)
	}
}

func TestPinnedEngineOfflineMediaAfterLogout(t *testing.T) {
	binary := os.Getenv("WACLI_TEST_BINARY")
	if binary == "" {
		t.Skip("set WACLI_TEST_BINARY to exercise offline media after logout")
	}
	dir := t.TempDir()
	policy := filepath.Join(dir, "content-policy.json")
	if err := os.WriteFile(policy, []byte(`{"version":1,"revision":2,"mode":"selected","chat_ids":["123@s.whatsapp.net"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "--store", filepath.Join(dir, "store"), "--content-policy", policy, "--json", "auth", "status")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initialize synthetic archive: %s %v", out, err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "store", "wacli.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	data := []byte("retained attachment after logout")
	hash := sha256.Sum256(data)
	cached := filepath.Join(dir, "fixture-cache.txt")
	if err := os.WriteFile(cached, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO chats(jid,kind,name,last_message_ts) VALUES('123@s.whatsapp.net','dm','Fixture',1000)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO messages(chat_jid,msg_id,ts,from_me,media_type,mime_type,file_sha256,file_length,local_path,direct_path) VALUES('123@s.whatsapp.net','cached',1000,0,'document','text/plain',?,?,?,'invalid-unused-location')`, hash[:], len(data), cached); err != nil {
		t.Fatal(err)
	}
	e := engine.New(binary, dir)
	if err := e.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "read-after-logout.txt")
	args := []string{"media", "download", "--chat", "123@s.whatsapp.net", "--id", "cached", "--output", output, "--cache-only"}
	raw, err := e.Call(context.Background(), false, args...)
	if err != nil || !strings.Contains(string(raw), `"source":"cache"`) {
		t.Fatal("offline cached media failed", string(raw), err)
	}
	if got, err := os.ReadFile(output); err != nil || string(got) != string(data) {
		t.Fatal("offline cache returned wrong bytes", err)
	}
	if err := os.Remove(cached); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(output); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Call(context.Background(), false, args...); err == nil || !strings.Contains(err.Error(), "attachment_not_cached") {
		t.Fatal("offline cache miss did not stop cleanly", err)
	}
}

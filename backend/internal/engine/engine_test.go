package engine_test

import (
	"context"
	"database/sql"
	"github.com/massi/localwhatsappmcp/internal/engine"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSupervisorRequiresPolicyAckAndStopsOwnedWriter(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
printf '%s\n' '{"event":"policy_loaded","data":{"revision":2},"ts":1}' >&2
printf '%s\n' '{"event":"qr_code","data":{"code":"fixture-qr"},"ts":1}' >&2
trap 'exit 0' TERM INT
while :; do sleep 1; done
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e := engine.New(bin, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 2, Mode: "initial"}, true); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for e.Status().QR == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if e.Status().QR != "fixture-qr" {
		t.Fatal("QR not forwarded")
	}
	if err := e.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPinnedEngineStructuredBoundary(t *testing.T) {
	binary := os.Getenv("WACLI_TEST_BINARY")
	if binary == "" {
		t.Skip("set WACLI_TEST_BINARY to exercise the built pinned engine")
	}
	dir := t.TempDir()
	policy := []byte(`{"version":1,"revision":1,"mode":"initial","chat_ids":[]}`)
	os.WriteFile(filepath.Join(dir, "content-policy.json"), policy, 0600)
	cmd := exec.Command(binary, "--store", filepath.Join(dir, "store"), "--content-policy", filepath.Join(dir, "content-policy.json"), "--json", "auth", "status")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initialize fixture archive: %s %v", out, err)
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "store", "wacli.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO chats(jid,kind,name,last_message_ts,archived) VALUES('123@s.whatsapp.net','dm','Work',1000,1);INSERT INTO messages(chat_jid,msg_id,ts,from_me,text,display_text) VALUES('123@s.whatsapp.net','fixture-1',1000,0,'fixture hello','fixture hello')`)
	if err != nil {
		t.Fatal(err)
	}
	session, err := sql.Open("sqlite3", filepath.Join(dir, "store", "session.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = session.Exec(`CREATE TABLE IF NOT EXISTS whatsmeow_lid_map (lid TEXT PRIMARY KEY, pn TEXT UNIQUE NOT NULL); INSERT INTO whatsmeow_lid_map VALUES ('999', '123')`); err != nil {
		t.Fatal(err)
	}
	session.Close()
	e := engine.New(binary, dir)
	chats, err := e.Catalog(context.Background())
	if err != nil || len(chats) != 1 || chats[0].Name != "Work" || !chats[0].Archived {
		t.Fatal(chats, err)
	}
	if !reflect.DeepEqual(chats[0].Aliases, []string{"123@s.whatsapp.net", "999@lid"}) {
		t.Fatalf("catalog lacks verified migrated identity: %+v", chats)
	}
	if _, err := db.Exec(`UPDATE chats SET archived=0 WHERE jid='123@s.whatsapp.net'`); err != nil {
		t.Fatal(err)
	}
	chats, err = e.Catalog(context.Background())
	if err != nil || len(chats) != 1 || chats[0].Archived {
		t.Fatalf("catalog did not reflect unarchive: %+v, %v", chats, err)
	}
	ids, err := e.Resolve(context.Background(), "123@s.whatsapp.net")
	if err != nil || len(ids) != 2 {
		t.Fatal(ids, err)
	}
	raw, err := e.Call(context.Background(), false, "messages", "list", "--chat", "123@s.whatsapp.net")
	if err != nil || !strings.Contains(string(raw), "fixture hello") {
		t.Fatal(string(raw), err)
	}
}

func TestLiveProgressReportsSyncingWithoutClaimingHistoryImport(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
printf '%s\n' '{"event":"policy_loaded","data":{"revision":2}}' '{"event":"connected"}' '{"event":"progress"}' >&2
exec /bin/cat >/dev/null
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e := engine.New(bin, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 2, Mode: "all"}, false); err != nil {
		t.Fatal(err)
	}
	defer e.Stop(ctx)
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && e.Status().Status != "syncing"; time.Sleep(time.Millisecond) {
	}
	status := e.Status()
	if status.Status != "syncing" || strings.Contains(status.Detail, "Importing") || status.LastEventAt == "" {
		t.Fatalf("misleading live progress: %+v", status)
	}
}

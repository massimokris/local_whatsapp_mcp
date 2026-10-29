package engine_test

import (
	"context"
	"encoding/json"
	"github.com/massi/localwhatsappmcp/internal/engine"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConnectionStatusIgnoresLegacySyncState(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
printf '%s\n' '{"event":"policy_loaded","data":{"revision":7}}' '{"event":"connected"}' >&2
printf '{"event":"sync_state","data":{"run_id":"%s","connection_id":1,"revision":7,"state":"error"}}\n' "legacy-run" >&2
printf '%s\n' '{"event":"progress","data":{"messages_synced":100}}' >&2
exec /bin/cat >/dev/null
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "store"), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "store", "sync-debts.json")
	want := []byte("legacy unfinished updates must be ignored, even if corrupt")
	if err := os.WriteFile(legacy, want, 0600); err != nil {
		t.Fatal(err)
	}
	e := engine.New(binary, dir)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 7, Mode: "all"}, false); err != nil {
		t.Fatal(err)
	}
	defer e.Stop(ctx)
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && !e.Status().Linked; time.Sleep(time.Millisecond) {
	}
	// Stop drains the owned event pipe, ensuring the deprecated sync event arrived.
	if err := e.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(e.Status())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sync_state") || strings.Contains(string(raw), "last_synced_at") || strings.Contains(string(raw), "messages could not be saved") {
		t.Fatalf("custom completion state remains: %s", raw)
	}
	got, err := os.ReadFile(legacy)
	if err != nil || string(got) != string(want) {
		t.Fatal("legacy tracking data was modified", err)
	}
}

func TestConnectionStatusFollowsUpstreamLifecycleWithoutCompletion(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
base=$(dirname "$0")
wait_stage(){ while [ ! -f "$base/$1" ]; do sleep 0.01; done; }
emit(){ printf '{"event":"%s"}\n' "$1" >&2; }
printf '%s\n' '{"event":"policy_loaded","data":{"revision":1}}' >&2
printf '%s\n' '{"event":"qr_code","data":{"code":"SYNTHETIC-QR-A"}}' >&2
emit paired
wait_stage interrupted
emit disconnected
wait_stage fresh_qr
printf '%s\n' '{"event":"qr_code","data":{"code":"SYNTHETIC-QR-B"}}' >&2
wait_stage paired_again
emit paired
wait_stage connect
emit connected
wait_stage disconnect
emit disconnected
wait_stage reconnect
emit connected
wait_stage logout
emit logged_out
emit connected
exec /bin/cat >/dev/null
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e := engine.New(binary, dir)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	defer e.Stop(ctx)
	if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 1, Mode: "all"}, false); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && e.Status().Status != "logging_in"; time.Sleep(time.Millisecond) {
	}
	if state := e.Status(); state.Linked || state.Status != "logging_in" || state.QR != "SYNTHETIC-QR-A" {
		t.Fatalf("accepted pairing must retain the QR without confirming connection: %+v", state)
	}
	for _, phase := range []struct {
		trigger, status string
		linked          bool
		qr              string
	}{
		{"interrupted", "connecting", false, ""},
		{"fresh_qr", "linking", false, "SYNTHETIC-QR-B"},
		{"paired_again", "logging_in", false, "SYNTHETIC-QR-B"},
		{"connect", "ready", true, ""},
		{"disconnect", "connecting", true, ""},
		{"reconnect", "ready", true, ""},
		{"logout", "attention", false, ""},
	} {
		if err := os.WriteFile(filepath.Join(dir, phase.trigger), nil, 0600); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && e.Status().Status != phase.status; time.Sleep(time.Millisecond) {
		}
		if state := e.Status(); state.Status != phase.status || state.Linked != phase.linked || state.QR != phase.qr {
			t.Fatalf("%s: %+v", phase.trigger, state)
		}
	}
}

func TestProcessRestartDoesNotInventAConnectedSession(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
base=$(dirname "$0")
printf '%s\n' '{"event":"policy_loaded","data":{"revision":1}}' >&2
if [ ! -f "$base/first" ]; then
 touch "$base/first"
 printf '%s\n' '{"event":"connected"}' >&2
 while [ ! -f "$base/exit" ]; do sleep 0.01; done
 exit 1
fi
touch "$base/restarted"
exec /bin/cat >/dev/null
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e := engine.New(binary, dir)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	defer e.Stop(ctx)
	if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 1, Mode: "all"}, false); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && e.Status().Status != "ready"; time.Sleep(time.Millisecond) {
	}
	if e.Status().Status != "ready" {
		t.Fatal("initial connection not observed")
	}
	if err := os.WriteFile(filepath.Join(dir, "exit"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	restarted := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if _, err := os.Stat(filepath.Join(dir, "restarted")); err == nil {
			restarted = true
			break
		}
	}
	if !restarted {
		t.Fatal("supervisor did not restart failed child")
	}
	if state := e.Status(); state.Status == "ready" {
		t.Fatalf("new process inherited old readiness: %+v", state)
	}
}

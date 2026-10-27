package engine_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

func TestExplicitCompletionIsRequiredEvenWithRecentArchiveTimestamp(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
printf '%s\n' '{"event":"policy_loaded","data":{"revision":7}}' '{"event":"connected"}' >&2
printf '{"event":"sync_state","data":{"run_id":"%s","connection_id":1,"revision":7,"state":"syncing"}}\n' "$WACLI_SYNC_RUN_ID" >&2
exec /bin/cat >/dev/null
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e := engine.New(binary, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 7, Mode: "all"}, false); err != nil {
		t.Fatal(err)
	}
	defer e.Stop(ctx)
	var status map[string]any
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		raw, _ := json.Marshal(e.Status())
		_ = json.Unmarshal(raw, &status)
		if status["sync_state"] == "syncing" {
			return
		}
	}
	t.Fatalf("received work must explicitly report syncing without completion evidence: %+v", status)
}

func TestSupervisorRequiresFreshCompletionAfterProcessLossAndRestart(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
base=$(dirname "$0")
printf '%s\n' '{"event":"policy_loaded","data":{"revision":7}}' >&2
printf '{"event":"connected","data":{"run_id":"%s","connection_id":1,"revision":7}}\n' "$WACLI_SYNC_RUN_ID" >&2
printf '{"event":"sync_state","data":{"run_id":"%s","connection_id":1,"revision":7,"state":"syncing"}}\n' "$WACLI_SYNC_RUN_ID" >&2
while [ ! -e "$base/complete" ]; do sleep 0.01; done
printf '{"event":"sync_state","data":{"run_id":"%s","connection_id":1,"revision":7,"state":"caught_up"}}\n' "$WACLI_SYNC_RUN_ID" >&2
while [ ! -e "$base/exit" ]; do sleep 0.01; done
exit 1
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e := engine.New(binary, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 7, Mode: "all"}, false); err != nil {
		t.Fatal(err)
	}
	defer e.Stop(ctx)
	wait := func(label string, predicate func(engine.Status) bool) {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			if predicate(e.Status()) {
				return
			}
		}
		t.Fatalf("did not reach %s; status=%+v", label, e.Status())
	}
	wait("initial pending work", func(s engine.Status) bool { return s.SyncState == engine.Syncing })
	if err := os.WriteFile(filepath.Join(dir, "complete"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	wait("explicit completion", func(s engine.Status) bool { return s.SyncState == engine.SyncCaughtUp })
	if err := os.Remove(filepath.Join(dir, "complete")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "exit"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	wait("lost engine", func(s engine.Status) bool { return s.Status == "attention" && s.SyncState == engine.SyncUnknown })
	wait("new process pending boundary", func(s engine.Status) bool { return s.SyncState == engine.Syncing })
	if err := e.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if status := e.Status(); status.SyncState == engine.SyncCaughtUp {
		t.Fatal("stopped engine retained completion")
	}
}

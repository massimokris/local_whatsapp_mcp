package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func waitHistoryIdle(t *testing.T, h *lifecycleHistory) {
	t.Helper()
	h.mu.Lock()
	idle, writing := h.idle, h.writing
	h.mu.Unlock()
	if !writing {
		return
	}
	select {
	case <-idle:
	case <-time.After(time.Second):
		t.Fatal("lifecycle history writer did not finish")
	}
}
func readHistory(t *testing.T, dir string) []lifecycleEntry {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, "lifecycle-history.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "PRIVATE_SENTINEL") {
		t.Fatal("history retained private input")
	}
	var entries []lifecycleEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}
func TestLifecycleHistoryBoundsAndRevalidatesSavedEntries(t *testing.T) {
	dir := t.TempDir()
	seed := `[{"sequence":1,"at":"2026-10-28T12:00:00Z","event":"qr_available","code":"PRIVATE_SENTINEL_QR","mode":"PRIVATE_SENTINEL_MODE"},{"sequence":2,"at":"2026-10-28T12:00:01Z","event":"PRIVATE_SENTINEL_EVENT"},{"sequence":3,"at":"2026-10-28T12:00:02Z","event":"terminal_logout","logout_code":123456,"on_connect":true,"reason":"PRIVATE_SENTINEL_REASON"}]`
	if err := os.WriteFile(filepath.Join(dir, "lifecycle-history.json"), []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}
	h := newLifecycleHistory(dir)
	h.record(lifecycleEntry{Event: "process_start", Mode: "auth"})
	waitHistoryIdle(t, h)
	entries := readHistory(t, dir)
	if len(entries) != 3 || entries[0].Sequence != 1 || entries[1].Sequence != 3 || entries[1].LogoutCode != 0 || entries[2].Sequence != 4 {
		t.Fatalf("saved history was not revalidated: %+v", entries)
	}
	for i := 0; i < 1024; i++ {
		h.record(lifecycleEntry{Event: "paired", Mode: "PRIVATE_SENTINEL_MODE", Exit: "PRIVATE_SENTINEL_ERROR"})
	}
	waitHistoryIdle(t, h)
	entries = readHistory(t, dir)
	if len(entries) != lifecycleHistoryLimit {
		t.Fatalf("history length=%d", len(entries))
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].Sequence <= entries[i-1].Sequence {
			t.Fatal("history order regressed")
		}
	}
	last := entries[len(entries)-1].Sequence
	resumed := newLifecycleHistory(dir)
	resumed.record(lifecycleEntry{Event: "process_start", Mode: "sync"})
	waitHistoryIdle(t, resumed)
	entries = readHistory(t, dir)
	if len(entries) != lifecycleHistoryLimit || entries[len(entries)-1].Sequence != last+1 || entries[len(entries)-1].Mode != "sync" {
		t.Fatal("history did not survive restart")
	}
}
func TestLifecycleHistoryRejectsUnsafeSavedFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "fifo", "public", "oversized", "malformed"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "lifecycle-history.json")
			victim := filepath.Join(t.TempDir(), "private-file")
			sentinel := []byte("PRIVATE_SENTINEL_TARGET")
			if err := os.WriteFile(victim, sentinel, 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "symlink":
				err = os.Symlink(victim, path)
			case "fifo":
				err = syscall.Mkfifo(path, 0600)
			case "public":
				err = os.WriteFile(path, []byte(`[{"sequence":1,"at":"2026-10-28T00:00:00Z","event":"paired"}]`), 0644)
			case "oversized":
				err = os.WriteFile(path, []byte(strings.Repeat("PRIVATE_SENTINEL", lifecycleHistoryByteLimit)), 0600)
			case "malformed":
				err = os.WriteFile(path, []byte("PRIVATE_SENTINEL_INVALID"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			h := newLifecycleHistory(dir)
			h.record(lifecycleEntry{Event: "paired"})
			waitHistoryIdle(t, h)
			entries := readHistory(t, dir)
			if len(entries) != 1 || entries[0].Sequence != 1 {
				t.Fatalf("unsafe history loaded: %+v", entries)
			}
			got, err := os.ReadFile(victim)
			if err != nil || string(got) != string(sentinel) {
				t.Fatal("symlink target changed")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
				t.Fatal("replacement history is not a private regular file")
			}
		})
	}
}
func TestLifecycleHistoryWriteFailureDoesNotChangeConnection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lifecycle-history.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
printf '%s\n' '{"event":"policy_loaded","data":{"revision":1}}' '{"event":"connected"}' '{"event":"logged_out","data":{"reason_code":401}}' >&2
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e := New(bin, dir)
	t.Cleanup(func() { _ = e.Stop(context.Background()); waitHistoryIdle(t, e.history) })
	for _, recover := range []bool{false, true} {
		if recover {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
		if err := e.Start(t.Context(), Policy{Version: 1, Revision: 1, Mode: "initial"}, true); err != nil {
			t.Fatal(err)
		}
		e.mu.Lock()
		done := e.done
		e.mu.Unlock()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("diagnostic failure held terminal process")
		}
		if err := e.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
		if state := e.Status(); state.Linked || state.Status != "attention" || !strings.Contains(state.Detail, "disconnected this linked device") {
			t.Fatal("diagnostic failure changed logout outcome")
		}
		if recover && len(readHistory(t, dir)) == 0 {
			t.Fatal("diagnostics did not recover")
		}
	}
}
func TestLifecycleHistoryStalledDiskCannotHoldStopOrRestart(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "wacli")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' '{\"event\":\"policy_loaded\",\"data\":{\"revision\":1}}' >&2\nexec /bin/cat >/dev/null\n"), 0700); err != nil {
		t.Fatal(err)
	}
	e := New(bin, dir)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	e.history.write = func(string, []byte, os.FileMode) error { once.Do(func() { close(entered) }); <-release; return nil }
	defer func() { close(release); waitHistoryIdle(t, e.history) }()
	for i := 0; i < 2; i++ {
		if err := e.Start(t.Context(), Policy{Version: 1, Revision: 1, Mode: "all"}, false); err != nil {
			t.Fatal(err)
		}
		<-entered
		stopped := make(chan error, 1)
		go func() { stopped <- e.Stop(context.Background()) }()
		select {
		case err := <-stopped:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("stalled diagnostic disk held Stop")
		}
		if e.Status().Status != "disconnected" {
			t.Fatal("diagnostics changed stopped state")
		}
	}
}

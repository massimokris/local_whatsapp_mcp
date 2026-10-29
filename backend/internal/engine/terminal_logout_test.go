package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTerminalLogoutDoesNotRestartInitialCapture(t *testing.T) {
	for _, terminal := range []string{`{"event":"logged_out"}`, `{"event":"stopping","data":{"reason":"logged_out"}}`} {
		t.Run(terminal, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "wacli")
			script := `#!/bin/sh
root=$(dirname "$0")
printf '%s\n' started >> "$root/starts"
printf '%s\n' '{"event":"policy_loaded","data":{"revision":1}}' '{"event":"connected"}' '` + terminal + `' '{"event":"connected"}' >&2
`
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			e := New(binary, dir)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			defer e.Stop(ctx)
			if err := e.Start(ctx, Policy{Version: 1, Revision: 1, Mode: "initial"}, true); err != nil {
				t.Fatal(err)
			}
			e.mu.Lock()
			done := e.done
			e.mu.Unlock()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("terminal logout restarted pairing")
			}
			state := e.Status()
			if state.Status != "attention" || state.Linked || !strings.Contains(state.Detail, "disconnected this linked device") {
				t.Fatalf("terminal logout hidden: %+v", state)
			}
			starts, err := os.ReadFile(filepath.Join(dir, "starts"))
			if err != nil || strings.Count(string(starts), "started\n") != 1 {
				t.Fatal("logout started another pairing process", err)
			}
		})
	}
}

func TestConnectionEventsKeepLogoutTerminalForOwnedProcess(t *testing.T) {
	e := New("unused", t.TempDir())
	now := time.Now()
	e.observeEvent(engineEvent{Event: "connected"}, now)
	e.observeEvent(engineEvent{Event: "disconnected"}, now)
	if state := e.Status(); state.Status != "connecting" || !state.Linked {
		t.Fatalf("disconnect lost saved link: %+v", state)
	}
	e.observeEvent(engineEvent{Event: "connected"}, now)
	if e.Status().Status != "ready" {
		t.Fatal("upstream reconnect not reflected")
	}
	e.observeEvent(engineEvent{Event: "logged_out"}, now)
	for _, name := range []string{"connected", "progress", "sync_state", "qr_code"} {
		e.observeEvent(engineEvent{Event: name}, now)
	}
	if state := e.Status(); state.Status != "attention" || state.Linked {
		t.Fatalf("late event revived logout: %+v", state)
	}
}

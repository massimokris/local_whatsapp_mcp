package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

// These codes are synthetic markers, never real WhatsApp pairing data.
func TestQRRotationClearsExpiredCodeBeforeAutomaticRestart(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
base=$(dirname "$0")
printf '%s\n' '{"event":"policy_loaded","data":{"revision":2}}' >&2
if [ ! -e "$base/started" ]; then
  touch "$base/started"
  printf '%s\n' '{"event":"qr_code","data":{"code":"fixture-first"}}' >&2
  while [ ! -e "$base/rotate" ]; do sleep 0.01; done
  printf '%s\n' '{"event":"qr_code","data":{"code":"fixture-rotated"}}' >&2
  while [ ! -e "$base/expire" ]; do sleep 0.01; done
  exit 1
fi
while [ ! -e "$base/release" ]; do sleep 0.01; done
printf '%s\n' '{"event":"qr_code","data":{"code":"fixture-renewed"}}' >&2
exec /bin/cat >/dev/null
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e := engine.New(bin, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	policy := engine.Policy{Version: 1, Revision: 2, Mode: "initial"}
	if err := e.Start(ctx, policy, true); err != nil {
		t.Fatal(err)
	}
	waitFor := func(label string, predicate func(engine.Status) bool) {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
			if predicate(e.Status()) {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("did not reach %s", label)
	}
	trigger := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	waitFor("first code", func(s engine.Status) bool { return s.Status == "linking" && s.QR == "fixture-first" })
	trigger("rotate")
	waitFor("automatic rotation", func(s engine.Status) bool { return s.Status == "linking" && s.QR == "fixture-rotated" })
	trigger("expire")
	waitFor("expired code removed", func(s engine.Status) bool { return s.Status == "attention" && s.QR == "" })
	trigger("release")
	waitFor("fresh code after restart", func(s engine.Status) bool { return s.Status == "linking" && s.QR == "fixture-renewed" })

	// The same Stop/Start sequence used by the refresh endpoint must await the
	// old process and remove its code before beginning another pairing attempt.
	if err := e.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if status := e.Status(); status.Status != "disconnected" || status.QR != "" {
		t.Fatal("stopped pairing attempt retained its code")
	}
	if err := os.Remove(filepath.Join(dir, "release")); err != nil {
		t.Fatal(err)
	}
	if err := e.Start(ctx, policy, true); err != nil {
		t.Fatal(err)
	}
	if status := e.Status(); status.Status != "connecting" || status.QR != "" {
		t.Fatal("replacement attempt exposed a stale code")
	}
	trigger("release")
	waitFor("manual restart code", func(s engine.Status) bool { return s.Status == "linking" && s.QR == "fixture-renewed" })
}

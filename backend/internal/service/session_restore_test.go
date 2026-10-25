package service_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/massi/localwhatsappmcp/internal/engine"
	"github.com/massi/localwhatsappmcp/internal/service"
)

func TestStartupRestoresVerifiedSavedSessionBeforeFirstState(t *testing.T) {
	for _, scenario := range []struct {
		name, response string
		linked, fails  bool
	}{
		{"linked offline account", `{"success":true,"data":{"authenticated":true}}`, true, false},
		{"abandoned QR attempt", `{"success":true,"data":{"authenticated":false}}`, false, false},
		{"unreadable saved session", `{"success":false,"error":"fixture read failure"}`, false, true},
		{"missing authentication field", `{"success":true,"data":{}}`, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dir := t.TempDir()
			opt := service.Options{DataDir: dir, CursorKey: []byte("01234567890123456789012345678901")}
			s, err := service.New(opt)
			if err != nil {
				t.Fatal(err)
			}
			// Persist the real link-request path. This also proves that link intent
			// from an unscanned QR must not be treated as a completed connection.
			if err = s.Resume(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			s.Close()
			bin := filepath.Join(dir, "wacli")
			script := `#!/bin/sh
set -eu
root="$(dirname "$0")"
[ "$1" = --store ] && [ "$2" = "$root/store" ] || exit 81
[ "$3" = --content-policy ] && [ "$4" = "$root/content-policy.json" ] || exit 82
[ -f "$4" ] || exit 83
case "$*" in
  *"--read-only auth status")
    printf '%s\n' status >> "$root/calls"
    cat "$root/status.json"
    ;;
  *) exit 84;;
esac
`
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "status.json"), []byte(scenario.response), 0600); err != nil {
				t.Fatal(err)
			}
			opt.Engine = engine.New(bin, dir)
			s, err = service.New(opt)
			if scenario.fails {
				if err == nil {
					s.Close()
					t.Fatal("unknown saved session was reported as disconnected")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			status := s.State()["whatsapp"].(engine.Status)
			if status.Linked != scenario.linked || (scenario.linked && status.Status != "connecting") || status.QR != "" {
				t.Fatalf("first state did not restore saved session truth: %+v", status)
			}
			if scope := s.State()["scope"].(engine.Policy); scope.Mode != "initial" {
				t.Fatal("restoring a session confirmed chat scope", scope)
			}
			if calls, err := os.ReadFile(filepath.Join(dir, "calls")); err != nil || string(calls) != "status\n" {
				t.Fatal("startup did not use one read-only status check", string(calls), err)
			}
			if _, err := os.Stat(filepath.Join(dir, "store")); !os.IsNotExist(err) {
				t.Fatal("read-only startup created a WhatsApp store", err)
			}
		})
	}
}

func TestStartupDoesNotInspectOrResumeIntentionallyDisconnectedSession(t *testing.T) {
	s, _, opt, _ := disconnectFixture(t)
	if err := s.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Close()
	// An invalid binary makes any attempted session inspection fail. A completed
	// intentional disconnect must continue to start without invoking it.
	opt.Engine = engine.New(filepath.Join(opt.DataDir, "must-not-run"), opt.DataDir)
	s, err := service.New(opt)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Resume(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	status := s.State()["whatsapp"].(engine.Status)
	if status.Linked || status.Status != "disconnected" || strings.Contains(status.Detail, "Reconnecting") {
		t.Fatal("startup restored an intentionally disconnected session", status)
	}
}

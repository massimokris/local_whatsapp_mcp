package engine_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

func TestLifecycleHistoryRetainsOnlySafeOrderedTransitions(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
printf '%s\n' '{"event":"policy_loaded","data":{"revision":1}}' >&2
printf '%s\n' '{"event":"qr_code","data":{"code":"PRIVATE_SENTINEL_QR","message":"PRIVATE_SENTINEL_CONTENT"}}' >&2
printf '%s\n' '{"event":"paired","data":{"id":"PRIVATE_SENTINEL_ACCOUNT"}}' >&2
printf '%s\n' '{"event":"connected"}' '{"event":"disconnected"}' >&2
printf '%s\n' '{"event":"PRIVATE_SENTINEL_EVENT","data":{"code":"PRIVATE_SENTINEL_UNKNOWN"}}' >&2
printf '%s\n' 'PRIVATE_SENTINEL_STDERR' >&2
printf '%s\n' '{"event":"logged_out","data":{"reason_code":401,"on_connect":false,"reason":"PRIVATE_SENTINEL_REASON"}}' >&2
`
	if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	e := engine.New(bin, dir)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 1, Mode: "initial"}, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "lifecycle-history.json")
	var entries []map[string]any
	var body []byte
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		body, _ = os.ReadFile(path)
		if json.Unmarshal(body, &entries) == nil && len(entries) > 0 && entries[len(entries)-1]["event"] == "process_exit" {
			break
		}
	}
	if len(entries) == 0 {
		t.Fatalf("engine dropped post-scan transition evidence: no lifecycle history")
	}
	if strings.Contains(string(body), "PRIVATE_SENTINEL") {
		t.Fatal("lifecycle history retained private input")
	}
	var categories []string
	for i, entry := range entries {
		categories = append(categories, entry["event"].(string))
		if entry["sequence"] != float64(i+1) {
			t.Errorf("sequence is not ordered: %v", entry)
		}
		at, err := time.Parse(time.RFC3339Nano, entry["at"].(string))
		if err != nil || at.Location() != time.UTC {
			t.Error("timestamp is not UTC")
		}
		for key := range entry {
			switch key {
			case "sequence", "at", "event", "mode", "logout_code", "on_connect", "exit":
			default:
				t.Errorf("unexpected field %s", key)
			}
		}
	}
	want := []string{"process_start", "qr_available", "paired", "login_confirmed", "disconnected", "terminal_logout", "process_exit"}
	if !reflect.DeepEqual(categories, want) {
		t.Fatalf("lifecycle order=%v want=%v", categories, want)
	}
	if entries[0]["mode"] != "auth" || entries[5]["logout_code"] != float64(401) || entries[5]["on_connect"] != false {
		t.Fatalf("missing bounded lifecycle classification: %v", entries)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("lifecycle history is not private", err)
	}
	if state := e.Status(); state.Linked || state.Status != "attention" || !strings.Contains(state.Detail, "disconnected this linked device") {
		t.Fatalf("diagnostic history changed terminal logout: %+v", state)
	}
}

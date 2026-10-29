package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
	"github.com/massi/localwhatsappmcp/internal/service"
)

// serve exposes its private handler before scheduling saved-session startup.
// This pins the legal ordering in which the user's first Connect wins that race.
// The actual supervisor owns the fixture processes and consumes their NDJSON.
func TestDelayedStartupResumeDoesNotInterruptFirstLink(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "wacli")
	script := `#!/bin/sh
set -eu
root=$(dirname "$0")
printf '%s\n' started >> "$root/starts"
printf '%s\n' '{"event":"policy_loaded","data":{"revision":1}}' >&2
printf '%s\n' '{"event":"connected"}' >&2
exec /bin/cat >/dev/null
`
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	s, err := service.New(service.Options{DataDir: dir, Engine: engine.New(binary, dir)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request := httptest.NewRequest(http.MethodPost, "/whatsapp/connect", strings.NewReader(`{}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	s.ControlHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("first connect failed: HTTP %d", response.Code)
	}
	waitState := func(want string) engine.Status {
		t.Helper()
		for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			status := s.State()["whatsapp"].(engine.Status)
			if status.Status == want {
				return status
			}
		}
		t.Fatalf("did not reach %s; got %+v", want, s.State()["whatsapp"])
		return engine.Status{}
	}
	waitState("ready")

	// The queued startup resume executes after the successful user request.
	// A wake event is intentionally absent from this startup ordering.
	if err := s.ResumeSavedSession(ctx); err != nil {
		t.Fatal(err)
	}
	starts, err := os.ReadFile(filepath.Join(dir, "starts"))
	if err != nil {
		t.Fatal(err)
	}
	startCount := strings.Count(string(starts), "started\n")
	status := s.State()["whatsapp"].(engine.Status)
	if startCount != 1 || status.Status != "ready" {
		t.Fatalf("delayed startup interrupted initial capture: starts=%d status=%+v", startCount, status)
	}
}

func TestSavedSessionStartupResumesOnceAndWakeStillRestarts(t *testing.T) {
	s, e := savedStartupFixture(t)
	for range 2 {
		if err := s.ResumeSavedSession(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if e.starts != 1 || e.stops != 1 || e.lastPolicy.Mode != "initial" {
		t.Fatalf("saved startup starts=%d stops=%d policy=%+v", e.starts, e.stops, e.lastPolicy)
	}
	if err := s.Resume(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if err := s.ResumeSavedSession(t.Context()); err != nil {
		t.Fatal(err)
	}
	if e.starts != 2 || e.stops != 2 {
		t.Fatalf("wake must restart once without reopening startup: starts=%d stops=%d", e.starts, e.stops)
	}
}

func TestLifecycleActionSupersedesSavedSessionStartup(t *testing.T) {
	for _, action := range []struct{ name, method, path, body string }{
		{"connect", "POST", "/whatsapp/connect", `{}`},
		{"wake", "POST", "/lifecycle/wake", `{}`},
		{"scope", "PUT", "/scope", `{"mode":"all","chat_ids":[]}`},
		{"disconnect", "POST", "/whatsapp/disconnect", `{}`},
	} {
		t.Run(action.name, func(t *testing.T) {
			s, e := savedStartupFixture(t)
			response := disconnectControl(s, action.method, action.path, action.body)
			if response.Code != http.StatusOK {
				t.Fatalf("%s failed: HTTP %d", action.name, response.Code)
			}
			starts, stops := e.starts, e.stops
			for range 2 {
				if err := s.ResumeSavedSession(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			if e.starts != starts || e.stops != stops {
				t.Fatalf("delayed startup interrupted %s: starts=%d→%d stops=%d→%d", action.name, starts, e.starts, stops, e.stops)
			}
		})
	}
}

func TestSavedStartupRequiresVerifiedAuthentication(t *testing.T) {
	for _, authenticated := range []bool{false, true} {
		name, authValue := "abandoned or revoked session", "false"
		if authenticated {
			name, authValue = "verified saved session", "true"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			options := service.Options{DataDir: dir}
			seed, err := service.New(options)
			if err != nil {
				t.Fatal(err)
			}
			if err := seed.Resume(t.Context(), true); err != nil {
				t.Fatal(err)
			}
			if err := seed.Close(); err != nil {
				t.Fatal(err)
			}
			store := filepath.Join(dir, "store")
			if err := os.Mkdir(store, 0700); err != nil {
				t.Fatal(err)
			}
			journal := []byte(`{"history:synthetic":{"kind":"Message","sequence":1}}`)
			if err := os.WriteFile(filepath.Join(store, "sync-debts.json"), journal, 0600); err != nil {
				t.Fatal(err)
			}
			binary := filepath.Join(dir, "wacli")
			script := `#!/bin/sh
set -eu
root=$(dirname "$0")
case "$*" in
  *"--read-only auth status")
    printf '%s\n' status >> "$root/calls"
    printf '%s\n' '{"success":true,"data":{"authenticated":` + authValue + `}}'
    exit 0
    ;;
esac
printf '%s\n' writer >> "$root/calls"
case "$*" in
  *"--events sync --follow"*) printf '%s\n' sync > "$root/mode" ;;
  *"--events auth --follow"*) printf '%s\n' auth > "$root/mode" ;;
  *) exit 82 ;;
esac
[ -f "$root/store/sync-debts.json" ] || exit 81
printf '%s\n' '{"event":"policy_loaded","data":{"revision":1}}' >&2
printf '%s\n' '{"event":"connected"}' >&2
exec /bin/cat >/dev/null
`
			if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			options.Engine = engine.New(binary, dir)
			s, err := service.New(options)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			if status := s.State()["whatsapp"].(engine.Status); status.Linked != authenticated {
				t.Fatalf("restored linked=%v, want %v", status.Linked, authenticated)
			}
			if err := s.ResumeSavedSession(t.Context()); err != nil {
				t.Fatal(err)
			}
			countWriters := func() int {
				t.Helper()
				calls, err := os.ReadFile(filepath.Join(dir, "calls"))
				if err != nil || strings.Count(string(calls), "status\n") < 1 {
					t.Fatal("startup did not verify the saved session")
				}
				return strings.Count(string(calls), "writer\n")
			}
			writers := countWriters()
			if authenticated {
				if writers != 1 {
					t.Fatalf("verified session did not resume: writers=%d", writers)
				}
				if mode, err := os.ReadFile(filepath.Join(dir, "mode")); err != nil || string(mode) != "sync\n" {
					t.Errorf("saved initial scope started QR authentication instead of ordinary sync: mode=%q", mode)
				}
				return
			}
			status := s.State()["whatsapp"].(engine.Status)
			if writers != 0 {
				for deadline := time.Now().Add(time.Second); status.Status != "attention" && time.Now().Before(deadline); time.Sleep(time.Millisecond) {
					status = s.State()["whatsapp"].(engine.Status)
				}
				t.Errorf("unauthenticated saved intent started a writer: writers=%d linked=%v status=%s detail=%q", writers, status.Linked, status.Status, status.Detail)
			}
			if err := s.Resume(t.Context(), true); err != nil {
				t.Fatal(err)
			}
			if got := countWriters(); got != writers+1 {
				t.Fatalf("explicit Connect did not start one writer: before=%d after=%d", writers, got)
			}
			if mode, err := os.ReadFile(filepath.Join(dir, "mode")); err != nil || string(mode) != "auth\n" {
				t.Errorf("explicit Connect did not allow QR authentication: mode=%q", mode)
			}
			if after, err := os.ReadFile(filepath.Join(store, "sync-debts.json")); err != nil || string(after) != string(journal) {
				t.Fatal("startup or explicit Connect erased unresolved work")
			}
		})
	}
}

func TestAutomaticLifecycleRechecksAuthenticationAfterStopping(t *testing.T) {
	for _, action := range []string{"wake", "scope"} {
		for _, scenario := range []struct {
			name          string
			status        engine.Status
			authenticated bool
			revokeOnStop  bool
			starts        int
		}{
			{"revoked", engine.Status{Status: "attention"}, false, false, 0},
			{"transient disconnect", engine.Status{Status: "connecting", Linked: true}, true, false, 1},
			{"unscanned QR", engine.Status{Status: "linking"}, false, false, 0},
			{"unconfirmed connection attempt", engine.Status{Status: "connecting"}, false, false, 0},
			{"revoked while stopping", engine.Status{Status: "ready", Linked: true}, true, true, 0},
		} {
			t.Run(action+"/"+scenario.name, func(t *testing.T) {
				s, e := savedStartupFixture(t)
				e.mu.Lock()
				e.status = scenario.status
				e.authenticated = scenario.authenticated
				e.revokeOnStop = scenario.revokeOnStop
				e.mu.Unlock()
				path, method, body := "/lifecycle/wake", "POST", `{}`
				if action == "scope" {
					path, method, body = "/scope", "PUT", `{"mode":"all","chat_ids":[]}`
				}
				if response := disconnectControl(s, method, path, body); response.Code != http.StatusOK {
					t.Fatalf("%s failed: HTTP %d", action, response.Code)
				}
				if e.starts != scenario.starts {
					t.Errorf("%s after %s started %d writers, want %d", action, scenario.name, e.starts, scenario.starts)
				}
				if scenario.starts == 0 {
					starts := e.starts
					if err := s.Resume(t.Context(), true); err != nil {
						t.Fatal(err)
					}
					if e.starts != starts+1 {
						t.Fatal("explicit Connect could not retry the revoked session")
					}
				}
			})
		}
	}
}

type restoredStartupEngine struct {
	disconnectEngine
	authenticated bool
	revokeOnStop  bool
}

func (e *restoredStartupEngine) RestoreSession(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status.Linked = e.authenticated
	if e.authenticated {
		e.status.Status = "connecting"
	}
	return nil
}

func (e *restoredStartupEngine) Stop(ctx context.Context) error {
	if err := e.disconnectEngine.Stop(ctx); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.revokeOnStop {
		// The cached status deliberately stays linked, as it can when late
		// events are ignored while the old supervisor is stopping.
		e.authenticated = false
	}
	return nil
}

func savedStartupFixture(t *testing.T) (*service.Service, *restoredStartupEngine) {
	t.Helper()
	options := service.Options{DataDir: t.TempDir()}
	s, err := service.New(options)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Resume(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	e := &restoredStartupEngine{authenticated: true}
	options.Engine = e
	s, err = service.New(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s, e
}

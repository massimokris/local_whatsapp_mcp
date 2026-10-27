package service

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

type syncFixtureEngine struct {
	manualFixtureEngine
	mu          sync.Mutex
	status      engine.Status
	stopEntered chan struct{}
	releaseStop chan struct{}
}

func (e *syncFixtureEngine) Status() engine.Status { e.mu.Lock(); defer e.mu.Unlock(); return e.status }
func (e *syncFixtureEngine) Stop(ctx context.Context) error {
	e.mu.Lock()
	entered, release := e.stopEntered, e.releaseStop
	e.mu.Unlock()
	if entered != nil {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
func (e *syncFixtureEngine) setStatus(status engine.Status) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.status = status
}

func TestPrivateAndMCPShareExplicitSyncStateWithoutTimestampInference(t *testing.T) {
	e := &syncFixtureEngine{}
	s, err := New(Options{DataDir: t.TempDir(), LocalURL: "http://127.0.0.1:32191", Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.applyScope(context.Background(), engine.Policy{Mode: "all"}); err != nil {
		t.Fatal(err)
	}
	credentials := createManualFixture(t, s, "Sync fixture")
	for _, tc := range []struct{ state, status, timestamp, want string }{
		{engine.SyncCaughtUp, "ready", "2001-01-01T00:00:00Z", engine.SyncCaughtUp},
		{engine.Syncing, "ready", time.Now().UTC().Format(time.RFC3339), engine.Syncing},
		{engine.SyncError, "attention", "", engine.SyncError},
		{"", "ready", "", engine.SyncUnknown},
		{"unfamiliar", "ready", "", engine.SyncUnknown},
		{engine.SyncCaughtUp, "disconnected", "", engine.SyncUnknown},
	} {
		e.setStatus(engine.Status{Status: tc.status, SyncState: tc.state, LastSyncedAt: tc.timestamp})
		private := manualControl(t, s, "GET", "/state", nil)
		var state struct {
			WhatsApp engine.Status `json:"whatsapp"`
		}
		if err = json.Unmarshal(private.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", s.LocalURL()+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"check_status","arguments":{}}}`))
		req.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		response := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(response, req)
		var mcp struct {
			Result struct {
				StructuredContent struct {
					WhatsApp engine.Status `json:"whatsapp"`
				} `json:"structuredContent"`
			} `json:"result"`
		}
		if err = json.Unmarshal(response.Body.Bytes(), &mcp); err != nil {
			t.Fatal(err)
		}
		if state.WhatsApp.SyncState != tc.want || mcp.Result.StructuredContent.WhatsApp.SyncState != tc.want {
			t.Fatalf("state %q/status %q private=%+v MCP=%s", tc.state, tc.status, state.WhatsApp, response.Body.String())
		}
		if state.WhatsApp.LastSyncedAt != tc.timestamp || mcp.Result.StructuredContent.WhatsApp.LastSyncedAt != tc.timestamp {
			t.Fatal("sync state changed historical timestamp")
		}
	}
}

func TestInFlightWakeAndScopeTransitionRemoveCachedEngineSuccess(t *testing.T) {
	for _, operation := range []string{"wake", "scope"} {
		t.Run(operation, func(t *testing.T) {
			e := &syncFixtureEngine{}
			s, err := New(Options{DataDir: t.TempDir(), Engine: e})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err = s.Resume(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			e.setStatus(engine.Status{Status: "ready", SyncState: engine.SyncCaughtUp})
			e.mu.Lock()
			e.stopEntered = make(chan struct{})
			e.releaseStop = make(chan struct{})
			entered, release := e.stopEntered, e.releaseStop
			e.mu.Unlock()
			done := make(chan error, 1)
			go func() {
				if operation == "wake" {
					done <- s.Resume(context.Background(), false)
				} else {
					done <- s.applyScope(context.Background(), engine.Policy{Mode: "all"})
				}
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("operation did not reach engine stop")
			}
			status := s.State()["whatsapp"].(engine.Status)
			close(release)
			if err = <-done; err != nil {
				t.Fatal(err)
			}
			e.mu.Lock()
			e.stopEntered = nil
			e.releaseStop = nil
			e.mu.Unlock()
			if status.SyncState == engine.SyncCaughtUp || status.Status != "connecting" {
				t.Fatalf("pending %s retained success: %+v", operation, status)
			}
		})
	}
}

func TestServiceFailureAndMissingEngineDoNotExposeSuccess(t *testing.T) {
	s, err := New(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if status := s.State()["whatsapp"].(engine.Status); status.SyncState != engine.SyncUnknown {
		t.Fatalf("missing engine: %+v", status)
	}
	e := &syncFixtureEngine{status: engine.Status{Status: "ready", SyncState: engine.SyncCaughtUp}}
	s.opt.Engine = e
	s.fail(context.DeadlineExceeded)
	if status := s.State()["whatsapp"].(engine.Status); status.SyncState == engine.SyncCaughtUp {
		t.Fatal("failed service operation retained success")
	}
}

func TestWakeSaveFailureDoesNotRestorePreWakeSuccess(t *testing.T) {
	e := &syncFixtureEngine{status: engine.Status{Status: "ready", SyncState: engine.SyncCaughtUp}}
	dir := t.TempDir()
	s, err := New(Options{DataDir: dir, Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// A directory in place of settings.json deterministically fails the atomic
	// rename even when fixtures run as an administrator.
	settings := filepath.Join(dir, "settings.json")
	if err = os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(settings, 0700); err != nil {
		t.Fatal(err)
	}
	if err = s.Resume(context.Background(), false); err == nil {
		t.Fatal("expected settings save failure")
	}
	if status := s.State()["whatsapp"].(engine.Status); status.SyncState == engine.SyncCaughtUp {
		t.Fatal("failed wake restored pre-wake completion")
	}
}

func TestIntentionalDisconnectDoesNotEraseUnrecoveredMessageFailure(t *testing.T) {
	e := &syncFixtureEngine{status: engine.Status{Status: "attention", SyncState: engine.SyncError}}
	s, err := New(Options{DataDir: t.TempDir(), Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.cfg.WhatsAppDisconnected = true
	if status := s.State()["whatsapp"].(engine.Status); status.SyncState != engine.SyncError || status.Status != "disconnected" {
		t.Fatalf("disconnect erased unrecovered failure: %+v", status)
	}
}

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/massi/localwhatsappmcp/internal/engine"
	"github.com/massi/localwhatsappmcp/internal/secret"
)

func manualControl(t *testing.T, s *Service, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.ControlHandler().ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(payload)))
	return w
}

func createManualFixture(t *testing.T, s *Service, name string) connectionCredentials {
	t.Helper()
	w := manualControl(t, s, "POST", "/agents/manual", map[string]string{"name": name})
	if w.Code != http.StatusOK {
		t.Fatalf("manual creation returned %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("credential response must not be cached")
	}
	var credentials connectionCredentials
	if err := json.Unmarshal(w.Body.Bytes(), &credentials); err != nil {
		t.Fatal(err)
	}
	if credentials.ID == "" || credentials.AccessToken == "" || credentials.ID == credentials.AccessToken {
		t.Fatal("manual connection needs an independent identity and credential")
	}
	return credentials
}

func TestManualConnectionRequiresConfirmedScopeAndValidName(t *testing.T) {
	s, err := New(Options{DataDir: t.TempDir(), LocalURL: "http://127.0.0.1:32191"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if w := manualControl(t, s, "POST", "/agents/manual", map[string]string{"name": "My agent"}); w.Code != http.StatusConflict {
		t.Fatal("manual creation bypassed pending scope confirmation")
	}
	if err := s.applyScope(context.Background(), engine.Policy{Mode: "all"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", " \n\t", strings.Repeat("x", 81), strings.Repeat("é", 41)} {
		if w := manualControl(t, s, "POST", "/agents/manual", map[string]string{"name": name}); w.Code != http.StatusBadRequest {
			t.Fatal("invalid connection name accepted")
		}
	}
	if w := manualControl(t, s, "POST", "/agents/manual", map[string]string{"name": "My agent", "client": "manual"}); w.Code != http.StatusBadRequest {
		t.Fatal("unexpected request fields were accepted")
	}
	s.cfg.Transition = true
	if w := manualControl(t, s, "POST", "/agents/manual", map[string]string{"name": "My agent"}); w.Code != http.StatusConflict {
		t.Fatal("manual creation bypassed an active scope transition")
	}
	s.cfg.Transition = false
	if len(s.cfg.Grants) != 0 {
		t.Fatal("failed creation published a grant")
	}
	c := createManualFixture(t, s, "  My agent  ")
	if c.Name != "My agent" || c.URL != "http://127.0.0.1:32191/mcp" {
		t.Fatal("manual details did not return a trimmed name and usable MCP URL")
	}
	g := s.cfg.Grants[c.ID]
	if g.Client != "manual" || g.SendEnabled || g.Connected || g.PairingInvitationID != "" {
		t.Fatal("manual grant has incorrect initial permissions or identity")
	}
	if len(s.cfg.Invites) != 0 {
		t.Fatal("manual creation unnecessarily made a pairing invitation")
	}
	if _, err := os.Stat(filepath.Join(s.opt.DataDir, "connections")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("manual creation installed helper client configuration")
	}
}

func TestManualCredentialsAuthenticatePersistAndRevokeIndependently(t *testing.T) {
	options := Options{DataDir: t.TempDir(), LocalURL: "http://127.0.0.1:32191", Engine: manualFixtureEngine{}}
	s, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	if err := s.applyScope(context.Background(), engine.Policy{Mode: "selected", ChatIDs: []string{"123@s.whatsapp.net"}}); err != nil {
		t.Fatal(err)
	}
	a, b := createManualFixture(t, s, "Same name"), createManualFixture(t, s, "Same name")
	if a.ID == b.ID || a.AccessToken == b.AccessToken {
		t.Fatal("distinct manual connections share identity or credentials")
	}
	if value, err := secret.Get(options.DataDir, AgentCredentialNamespace, a.ID); err != nil || value != a.AccessToken {
		t.Fatal("creation returned a credential before it was saved")
	}
	call := func(token, tool, arguments string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + arguments + `}}`
		req := httptest.NewRequest("POST", options.LocalURL+"/mcp", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		s.LocalHandler().ServeHTTP(w, req)
		return w
	}
	if w := call(a.AccessToken, "check_status", `{}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "scope_revision") {
		t.Fatal("manually configured bearer could not call MCP")
	}
	if w := call(a.AccessToken, "send_text", `{"chat_id":"123@s.whatsapp.net","text":"hello","request_id":"manual-fixture"}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "sending is disabled") {
		t.Fatal("manual connection bypassed default send restriction")
	}
	if w := call(a.AccessToken, "read_messages", `{"chat_id":"999@s.whatsapp.net"}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "outside the current chat scope") {
		t.Fatal("manual connection bypassed shared chat scope")
	}
	state, err := json.Marshal(s.State())
	if err != nil {
		t.Fatal(err)
	}
	settings, err := os.ReadFile(filepath.Join(options.DataDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{a.AccessToken, b.AccessToken} {
		if bytes.Contains(state, []byte(token)) || bytes.Contains(settings, []byte(token)) {
			t.Fatal("durable bearer leaked into state or settings")
		}
	}
	if bytes.Contains(state, []byte(`"digest"`)) || bytes.Contains(state, []byte(`"access_token"`)) {
		t.Fatal("state includes credential fields")
	}
	// Both privileged routes remain absent from the HTTP listener, including for
	// an authenticated client. The private socket is the administration boundary.
	for _, route := range []struct{ method, path string }{{"POST", "/agents/manual"}, {"GET", "/agents/" + b.ID + "/credentials"}} {
		req := httptest.NewRequest(route.method, options.LocalURL+route.path, strings.NewReader(`{"name":"Intruder"}`))
		req.Header.Set("Authorization", "Bearer "+a.AccessToken)
		w := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatal("manual credential administration is exposed through HTTP")
		}
	}
	if w := manualControl(t, s, "DELETE", "/agents/"+a.ID, nil); w.Code != http.StatusOK {
		t.Fatal("manual grant could not be revoked")
	}
	if w := call(a.AccessToken, "check_status", `{}`); w.Code != http.StatusUnauthorized {
		t.Fatal("revoked manual bearer still authenticates")
	}
	if w := manualControl(t, s, "GET", "/agents/"+a.ID+"/credentials", nil); w.Code != http.StatusNotFound {
		t.Fatal("revoked bearer can still be copied")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(options)
	if err != nil {
		t.Fatal(err)
	}
	w := manualControl(t, s, "GET", "/agents/"+b.ID+"/credentials", nil)
	var after connectionCredentials
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &after) != nil || after != b {
		t.Fatal("reopened manual connection did not retain its URL and credential")
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("reopened credential response must not be cached")
	}
	if w := call(b.AccessToken, "check_status", `{}`); w.Code != http.StatusOK {
		t.Fatal("independent manual bearer stopped authenticating after restart")
	}
	if _, err := s.VerifyToken(context.Background(), a.AccessToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("restart restored revoked manual credential")
	}
	if w := manualControl(t, s, "GET", "/agents/"+a.ID+"/credentials", nil); w.Code != http.StatusNotFound {
		t.Fatal("restart restored revoked credential disclosure")
	}
}

type manualFixtureEngine struct{}

func (manualFixtureEngine) RestoreSession(context.Context) error             { return nil }
func (manualFixtureEngine) Start(context.Context, engine.Policy, bool) error { return nil }
func (manualFixtureEngine) Stop(context.Context) error                       { return nil }
func (manualFixtureEngine) Logout(context.Context) error                     { return nil }
func (manualFixtureEngine) Status() engine.Status {
	return engine.Status{Status: "ready"}
}
func (manualFixtureEngine) Catalog(context.Context) ([]engine.Chat, error) {
	return []engine.Chat{{ID: "123@s.whatsapp.net", Name: "Selected"}, {ID: "999@s.whatsapp.net", Name: "Excluded"}}, nil
}
func (manualFixtureEngine) Resolve(_ context.Context, id string) ([]string, error) {
	return []string{id}, nil
}
func (manualFixtureEngine) Call(context.Context, bool, ...string) (json.RawMessage, error) {
	return nil, errors.New("unexpected fixture engine command")
}

func TestSavedConnectionCredentialsPreservePairedAndLegacyConnections(t *testing.T) {
	for _, client := range []string{"generic", "codex", "claude", "stdio"} {
		t.Run(client, func(t *testing.T) {
			s, options := httpPairingFixture(t)
			defer func() {
				if s != nil {
					s.Close()
				}
			}()
			paired := httpPairingRedeem(t, s, httpPairingInvite(t, s))
			id, bearer := paired["id"], paired["access_token"]
			grant := s.cfg.Grants[id]
			// Older helper-installed connections retain their saved client label
			// and permissions. Viewing credentials must not migrate or reset them.
			grant.Client = client
			grant.SendEnabled = client != "generic"
			s.cfg.Grants[id] = grant
			if err := s.saveLocked(); err != nil {
				t.Fatal(err)
			}
			expected := connectionCredentials{ID: id, Name: grant.Name, URL: paired["url"], AccessToken: bearer}
			for restart := range 2 {
				if restart == 1 {
					if err := s.Close(); err != nil {
						t.Fatal(err)
					}
					var err error
					s, err = New(options)
					if err != nil {
						t.Fatal(err)
					}
				}
				before, err := os.ReadFile(filepath.Join(options.DataDir, "settings.json"))
				if err != nil {
					t.Fatal(err)
				}
				w := manualControl(t, s, "GET", "/agents/"+id+"/credentials", nil)
				var actual connectionCredentials
				if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &actual) != nil || actual != expected {
					t.Fatal("saved connection did not return its existing credential and MCP URL")
				}
				if w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("saved credential response must not be cached")
				}
				if s.cfg.Grants[id] != grant || len(s.cfg.Grants) != 1 || len(s.cfg.Invites) != 0 {
					t.Fatal("credential retrieval changed connection identity, permissions, or grants")
				}
				after, err := os.ReadFile(filepath.Join(options.DataDir, "settings.json"))
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("credential retrieval rewrote connection settings", err)
				}
				if stored, err := secret.Get(options.DataDir, AgentCredentialNamespace, id); err != nil || stored != bearer {
					t.Fatal("credential retrieval rotated the stored bearer", err)
				}
				state, _ := json.Marshal(s.State())
				if bytes.Contains(state, []byte(bearer)) || bytes.Contains(state, []byte(`"access_token"`)) {
					t.Fatal("opening credentials exposed bearer in general state")
				}
				public := httptest.NewRecorder()
				req := httptest.NewRequest("GET", options.LocalURL+"/agents/"+id+"/credentials", nil)
				req.Header.Set("Authorization", "Bearer "+bearer)
				s.LocalHandler().ServeHTTP(public, req)
				if public.Code != http.StatusNotFound || strings.Contains(public.Body.String(), bearer) {
					t.Fatal("saved credential retrieval was exposed on HTTP MCP listener")
				}
			}
			if w := manualControl(t, s, "DELETE", "/agents/"+id, nil); w.Code != http.StatusOK {
				t.Fatal("paired connection could not be revoked")
			}
			for _, missing := range []string{id, "unknown"} {
				w := manualControl(t, s, "GET", "/agents/"+missing+"/credentials", nil)
				if w.Code != http.StatusNotFound || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), bearer) {
					t.Fatal("credentials were disclosed for a missing or revoked connection")
				}
			}
		})
	}
}

func TestManualCredentialCreationFailsClosedForDamagedStorage(t *testing.T) {
	for _, damage := range []string{"missing database", "corrupt database", "missing signer", "mismatched signer"} {
		t.Run(damage, func(t *testing.T) {
			s, err := New(Options{DataDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err := s.applyScope(context.Background(), engine.Policy{Mode: "all"}); err != nil {
				t.Fatal(err)
			}
			damageManualStorage(t, s, damage, "")
			if _, err := s.createManualConnection("Blocked"); !errors.Is(err, ErrCredentials) {
				t.Fatal("manual creation silently repaired damaged credential storage", err)
			}
			if len(s.cfg.Grants) != 0 {
				t.Fatal("failed storage published a manual grant")
			}
			if damage == "missing database" {
				if _, err := os.Stat(filepath.Join(s.opt.DataDir, "credentials.db")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("manual creation recreated a lost database")
				}
			}
		})
	}
}

func TestSavedConnectionDetailsAndRestartFailClosedForDamagedStorage(t *testing.T) {
	for _, client := range []string{"manual", "generic"} {
		for _, damage := range []string{"missing database", "corrupt database", "missing signer", "mismatched signer", "missing bearer", "mismatched bearer"} {
			t.Run(client+"/"+damage, func(t *testing.T) {
				options := Options{DataDir: t.TempDir(), LocalURL: "http://127.0.0.1:32191"}
				s, err := New(options)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.applyScope(context.Background(), engine.Policy{Mode: "all"}); err != nil {
					t.Fatal(err)
				}
				var c connectionCredentials
				if client == "manual" {
					c = createManualFixture(t, s, "Blocked")
				} else {
					paired := httpPairingRedeem(t, s, httpPairingInvite(t, s))
					c = connectionCredentials{ID: paired["id"], AccessToken: paired["access_token"]}
				}
				grant := s.cfg.Grants[c.ID]
				damageManualStorage(t, s, damage, c.ID)
				w := manualControl(t, s, "GET", "/agents/"+c.ID+"/credentials", nil)
				if w.Code == http.StatusOK || strings.Contains(w.Body.String(), c.AccessToken) || !strings.Contains(w.Body.String(), ErrCredentials.Error()) {
					t.Fatal("credential lookup exposed stale data or raw storage diagnostics")
				}
				if len(s.cfg.Grants) != 1 || s.cfg.Grants[c.ID] != grant {
					t.Fatal("failed retrieval changed the saved grant")
				}
				if damage == "missing database" {
					if _, err := os.Stat(filepath.Join(options.DataDir, "credentials.db")); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("credential retrieval recreated the missing database")
					}
				}
				s.Close()
				if _, err := New(options); !errors.Is(err, ErrCredentials) {
					t.Fatal("restart silently repaired damaged credentials", err)
				}
			})
		}
	}
}

func damageManualStorage(t *testing.T, s *Service, damage, id string) {
	t.Helper()
	var err error
	path := filepath.Join(s.opt.DataDir, "credentials.db")
	switch damage {
	case "missing database":
		err = os.Remove(path)
	case "corrupt database":
		err = os.WriteFile(path, []byte("synthetic invalid SQLite database"), 0600)
	case "missing signer":
		err = secret.Delete(s.opt.DataDir, cursorCredentialNamespace, cursorCredentialAccount)
	case "mismatched signer":
		err = secret.SetExisting(s.opt.DataDir, cursorCredentialNamespace, cursorCredentialAccount, "synthetic-wrong-signing-key")
	case "missing bearer":
		err = secret.Delete(s.opt.DataDir, AgentCredentialNamespace, id)
	case "mismatched bearer":
		err = secret.SetExisting(s.opt.DataDir, AgentCredentialNamespace, id, "synthetic-wrong-bearer")
	default:
		t.Fatal("unknown damage fixture")
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestManualSettingsFailureDoesNotPublishGrantAndCanRetry(t *testing.T) {
	s, err := New(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.applyScope(context.Background(), engine.Policy{Mode: "all"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.opt.DataDir, "settings.json")
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if c, err := s.createManualConnection("Retry me"); err == nil || c.AccessToken != "" {
		t.Fatal("manual creation returned a bearer without saving its grant")
	}
	if len(s.cfg.Grants) != 0 {
		t.Fatal("failed settings write published a manual grant")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".saved", path); err != nil {
		t.Fatal(err)
	}
	c := createManualFixture(t, s, "Retry me")
	if id, err := s.VerifyToken(context.Background(), c.AccessToken); err != nil || id != c.ID {
		t.Fatal("manual creation did not recover after settings repair", err)
	}
}

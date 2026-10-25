package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
	"github.com/massi/localwhatsappmcp/internal/secret"
)

func httpPairingFixture(t *testing.T) (*Service, Options) {
	t.Helper()
	options := Options{DataDir: t.TempDir(), LocalURL: "http://127.0.0.1:32191", Engine: manualFixtureEngine{}}
	s, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.applyScope(context.Background(), engine.Policy{Mode: "selected", ChatIDs: []string{"123@s.whatsapp.net"}}); err != nil {
		t.Fatal(err)
	}
	return s, options
}

func httpPairingRequest(s *Service, method, body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, s.LocalURL()+"/pairing/redeem", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	w := httptest.NewRecorder()
	s.LocalHandler().ServeHTTP(w, req)
	return w
}

func httpPairingInvite(t *testing.T, s *Service) string {
	t.Helper()
	invitation, err := s.invite("My local agent", "")
	if err != nil {
		t.Fatal(err)
	}
	return invitation["token"].(string)
}

func httpPairingRedeem(t *testing.T, s *Service, token string) map[string]string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"token": token})
	w := httpPairingRequest(s, http.MethodPost, string(body), "application/json")
	if w.Code != http.StatusOK {
		t.Fatalf("pairing status %d", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("pairing credentials must not be cached")
	}
	var grant map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &grant); err != nil {
		t.Fatal(err)
	}
	if len(grant) != 4 || grant["id"] == "" || grant["access_token"] == "" || grant["access_token"] == token || grant["url"] != s.LocalURL()+"/mcp" || grant["token_type"] != "Bearer" {
		t.Fatal("pairing response does not match the generic HTTP contract")
	}
	return grant
}

func TestPairingInstructionsAreClientIndependent(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	for _, client := range []string{"", "codex", "claude", "stdio", "another client"} {
		body := map[string]string{"name": "My agent"}
		if client != "" {
			body["client"] = client
		}
		var invitation struct {
			ID           string `json:"invitation_id"`
			Token        string `json:"token"`
			Instructions string `json:"instructions"`
			ExpiresAt    string `json:"expires_at"`
		}
		pairingIdentityControl(t, s, "POST", "/pairing", body, &invitation)
		for _, required := range []string{s.LocalURL() + "/mcp", "POST " + s.LocalURL() + "/pairing/redeem", "Content-Type: application/json", `{"token":"` + invitation.Token + `"}`, "access_token", "Authorization: Bearer", "check_status", invitation.ExpiresAt} {
			if !strings.Contains(invitation.Instructions, required) {
				t.Fatalf("generic instructions omit %q", required)
			}
		}
		for _, forbidden := range []string{"helper", "codex", "claude", "stdio", "--data-dir", "--client", "doctor", "config.toml"} {
			if strings.Contains(strings.ToLower(invitation.Instructions), forbidden) {
				t.Fatalf("generic instructions retain client-specific %q", forbidden)
			}
		}
		grant := httpPairingRedeem(t, s, invitation.Token)
		stored := s.cfg.Grants[grant["id"]]
		if stored.Client != "generic" || stored.PairingInvitationID != invitation.ID || stored.SendEnabled || stored.Connected {
			t.Fatal("generic grant did not preserve identity and default permissions")
		}
	}
}

func TestHTTPPairingCredentialsEnforcePolicyPersistAndRevoke(t *testing.T) {
	s, options := httpPairingFixture(t)
	defer func() { s.Close() }()
	a := httpPairingRedeem(t, s, httpPairingInvite(t, s))
	b := httpPairingRedeem(t, s, httpPairingInvite(t, s))
	if a["access_token"] == b["access_token"] || a["id"] == b["id"] {
		t.Fatal("connections must have independent credentials")
	}
	if w := httpPairingRequest(s, "POST", `{"token":"`+a["access_token"]+`"}`, "application/json"); w.Code != http.StatusUnauthorized || len(s.cfg.Grants) != 2 {
		t.Fatal("a persistent bearer was accepted as an invitation to mint another connection")
	}
	if value, err := secret.Get(options.DataDir, AgentCredentialNamespace, a["id"]); err != nil || value != a["access_token"] {
		t.Fatal("response was returned before credential storage succeeded", err)
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
	if w := call(a["access_token"], "check_status", `{}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "scope_revision") {
		t.Fatal("HTTP pairing bearer could not call MCP")
	}
	if w := call(a["access_token"], "send_text", `{"chat_id":"123@s.whatsapp.net","text":"hello","request_id":"pairing-fixture"}`); !strings.Contains(w.Body.String(), "sending is disabled") {
		t.Fatal("HTTP pairing bypassed default sending restriction")
	}
	if w := call(a["access_token"], "read_messages", `{"chat_id":"999@s.whatsapp.net"}`); !strings.Contains(w.Body.String(), "outside the current chat scope") {
		t.Fatal("HTTP pairing bypassed shared chat scope")
	}
	state, _ := json.Marshal(s.State())
	settings, err := os.ReadFile(filepath.Join(options.DataDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, bearer := range []string{a["access_token"], b["access_token"]} {
		if bytes.Contains(state, []byte(bearer)) || bytes.Contains(settings, []byte(bearer)) {
			t.Fatal("bearer leaked into state or settings")
		}
	}
	if bytes.Contains(state, []byte(`"access_token"`)) || bytes.Contains(state, []byte(`"digest"`)) {
		t.Fatal("state contains credential fields")
	}
	pairingIdentityControl(t, s, "DELETE", "/agents/"+a["id"], nil, nil)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(options)
	if err != nil {
		t.Fatal(err)
	}
	if w := call(a["access_token"], "check_status", `{}`); w.Code != http.StatusUnauthorized {
		t.Fatal("restart restored a revoked HTTP pairing credential")
	}
	if w := call(b["access_token"], "check_status", `{}`); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "scope_revision") {
		t.Fatal("restart lost the other paired connection")
	}
	for _, route := range []struct{ method, path string }{{"POST", "/pairing"}, {"POST", "/agents/manual"}, {"GET", "/agents/" + b["id"] + "/credentials"}, {"PATCH", "/agents/" + b["id"]}, {"DELETE", "/agents/" + b["id"]}, {"PUT", "/scope"}, {"GET", "/state"}} {
		req := httptest.NewRequest(route.method, options.LocalURL+route.path, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+b["access_token"])
		w := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("private route exposed through HTTP: %s %s", route.method, route.path)
		}
	}
}

func TestHTTPPairingRejectsInvalidRequestsWithoutConsumingInvitation(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	token := httpPairingInvite(t, s)
	valid := `{"token":"` + token + `"}`
	for _, test := range []struct {
		name, method, body, contentType string
		status                          int
	}{
		{"get", "GET", valid, "application/json", 405},
		{"put", "PUT", valid, "application/json", 405},
		{"options", "OPTIONS", valid, "application/json", 405},
		{"missing media type", "POST", valid, "", 415},
		{"form content", "POST", valid, "application/x-www-form-urlencoded", 415},
		{"plain text", "POST", valid, "text/plain", 415},
		{"malformed media type", "POST", valid, "application/json; charset", 415},
		{"empty body", "POST", "", "application/json", 400},
		{"missing token", "POST", `{}`, "application/json", 400},
		{"empty token", "POST", `{"token":""}`, "application/json", 400},
		{"whitespace token", "POST", `{"token":"  "}`, "application/json", 400},
		{"null body", "POST", `null`, "application/json", 400},
		{"null token", "POST", `{"token":null}`, "application/json", 400},
		{"wrong token type", "POST", `{"token":1}`, "application/json", 400},
		{"unknown field", "POST", `{"token":"` + token + `","client":"anything"}`, "application/json", 400},
		{"duplicate field", "POST", `{"token":"` + token + `","token":"` + token + `"}`, "application/json", 400},
		{"trailing JSON", "POST", valid + `{}`, "application/json", 400},
		{"malformed JSON", "POST", valid[:len(valid)-1], "application/json", 400},
		{"oversized whitespace", "POST", valid + strings.Repeat(" ", pairingBodyLimit), "application/json", 413},
		{"wrong token", "POST", `{"token":"unrecognized-fixture-code"}`, "application/json", 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := httpPairingRequest(s, test.method, test.body, test.contentType)
			if w.Code != test.status || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unexpected status/cache policy: %d", w.Code)
			}
			if test.status == 405 && w.Header().Get("Allow") != "POST" {
				t.Fatal("unsupported method omitted Allow header")
			}
			if strings.Contains(w.Body.String(), token) || len(s.cfg.Grants) != 0 || len(s.cfg.Invites) != 1 {
				t.Fatal("invalid request disclosed/consumed an invitation or created a grant")
			}
		})
	}
	w := httpPairingRequest(s, "POST", valid, "application/json; charset=utf-8")
	if w.Code != http.StatusOK {
		t.Fatal("valid request could not redeem preserved invitation")
	}
}

func TestHTTPPairingChecksHostAndOriginBeforeRedemption(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	token := httpPairingInvite(t, s)
	for _, test := range []struct{ host, origin string }{{"attacker.example", ""}, {"127.0.0.1:32191", "https://attacker.example"}, {"127.0.0.1:32191", "null"}} {
		req := httptest.NewRequest("POST", s.LocalURL()+"/pairing/redeem", strings.NewReader(`{"token":"`+token+`"}`))
		req.Host = test.host
		req.Header.Set("Origin", test.origin)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(w, req)
		if w.Code != http.StatusForbidden || w.Header().Get("Cache-Control") != "no-store" || len(s.cfg.Grants) != 0 {
			t.Fatal("invalid host/origin reached pairing")
		}
	}
	httpPairingRedeem(t, s, token)
}

func TestHTTPPairingExpiryReplayAndConcurrentRedemption(t *testing.T) {
	s, options := httpPairingFixture(t)
	defer func() { s.Close() }()
	expired := httpPairingInvite(t, s)
	inv := s.cfg.Invites[digest(expired)]
	inv.Expires = time.Now().Add(-time.Second)
	s.cfg.Invites[digest(expired)] = inv
	if w := httpPairingRequest(s, "POST", `{"token":"`+expired+`"}`, "application/json"); w.Code != http.StatusUnauthorized || len(s.cfg.Grants) != 0 {
		t.Fatal("expired invitation authorized a connection")
	}
	token := httpPairingInvite(t, s)
	const count = 12
	start := make(chan struct{})
	results := make(chan int, count)
	var workers sync.WaitGroup
	for range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			results <- httpPairingRequest(s, "POST", `{"token":"`+token+`"}`, "application/json").Code
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	winners := 0
	for status := range results {
		if status == http.StatusOK {
			winners++
		} else if status != http.StatusUnauthorized {
			t.Fatalf("unexpected concurrent redemption status %d", status)
		}
	}
	if winners != 1 || len(s.cfg.Grants) != 1 {
		t.Fatal("concurrent redemption did not produce exactly one grant")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	s, err = New(options)
	if err != nil {
		t.Fatal(err)
	}
	if w := httpPairingRequest(s, "POST", `{"token":"`+token+`"}`, "application/json"); w.Code != http.StatusUnauthorized {
		t.Fatal("restart restored a consumed invitation")
	}
}

func TestHTTPPairingLifecycleGatesRetainInvitation(t *testing.T) {
	for _, gate := range []string{"initial scope", "scope transition", "closed"} {
		t.Run(gate, func(t *testing.T) {
			s, _ := httpPairingFixture(t)
			defer s.Close()
			token := httpPairingInvite(t, s)
			status := http.StatusConflict
			switch gate {
			case "initial scope":
				s.cfg.Scope.Mode = "initial"
			case "scope transition":
				s.cfg.Transition = true
			case "closed":
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				status = http.StatusUnauthorized
			}
			if w := httpPairingRequest(s, "POST", `{"token":"`+token+`"}`, "application/json"); w.Code != status || len(s.cfg.Grants) != 0 || len(s.cfg.Invites) != 1 {
				t.Fatalf("lifecycle gate did not preserve invitation: status %d", w.Code)
			}
			if _, err := s.invite("Blocked", ""); err == nil {
				t.Fatal("lifecycle gate allowed invitation creation")
			}
		})
	}
}

func TestHTTPPairingStorageFailureCanRetry(t *testing.T) {
	s, options := httpPairingFixture(t)
	defer s.Close()
	token := httpPairingInvite(t, s)
	path := filepath.Join(options.DataDir, "settings.json")
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.invite("Unsaved invitation", ""); err == nil || len(s.cfg.Invites) != 1 {
		t.Fatal("failed invitation save changed the available invitations")
	}
	w := httpPairingRequest(s, "POST", `{"token":"`+token+`"}`, "application/json")
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), options.DataDir) || len(s.cfg.Grants) != 0 || len(s.cfg.Invites) != 1 {
		t.Fatal("failed settings save disclosed private paths or changed pairing state")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".saved", path); err != nil {
		t.Fatal(err)
	}
	grant := httpPairingRedeem(t, s, token)
	if _, err := s.VerifyToken(context.Background(), grant["access_token"]); err != nil {
		t.Fatal("retried credential does not authenticate", err)
	}
}

package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/massi/localwhatsappmcp/internal/service"
)

func TestLocalListenerRejectsExternalHostsAndOrigins(t *testing.T) {
	const localURL = "http://127.0.0.1:32191"
	s, err := service.New(service.Options{DataDir: t.TempDir(), LocalURL: localURL, CursorKey: []byte("01234567890123456789012345678901")})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, test := range []struct {
		name, host, origin string
		status             int
	}{
		{"local authentication required", "127.0.0.1:32191", "", http.StatusUnauthorized},
		{"local origin authentication required", "127.0.0.1:32191", localURL, http.StatusUnauthorized},
		{"former public hostname", "bridge.example", "", http.StatusForbidden},
		{"external origin", "127.0.0.1:32191", "https://bridge.example", http.StatusForbidden},
		{"null origin", "127.0.0.1:32191", "null", http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, localURL+"/mcp", nil)
			req.Host = test.host
			req.Header.Set("Origin", test.origin)
			w := httptest.NewRecorder()
			s.LocalHandler().ServeHTTP(w, req)
			if w.Code != test.status {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if strings.Contains(w.Header().Get("WWW-Authenticate"), "oauth") {
				t.Fatal("local listener advertised removed OAuth discovery")
			}
		})
	}
	for _, path := range []string{"/state", "/pairing", "/cloud", "/cloud/setup", "/oauth/bootstrap", "/oauth/token", "/.well-known/oauth-protected-resource", "/.well-known/oauth-authorization-server"} {
		w := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, localURL+path, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("removed or private route %s returned %d", path, w.Code)
		}
	}
}

func TestLegacyCloudStateCannotRestoreHostedAccess(t *testing.T) {
	dir := t.TempDir()
	localDigest := sha256.Sum256([]byte("local-fixture-secret"))
	hostedDigest := sha256.Sum256([]byte("legacy-hosted-token"))
	settings := `{"version":1,"scope":{"version":1,"revision":2,"mode":"selected","chat_ids":["123@s.whatsapp.net"]},"grants":{"local":{"id":"local","name":"Local agent","client":"stdio","send_enabled":true,"digest":"` + hex.EncodeToString(localDigest[:]) + `"},"hosted":{"id":"hosted","name":"Former hosted agent","client":"cloud","send_enabled":true,"digest":"` + hex.EncodeToString(hostedDigest[:]) + `"}},"invitations":{},"sends":{},"linked":false}`
	legacyFiles := map[string]string{
		"settings.json":  settings,
		"cloud.json":     `{"enabled":true,"provider":"ngrok","hostname":"bridge.example","credential_ref":"legacy-fixture"}`,
		"oauth-old.json": `malformed legacy credential state must never be loaded`,
	}
	for name, contents := range legacyFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	options := service.Options{DataDir: dir, LocalURL: "http://127.0.0.1:32191", CursorKey: []byte("01234567890123456789012345678901")}
	for range 2 {
		s, err := service.New(options)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Resume(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		state := s.State()
		if _, exists := state["cloud"]; exists {
			t.Fatal("state still offers cloud access")
		}
		grants := state["agents"].([]service.Grant)
		if len(grants) != 1 || grants[0].ID != "local" || !grants[0].SendEnabled {
			t.Fatalf("local grant not preserved or hosted grant restored: %+v", grants)
		}
		if id, err := s.VerifyToken(context.Background(), "local-fixture-secret"); err != nil || id != "local" {
			t.Fatal("local credential stopped working", err)
		}
		if _, err := s.VerifyToken(context.Background(), "legacy-hosted-token"); err == nil {
			t.Fatal("hosted credential accepted")
		}
		for _, route := range []struct{ method, path string }{{"POST", "/cloud/instructions"}, {"POST", "/cloud/setup"}, {"POST", "/cloud/retry"}, {"GET", "/cloud/config"}, {"GET", "/cloud/verify"}, {"DELETE", "/cloud"}} {
			w := httptest.NewRecorder()
			s.ControlHandler().ServeHTTP(w, httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`)))
			if w.Code != http.StatusNotFound {
				t.Errorf("removed control route %s returned %d", route.path, w.Code)
			}
		}
		s.Close()
	}
	for _, name := range []string{"cloud.json", "oauth-old.json"} {
		contents, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(contents) != legacyFiles[name] {
			t.Errorf("legacy file %s was changed", name)
		}
	}
	contents, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	var saved struct {
		Grants map[string]service.Grant `json:"grants"`
	}
	if err != nil || json.Unmarshal(contents, &saved) != nil || len(saved.Grants) != 1 || saved.Grants["local"].ID != "local" {
		t.Fatal("restart did not preserve only the local grant")
	}
}

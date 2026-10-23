package cloud_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/massi/localwhatsappmcp/internal/cloud"
	"github.com/massi/localwhatsappmcp/internal/service"
)

type memorySecrets struct {
	mu   sync.Mutex
	data map[string]string
}

func (s *memorySecrets) Set(a, b, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[a+b] = v
	return nil
}
func (s *memorySecrets) Get(a, b string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[a+b], nil
}
func (s *memorySecrets) Delete(a, b string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, a+b)
	return nil
}
func TestCloudPrivateSetupAndPublicBootstrapAreSeparate(t *testing.T) {
	secrets := &memorySecrets{data: map[string]string{}}
	app, err := service.New(service.Options{DataDir: t.TempDir(), LocalURL: "http://127.0.0.1:32191", HelperPath: "/Applications/LocalWhatsAppMCP.app/Contents/Helpers/localwhatsappmcp", CursorKey: bytes.Repeat([]byte{1}, 32), ExtensionFactory: func(s *service.Service) (service.Extension, error) { return cloud.New(s, secrets) }})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	control := func(method, path string, data any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(data)
		w := httptest.NewRecorder()
		app.ControlHandler().ServeHTTP(w, httptest.NewRequest(method, "http://local"+path, bytes.NewReader(b)))
		return w
	}
	if w := control("PUT", "/scope", map[string]any{"mode": "all", "chat_ids": []string{}}); w.Code != 200 {
		t.Fatalf("scope: %s", w.Body.String())
	}
	w := control("POST", "/cloud/instructions", map[string]string{"provider": "ngrok", "name": "Test hosted client"})
	var instruction struct{ Instructions string }
	json.Unmarshal(w.Body.Bytes(), &instruction)
	lines := strings.Split(instruction.Instructions, "\n")
	token := ""
	for _, line := range lines {
		if len(line) == 43 && !strings.Contains(line, " ") {
			token = line
		}
	}
	if token == "" {
		t.Fatal("no invitation")
	}
	payload := map[string]any{"token": token, "provider": "ngrok", "hostname": "bridge.example", "executable": "/usr/bin/true", "credential": "fixture-secret", "client_id": "client-a", "redirect_uris": []string{"https://client.example/callback"}}
	w = control("POST", "/cloud/setup", payload)
	if w.Code != 200 {
		t.Fatalf("setup: %d %s", w.Code, w.Body.String())
	}
	var response map[string]string
	json.Unmarshal(w.Body.Bytes(), &response)
	if control("POST", "/cloud/setup", payload).Code != 403 {
		t.Fatal("cloud invitation replay admitted")
	}
	public := app.PublicHandler()
	req := httptest.NewRequest("POST", "https://bridge.example/oauth/bootstrap", strings.NewReader(url.Values{"handoff": {response["handoff"]}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://127.0.0.1:45219")
	w = httptest.NewRecorder()
	public.ServeHTTP(w, req)
	if w.Code != 303 || len(w.Result().Cookies()) != 1 {
		t.Fatalf("private browser bootstrap: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	public.ServeHTTP(w, httptest.NewRequest("GET", "https://bridge.example/cloud/config", nil))
	if w.Code != 404 {
		t.Fatal("private cloud control exposed publicly")
	}
	w = httptest.NewRecorder()
	public.ServeHTTP(w, httptest.NewRequest("POST", "https://bridge.example/mcp", strings.NewReader(`{}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatal("MCP is not authenticated")
	}
	if _, err = app.VerifyToken(context.Background(), response["handoff"]); err == nil {
		t.Fatal("browser handoff accepted as MCP bearer")
	}
}

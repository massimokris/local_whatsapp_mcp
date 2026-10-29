package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

const publicFixtureURL = "https://fixture.ngrok-free.app"

func ngrokRequest(t *testing.T, s *Service, base, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, base+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.LocalHandler().ServeHTTP(w, req)
	return w
}

func TestNgrokWildcardHostAndOriginValidation(t *testing.T) {
	for _, host := range []string{"fixture.ngrok-free.app", "nested.fixture.ngrok-free.app", "FIXTURE.ngrok-free.app:443"} {
		if _, ok := ngrokBaseURL(host); !ok {
			t.Fatalf("valid ngrok host rejected: %s", host)
		}
	}
	for _, host := range []string{"ngrok-free.app", "evilngrok-free.app", "fixture.ngrok-free.app.attacker.example", "fixture.ngrok-free.app.", "fixture..ngrok-free.app", "-fixture.ngrok-free.app", "fixture-.ngrok-free.app", "fixture.ngrok-free.app:80", "fixture.ngrok-free.app:0443", "fixture.ngrok-free.app:", "fixture.ngrok-free.app:443/path", "user@fixture.ngrok-free.app", "fixture.ngrok.app"} {
		if _, ok := ngrokBaseURL(host); ok {
			t.Fatalf("invalid ngrok host accepted: %s", host)
		}
	}
	s, _ := httpPairingFixture(t)
	defer s.Close()
	for _, tc := range []struct {
		host, origin string
		want         int
	}{
		{"127.0.0.1:32191", publicFixtureURL, 401},
		{"127.0.0.1:32191", publicFixtureURL + "/", 401},
		{"127.0.0.1:32191", s.LocalURL(), 401},
		{"fixture.ngrok-free.app", publicFixtureURL, 401},
		{"fixture.ngrok-free.app", publicFixtureURL + ":443", 401},
		{"fixture.ngrok-free.app", "", 401},
		{"fixture.ngrok-free.app", "https://another.ngrok-free.app", 403},
		{"fixture.ngrok-free.app", s.LocalURL(), 403},
		{"127.0.0.1:32191", "https://attacker.example", 403},
		{"127.0.0.1:32191", "http://fixture.ngrok-free.app", 403},
		{"127.0.0.1:32191", publicFixtureURL + "/path", 403},
		{"127.0.0.1:32191", publicFixtureURL + "?query=1", 403},
		{"127.0.0.1:32191", "https://user@fixture.ngrok-free.app", 403},
		{"127.0.0.1:32191", "null", 403},
		{"fixture.ngrok-free.app.attacker.example", "", 403},
	} {
		req := httptest.NewRequest("GET", s.LocalURL()+"/mcp", nil)
		req.Host = tc.host
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("X-Forwarded-Host", "fixture.ngrok-free.app")
		w := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("host %s origin %s: status %d want %d", tc.host, tc.origin, w.Code, tc.want)
		}
	}
}

func TestNgrokPairingUsesValidatedRequestHostWithoutConfiguration(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	token := httpPairingInvite(t, s)
	body, _ := json.Marshal(map[string]string{"token": token})
	for _, tc := range []struct{ host, origin string }{
		{"fixture.ngrok-free.app.attacker.example", ""},
		{"fixture.ngrok-free.app", "https://attacker.example"},
		{"unrecognized.example", publicFixtureURL},
	} {
		req := httptest.NewRequest("POST", publicFixtureURL+"/pairing/redeem", strings.NewReader(string(body)))
		req.Host = tc.host
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("X-Forwarded-Host", "fixture.ngrok-free.app")
		w := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(w, req)
		if w.Code != 403 || len(s.cfg.Grants) != 0 {
			t.Fatal("invalid host/origin consumed pairing code")
		}
	}
	req := httptest.NewRequest("POST", publicFixtureURL+"/pairing/redeem", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-Host", "attacker.example")
	req.Header.Set("X-Forwarded-Proto", "http")
	paired := httptest.NewRecorder()
	s.LocalHandler().ServeHTTP(paired, req)
	var c map[string]string
	if paired.Code != 200 || json.Unmarshal(paired.Body.Bytes(), &c) != nil || c["url"] != publicFixtureURL+"/mcp" {
		t.Fatal("ngrok pairing did not advertise validated HTTPS endpoint")
	}
	if paired.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("pairing response can be cached")
	}
	for _, base := range []string{s.LocalURL(), publicFixtureURL} {
		w := ngrokRequest(t, s, base, "POST", "/mcp", c["access_token"], `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"find_conversations","arguments":{}}}`)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Selected") || strings.Contains(w.Body.String(), "999@s.whatsapp.net") {
			t.Fatal("ngrok bearer cannot read scoped catalog")
		}
	}
	send := ngrokRequest(t, s, publicFixtureURL, "POST", "/mcp", c["access_token"], `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"send_text","arguments":{"chat_id":"123@s.whatsapp.net","text":"fixture","request_id":"public-disabled"}}}`)
	if send.Code != 200 || !strings.Contains(send.Body.String(), "sending is disabled") {
		t.Fatal("ngrok bypassed sending permission")
	}
	for _, path := range []string{"/state", "/network", "/network/ngrok", "/agents/" + c["id"] + "/credentials", "/pairing"} {
		if w := ngrokRequest(t, s, publicFixtureURL, "GET", path, c["access_token"], ""); w.Code != 404 {
			t.Fatal("private administration route exposed via ngrok")
		}
	}
	// Private/manual details and local redemption keep their original local URL.
	saved, err := s.credentialsForConnection(c["id"])
	if err != nil || saved.URL != s.LocalURL()+"/mcp" {
		t.Fatal("private saved credentials no longer use local endpoint")
	}
	local := httpPairingRedeem(t, s, httpPairingInvite(t, s))
	if local["url"] != s.LocalURL()+"/mcp" {
		t.Fatal("local redemption advertised remote endpoint")
	}
	if w := manualControl(t, s, "DELETE", "/agents/"+c["id"], nil); w.Code != 200 {
		t.Fatal("ngrok grant revocation failed")
	}
	if w := ngrokRequest(t, s, publicFixtureURL, "GET", "/mcp", c["access_token"], ""); w.Code != 401 {
		t.Fatal("revoked ngrok bearer remained authorized")
	}
}

type ngrokMediaEngine struct{ manualFixtureEngine }

func (ngrokMediaEngine) Call(_ context.Context, _ bool, args ...string) (json.RawMessage, error) {
	if args[0] == "messages" {
		return json.RawMessage(`{"ChatJID":"123@s.whatsapp.net","MsgID":"fixture-media"}`), nil
	}
	for i, arg := range args {
		if arg == "--output" && i+1 < len(args) {
			if err := os.WriteFile(args[i+1], []byte("synthetic attachment"), 0600); err != nil {
				return nil, err
			}
		}
	}
	return json.RawMessage(`{"downloaded":true,"source":"cache","mime_type":"text/plain"}`), nil
}
func TestNgrokAttachmentLinksAndAuthorization(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	s.opt.Engine = ngrokMediaEngine{}
	c := createManualFixture(t, s, "Agent")
	var publicPath string
	for _, base := range []string{s.LocalURL(), publicFixtureURL} {
		w := ngrokRequest(t, s, base, "POST", "/mcp", c.AccessToken, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_attachment","arguments":{"chat_id":"123@s.whatsapp.net","message_id":"fixture-media"}}}`)
		var envelope struct {
			Result struct {
				StructuredContent map[string]any `json:"structuredContent"`
			} `json:"result"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &envelope) != nil {
			t.Fatal("attachment RPC failed")
		}
		link, _ := envelope.Result.StructuredContent["url"].(string)
		if !strings.HasPrefix(link, base+"/attachments/") {
			t.Fatal("attachment used unvalidated host or wrong endpoint")
		}
		path := strings.TrimPrefix(link, base)
		if w := ngrokRequest(t, s, base, "GET", path, c.AccessToken, ""); w.Code != 200 || w.Body.String() != "synthetic attachment" {
			t.Fatal("authorized ngrok attachment failed")
		}
		if w := ngrokRequest(t, s, base, "GET", path, "", ""); w.Code != 401 {
			t.Fatal("ngrok attachment bypassed bearer authentication")
		}
		if base == publicFixtureURL {
			publicPath = path
		}
	}
	if err := s.applyScope(context.Background(), engine.Policy{Mode: "selected", ChatIDs: []string{}}); err != nil {
		t.Fatal(err)
	}
	if w := ngrokRequest(t, s, publicFixtureURL, "GET", publicPath, c.AccessToken, ""); w.Code == 200 {
		t.Fatal("ngrok attachment survived scope removal")
	}
}

// Signal the actual body-read seam while MCP owns its request lifetime.
type observedNetworkBody struct {
	io.ReadCloser
	once    sync.Once
	started chan struct{}
}

func (b *observedNetworkBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	return b.ReadCloser.Read(p)
}
func TestNgrokRevocationInterruptsIncompleteHTTPBody(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	c := createManualFixture(t, s, "Partial HTTP client")
	started := make(chan struct{})
	handler := s.LocalHandler()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = &observedNetworkBody{ReadCloser: r.Body, started: started}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := fmt.Fprintf(conn, "POST /mcp HTTP/1.1\r\nHost: fixture.ngrok-free.app\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nContent-Length: 128\r\n\r\n{", c.AccessToken); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("partial request never entered body read")
	}
	changed := make(chan int, 1)
	go func() { changed <- manualControl(t, s, "DELETE", "/agents/"+c.ID, nil).Code }()
	select {
	case code := <-changed:
		if code != 200 {
			t.Fatalf("revocation status %d", code)
		}
	case <-time.After(2 * time.Second):
		conn.Close()
		<-changed
		t.Fatal("revocation blocked on unfinished HTTP request")
	}
}
func TestNgrokAuthenticatedHTTPCompletionAllowsConnectionReuse(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	c := createManualFixture(t, s, "Keepalive client")
	var opened atomic.Int32
	server := httptest.NewUnstartedServer(s.LocalHandler())
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	transport := &http.Transport{MaxConnsPerHost: 1}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	local, _ := url.Parse(s.LocalURL())
	for _, host := range []string{"fixture.ngrok-free.app", "fixture.ngrok-free.app", local.Host} {
		req, err := http.NewRequest("POST", server.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"check_status","arguments":{}}}`))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = host
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		response, err := client.Do(req)
		if err != nil {
			t.Fatal("completed request poisoned connection", err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("reused connection status %d", response.StatusCode)
		}
	}
	if opened.Load() != 1 {
		t.Fatalf("completed requests did not reuse connection: %d", opened.Load())
	}
}

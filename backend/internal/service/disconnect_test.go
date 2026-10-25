package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
	"github.com/massi/localwhatsappmcp/internal/service"
)

type disconnectEngine struct {
	fixtureEngine
	mu                 sync.Mutex
	starts, stops      int
	logouts, writes    int
	activeWrites       int
	stopError          bool
	logoutError        bool
	logoutWhileWriting bool
	status             engine.Status
	lastPolicy         engine.Policy
	writeStarted       chan struct{}
	afterLogout        func()
	cachedContent      string
	cacheCalls         int
}

func (e *disconnectEngine) Start(_ context.Context, policy engine.Policy, _ bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.starts++
	e.lastPolicy = policy
	e.status = engine.Status{Status: "ready", Linked: true, QR: "stale-fixture-qr"}
	return nil
}
func (e *disconnectEngine) Stop(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stops++
	if e.stopError {
		return errors.New("fixture stop failure")
	}
	return nil
}
func (e *disconnectEngine) Logout(context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.logouts++
	e.logoutWhileWriting = e.activeWrites != 0
	if e.logoutError {
		return errors.New("fixture logout failure; retry Disconnect")
	}
	e.status = engine.Status{Status: "disconnected"}
	if e.afterLogout != nil {
		e.afterLogout()
	}
	return nil
}
func (e *disconnectEngine) Status() engine.Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}
func (e *disconnectEngine) Call(ctx context.Context, write bool, args ...string) (json.RawMessage, error) {
	if !write {
		if len(args) > 1 && args[0] == "media" && args[1] == "download" {
			e.mu.Lock()
			e.cacheCalls++
			content := e.cachedContent
			e.mu.Unlock()
			var path string
			cacheOnly := false
			for i, arg := range args {
				if arg == "--cache-only" {
					cacheOnly = true
				}
				if arg == "--output" && i+1 < len(args) {
					path = args[i+1]
				}
				if arg == "--retry-on-phone" {
					return nil, errors.New("offline cache attempted phone recovery")
				}
			}
			if !cacheOnly {
				return nil, errors.New("offline cache omitted --cache-only")
			}
			if content == "" {
				return nil, service.ErrDisconnected
			}
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				return nil, err
			}
			return json.RawMessage(`{"downloaded":true,"source":"cache","mime_type":"text/plain"}`), nil
		}
		if len(args) > 1 && args[0] == "messages" && args[1] == "show" {
			return json.RawMessage(`{"ChatJID":"123@s.whatsapp.net","MsgID":"fixture-attachment"}`), nil
		}
		return json.RawMessage(`{"messages":[]}`), nil
	}
	e.mu.Lock()
	e.writes++
	e.activeWrites++
	started := e.writeStarted
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.activeWrites--
		e.mu.Unlock()
	}()
	if started != nil {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return json.RawMessage(`{"id":"fixture-send"}`), nil
}

func disconnectControl(s *service.Service, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.ControlHandler().ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}
func disconnectFixture(t *testing.T) (*service.Service, *disconnectEngine, service.Options, string) {
	t.Helper()
	e := &disconnectEngine{}
	opt := service.Options{CursorKey: []byte("01234567890123456789012345678901"), DataDir: t.TempDir(), Engine: e}
	s, err := service.New(opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, r := range []*httptest.ResponseRecorder{
		disconnectControl(s, "POST", "/whatsapp/connect", `{}`),
		disconnectControl(s, "PUT", "/scope", `{"mode":"selected","chat_ids":["123@s.whatsapp.net"]}`),
	} {
		if r.Code != 200 {
			t.Fatal(r.Body.String())
		}
	}
	invite := disconnectControl(s, "POST", "/pairing", `{"name":"Fixture","client":"stdio"}`)
	var invitation struct{ Token string }
	if json.Unmarshal(invite.Body.Bytes(), &invitation) != nil || invitation.Token == "" {
		t.Fatal(invite.Body.String())
	}
	redemption := disconnectControl(s, "POST", "/pairing/redeem", `{"token":"`+invitation.Token+`"}`)
	var grant struct{ ID, Secret string }
	if json.Unmarshal(redemption.Body.Bytes(), &grant) != nil || grant.Secret == "" {
		t.Fatal(redemption.Body.String())
	}
	if r := disconnectControl(s, "PATCH", "/agents/"+grant.ID, `{"send_enabled":true}`); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	return s, e, opt, grant.Secret
}
func disconnectMCP(s *service.Service, token, name, args string) string {
	r := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	s.LocalHandler().ServeHTTP(w, r)
	return w.Body.String()
}
func assertDisconnected(t *testing.T, s *service.Service) {
	t.Helper()
	status := s.State()["whatsapp"].(engine.Status)
	if status.Status != "disconnected" || status.Linked || status.DisconnectPending || status.QR != "" {
		t.Fatalf("not disconnected: %+v", status)
	}
}

func TestDisconnectPreservesArchiveScopeAndAgentsAcrossRestart(t *testing.T) {
	s, e, opt, token := disconnectFixture(t)
	archive := filepath.Join(opt.DataDir, "store", "wacli.db")
	if err := os.MkdirAll(filepath.Dir(archive), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("retained synthetic archive"), 0600); err != nil {
		t.Fatal(err)
	}
	beforeScope := s.State()["scope"].(engine.Policy)
	policyBefore, err := os.ReadFile(filepath.Join(opt.DataDir, "content-policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	starts := e.starts
	public := httptest.NewRecorder()
	s.LocalHandler().ServeHTTP(public, httptest.NewRequest("POST", "/whatsapp/disconnect", strings.NewReader(`{}`)))
	if public.Code != 404 || e.logouts != 0 {
		t.Fatal("private disconnect was exposed through MCP HTTP")
	}
	if r := disconnectControl(s, "POST", "/whatsapp/disconnect", `{}`); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	assertDisconnected(t, s)
	if e.logouts != 1 {
		t.Fatalf("logout count %d", e.logouts)
	}
	for range 2 {
		if r := disconnectControl(s, "POST", "/lifecycle/wake", `{}`); r.Code != 200 {
			t.Fatal(r.Body.String())
		}
	}
	if r := disconnectControl(s, "POST", "/whatsapp/disconnect", `{}`); r.Code != 200 || e.logouts != 1 {
		t.Fatal("repeat logout was not idempotent", r.Body.String())
	}
	if e.starts != starts {
		t.Fatal("wake reconnected disconnected WhatsApp")
	}
	if _, err := s.VerifyToken(context.Background(), token); err != nil {
		t.Fatal("disconnect revoked the agent", err)
	}
	grants := s.State()["agents"].([]service.Grant)
	if len(grants) != 1 || !grants[0].SendEnabled {
		t.Fatal("agent sending grant changed", grants)
	}
	if got := disconnectMCP(s, token, "read_messages", `{"chat_id":"123@s.whatsapp.net"}`); strings.Contains(got, `"isError":true`) {
		t.Fatal("retained archive cannot be read", got)
	}
	if got := disconnectMCP(s, token, "read_messages", `{"chat_id":"999@s.whatsapp.net"}`); !strings.Contains(got, "outside") {
		t.Fatal("disconnected archive leaked outside scope", got)
	}
	for _, call := range [][2]string{
		{"send_text", `{"chat_id":"123@s.whatsapp.net","text":"hello","request_id":"after-disconnect"}`},
		{"get_attachment", `{"chat_id":"123@s.whatsapp.net","message_id":"fixture-attachment"}`},
	} {
		if got := disconnectMCP(s, token, call[0], call[1]); !strings.Contains(got, "WhatsApp is disconnected") {
			t.Fatal("network action was not blocked", call[0], got)
		}
	}
	if e.writes != 0 {
		t.Fatal("disconnected operation reached the engine")
	}
	if got := disconnectMCP(s, token, "check_status", `{}`); !strings.Contains(got, "disconnected") || strings.Contains(got, "stale-fixture-qr") {
		t.Fatal("MCP reported stale connection state", got)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = service.New(opt)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Resume(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	assertDisconnected(t, s)
	if e.starts != starts || !reflect.DeepEqual(s.State()["scope"], beforeScope) {
		t.Fatal("restart changed saved scope or resumed sync")
	}
	if _, err = s.VerifyToken(context.Background(), token); err != nil {
		t.Fatal("restart lost the agent credential", err)
	}
	if data, err := os.ReadFile(archive); err != nil || string(data) != "retained synthetic archive" {
		t.Fatal("disconnect changed the retained archive", err)
	}
	if data, err := os.ReadFile(filepath.Join(opt.DataDir, "content-policy.json")); err != nil || string(data) != string(policyBefore) {
		t.Fatal("disconnect changed the scope policy", err)
	}
	if r := disconnectControl(s, "PUT", "/scope", `{"mode":"selected","chat_ids":[]}`); r.Code != 200 || e.starts != starts {
		t.Fatal("scope change resurrected connection", r.Body.String())
	}
	if r := disconnectControl(s, "POST", "/whatsapp/connect", `{}`); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if e.starts != starts+1 || e.lastPolicy.Mode != "selected" || len(e.lastPolicy.ChatIDs) != 0 {
		t.Fatal("explicit reconnect did not preserve current scope")
	}
	if s.State()["whatsapp"].(engine.Status).Status != "ready" {
		t.Fatal("explicit reconnect remained disconnected")
	}
}

func TestFailedDisconnectRemainsRetryableAcrossWakeAndRestart(t *testing.T) {
	for _, failure := range []string{"stop", "logout"} {
		t.Run(failure, func(t *testing.T) {
			s, e, opt, _ := disconnectFixture(t)
			starts := e.starts
			e.stopError = failure == "stop"
			e.logoutError = failure == "logout"
			if r := disconnectControl(s, "POST", "/whatsapp/disconnect", `{}`); r.Code == 200 {
				t.Fatal("failed logout reported success")
			}
			status := s.State()["whatsapp"].(engine.Status)
			if status.Status != "attention" || !status.Linked || !status.DisconnectPending || status.QR != "" {
				t.Fatal("failed logout lost the retry state", status)
			}
			if failure == "stop" && e.logouts != 0 {
				t.Fatal("logout raced the active writer")
			}
			e.stopError = false
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			s, err = service.New(opt)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err = s.Resume(context.Background(), false); err != nil || e.starts != starts {
				t.Fatal("restart resumed after failed logout", err)
			}
			if err = s.Resume(context.Background(), true); err == nil || e.starts != starts {
				t.Fatal("reconnect abandoned unfinished logout")
			}
			e.logoutError = false
			if r := disconnectControl(s, "POST", "/whatsapp/disconnect", `{}`); r.Code != 200 {
				t.Fatal("logout retry failed", r.Body.String())
			}
			assertDisconnected(t, s)
		})
	}
}

func TestDisconnectSaveFailuresDoNotReportSuccessOrResume(t *testing.T) {
	for _, phase := range []string{"intent", "result"} {
		t.Run(phase, func(t *testing.T) {
			s, e, opt, _ := disconnectFixture(t)
			settings := filepath.Join(opt.DataDir, "settings.json")
			backup := filepath.Join(opt.DataDir, "saved-settings.json")
			blockSave := func() {
				if err := os.Rename(settings, backup); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(settings, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "intent" {
				blockSave()
			} else {
				e.afterLogout = blockSave
			}
			stops, starts := e.stops, e.starts
			if r := disconnectControl(s, "POST", "/whatsapp/disconnect", `{}`); r.Code == 200 {
				t.Fatal("failed state persistence reported success")
			}
			if phase == "intent" && (e.stops != stops || e.logouts != 0 || s.State()["whatsapp"].(engine.Status).Status != "ready") {
				t.Fatal("failed intent save changed the connection")
			}
			if phase == "result" && s.State()["whatsapp"].(engine.Status).Status != "attention" {
				t.Fatal("failed result save did not preserve retry")
			}
			if err := os.Remove(settings); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(backup, settings); err != nil {
				t.Fatal(err)
			}
			e.afterLogout = nil
			if phase == "result" {
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				var err error
				s, err = service.New(opt)
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				if err = s.Resume(context.Background(), false); err != nil || e.starts != starts {
					t.Fatal("result-save failure restarted sync", err)
				}
			}
			if r := disconnectControl(s, "POST", "/whatsapp/disconnect", `{}`); r.Code != 200 {
				t.Fatal("retry after storage recovery failed", r.Body.String())
			}
			assertDisconnected(t, s)
		})
	}
}

func TestDisconnectCancelsAndAwaitsPendingSendBeforeLogout(t *testing.T) {
	s, e, _, token := disconnectFixture(t)
	e.writeStarted = make(chan struct{})
	sent := make(chan string, 1)
	go func() {
		sent <- disconnectMCP(s, token, "send_text", `{"chat_id":"123@s.whatsapp.net","text":"hello","request_id":"pending"}`)
	}()
	select {
	case <-e.writeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("send never reached engine")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sent:
	case <-ctx.Done():
		t.Fatal("pending request was not cancelled")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.logoutWhileWriting || e.logouts != 1 || e.activeWrites != 0 || e.writes != 1 {
		t.Fatal("disconnect did not await the in-flight write")
	}
}

func TestFailedReconnectPersistenceKeepsDisconnectIntent(t *testing.T) {
	s, e, opt, token := disconnectFixture(t)
	if err := s.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	starts := e.starts
	settings := filepath.Join(opt.DataDir, "settings.json")
	backup := filepath.Join(opt.DataDir, "saved-settings.json")
	if err := os.Rename(settings, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(settings, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Resume(context.Background(), true); err == nil {
		t.Fatal("failed reconnect save reported success")
	}
	assertDisconnected(t, s)
	if e.starts != starts {
		t.Fatal("reconnect started before its intent was saved")
	}
	if got := disconnectMCP(s, token, "send_text", `{"chat_id":"123@s.whatsapp.net","text":"hello","request_id":"failed-reconnect"}`); !strings.Contains(got, "WhatsApp is disconnected") || e.writes != 0 {
		t.Fatal("failed reconnect reenabled sends", got)
	}
	if err := os.Remove(settings); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, settings); err != nil {
		t.Fatal(err)
	}
	if err := s.Resume(context.Background(), true); err != nil || e.starts != starts+1 {
		t.Fatal("reconnect after persistence recovered failed", err)
	}
}

func TestDisconnectedAttachmentsUseVerifiedCacheAndCurrentScope(t *testing.T) {
	s, e, _, token := disconnectFixture(t)
	e.cachedContent = "cached before disconnect"
	if err := s.Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	body := disconnectMCP(s, token, "get_attachment", `{"chat_id":"123@s.whatsapp.net","message_id":"fixture-attachment","max_bytes":6}`)
	var result struct {
		Result struct {
			Structured struct {
				Source string `json:"source"`
				URL    string `json:"url"`
				Data   string `json:"data_base64"`
				More   bool   `json:"has_more"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil || result.Result.Structured.Source != "cache" || result.Result.Structured.Data != "Y2FjaGVk" || !result.Result.Structured.More {
		t.Fatal("cached attachment unavailable after logout", body, err)
	}
	r := httptest.NewRequest("GET", result.Result.Structured.URL, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.LocalHandler().ServeHTTP(w, r)
	if w.Code != 200 || w.Body.String() != e.cachedContent {
		t.Fatal("fresh cached URL unavailable after logout", w.Code, w.Body.String())
	}
	if e.cacheCalls != 1 || e.writes != 0 {
		t.Fatal("offline attachment attempted a network-capable command")
	}
	if r := disconnectControl(s, "PUT", "/scope", `{"mode":"selected","chat_ids":[]}`); r.Code != 200 {
		t.Fatal(r.Body.String())
	}
	if body := disconnectMCP(s, token, "get_attachment", `{"chat_id":"123@s.whatsapp.net","message_id":"fixture-attachment"}`); !strings.Contains(body, "outside") || e.cacheCalls != 1 {
		t.Fatal("cached attachment ignored scope removal", body)
	}
}

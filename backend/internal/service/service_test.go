package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/massi/localwhatsappmcp/internal/engine"
	"github.com/massi/localwhatsappmcp/internal/service"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPairingNeedsConfirmedScopeAndRevokesOneAgent(t *testing.T) {
	s, err := service.New(service.Options{CursorKey: []byte("01234567890123456789012345678901"), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		s.ControlHandler().ServeHTTP(r, httptest.NewRequest(method, path, strings.NewReader(body)))
		return r
	}
	if r := request("POST", "/pairing", `{"name":"A","client":"stdio"}`); r.Code != 409 {
		t.Fatalf("initial pairing status %d", r.Code)
	}
	if r := request("PUT", "/scope", `{"mode":"selected","chat_ids":["123@s.whatsapp.net"]}`); r.Code != 200 {
		t.Fatalf("scope %d %s", r.Code, r.Body)
	}
	redeem := func(name string) (string, string) {
		p := request("POST", "/pairing", `{"name":"`+name+`","client":"stdio"}`)
		var invite struct {
			Token string `json:"token"`
		}
		json.Unmarshal(p.Body.Bytes(), &invite)
		if invite.Token == "" {
			t.Fatal("missing pairing token")
		}
		r := request("POST", "/pairing/redeem", `{"token":"`+invite.Token+`"}`)
		var grant struct {
			ID     string `json:"id"`
			Secret string `json:"secret"`
		}
		json.Unmarshal(r.Body.Bytes(), &grant)
		if r.Code != 200 || grant.Secret == "" {
			t.Fatalf("redemption %d", r.Code)
		}
		if again := request("POST", "/pairing/redeem", `{"token":"`+invite.Token+`"}`); again.Code != 401 {
			t.Fatal("pairing token replay accepted")
		}
		return grant.ID, grant.Secret
	}
	a, as := redeem("A")
	b, bs := redeem("B")
	if _, err := s.VerifyToken(context.Background(), as); err != nil {
		t.Fatal(err)
	}
	if r := request("DELETE", "/agents/"+a, ""); r.Code != 200 {
		t.Fatal(r.Code)
	}
	if _, err := s.VerifyToken(context.Background(), as); err == nil {
		t.Fatal("revoked secret accepted")
	}
	if id, err := s.VerifyToken(context.Background(), bs); err != nil || id != b {
		t.Fatal("other grant disrupted")
	}
	pub := httptest.NewRecorder()
	s.PublicHandler().ServeHTTP(pub, httptest.NewRequest(http.MethodGet, "/state", nil))
	if pub.Code != 404 {
		t.Fatal("private route public")
	}
}

func TestMCPFiltersOldArchiveAndBlocksDirectSend(t *testing.T) {
	e := &fixtureEngine{}
	s, err := service.New(service.Options{CursorKey: []byte("01234567890123456789012345678901"), DataDir: t.TempDir(), Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	control := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.ControlHandler().ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	control("PUT", "/scope", `{"mode":"selected","chat_ids":["123@s.whatsapp.net"]}`)
	p := control("POST", "/pairing", `{"name":"Test","client":"stdio"}`)
	var i map[string]any
	json.Unmarshal(p.Body.Bytes(), &i)
	r := control("POST", "/pairing/redeem", `{"token":"`+i["token"].(string)+`"}`)
	var g map[string]string
	json.Unmarshal(r.Body.Bytes(), &g)
	call := func(name, args string) string {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`))
		req.Header.Set("Authorization", "Bearer "+g["secret"])
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		s.PublicHandler().ServeHTTP(w, req)
		return w.Body.String()
	}
	if body := call("find_conversations", `{}`); !strings.Contains(body, "Work") || strings.Contains(body, "Family") {
		t.Fatalf("catalog leak or missing Work: %s", body)
	}
	if body := call("read_messages", `{"chat_id":"999@s.whatsapp.net"}`); !strings.Contains(body, "outside") {
		t.Fatal(body)
	}
	if body := call("send_text", `{"chat_id":"123@s.whatsapp.net","text":"hello","request_id":"same"}`); !strings.Contains(body, "sending is disabled") {
		t.Fatal(body)
	}
	if e.sends != 0 {
		t.Fatal("denied send dispatched")
	}
	control("PATCH", "/agents/"+g["id"], `{"send_enabled":true}`)
	for range 2 {
		if body := call("send_text", `{"chat_id":"123@s.whatsapp.net","text":"hello","request_id":"same"}`); !strings.Contains(body, "accepted") {
			t.Fatal(body)
		}
	}
	if e.sends != 1 {
		t.Fatalf("duplicate send %d", e.sends)
	}
}

type fixtureEngine struct{ sends int }

func (e *fixtureEngine) Start(context.Context, engine.Policy, bool) error { return nil }
func (e *fixtureEngine) Stop(context.Context) error                       { return nil }
func (e *fixtureEngine) Status() engine.Status                            { return engine.Status{Status: "ready"} }
func (e *fixtureEngine) Catalog(context.Context) ([]engine.Chat, error) {
	return []engine.Chat{{ID: "123@s.whatsapp.net", Name: "Work"}, {ID: "999@s.whatsapp.net", Name: "Family"}}, nil
}
func (e *fixtureEngine) Resolve(_ context.Context, id string) ([]string, error) {
	return []string{id}, nil
}
func (e *fixtureEngine) Call(_ context.Context, write bool, args ...string) (json.RawMessage, error) {
	if write {
		e.sends++
		return json.RawMessage(`{"id":"sent-1"}`), nil
	}
	return json.RawMessage(`{"messages":[]}`), nil
}

func TestFailedPolicyTransitionSurvivesRestartAndNeverRestoresInitial(t *testing.T) {
	dir := t.TempDir()
	e := &failureEngine{}
	opts := service.Options{CursorKey: []byte("01234567890123456789012345678901"), DataDir: dir, Engine: e}
	s, err := service.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	req := func(s *service.Service, method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.ControlHandler().ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	req(s, "POST", "/whatsapp/connect", `{}`)
	e.fail = true
	r := req(s, "PUT", "/scope", `{"mode":"selected","chat_ids":["123@s.whatsapp.net"]}`)
	if r.Code == 200 {
		t.Fatal("failed writer acknowledged scope")
	}
	s.Close()
	e.fail = false
	s, err = service.New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state := req(s, "GET", "/state", "").Body.String()
	if !strings.Contains(state, `"mode":"selected"`) || !strings.Contains(state, "applying_scope") {
		t.Fatal(state)
	}
	if err = s.Resume(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	state = req(s, "GET", "/state", "").Body.String()
	if strings.Contains(state, "applying_scope") || strings.Contains(state, `"mode":"initial"`) {
		t.Fatal(state)
	}
}

type failureEngine struct {
	fixtureEngine
	fail bool
}

func (e *failureEngine) Start(context.Context, engine.Policy, bool) error {
	if e.fail {
		return fmt.Errorf("fixture writer failed")
	}
	return nil
}

func TestCheckpointAndCachedAttachmentRecheckCurrentScope(t *testing.T) {
	e := &mediaEngine{}
	dir := t.TempDir()
	s, err := service.New(service.Options{CursorKey: []byte("01234567890123456789012345678901"), DataDir: dir, Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctl := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.ControlHandler().ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	ctl("PUT", "/scope", `{"mode":"selected","chat_ids":["123@s.whatsapp.net"]}`)
	var invite map[string]any
	json.Unmarshal(ctl("POST", "/pairing", `{"name":"Fixture","client":"stdio"}`).Body.Bytes(), &invite)
	var grant map[string]string
	json.Unmarshal(ctl("POST", "/pairing/redeem", `{"token":"`+invite["token"].(string)+`"}`).Body.Bytes(), &grant)
	pub := s.PublicHandler()
	call := func(name string, args map[string]any) map[string]any {
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		r := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+grant["secret"])
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		w := httptest.NewRecorder()
		pub.ServeHTTP(w, r)
		var result struct {
			Result map[string]any `json:"result"`
		}
		if json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal(w.Body.String())
		}
		return result.Result
	}
	updates := call("get_updates", map[string]any{})
	raw, _ := json.Marshal(updates)
	if strings.Contains(string(raw), "999@s.whatsapp.net") || strings.Contains(string(raw), "hidden-id") {
		t.Fatal("hidden change exposed")
	}
	checkpoint := updates["structuredContent"].(map[string]any)["next_checkpoint"].(string)
	attachment := call("get_attachment", map[string]any{"chat_id": "123@s.whatsapp.net", "message_id": "a"})
	payload := attachment["structuredContent"].(map[string]any)
	if payload["data_base64"] != "Zml4dHVyZS1maWxl" {
		t.Fatal(payload)
	}
	url := payload["url"].(string)
	get := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", url, nil)
		r.Header.Set("Authorization", "Bearer "+grant["secret"])
		w := httptest.NewRecorder()
		pub.ServeHTTP(w, r)
		return w
	}
	if got := get(); got.Code != 200 || got.Body.String() != "fixture-file" {
		t.Fatal(got.Code, got.Body.String())
	}
	e.content = strings.Repeat("x", 1<<20) + "END"
	large := call("get_attachment", map[string]any{"chat_id": "123@s.whatsapp.net", "message_id": "large"})["structuredContent"].(map[string]any)
	if large["has_more"] != true || large["next_offset"] != float64(1<<20) {
		t.Fatal("large file is not readable through MCP chunks", large)
	}
	tail := call("get_attachment", map[string]any{"chat_id": "123@s.whatsapp.net", "message_id": "large", "offset": 1 << 20})["structuredContent"].(map[string]any)
	if tail["data_base64"] != "RU5E" || tail["has_more"] != false {
		t.Fatal("last file chunk incorrect", tail)
	}
	ctl("PUT", "/scope", `{"mode":"selected","chat_ids":[]}`)
	if get().Code == 200 {
		t.Fatal("removed chat attachment exposed")
	}
	expired := call("get_updates", map[string]any{"checkpoint": checkpoint})
	if expired["isError"] != true {
		t.Fatal("checkpoint from removed scope accepted")
	}
}

type mediaEngine struct {
	fixtureEngine
	content string
}

func (e *mediaEngine) Call(ctx context.Context, write bool, args ...string) (json.RawMessage, error) {
	if len(args) > 1 && args[0] == "messages" && args[1] == "show" {
		var chat, id string
		for i, a := range args {
			if a == "--chat" {
				chat = args[i+1]
			}
			if a == "--id" {
				id = args[i+1]
			}
		}
		b, _ := json.Marshal(map[string]string{"ChatJID": chat, "MsgID": id})
		return b, nil
	}
	if args[0] == "media" {
		for i, a := range args {
			if a == "--output" {
				data := e.content
				if data == "" {
					data = "fixture-file"
				}
				os.WriteFile(args[i+1], []byte(data), 0600)
			}
		}
		return json.RawMessage(`{"downloaded":true,"source":"cache","mime_type":"text/plain","bytes":12}`), nil
	}
	if len(args) > 1 && args[1] == "changes" {
		return json.RawMessage(`{"changes":[{"sequence":1,"operation":"upsert","chat_jid":"999@s.whatsapp.net","msg_id":"hidden-id"},{"sequence":2,"operation":"upsert","chat_jid":"123@s.whatsapp.net","msg_id":"allowed-id"}],"next_checkpoint":"private-upstream-checkpoint","has_more":false}`), nil
	}
	return e.fixtureEngine.Call(ctx, write, args...)
}

func TestFailedAgentChangesRemainVisibleAndConsistentAfterRestart(t *testing.T) {
	for _, method := range []string{"DELETE", "PATCH"} {
		t.Run(method, func(t *testing.T) {
			dir := t.TempDir()
			opt := service.Options{CursorKey: []byte("01234567890123456789012345678901"), DataDir: dir}
			s, err := service.New(opt)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { s.Close() }()
			request := func(method, path, body string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				s.ControlHandler().ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
				return w
			}
			request("PUT", "/scope", `{"mode":"all"}`)
			var invite map[string]string
			json.Unmarshal(request("POST", "/pairing", `{"name":"Keep visible","client":"stdio"}`).Body.Bytes(), &invite)
			var grant map[string]string
			json.Unmarshal(request("POST", "/pairing/redeem", `{"token":"`+invite["token"]+`"}`).Body.Bytes(), &grant)
			if r := request("PATCH", "/agents/"+grant["id"], `{"send_enabled":true}`); r.Code != 200 {
				t.Fatal(r.Body)
			}
			// Make the settings destination unwritable without relying on user permissions.
			path := dir + "/settings.json"
			if err := os.Rename(path, path+".saved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			if r := request(method, "/agents/"+grant["id"], `{"send_enabled":false}`); r.Code == 200 {
				t.Fatal("failed settings write reported success")
			}
			assertPrior := func() {
				t.Helper()
				body := request("GET", "/state", "").Body.String()
				if !strings.Contains(body, `"name":"Keep visible"`) || !strings.Contains(body, `"send_enabled":true`) {
					t.Fatalf("failed change hidden in UI: %s", body)
				}
				if _, err := s.VerifyToken(context.Background(), grant["secret"]); err != nil {
					t.Fatal("failed revocation changed authorization", err)
				}
			}
			assertPrior()
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(path+".saved", path); err != nil {
				t.Fatal(err)
			}
			s.Close()
			s, err = service.New(opt)
			if err != nil {
				t.Fatal(err)
			}
			assertPrior()
			if r := request(method, "/agents/"+grant["id"], `{"send_enabled":false}`); r.Code != 200 {
				t.Fatal("retry failed", r.Body)
			}
			s.Close()
			s, err = service.New(opt)
			if err != nil {
				t.Fatal(err)
			}
			body := request("GET", "/state", "").Body.String()
			if method == "DELETE" {
				if strings.Contains(body, "Keep visible") {
					t.Fatal("revoked agent restored")
				}
				if _, err := s.VerifyToken(context.Background(), grant["secret"]); err == nil {
					t.Fatal("revoked credential restored")
				}
			} else if strings.Contains(body, `"send_enabled":true`) {
				t.Fatal("sending restored")
			}
		})
	}
}

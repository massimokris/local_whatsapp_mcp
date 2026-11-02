package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/massi/localwhatsappmcp/internal/service"
)

// argsEngine records archive reads and pages once so the upstream cursor is exercised.
type argsEngine struct {
	fixtureEngine
	calls [][]string
}

func (e *argsEngine) Call(_ context.Context, _ bool, args ...string) (json.RawMessage, error) {
	e.calls = append(e.calls, append([]string(nil), args...))
	for _, a := range args {
		if a == "--cursor" {
			return json.RawMessage(`{"messages":[],"page":{"has_more":false},"source":{"scope":"local_archive"}}`), nil
		}
	}
	return json.RawMessage(`{"messages":[],"page":{"has_more":true,"next_cursor":"upstream-page-2"},"source":{"scope":"local_archive"}}`), nil
}

func searchFixture(t *testing.T) (*argsEngine, func(name string, args map[string]any) map[string]any) {
	t.Helper()
	e := &argsEngine{}
	s, err := service.New(service.Options{CursorKey: []byte("01234567890123456789012345678901"), DataDir: t.TempDir(), Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ctl := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.ControlHandler().ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	ctl("PUT", "/scope", `{"mode":"selected","chat_ids":["123@s.whatsapp.net"]}`)
	var invite map[string]any
	json.Unmarshal(ctl("POST", "/pairing", `{"name":"Fixture"}`).Body.Bytes(), &invite)
	var grant map[string]string
	json.Unmarshal(ctl("POST", "/pairing/redeem", `{"token":"`+invite["token"].(string)+`"}`).Body.Bytes(), &grant)
	pub := s.LocalHandler()
	return e, func(name string, args map[string]any) map[string]any {
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		r := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+grant["secret"])
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		w := httptest.NewRecorder()
		pub.ServeHTTP(w, r)
		var out struct {
			Result map[string]any `json:"result"`
		}
		if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Result["isError"] == true {
			t.Fatalf("%s failed: %s", name, w.Body.String())
		}
		return out.Result
	}
}

func separatorIndex(args []string) int {
	for i, a := range args {
		if a == "--" {
			return i
		}
	}
	return -1
}

func TestSearchTextFollowsArgumentSeparator(t *testing.T) {
	for _, text := range []string{"--from", "-x", "--chat=other@g.us", "hello"} {
		t.Run(text, func(t *testing.T) {
			e, call := searchFixture(t)
			call("search_messages", map[string]any{"query": text, "chat_id": "123@s.whatsapp.net"})
			if len(e.calls) != 1 {
				t.Fatalf("expected one archive call, got %v", e.calls)
			}
			args := e.calls[0]
			n := len(args)
			if n < 2 || args[n-2] != "--" || args[n-1] != text {
				t.Fatalf("search text is not the only argument after the separator: %q", args)
			}
			sep := separatorIndex(args)
			chat := -1
			for i, a := range args {
				if a == "--chat" {
					chat = i
				}
			}
			if sep != n-2 || chat < 0 || chat > sep || args[chat+1] != "123@s.whatsapp.net" {
				t.Fatalf("authorized chat filter must precede the separator: %q", args)
			}
		})
	}
}

func TestSearchFlagsPrecedeSeparator(t *testing.T) {
	e, call := searchFixture(t)
	query := map[string]any{"query": "--type", "chat_id": "123@s.whatsapp.net", "sender_id": "555@s.whatsapp.net",
		"after": "2026-01-01", "before": "2026-02-01", "has_media": true, "media_type": "image"}
	first := call("search_messages", query)["structuredContent"].(map[string]any)
	query["cursor"] = first["next_cursor"]
	call("search_messages", query)
	if len(e.calls) != 2 {
		t.Fatalf("expected first page and resumed page, got %v", e.calls)
	}
	for _, args := range e.calls {
		sep := separatorIndex(args)
		if sep < 0 || !reflect.DeepEqual(args[sep:], []string{"--", "--type"}) {
			t.Fatalf("only the search text may follow the separator: %q", args)
		}
		for _, flag := range []string{"--chat", "--limit", "--after", "--before", "--from", "--has-media", "--type"} {
			if i := indexOf(args[:sep], flag); i < 0 {
				t.Fatalf("flag %s missing before separator: %q", flag, args)
			}
		}
	}
	if indexOf(e.calls[1][:separatorIndex(e.calls[1])], "--cursor") < 0 {
		t.Fatalf("resumed search must pass the upstream cursor before the separator: %q", e.calls[1])
	}
}

func TestReadMessagesListUnchanged(t *testing.T) {
	e, call := searchFixture(t)
	call("read_messages", map[string]any{"chat_id": "123@s.whatsapp.net", "sender_id": "555@s.whatsapp.net", "after": "2026-01-01", "before": "2026-02-01"})
	if len(e.calls) != 1 {
		t.Fatalf("expected one archive call, got %v", e.calls)
	}
	args := e.calls[0]
	want := []string{"messages", "list", "--chat", "123@s.whatsapp.net", "--limit", args[5], "--after", "2026-01-01", "--before", "2026-02-01", "--sender", "555@s.whatsapp.net"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("read_messages arguments changed: %q", args)
	}
}

func indexOf(args []string, value string) int {
	for i, a := range args {
		if a == value {
			return i
		}
	}
	return -1
}

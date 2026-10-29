package service

import (
	"context"
	"encoding/json"
	"github.com/massi/localwhatsappmcp/internal/engine"
	"net/http/httptest"
	"strings"
	"testing"
)

type connectionOnlyFixture struct{ manualFixtureEngine }

func (connectionOnlyFixture) Call(context.Context, bool, ...string) (json.RawMessage, error) {
	return json.RawMessage(`{"messages":[],"page":{"has_more":false},"source":{"scope":"retained","last_sync_at":"2026-01-01T00:00:00Z"}}`), nil
}

func TestPrivateAndMCPExposeConnectionWithoutCompletionMetadata(t *testing.T) {
	s, err := New(Options{DataDir: t.TempDir(), LocalURL: "http://127.0.0.1:32191", Engine: connectionOnlyFixture{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.applyScope(t.Context(), engine.Policy{Mode: "all"}); err != nil {
		t.Fatal(err)
	}
	credentials := createManualFixture(t, s, "Connection fixture")
	private := manualControl(t, s, "GET", "/state", nil)
	for _, name := range []string{"check_status", "read_messages"} {
		args := `{}`
		if name == "read_messages" {
			args = `{"chat_id":"123@s.whatsapp.net"}`
		}
		req := httptest.NewRequest("POST", s.LocalURL()+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`))
		req.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		response := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(response, req)
		if response.Code != 200 || strings.Contains(response.Body.String(), `"isError":true`) {
			t.Fatalf("%s response=%s", name, response.Body.String())
		}
		for _, body := range []string{private.Body.String(), response.Body.String()} {
			for _, field := range []string{"sync_state", "last_synced_at", "last_sync_at"} {
				if strings.Contains(body, field) {
					t.Fatalf("%s exposed removed field %s", name, field)
				}
			}
		}
	}
}

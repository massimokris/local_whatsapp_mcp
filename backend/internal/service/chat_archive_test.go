package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

func TestConversationArchiveMetadataMirrorsCatalogWithoutChangingScope(t *testing.T) {
	const archivedID = "123@s.whatsapp.net"
	const activeID = "456@g.us"
	const hiddenID = "999@s.whatsapp.net"
	e := &chatArchiveFixtureEngine{chats: []engine.Chat{
		{ID: archivedID, Name: "Archived fixture", Kind: "direct", Archived: true},
		{ID: activeID, Name: "Active fixture", Kind: "group", Archived: false},
		{ID: hiddenID, Name: "Hidden fixture", Kind: "direct", Archived: true},
	}}
	s, err := New(Options{DataDir: t.TempDir(), LocalURL: "http://127.0.0.1:32191", Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.applyScope(context.Background(), engine.Policy{Mode: "selected", ChatIDs: []string{archivedID, activeID}}); err != nil {
		t.Fatal(err)
	}
	credentials := createManualFixture(t, s, "Archive metadata fixture")
	scopeBefore := s.State()["scope"]

	findConversations := func() map[string]bool {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, s.LocalURL()+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"find_conversations","arguments":{}}}`))
		request.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		response := httptest.NewRecorder()
		s.LocalHandler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("find_conversations status %d", response.Code)
		}
		var result struct {
			Result struct {
				IsError           bool            `json:"isError"`
				StructuredContent json.RawMessage `json:"structuredContent"`
			} `json:"result"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Result.IsError || len(result.Error) != 0 {
			t.Fatalf("find_conversations failed: %s", response.Body)
		}
		return decodeChatArchiveFlags(t, result.Result.StructuredContent)
	}

	if got, want := findConversations(), map[string]bool{archivedID: true, activeID: false}; !reflect.DeepEqual(got, want) {
		t.Fatalf("scoped archive metadata: got %v, want %v", got, want)
	}
	// Sync changes metadata for the same chats; archive status does not grant or
	// revoke conversation access and must not require re-confirming chat scope.
	e.chats[0].Archived = false
	e.chats[1].Archived = true
	if got, want := findConversations(), map[string]bool{archivedID: false, activeID: true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("updated scoped archive metadata: got %v, want %v", got, want)
	}
	if got := s.State()["scope"]; !reflect.DeepEqual(got, scopeBefore) {
		t.Fatalf("metadata update changed scope: got %v, want %v", got, scopeBefore)
	}

	// The private native catalog remains the complete metadata catalog, while
	// find_conversations above must keep the unselected chat hidden.
	response := manualControl(t, s, http.MethodGet, "/catalog", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("private catalog status %d", response.Code)
	}
	if got, want := decodeChatArchiveFlags(t, response.Body.Bytes()), map[string]bool{archivedID: false, activeID: true, hiddenID: true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("private archive metadata: got %v, want %v", got, want)
	}
}

func decodeChatArchiveFlags(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	var catalog struct {
		Chats []struct {
			ID       string `json:"id"`
			Archived *bool  `json:"archived"`
		} `json:"chats"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	flags := make(map[string]bool, len(catalog.Chats))
	for _, chat := range catalog.Chats {
		if chat.Archived == nil {
			t.Fatalf("conversation %q omitted its archived field", chat.ID)
		}
		if _, duplicate := flags[chat.ID]; duplicate {
			t.Fatalf("duplicate conversation %q", chat.ID)
		}
		flags[chat.ID] = *chat.Archived
	}
	return flags
}

type chatArchiveFixtureEngine struct {
	manualFixtureEngine
	chats []engine.Chat
}

func (e *chatArchiveFixtureEngine) Catalog(context.Context) ([]engine.Chat, error) {
	return append([]engine.Chat(nil), e.chats...), nil
}

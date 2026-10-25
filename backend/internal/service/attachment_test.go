package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLegacyAttachmentHandleStillServesAuthorizedCachedBytes(t *testing.T) {
	s, options := httpPairingFixture(t)
	defer s.Close()
	connection, err := s.createManualConnection("Existing agent")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(options.DataDir, "attachments")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "legacy-cached-file"), []byte("cached attachment bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	// Previously issued URLs include message/name fields in their encrypted
	// payload. Removing that unused metadata must not invalidate those URLs.
	const legacyPayload = `{"file":"legacy-cached-file","chat":"123@s.whatsapp.net","message":"legacy-message","mime":"text/plain","name":"attachment"}`
	token := s.seal(cursor{Agent: connection.ID, Revision: s.cfg.Scope.Revision, Query: "attachment", Upstream: legacyPayload, Expires: time.Now().Add(time.Hour).Unix()})
	req := httptest.NewRequest(http.MethodGet, s.LocalURL()+"/attachments/"+token, nil)
	req.Header.Set("Authorization", "Bearer "+connection.AccessToken)
	w := httptest.NewRecorder()
	s.LocalHandler().ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "cached attachment bytes" {
		t.Fatalf("old attachment handle stopped serving its cache: %d %s", w.Code, w.Body)
	}
	if w.Header().Get("Content-Type") != "text/plain" || w.Header().Get("Content-Disposition") != `attachment; filename="attachment"` {
		t.Fatalf("old attachment handle lost its response headers: %v", w.Header())
	}
}

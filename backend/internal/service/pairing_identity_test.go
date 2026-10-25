package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func pairingIdentityControl(t *testing.T, s *Service, method, path string, body any, out any) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.ControlHandler().ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewReader(payload)))
	if w.Code != http.StatusOK {
		t.Fatalf("%s %s returned %d", method, path, w.Code)
	}
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPairingIdentityDistinguishesSameNameInvitationsAcrossRestart(t *testing.T) {
	options := Options{DataDir: t.TempDir()}
	s, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	pairingIdentityControl(t, s, "PUT", "/scope", map[string]string{"mode": "all"}, nil)
	type inviteResponse struct {
		ID    string `json:"invitation_id"`
		Token string `json:"token"`
	}
	var a, b inviteResponse
	for _, out := range []*inviteResponse{&a, &b} {
		pairingIdentityControl(t, s, "POST", "/pairing", map[string]string{"name": "My agent", "client": "stdio"}, out)
		if out.ID == "" || out.Token == "" || out.ID == out.Token {
			t.Fatal("invitation must have independent identity and redemption token")
		}
	}
	if a.ID == b.ID {
		t.Fatal("same-name invitations share an identity")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(options)
	if err != nil {
		t.Fatal(err)
	}
	// Knowing the non-secret correlation identifier never authorizes setup.
	w := httptest.NewRecorder()
	payload, _ := json.Marshal(map[string]string{"token": a.ID})
	s.ControlHandler().ServeHTTP(w, httptest.NewRequest("POST", "/pairing/redeem", bytes.NewReader(payload)))
	if w.Code != http.StatusUnauthorized {
		t.Fatal("invitation identity was accepted as a redemption token")
	}
	for index, invitation := range []inviteResponse{a, b} {
		var grant map[string]string
		pairingIdentityControl(t, s, "POST", "/pairing/redeem", map[string]string{"token": invitation.Token}, &grant)
		var state struct {
			Agents []Grant `json:"agents"`
		}
		pairingIdentityControl(t, s, "GET", "/state", nil, &state)
		if len(state.Agents) != index+1 {
			t.Fatal("redemption changed the wrong number of connections")
		}
		matched := false
		for _, agent := range state.Agents {
			if agent.ID == grant["id"] {
				matched = agent.PairingInvitationID == invitation.ID
			}
			if index == 0 && agent.PairingInvitationID == b.ID {
				t.Fatal("redeeming A made pending invitation B appear completed")
			}
		}
		if !matched {
			t.Fatal("private state does not identify the redeemed invitation")
		}
	}
}

func TestLegacyPairingWithoutIdentityStillRedeemsAndAuthenticates(t *testing.T) {
	options := Options{DataDir: t.TempDir()}
	s, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	const legacyToken = "synthetic-legacy-invitation-token"
	// Use the old settings format directly so its client metadata remains in
	// the fixture even though current invitations no longer retain that field.
	expires := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	legacySettings := `{"version":1,"credential_storage_version":1,"scope":{"version":1,"revision":2,"mode":"all","chat_ids":[]},"grants":{},"invitations":{"` + digest(legacyToken) + `":{"name":"Legacy agent","client":"stdio","expires":"` + expires + `"}},"sends":{}}`
	if err := os.WriteFile(filepath.Join(options.DataDir, "settings.json"), []byte(legacySettings), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(options)
	if err != nil {
		t.Fatal(err)
	}
	var grant map[string]string
	pairingIdentityControl(t, s, "POST", "/pairing/redeem", map[string]string{"token": legacyToken}, &grant)
	if grant["client"] != "generic" {
		t.Fatal("legacy invitation client metadata changed generic pairing")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(options)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := s.VerifyToken(context.Background(), grant["secret"]); err != nil || id != grant["id"] {
		t.Fatal("legacy grant without invitation identity stopped authenticating", err)
	}
	var state struct {
		Agents []map[string]any `json:"agents"`
	}
	pairingIdentityControl(t, s, "GET", "/state", nil, &state)
	if len(state.Agents) != 1 {
		t.Fatal("legacy grant was not retained")
	}
	if _, ok := state.Agents[0]["pairing_invitation_id"]; ok {
		t.Fatal("legacy grant was assigned an invented invitation identity")
	}
}

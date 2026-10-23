package cloud

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestOwnerHandoffIsSingleUseAndBoundToExpectedClient(t *testing.T) {
	o, err := NewOAuth(OAuthOptions{Issuer: "https://bridge.example", Path: t.TempDir() + "/oauth.json", GrantActive: func(id string) bool { return id == "agent-a" }})
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := o.Prepare("agent-a", "https://client.example/metadata", []string{"https://client.example/callback"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "https://bridge.example/oauth/bootstrap", strings.NewReader(url.Values{"handoff": {handoff}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	o.ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther || len(w.Result().Cookies()) != 1 {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body.String())
	}
	replay := httptest.NewRequest("POST", "https://bridge.example/oauth/bootstrap", strings.NewReader(url.Values{"handoff": {handoff}}.Encode()))
	replay.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rw := httptest.NewRecorder()
	o.ServeHTTP(rw, replay)
	if rw.Code != http.StatusForbidden {
		t.Fatalf("replay: %d", rw.Code)
	}
	cookie := w.Result().Cookies()[0]
	wrong := httptest.NewRequest("GET", "https://bridge.example/oauth/authorize?client_id=other&redirect_uri=https%3A%2F%2Fclient.example%2Fcallback&response_type=code&code_challenge="+strings.Repeat("a", 43)+"&code_challenge_method=S256&resource=https%3A%2F%2Fbridge.example%2Fmcp", nil)
	wrong.AddCookie(cookie)
	dw := httptest.NewRecorder()
	o.ServeHTTP(dw, wrong)
	if dw.Code < 400 {
		t.Fatalf("wrong client admitted: %d", dw.Code)
	}
}

func TestDiscoveryAdvertisesOnlyPKCECodeFlow(t *testing.T) {
	o, err := NewOAuth(OAuthOptions{Issuer: "https://bridge.example", Path: t.TempDir() + "/oauth.json", GrantActive: func(string) bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	o.ServeHTTP(w, httptest.NewRequest("GET", "https://bridge.example/.well-known/oauth-authorization-server", nil))
	var d map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d["issuer"] != "https://bridge.example" || d["authorization_response_iss_parameter_supported"] != true {
		t.Fatal(d)
	}
	if got := d["code_challenge_methods_supported"].([]any); len(got) != 1 || got[0] != "S256" {
		t.Fatal(got)
	}
}

func TestOAuthPKCEFlowPersistsHashedCredentialsAndRevokesImmediately(t *testing.T) {
	path := t.TempDir() + "/oauth.json"
	active := true
	opt := OAuthOptions{Issuer: "https://bridge.example", Path: path, GrantActive: func(id string) bool { return active && id == "agent-a" }}
	o, err := NewOAuth(opt)
	if err != nil {
		t.Fatal(err)
	}
	post := func(path string, values url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://bridge.example"+path, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Origin", "https://bridge.example")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		o.ServeHTTP(w, r)
		return w
	}
	reg := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "https://bridge.example/oauth/register", strings.NewReader(`{"client_name":"Test cloud","redirect_uris":["https://client.example/callback"],"token_endpoint_auth_method":"none"}`))
	o.ServeHTTP(reg, r)
	if reg.Code != 201 {
		t.Fatalf("registration: %d %s", reg.Code, reg.Body.String())
	}
	var client struct {
		ID string `json:"client_id"`
	}
	json.Unmarshal(reg.Body.Bytes(), &client)
	h, err := o.Prepare("agent-a", client.ID, []string{"https://client.example/callback"})
	if err != nil {
		t.Fatal(err)
	}
	boot := post("/oauth/bootstrap", url.Values{"handoff": {h}}, nil)
	cookie := boot.Result().Cookies()[0]
	verifier := strings.Repeat("v", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	params := url.Values{"client_id": {client.ID}, "redirect_uri": {"https://client.example/callback"}, "response_type": {"code"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}, "resource": {"https://bridge.example/mcp"}, "state": {"return-state"}, "scope": {"whatsapp.access"}}
	ar := httptest.NewRequest("GET", "https://bridge.example/oauth/authorize?"+params.Encode(), nil)
	ar.AddCookie(cookie)
	aw := httptest.NewRecorder()
	o.ServeHTTP(aw, ar)
	if aw.Code != 200 {
		t.Fatalf("authorize: %d %s", aw.Code, aw.Body.String())
	}
	matches := regexp.MustCompile(`name="request" value="([^"]+)"`).FindStringSubmatch(aw.Body.String())
	if len(matches) != 2 {
		t.Fatal(aw.Body.String())
	}
	csrf := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(aw.Body.String())
	if len(csrf) != 2 {
		t.Fatal(aw.Body.String())
	}
	denied := post("/oauth/consent", url.Values{"request": {matches[1]}, "csrf": {"wrong"}}, cookie)
	if denied.Code != 403 {
		t.Fatal("CSRF accepted")
	}
	consent := post("/oauth/consent", url.Values{"request": {matches[1]}, "csrf": {csrf[1]}}, cookie)
	if consent.Code != 302 {
		t.Fatalf("consent: %d %s", consent.Code, consent.Body.String())
	}
	redirect, _ := url.Parse(consent.Header().Get("Location"))
	if redirect.Query().Get("iss") != opt.Issuer || redirect.Query().Get("state") != "return-state" {
		t.Fatal(redirect)
	}
	code := redirect.Query().Get("code")
	exchange := url.Values{"grant_type": {"authorization_code"}, "client_id": {client.ID}, "redirect_uri": {"https://client.example/callback"}, "code": {code}, "code_verifier": {verifier}, "resource": {"https://bridge.example/mcp"}}
	tw := post("/oauth/token", exchange, nil)
	if tw.Code != 200 {
		t.Fatalf("token: %d %s", tw.Code, tw.Body.String())
	}
	var tokens map[string]any
	json.Unmarshal(tw.Body.Bytes(), &tokens)
	access := tokens["access_token"].(string)
	refresh := tokens["refresh_token"].(string)
	replay := post("/oauth/token", exchange, nil)
	if replay.Code == 200 {
		t.Fatal("code replay accepted")
	}
	content, _ := os.ReadFile(path)
	if bytes.Contains(content, []byte(access)) || bytes.Contains(content, []byte(refresh)) || bytes.Contains(content, []byte(code)) {
		t.Fatal("recoverable credentials on disk")
	}
	o, err = NewOAuth(opt)
	if err != nil {
		t.Fatal(err)
	}
	if id, e := o.Verify(context.Background(), access); e != nil || id != "agent-a" {
		t.Fatalf("restart: %s %v", id, e)
	}
	rw := post("/oauth/token", url.Values{"grant_type": {"refresh_token"}, "client_id": {client.ID}, "refresh_token": {refresh}, "resource": {"https://bridge.example/mcp"}}, nil)
	if rw.Code != 200 {
		t.Fatalf("refresh: %d %s", rw.Code, rw.Body.String())
	}
	if _, err = o.Verify(context.Background(), access); err == nil {
		t.Fatal("rotated access remains valid")
	}
	var rotated map[string]any
	json.Unmarshal(rw.Body.Bytes(), &rotated)
	active = false
	if _, err = o.Verify(context.Background(), rotated["access_token"].(string)); err == nil {
		t.Fatal("revoked grant accepted")
	}
}

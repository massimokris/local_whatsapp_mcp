// Package cloud implements the optional public connection without exposing app control.
package cloud

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/massi/localwhatsappmcp/internal/persistence"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type OAuthOptions struct {
	Issuer      string
	Path        string
	GrantActive func(string) bool
	Now         func() time.Time
}
type ownerSession struct {
	Grant     string
	Client    string
	Redirects []string
	Expires   time.Time
}
type OAuth struct {
	mu       sync.Mutex
	opt      OAuthOptions
	handoffs map[string]ownerSession
	sessions map[string]ownerSession
	store    *oauthStore
	manager  *manage.Manager
	requests map[string]consentRequest
}

func opaque() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func fingerprint(s string) string { d := sha256.Sum256([]byte(s)); return hex.EncodeToString(d[:]) }
func validHTTPS(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
func NewOAuth(opt OAuthOptions) (*OAuth, error) {
	u, e := url.Parse(opt.Issuer)
	if e != nil || !validHTTPS(opt.Issuer) || u.RawQuery != "" || u.Path != "" {
		return nil, errors.New("cloud issuer must be a bare HTTPS origin")
	}
	if opt.GrantActive == nil {
		return nil, errors.New("grant verification is required")
	}
	o := &OAuth{opt: opt, handoffs: map[string]ownerSession{}, sessions: map[string]ownerSession{}}
	if e := o.initProtocol(); e != nil {
		return nil, e
	}
	return o, nil
}

// Prepare is private IPC only. The handoff is written directly to a temporary
// browser POST document, never to diagnostic output or an authorization URL.
func (o *OAuth) Prepare(grant, client string, redirects []string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.prune()
	if !o.opt.GrantActive(grant) || len(redirects) == 0 {
		return "", errors.New("cloud setup is not authorized")
	}
	for _, r := range redirects {
		if !validHTTPS(r) {
			return "", errors.New("an exact HTTPS redirect URI is required")
		}
	}
	token := opaque()
	o.handoffs[fingerprint(token)] = ownerSession{grant, client, append([]string(nil), redirects...), o.now().Add(10 * time.Minute)}
	return token, nil
}
func (o *OAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.prune()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	switch r.URL.Path {
	case "/.well-known/oauth-authorization-server":
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return
		}
		b := o.opt.Issuer
		writeJSON(w, 200, map[string]any{"issuer": b, "authorization_endpoint": b + "/oauth/authorize", "token_endpoint": b + "/oauth/token", "registration_endpoint": b + "/oauth/register", "response_types_supported": []string{"code"}, "grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"}, "token_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": []string{"whatsapp.access"}, "client_id_metadata_document_supported": true, "authorization_response_iss_parameter_supported": true})
	case "/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp":
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return
		}
		writeJSON(w, 200, map[string]any{"resource": o.opt.Issuer + "/mcp", "authorization_servers": []string{o.opt.Issuer}, "scopes_supported": []string{"whatsapp.access"}, "bearer_methods_supported": []string{"header"}})
	case "/oauth/bootstrap":
		o.bootstrap(w, r)
	case "/oauth/ready":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><title>LocalWhatsAppMCP</title><body><h1>Ready to connect</h1><p>Continue connecting your cloud agent in this browser. This window can stay open. Reading starts when you finish the connection; sending stays off.</p></body></html>`))
	case "/oauth/authorize":
		o.authorize(w, r)
	case "/oauth/consent":
		o.consent(w, r)
	case "/oauth/register":
		o.register(w, r)
	case "/oauth/token":
		o.token(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (o *OAuth) bootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if e := r.ParseForm(); e != nil {
		http.Error(w, "invalid handoff", 400)
		return
	}
	k := fingerprint(r.PostForm.Get("handoff"))
	s, ok := o.handoffs[k]
	delete(o.handoffs, k)
	if !ok || o.now().After(s.Expires) || !o.opt.GrantActive(s.Grant) {
		http.Error(w, "This private setup handoff has expired or was already used.", 403)
		return
	}
	token := opaque()
	o.sessions[fingerprint(token)] = s
	http.SetCookie(w, &http.Cookie{Name: "__Host-lwm-owner", Value: token, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, "/oauth/ready", http.StatusSeeOther)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func atomicJSON(path string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	return persistence.Write(path, b, 0600)
}
func contains(a []string, s string) bool {
	for _, v := range a {
		if v == s {
			return true
		}
	}
	return false
}
func cleanOrigin(s string) string { return strings.TrimRight(s, "/") }

func (o *OAuth) prune() {
	now := o.now()
	for k, v := range o.handoffs {
		if now.After(v.Expires) {
			delete(o.handoffs, k)
		}
	}
	for k, v := range o.sessions {
		if now.After(v.Expires) {
			delete(o.sessions, k)
		}
	}
	for k, v := range o.requests {
		if now.After(v.Expires) {
			delete(o.requests, k)
		}
	}
}

func (o *OAuth) now() time.Time {
	if o.opt.Now != nil {
		return o.opt.Now()
	}
	return time.Now()
}

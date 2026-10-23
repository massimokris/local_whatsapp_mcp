package cloud

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-oauth2/oauth2/v4"
	"github.com/go-oauth2/oauth2/v4/manage"
)

type consentRequest struct {
	Owner, CSRF string
	Params      url.Values
	Expires     time.Time
}

func (o *OAuth) initProtocol() error {
	var e error
	o.store, e = newOAuthStore(o.opt.Path, o.opt.Issuer)
	if e != nil {
		return e
	}
	o.manager = manage.NewDefaultManager()
	o.manager.MapClientStorage(o.store)
	o.manager.MapTokenStorage(o.store)
	o.manager.SetAuthorizeCodeExp(2 * time.Minute)
	o.manager.SetAuthorizeCodeTokenCfg(&manage.Config{AccessTokenExp: 15 * time.Minute, RefreshTokenExp: 30 * 24 * time.Hour, IsGenerateRefresh: true})
	o.manager.SetRefreshTokenCfg(&manage.RefreshingConfig{AccessTokenExp: 15 * time.Minute, IsGenerateRefresh: true, IsRemoveAccess: true, IsRemoveRefreshing: true})
	o.manager.SetValidateURIHandler(func(registered, redirect string) error {
		if !contains(strings.Split(registered, "\n"), redirect) {
			return errors.New("redirect URI is not registered")
		}
		return nil
	})
	o.manager.SetExtractExtensionHandler(func(req *oauth2.TokenGenerateRequest, t oauth2.ExtendableTokenInfo) {
		v := t.GetExtension()
		if v == nil {
			v = url.Values{}
		}
		v.Set("resource", o.opt.Issuer+"/mcp")
		t.SetExtension(v)
	})
	o.requests = map[string]consentRequest{}
	return nil
}
func (o *OAuth) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if len(o.store.data.Clients) >= 256 {
		http.Error(w, "registration limit reached", 429)
		return
	}
	var c clientRecord
	if json.NewDecoder(r.Body).Decode(&c) != nil || len(c.Redirects) == 0 || len(c.Redirects) > 8 || len(c.Name) > 120 || c.AuthMethod != "" && c.AuthMethod != "none" {
		oauthError(w, "invalid_client_metadata")
		return
	}
	for _, redirect := range c.Redirects {
		if !validHTTPS(redirect) {
			oauthError(w, "invalid_redirect_uri")
			return
		}
	}
	c.ID = opaque()
	c.AuthMethod = "none"
	if c.Name == "" {
		c.Name = "Cloud agent"
	}
	o.store.data.Clients[c.ID] = c
	if e := o.store.save(); e != nil {
		delete(o.store.data.Clients, c.ID)
		http.Error(w, "could not save client registration", 500)
		return
	}
	writeJSON(w, 201, map[string]any{"client_id": c.ID, "client_name": c.Name, "redirect_uris": c.Redirects, "token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}})
}
func (o *OAuth) authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	q := r.URL.Query()
	cookie, e := r.Cookie("__Host-lwm-owner")
	if e != nil {
		http.Error(w, "Prepare this browser with the LocalWhatsAppMCP cloud setup helper, then connect your agent again.", 403)
		return
	}
	owner := fingerprint(cookie.Value)
	session, ok := o.sessions[owner]
	if !ok || o.now().After(session.Expires) || !o.opt.GrantActive(session.Grant) {
		http.Error(w, "Cloud setup expired. Run the setup helper again.", 403)
		return
	}
	if session.Client != "" && session.Client != q.Get("client_id") || !contains(session.Redirects, q.Get("redirect_uri")) {
		http.Error(w, "This setup belongs to a different client or redirect URI.", 403)
		return
	}
	if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || len(q.Get("code_challenge")) != 43 || q.Get("resource") != o.opt.Issuer+"/mcp" || q.Get("scope") != "" && q.Get("scope") != "whatsapp.access" {
		oauthError(w, "invalid_request")
		return
	}
	if _, e := base64Challenge(q.Get("code_challenge")); e != nil {
		oauthError(w, "invalid_request")
		return
	}
	c, e := o.client(r.Context(), q.Get("client_id"))
	if e != nil || !contains(c.Redirects, q.Get("redirect_uri")) {
		oauthError(w, "invalid_client")
		return
	}
	if len(o.requests) >= 128 {
		http.Error(w, "too many pending requests", 429)
		return
	}
	id, csrf := opaque(), opaque()
	o.requests[id] = consentRequest{Owner: owner, CSRF: fingerprint(csrf), Params: q, Expires: o.now().Add(5 * time.Minute)}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = consentTemplate.Execute(w, map[string]string{"Name": c.Name, "Client": c.ID, "Redirect": q.Get("redirect_uri"), "Request": id, "CSRF": csrf})
}

var consentTemplate = template.Must(template.New("consent").Parse(`<!doctype html><html><head><meta name="viewport" content="width=device-width"><title>Connect to LocalWhatsAppMCP</title><style>body{font:14px ui-monospace,monospace;line-height:1.6;max-width:520px;margin:60px auto;padding:24px;background:#fafafa;color:#151515}button{font:inherit;padding:12px 24px}small{overflow-wrap:anywhere;color:#737373}@media(prefers-color-scheme:dark){body{background:#0f0f0f;color:#f5f5f5}}</style></head><body><h1>Connect {{.Name}}</h1><p>Read your selected WhatsApp chats and retrieve attachments. Sending stays disabled until you enable it in the app.</p><small>Client: {{.Client}}<br>Return to: {{.Redirect}}</small><form method="post" action="/oauth/consent"><input type="hidden" name="request" value="{{.Request}}"><input type="hidden" name="csrf" value="{{.CSRF}}"><p><button type="submit">Connect</button></p></form></body></html>`))

func (o *OAuth) consent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.Header.Get("Origin") != o.opt.Issuer {
		http.Error(w, "invalid consent origin", 403)
		return
	}
	if r.ParseForm() != nil {
		oauthError(w, "invalid_request")
		return
	}
	cookie, e := r.Cookie("__Host-lwm-owner")
	if e != nil {
		http.Error(w, "invalid browser session", 403)
		return
	}
	id := r.PostForm.Get("request")
	request, ok := o.requests[id]
	owner := fingerprint(cookie.Value)
	session, sok := o.sessions[owner]
	if !ok || !sok || request.Owner != owner || o.now().After(request.Expires) || o.now().After(session.Expires) || subtle.ConstantTimeCompare([]byte(request.CSRF), []byte(fingerprint(r.PostForm.Get("csrf")))) != 1 || !o.opt.GrantActive(session.Grant) {
		http.Error(w, "invalid or expired consent", 403)
		return
	}
	delete(o.requests, id)
	delete(o.sessions, owner)
	p := request.Params
	ti, e := o.manager.GenerateAuthToken(r.Context(), oauth2.Code, &oauth2.TokenGenerateRequest{ClientID: p.Get("client_id"), UserID: session.Grant, RedirectURI: p.Get("redirect_uri"), Scope: "whatsapp.access", CodeChallenge: p.Get("code_challenge"), CodeChallengeMethod: oauth2.CodeChallengeS256, Request: r})
	if e != nil {
		http.Error(w, "could not authorize this connection", 500)
		return
	}
	dest, _ := url.Parse(p.Get("redirect_uri"))
	q := dest.Query()
	q.Set("code", ti.GetCode())
	q.Set("state", p.Get("state"))
	q.Set("iss", o.opt.Issuer)
	dest.RawQuery = q.Encode()
	http.SetCookie(w, &http.Cookie{Name: "__Host-lwm-owner", Value: "", Path: "/", Secure: true, HttpOnly: true, MaxAge: -1})
	http.Redirect(w, r, dest.String(), 302)
}
func (o *OAuth) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if r.ParseForm() != nil {
		oauthError(w, "invalid_request")
		return
	}
	p := r.PostForm
	if p.Get("resource") != o.opt.Issuer+"/mcp" || p.Get("client_secret") != "" || r.Header.Get("Authorization") != "" {
		oauthError(w, "invalid_target")
		return
	}
	var ti oauth2.TokenInfo
	var e error
	switch p.Get("grant_type") {
	case "authorization_code":
		prior, err := o.store.GetByCode(r.Context(), p.Get("code"))
		if err != nil || prior.GetClientID() != p.Get("client_id") || prior.GetRedirectURI() != p.Get("redirect_uri") || !o.opt.GrantActive(prior.GetUserID()) || len(p.Get("code_verifier")) < 43 || len(p.Get("code_verifier")) > 128 {
			oauthError(w, "invalid_grant")
			return
		}
		ti, e = o.manager.GenerateAccessToken(r.Context(), oauth2.AuthorizationCode, &oauth2.TokenGenerateRequest{ClientID: p.Get("client_id"), Code: p.Get("code"), CodeVerifier: p.Get("code_verifier"), RedirectURI: p.Get("redirect_uri"), Request: r})
	case "refresh_token":
		prior, err := o.manager.LoadRefreshToken(r.Context(), p.Get("refresh_token"))
		if err != nil || prior.GetClientID() != p.Get("client_id") || !o.opt.GrantActive(prior.GetUserID()) || p.Get("scope") != "" && p.Get("scope") != "whatsapp.access" {
			oauthError(w, "invalid_grant")
			return
		}
		ti, e = o.manager.RefreshAccessToken(r.Context(), &oauth2.TokenGenerateRequest{ClientID: p.Get("client_id"), Refresh: p.Get("refresh_token"), Request: r})
	default:
		oauthError(w, "unsupported_grant_type")
		return
	}
	if e != nil {
		oauthError(w, "invalid_grant")
		return
	}
	writeJSON(w, 200, map[string]any{"access_token": ti.GetAccess(), "token_type": "Bearer", "expires_in": int64(ti.GetAccessExpiresIn().Seconds()), "refresh_token": ti.GetRefresh(), "scope": "whatsapp.access", "resource": o.opt.Issuer + "/mcp"})
}
func (o *OAuth) Verify(ctx context.Context, token string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if strings.HasPrefix(token, "digest:") {
		return "", errors.New("invalid access token")
	}
	ti, e := o.manager.LoadAccessToken(ctx, token)
	if e != nil {
		return "", errors.New("invalid access token")
	}
	extended, ok := ti.(oauth2.ExtendableTokenInfo)
	if !ok || extended.GetExtension().Get("resource") != o.opt.Issuer+"/mcp" || ti.GetScope() != "whatsapp.access" || !o.opt.GrantActive(ti.GetUserID()) {
		return "", errors.New("invalid or revoked grant")
	}
	return ti.GetUserID(), nil
}
func oauthError(w http.ResponseWriter, code string) {
	writeJSON(w, 400, map[string]string{"error": code})
}

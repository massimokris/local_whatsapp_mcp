package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/massi/localwhatsappmcp/internal/engine"
	"github.com/massi/localwhatsappmcp/internal/persistence"
	"github.com/massi/localwhatsappmcp/internal/secret"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Grant struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Client      string `json:"client"`
	SendEnabled bool   `json:"send_enabled"`
	Connected   bool   `json:"connected"`
	LastSeenAt  string `json:"last_seen_at,omitempty"`
	Digest      string `json:"digest,omitempty"`
}
type invitation struct {
	Name    string    `json:"name"`
	Client  string    `json:"client"`
	Expires time.Time `json:"expires"`
}
type sendRecord struct {
	Hash      string `json:"hash"`
	Status    string `json:"status"`
	MessageID string `json:"message_id,omitempty"`
}
type config struct {
	Version int                   `json:"version"`
	Scope   engine.Policy         `json:"scope"`
	Grants  map[string]Grant      `json:"grants"`
	Invites map[string]invitation `json:"invitations"`
	Sends   map[string]sendRecord `json:"sends"`

	Linked     bool `json:"linked"`
	Transition bool `json:"transition"`
}
type Extension interface {
	PublicHandler() http.Handler
	ControlHandler() http.Handler
	State() any
	Close() error
}
type Options struct {
	CursorKey        []byte // Optional deterministic key for isolated fixtures; production uses Keychain.
	DataDir          string
	HelperPath       string
	Engine           engine.Boundary
	LocalURL         string
	ExtensionFactory func(*Service) (Extension, error)
}
type Service struct {
	cursorKey        []byte
	mu               sync.Mutex
	operations       sync.RWMutex
	cfg              config
	opt              Options
	ext              Extension
	externalVerifier func(context.Context, string) (string, error)
	lastError        string
	publicURL        string
	generation       context.Context
	cancelGeneration context.CancelFunc
	closed           bool
}

var ErrUnauthorized = errors.New("agent is not authorized")
var ErrScope = errors.New("conversation is outside the current chat scope")
var ErrTransition = errors.New("chat scope is being applied; retry after sync recovers")
var ErrSetup = errors.New("confirm chat scope in LocalWhatsAppMCP first")

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func digest(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func New(opt Options) (*Service, error) {
	if opt.DataDir == "" {
		return nil, errors.New("application data directory is required")
	}
	if err := os.MkdirAll(opt.DataDir, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(opt.DataDir, 0700); err != nil {
		return nil, err
	}
	s := &Service{opt: opt}
	s.generation, s.cancelGeneration = context.WithCancel(context.Background())
	b, err := os.ReadFile(filepath.Join(opt.DataDir, "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		for _, p := range []string{"content-policy.json", "store/session.db", "store/wacli.db"} {
			if _, e := os.Stat(filepath.Join(opt.DataDir, p)); e == nil {
				return nil, errors.New("saved configuration is missing; restore settings.json before reopening this archive")
			}
		}
		s.cfg = config{Version: 1, Scope: engine.Policy{Version: 1, Revision: 1, Mode: "initial", ChatIDs: []string{}}, Grants: map[string]Grant{}, Invites: map[string]invitation{}, Sends: map[string]sendRecord{}}
		if err = s.saveLocked(); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if json.Unmarshal(b, &s.cfg) != nil || s.cfg.Version != 1 || s.cfg.Scope.Version != 1 || s.cfg.Scope.Revision < 1 || s.cfg.Grants == nil || s.cfg.Invites == nil || s.cfg.Sends == nil {
		return nil, errors.New("saved configuration is invalid; restore it before restarting")
	}
	if len(opt.CursorKey) > 0 {
		s.cursorKey = append([]byte(nil), opt.CursorKey...)
	} else {
		account := digest(opt.DataDir)
		key, keyErr := secret.Get("com.localwhatsappmcp.cursor", account)
		if errors.Is(keyErr, secret.ErrNotFound) {
			key = randomToken()
			keyErr = secret.Set("com.localwhatsappmcp.cursor", account, key)
		}
		if keyErr != nil {
			return nil, keyErr
		}
		s.cursorKey, keyErr = base64.RawURLEncoding.DecodeString(key)
		if keyErr != nil {
			return nil, errors.New("app credential is invalid in Keychain")
		}
	}
	if len(s.cursorKey) != 32 {
		return nil, errors.New("app cursor key must contain 32 bytes")
	}
	if !validMode(s.cfg.Scope.Mode, true) {
		return nil, errors.New("saved chat scope is invalid")
	}
	if err := s.publishPolicy(); err != nil {
		return nil, err
	}
	if opt.ExtensionFactory != nil {
		s.ext, err = opt.ExtensionFactory(s)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}
func validMode(v string, initial bool) bool {
	return v == "all" || v == "selected" || (initial && v == "initial")
}
func AtomicJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return AtomicWrite(path, append(b, '\n'), 0600)
}
func AtomicWrite(path string, b []byte, mode os.FileMode) error {
	return persistence.Write(path, b, mode)
}
func (s *Service) saveLocked() error {
	return AtomicJSON(filepath.Join(s.opt.DataDir, "settings.json"), s.cfg)
}
func (s *Service) publishPolicy() error {
	return AtomicJSON(filepath.Join(s.opt.DataDir, "content-policy.json"), s.cfg.Scope)
}
func (s *Service) DataDir() string      { return s.opt.DataDir }
func (s *Service) LocalURL() string     { s.mu.Lock(); defer s.mu.Unlock(); return s.opt.LocalURL }
func (s *Service) SetLocalURL(u string) { s.mu.Lock(); s.opt.LocalURL = u; s.mu.Unlock() }
func (s *Service) PublicURL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.publicURL != "" {
		return s.publicURL
	}
	return s.opt.LocalURL
}
func (s *Service) SetPublicURL(u string) { s.mu.Lock(); s.publicURL = u; s.mu.Unlock() }
func (s *Service) SetExternalVerifier(f func(context.Context, string) (string, error)) {
	s.mu.Lock()
	s.externalVerifier = f
	s.mu.Unlock()
}
func (s *Service) CreateCloudGrant(name, client string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Scope.Mode == "initial" {
		return "", ErrSetup
	}
	g := Grant{ID: randomToken(), Name: name, Client: client}
	s.cfg.Grants[g.ID] = g
	if err := s.saveLocked(); err != nil {
		delete(s.cfg.Grants, g.ID)
		return "", err
	}
	return g.ID, nil
}
func (s *Service) VerifyCloudGrant(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.cfg.Grants[id]
	return ok && !s.closed
}
func (s *Service) TouchGrant(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.cfg.Grants[id]
	if ok {
		g.Connected = true
		g.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
		s.cfg.Grants[id] = g
	}
}
func (s *Service) VerifyToken(ctx context.Context, token string) (string, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return "", ErrUnauthorized
	}
	for id, g := range s.cfg.Grants {
		if g.Digest != "" && g.Digest == digest(token) {
			s.mu.Unlock()
			s.TouchGrant(id)
			return id, nil
		}
	}
	verify := s.externalVerifier
	s.mu.Unlock()
	if verify != nil {
		id, err := verify(ctx, token)
		if err == nil && s.VerifyCloudGrant(id) {
			s.TouchGrant(id)
			return id, nil
		}
	}
	return "", ErrUnauthorized
}
func (s *Service) State() map[string]any {
	s.mu.Lock()
	grants := make([]Grant, 0, len(s.cfg.Grants))
	for _, g := range s.cfg.Grants {
		g.Digest = ""
		if t, e := time.Parse(time.RFC3339, g.LastSeenAt); e != nil || time.Since(t) > 5*time.Minute {
			g.Connected = false
		}
		grants = append(grants, g)
	}
	scope := s.cfg.Scope
	err := s.lastError
	url := s.opt.LocalURL
	transition := s.cfg.Transition
	s.mu.Unlock()
	status := engine.Status{Status: "disconnected", Detail: "Connect WhatsApp to import available history."}
	if s.opt.Engine != nil {
		status = s.opt.Engine.Status()
	}
	mcpStatus := "ready"
	if scope.Mode == "initial" {
		mcpStatus = "setup_required"
	}
	if transition {
		mcpStatus = "applying_scope"
	}
	cloud := any(map[string]any{"enabled": false, "status": "disabled", "provider": "", "hostname": "", "detail": ""})
	if s.ext != nil {
		cloud = s.ext.State()
	}
	r := map[string]any{"whatsapp": status, "scope": scope, "agents": grants, "mcp": map[string]any{"url": url, "status": mcpStatus}, "cloud": cloud}
	if err != "" {
		r["error"] = err
	}
	return r
}
func (s *Service) Resume(ctx context.Context, link bool) error {
	s.operations.Lock()
	defer s.operations.Unlock()
	s.mu.Lock()
	p := s.cfg.Scope
	if link {
		s.cfg.Linked = true
	}
	linked := s.cfg.Linked
	if err := s.saveLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	if !linked || s.opt.Engine == nil {
		return nil
	}
	if err := s.opt.Engine.Stop(ctx); err != nil {
		return s.fail(err)
	}
	if err := s.opt.Engine.Start(ctx, p, link || p.Mode == "initial"); err != nil {
		return s.fail(err)
	}
	s.mu.Lock()
	s.cfg.Transition = false
	s.lastError = ""
	s.generation, s.cancelGeneration = context.WithCancel(context.Background())
	err := s.saveLocked()
	s.mu.Unlock()
	return err
}
func (s *Service) fail(err error) error {
	s.mu.Lock()
	s.lastError = err.Error()
	s.mu.Unlock()
	return err
}
func (s *Service) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.cancelGeneration()
	s.mu.Unlock()
	s.operations.Lock()
	defer s.operations.Unlock()
	var err error
	if s.ext != nil {
		err = s.ext.Close()
	}
	if s.opt.Engine != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if e := s.opt.Engine.Stop(ctx); e != nil {
			err = e
		}
	}
	return err
}
func validChat(id string) bool {
	if len(id) > 128 || strings.ContainsAny(id, " \t\r\n/:\\") {
		return false
	}
	parts := strings.Split(id, "@")
	if len(parts) != 2 || parts[0] == "" {
		return false
	}
	return parts[1] == "s.whatsapp.net" || parts[1] == "g.us" || parts[1] == "lid"
}
func (s *Service) applyScope(ctx context.Context, p engine.Policy) error {
	if !validMode(p.Mode, false) {
		return errors.New("scope mode must be all or selected")
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, id := range p.ChatIDs {
		if !validChat(id) {
			return errors.New("choose a valid conversation identity")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	p.ChatIDs = ids
	p.Version = 1
	s.mu.Lock()
	s.cancelGeneration()
	s.cfg.Transition = true
	s.mu.Unlock()
	s.operations.Lock()
	defer s.operations.Unlock()
	s.mu.Lock()
	p.Revision = s.cfg.Scope.Revision + 1
	s.cfg.Scope = p
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return s.fail(err)
	}
	if s.opt.Engine != nil {
		if err = s.opt.Engine.Stop(ctx); err != nil {
			return s.fail(err)
		}
	}
	if err = s.publishPolicy(); err != nil {
		return s.fail(err)
	}
	s.mu.Lock()
	linked := s.cfg.Linked
	s.mu.Unlock()
	if linked && s.opt.Engine != nil {
		if err = s.opt.Engine.Start(ctx, p, false); err != nil {
			return s.fail(err)
		}
	}
	s.mu.Lock()
	s.cfg.Transition = false
	s.lastError = ""
	s.generation, s.cancelGeneration = context.WithCancel(context.Background())
	err = s.saveLocked()
	s.mu.Unlock()
	return err
}
func (s *Service) invite(name, client string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Scope.Mode == "initial" {
		return nil, ErrSetup
	}
	if len(name) > 80 || strings.TrimSpace(name) == "" {
		return nil, errors.New("give the agent a name")
	}
	if client != "codex" && client != "claude" && client != "stdio" {
		return nil, errors.New("choose codex, claude, or stdio")
	}
	token := randomToken()
	expiry := time.Now().Add(10 * time.Minute)
	for k, v := range s.cfg.Invites {
		if time.Now().After(v.Expires) {
			delete(s.cfg.Invites, k)
		}
	}
	s.cfg.Invites[digest(token)] = invitation{Name: name, Client: client, Expires: expiry}
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	instructions := fmt.Sprintf("Connect this agent to LocalWhatsAppMCP. Run the bundled helper %q setup --client %s --data-dir %q --token-stdin and provide this single-use token on standard input (do not put it in command arguments):\n%s\nThe helper installs the credential into macOS Keychain and configures the client. Do not run the headers command in the conversation. This invitation expires at %s; sending starts disabled. Run the helper doctor command after installation. For another stdio client, use the non-secret command printed by setup.", s.opt.HelperPath, client, s.opt.DataDir, token, expiry.UTC().Format(time.RFC3339))
	return map[string]any{"token": token, "instructions": instructions, "expires_at": expiry.UTC().Format(time.RFC3339)}, nil
}
func (s *Service) redeem(token string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := digest(token)
	inv, ok := s.cfg.Invites[key]
	if !ok || time.Now().After(inv.Expires) {
		return nil, ErrUnauthorized
	}
	if s.cfg.Scope.Mode == "initial" {
		return nil, ErrSetup
	}
	id, secret := randomToken(), randomToken()
	g := Grant{ID: id, Name: inv.Name, Client: inv.Client, Digest: digest(secret)}
	delete(s.cfg.Invites, key)
	s.cfg.Grants[id] = g
	if err := s.saveLocked(); err != nil {
		delete(s.cfg.Grants, id)
		return nil, err
	}
	return map[string]string{"id": id, "secret": secret, "client": inv.Client, "url": s.opt.LocalURL}, nil
}
func (s *Service) HelperPath() string { return s.opt.HelperPath }

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
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Grant struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	Client                   string `json:"client"`
	PairingInvitationID      string `json:"pairing_invitation_id,omitempty"`
	SendEnabled              bool   `json:"send_enabled"`
	Connected                bool   `json:"connected"`
	LastSeenAt               string `json:"last_seen_at,omitempty"`
	Digest                   string `json:"digest,omitempty"`
	CredentialStorageVersion int    `json:"credential_storage_version,omitempty"`
}
type invitation struct {
	ID      string    `json:"id,omitempty"`
	Name    string    `json:"name"`
	Expires time.Time `json:"expires"`
}
type sendRecord struct {
	Hash      string `json:"hash"`
	Status    string `json:"status"`
	MessageID string `json:"message_id,omitempty"`
	Error     string `json:"error,omitempty"`
}
type config struct {
	Version                  int                   `json:"version"`
	CredentialStorageVersion int                   `json:"credential_storage_version,omitempty"`
	Scope                    engine.Policy         `json:"scope"`
	Grants                   map[string]Grant      `json:"grants"`
	Invites                  map[string]invitation `json:"invitations"`
	Sends                    map[string]sendRecord `json:"sends"`

	Linked               bool `json:"linked"`
	Transition           bool `json:"transition"`
	WhatsAppDisconnected bool `json:"whatsapp_disconnected,omitempty"`
	DisconnectPending    bool `json:"disconnect_pending,omitempty"`
}
type Options struct {
	CursorKey []byte // Optional deterministic key for isolated fixtures; production persists the key in private SQLite storage.
	DataDir   string
	Engine    engine.Boundary
	LocalURL  string
}
type Service struct {
	cursorKey        []byte
	mu               sync.Mutex
	operations       sync.RWMutex
	cfg              config
	opt              Options
	lastError        string
	generation       context.Context
	cancelGeneration context.CancelFunc
	closed           bool
	disconnecting    bool
	resuming         bool
}

var ErrUnauthorized = errors.New("agent is not authorized")
var ErrScope = errors.New("conversation is outside the current chat scope")
var ErrTransition = errors.New("chat scope is being applied; retry after sync recovers")
var ErrSetup = errors.New("confirm chat scope in LocalWhatsAppMCP first")
var ErrDisconnected = errors.New("WhatsApp is disconnected; connect it in the app before sending messages or downloading attachments")

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
		for _, p := range []string{"content-policy.json", "store/session.db", "store/wacli.db", "credentials.db"} {
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
	// Legacy hosted connections have no local bearer credential and cannot be
	// restored by this local-only service. Retain every local connection.
	for id, grant := range s.cfg.Grants {
		if grant.Client == "cloud" {
			delete(s.cfg.Grants, id)
		}
	}
	if !validMode(s.cfg.Scope.Mode, true) {
		return nil, errors.New("saved chat scope is invalid")
	}
	if err := s.loadCredentials(); err != nil {
		return nil, err
	}
	if err := s.publishPolicy(); err != nil {
		return nil, err
	}
	if s.cfg.Linked && !s.cfg.WhatsAppDisconnected && opt.Engine != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := opt.Engine.RestoreSession(ctx); err != nil {
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
	return persistence.Write(path, append(b, '\n'), 0600)
}
func (s *Service) saveLocked() error {
	return AtomicJSON(filepath.Join(s.opt.DataDir, "settings.json"), s.cfg)
}
func (s *Service) publishPolicy() error {
	return AtomicJSON(filepath.Join(s.opt.DataDir, "content-policy.json"), s.cfg.Scope)
}
func (s *Service) LocalURL() string { return s.opt.LocalURL }
func (s *Service) VerifyToken(_ context.Context, token string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return "", ErrUnauthorized
	}
	tokenDigest := digest(token)
	for id, g := range s.cfg.Grants {
		if g.Digest != "" && g.Digest == tokenDigest {
			g.Connected = true
			g.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
			s.cfg.Grants[id] = g
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
		g.CredentialStorageVersion = 0
		if t, e := time.Parse(time.RFC3339, g.LastSeenAt); e != nil || time.Since(t) > 5*time.Minute {
			g.Connected = false
		}
		grants = append(grants, g)
	}
	scope := s.cfg.Scope
	err := s.lastError
	url := s.opt.LocalURL
	transition := s.cfg.Transition
	resuming, closed := s.resuming, s.closed
	disconnected, pending, disconnecting := s.cfg.WhatsAppDisconnected, s.cfg.DisconnectPending, s.disconnecting
	s.mu.Unlock()
	status := engine.Status{SyncState: engine.SyncUnknown, Status: "disconnected", Detail: "Connect WhatsApp to import available history."}
	if s.opt.Engine != nil {
		status = s.opt.Engine.Status()
	}
	if disconnected {
		syncState := engine.SyncUnknown
		if status.SyncState == engine.SyncError {
			syncState = engine.SyncError
		}
		status = engine.Status{SyncState: syncState, Status: "disconnected", Detail: "WhatsApp is disconnected. Your archive stays on this Mac."}
		if pending {
			// A failed or interrupted logout must still offer Disconnect on reopen.
			status = engine.Status{SyncState: syncState, Status: "attention", Linked: true, DisconnectPending: true, Detail: "WhatsApp sign-out is unfinished. Retry Disconnect."}
		}
		if disconnecting {
			status.Status = "disconnecting"
			status.Detail = "Signing out of this WhatsApp linked device."
		}
	}
	status.SyncState = engine.NormalizeSyncState(status.SyncState)
	if closed || transition || resuming || err != "" {
		if status.SyncState != engine.SyncError {
			status.SyncState = engine.SyncUnknown
		}
	}
	if transition || resuming {
		status.Status = "connecting"
		status.Detail = "Reconnecting to WhatsApp and applying chat scope."
	}
	if status.SyncState == engine.SyncCaughtUp {
		switch status.Status {
		case "ready", "connected", "syncing", "importing", "receiving_history":
		default:
			status.SyncState = engine.SyncUnknown
		}
	}
	mcpStatus := "ready"
	if scope.Mode == "initial" {
		mcpStatus = "setup_required"
	}
	if transition {
		mcpStatus = "applying_scope"
	}
	r := map[string]any{"whatsapp": status, "scope": scope, "agents": grants, "mcp": map[string]any{"url": url, "status": mcpStatus}}
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
	if s.closed {
		s.mu.Unlock()
		return ErrUnauthorized
	}
	if s.cfg.WhatsAppDisconnected && !link {
		s.mu.Unlock()
		return nil
	}
	if link && s.cfg.DisconnectPending {
		s.mu.Unlock()
		return errors.New("finish disconnecting WhatsApp before connecting again; retry Disconnect")
	}
	s.resuming = true
	defer func() {
		s.mu.Lock()
		s.resuming = false
		s.mu.Unlock()
	}()
	candidate := s.cfg
	if link {
		candidate.Linked = true
		candidate.WhatsAppDisconnected = false
	}
	if err := AtomicJSON(filepath.Join(s.opt.DataDir, "settings.json"), candidate); err != nil {
		s.lastError = err.Error()
		s.mu.Unlock()
		return err
	}
	s.cfg = candidate
	linked := s.cfg.Linked && !s.cfg.WhatsAppDisconnected
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

func (s *Service) Disconnect(ctx context.Context) error {
	// Cancel in-flight MCP work before waiting for its operation lock. No command
	// can overlap logout or restart the writer behind the user's disconnect.
	s.mu.Lock()
	s.cancelGeneration()
	s.mu.Unlock()
	s.operations.Lock()
	defer s.operations.Unlock()
	defer func() {
		s.mu.Lock()
		s.disconnecting = false
		s.generation, s.cancelGeneration = context.WithCancel(context.Background())
		s.mu.Unlock()
	}()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrUnauthorized
	}
	if s.cfg.WhatsAppDisconnected && !s.cfg.DisconnectPending {
		s.mu.Unlock()
		return nil
	}
	candidate := s.cfg
	candidate.Linked = false
	candidate.WhatsAppDisconnected = true
	candidate.DisconnectPending = true
	// Persist intent first. Even a crash during logout must never auto-reconnect.
	err := AtomicJSON(filepath.Join(s.opt.DataDir, "settings.json"), candidate)
	if err == nil {
		s.cfg = candidate
		s.disconnecting = true
		s.lastError = ""
	}
	s.mu.Unlock()
	if err != nil {
		return s.fail(errors.New("could not save the disconnect request; retry Disconnect"))
	}
	if s.opt.Engine == nil {
		return s.fail(errors.New("WhatsApp engine is unavailable; reopen the app and retry Disconnect"))
	}
	if err = s.opt.Engine.Stop(ctx); err != nil {
		return s.fail(errors.New("WhatsApp sync has not stopped yet; retry Disconnect"))
	}
	if err = s.opt.Engine.Logout(ctx); err != nil {
		return s.fail(err)
	}
	s.mu.Lock()
	candidate = s.cfg
	candidate.DisconnectPending = false
	err = AtomicJSON(filepath.Join(s.opt.DataDir, "settings.json"), candidate)
	if err == nil {
		s.cfg = candidate
		s.lastError = ""
	}
	s.mu.Unlock()
	if err != nil {
		return s.fail(errors.New("WhatsApp signed out, but the result could not be saved; retry Disconnect"))
	}
	return nil
}

func (s *Service) requireWhatsAppConnection() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.WhatsAppDisconnected {
		return ErrDisconnected
	}
	return nil
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
	linked := s.cfg.Linked && !s.cfg.WhatsAppDisconnected
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
func (s *Service) invite(name string) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrUnauthorized
	}
	if s.cfg.Scope.Mode == "initial" {
		return nil, ErrSetup
	}
	if s.cfg.Transition {
		return nil, ErrTransition
	}
	name = strings.TrimSpace(name)
	if len(name) > 80 || name == "" {
		return nil, errors.New("give the agent a name")
	}
	token, invitationID := randomToken(), randomToken()
	expiry := time.Now().Add(10 * time.Minute)
	candidate := s.cfg
	candidate.Invites = maps.Clone(s.cfg.Invites)
	for k, v := range candidate.Invites {
		if !time.Now().Before(v.Expires) {
			delete(candidate.Invites, k)
		}
	}
	// This identifier only correlates private UI state; the separate token is
	// still the sole authority to redeem the invitation.
	candidate.Invites[digest(token)] = invitation{ID: invitationID, Name: name, Expires: expiry}
	if err := AtomicJSON(filepath.Join(s.opt.DataDir, "settings.json"), candidate); err != nil {
		return nil, err
	}
	s.cfg = candidate
	baseURL := strings.TrimRight(s.opt.LocalURL, "/")
	instructions := fmt.Sprintf(`Connect yourself to the LocalWhatsAppMCP server running on this Mac at %s/mcp using Streamable HTTP.


Exchange this single-use pairing code by sending POST %s/pairing/redeem with Content-Type: application/json and this JSON body: {"token":"%s"}


The response contains id, url, access_token, and token_type (Bearer). Save access_token privately without printing it in the conversation or logs, and configure your own MCP client to use the returned url with Authorization: Bearer <access_token>.

Reload your MCP connection if needed and call check_status to verify it. This pairing code expires at %s. Sending starts disabled; the app controls sending permission and chat scope.

After check_status succeeds, give the user a short overview of what you can do with their connected WhatsApp chats and one or two example requests.`, baseURL, baseURL, token, expiry.UTC().Format(time.RFC3339))
	return map[string]any{"invitation_id": invitationID, "token": token, "instructions": instructions, "expires_at": expiry.UTC().Format(time.RFC3339)}, nil
}
func (s *Service) redeem(token string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrUnauthorized
	}
	key := digest(token)
	inv, ok := s.cfg.Invites[key]
	if !ok || !time.Now().Before(inv.Expires) {
		return nil, ErrUnauthorized
	}
	if s.cfg.Scope.Mode == "initial" {
		return nil, ErrSetup
	}
	if s.cfg.Transition {
		return nil, ErrTransition
	}
	// Store the bearer before publishing its grant. A crash can leave an unused
	// secret, but never a grant whose credential is missing. A completed grant
	// is marked so startup will fail closed if its database is later lost.
	if err := s.checkSigningKey(); err != nil {
		return nil, err
	}
	id, token := randomToken(), randomToken()
	if err := secret.SetExisting(s.opt.DataDir, AgentCredentialNamespace, id, token); err != nil {
		return nil, ErrCredentials
	}
	g := Grant{ID: id, Name: inv.Name, Client: "generic", PairingInvitationID: inv.ID, Digest: digest(token), CredentialStorageVersion: credentialStorageVersion}
	candidate := s.cfg
	candidate.Invites = maps.Clone(s.cfg.Invites)
	candidate.Grants = maps.Clone(s.cfg.Grants)
	delete(candidate.Invites, key)
	candidate.Grants[id] = g
	if err := AtomicJSON(filepath.Join(s.opt.DataDir, "settings.json"), candidate); err != nil {
		return nil, err
	}
	s.cfg = candidate
	return map[string]string{"id": id, "secret": token, "client": g.Client, "url": s.opt.LocalURL}, nil
}

package service

import (
	"errors"
	"maps"
	"path/filepath"
	"strings"

	"github.com/massi/localwhatsappmcp/internal/secret"
)

var errConnectionNotFound = errors.New("connection was not found")

// connectionCredentials is returned only over the private control socket
// when the user explicitly creates a manual connection or opens any saved
// connection. State and MCP responses never include the bearer.
type connectionCredentials struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	AccessToken string `json:"access_token"`
}

func (s *Service) createManualConnection(name string) (connectionCredentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return connectionCredentials{}, ErrUnauthorized
	}
	if s.cfg.Scope.Mode == "initial" {
		return connectionCredentials{}, ErrSetup
	}
	if s.scopeTransitionLocked() {
		return connectionCredentials{}, ErrTransition
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return connectionCredentials{}, errors.New("give the agent a name")
	}
	if err := s.checkSigningKey(); err != nil {
		return connectionCredentials{}, err
	}
	id, token := randomToken(), randomToken()
	// Commit the bearer first, matching pairing redemption's failure boundary:
	// an interrupted settings write may leave an unused secret, never an active
	// grant whose bearer was not saved. A retry creates a fresh connection.
	if err := secret.SetExisting(s.opt.DataDir, AgentCredentialNamespace, id, token); err != nil {
		return connectionCredentials{}, ErrCredentials
	}
	grant := Grant{ID: id, Name: name, Client: "manual", Digest: digest(token), CredentialStorageVersion: credentialStorageVersion}
	candidate := s.cfg
	candidate.Grants = maps.Clone(s.cfg.Grants)
	candidate.Grants[id] = grant
	if err := AtomicJSON(filepath.Join(s.opt.DataDir, "settings.json"), candidate); err != nil {
		return connectionCredentials{}, err
	}
	s.cfg = candidate
	return s.connectionCredentialsLocked(grant, token), nil
}

func (s *Service) credentialsForConnection(id string) (connectionCredentials, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return connectionCredentials{}, ErrUnauthorized
	}
	grant, ok := s.cfg.Grants[id]
	if !ok {
		return connectionCredentials{}, errConnectionNotFound
	}
	if err := s.checkSigningKey(); err != nil {
		return connectionCredentials{}, err
	}
	token, err := secret.Get(s.opt.DataDir, AgentCredentialNamespace, id)
	if err != nil || token == "" || digest(token) != grant.Digest || grant.CredentialStorageVersion != credentialStorageVersion {
		return connectionCredentials{}, ErrCredentials
	}
	return s.connectionCredentialsLocked(grant, token), nil
}

func (s *Service) connectionCredentialsLocked(grant Grant, token string) connectionCredentials {
	return connectionCredentials{ID: grant.ID, Name: grant.Name, URL: strings.TrimRight(s.opt.LocalURL, "/") + "/mcp", AccessToken: token}
}

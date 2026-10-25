package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/massi/localwhatsappmcp/internal/secret"
)

const credentialStorageVersion = 1
const AgentCredentialNamespace = "com.localwhatsappmcp.agent"
const cursorCredentialNamespace = "com.localwhatsappmcp.cursor"
const cursorCredentialAccount = "archive"

// ErrCredentials lets the native app present a recovery message without
// exposing SQLite diagnostics, file contents, or credential values.
var ErrCredentials = errors.New("local credential database is unavailable; restore credentials.db from the same backup as settings.json and reopen the app")

func (s *Service) loadCredentials() error {
	if s.cfg.CredentialStorageVersion < 0 || s.cfg.CredentialStorageVersion > credentialStorageVersion {
		return ErrCredentials
	}
	migrating := s.cfg.CredentialStorageVersion == 0
	var key string
	var err error
	if migrating {
		key, err = secret.GetOrCreate(s.opt.DataDir, cursorCredentialNamespace, cursorCredentialAccount, func() string {
			if len(s.opt.CursorKey) > 0 {
				return base64.RawURLEncoding.EncodeToString(s.opt.CursorKey)
			}
			return randomToken()
		})
	} else {
		// Never replace a missing database after migration. That would silently
		// rotate credentials and destroy the ability to recover installed clients.
		key, err = secret.Get(s.opt.DataDir, cursorCredentialNamespace, cursorCredentialAccount)
	}
	if err != nil {
		return ErrCredentials
	}
	s.cursorKey, err = base64.RawURLEncoding.DecodeString(key)
	if err != nil || len(s.cursorKey) != 32 {
		return ErrCredentials
	}
	for id, grant := range s.cfg.Grants {
		if grant.CredentialStorageVersion < 0 || grant.CredentialStorageVersion > credentialStorageVersion {
			return ErrCredentials
		}
		if migrating && grant.Digest != "" {
			managed, err := s.hasInstalledConnection(id, grant.Client)
			if err != nil {
				return err
			}
			if managed {
				// Existing Keychain credentials are deliberately never read. Install
				// a new bearer under the same helper connection ID. If saving settings
				// fails, GetOrCreate returns this same bearer on the next attempt.
				token, err := secret.GetOrCreate(s.opt.DataDir, AgentCredentialNamespace, id, randomToken)
				if err != nil {
					return ErrCredentials
				}
				grant.Digest = digest(token)
				grant.CredentialStorageVersion = credentialStorageVersion
				s.cfg.Grants[id] = grant
			}
		}
		if grant.CredentialStorageVersion == credentialStorageVersion {
			token, err := secret.Get(s.opt.DataDir, AgentCredentialNamespace, id)
			if err != nil || token == "" || digest(token) != grant.Digest {
				return ErrCredentials
			}
		}
	}
	if migrating {
		// One atomic settings replacement publishes all rotated grant digests and
		// the marker only after their SQLite transactions have committed.
		s.cfg.CredentialStorageVersion = credentialStorageVersion
		if err := s.saveLocked(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) checkSigningKey() error {
	key, err := secret.Get(s.opt.DataDir, cursorCredentialNamespace, cursorCredentialAccount)
	if err != nil || key != base64.RawURLEncoding.EncodeToString(s.cursorKey) {
		return ErrCredentials
	}
	return nil
}

func (s *Service) hasInstalledConnection(id, client string) (bool, error) {
	if id == "" || len(id) > 100 || strings.ContainsAny(id, "/\\.") || (client != "codex" && client != "claude" && client != "stdio") {
		return false, nil
	}
	b, err := os.ReadFile(filepath.Join(s.opt.DataDir, "connections", id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, ErrCredentials
	}
	var connection struct {
		ID     string `json:"id"`
		Client string `json:"client"`
	}
	if json.Unmarshal(b, &connection) != nil || connection.ID != id || connection.Client != client {
		return false, fmt.Errorf("%w; repair the saved connection metadata before retrying", ErrCredentials)
	}
	return true, nil
}

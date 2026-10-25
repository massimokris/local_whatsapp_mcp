package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
	"github.com/massi/localwhatsappmcp/internal/secret"
)

func legacyCredentialConfig() config {
	return config{
		Version: 1,
		Scope:   engine.Policy{Version: 1, Revision: 7, Mode: "selected", ChatIDs: []string{"123@s.whatsapp.net"}},
		Grants: map[string]Grant{
			"installed":    {ID: "installed", Name: "Installed", Client: "stdio", SendEnabled: true, Digest: digest("legacy-installed")},
			"unconfigured": {ID: "unconfigured", Name: "Unconfigured", Client: "codex", Digest: digest("legacy-unconfigured")},
		},
		Invites: map[string]invitation{},
		Sends:   map[string]sendRecord{"prior": {Hash: "retained", Status: "sent", MessageID: "existing-message"}},
		Linked:  true, WhatsAppDisconnected: true,
	}
}

func writeLegacyCredentials(t *testing.T, dir string, cfg config) {
	t.Helper()
	if err := AtomicJSON(filepath.Join(dir, "settings.json"), cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "connections"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"installed", "revoked"} {
		if err := AtomicJSON(filepath.Join(dir, "connections", id+".json"), map[string]string{"id": id, "client": "stdio"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCredentialMigrationRotatesInstalledBearersAndPreservesArchivePolicy(t *testing.T) {
	dir := t.TempDir()
	prior := legacyCredentialConfig()
	writeLegacyCredentials(t, dir, prior)
	archivePath := filepath.Join(dir, "archive-fixture")
	if err := os.WriteFile(archivePath, []byte("retained archive fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	oldSigner := &Service{cursorKey: bytes.Repeat([]byte{1}, 32)}
	c := cursor{Agent: "installed", Query: "read", Revision: 7, Expires: time.Now().Add(time.Hour).Unix()}
	oldCheckpoint := oldSigner.seal(c)
	s, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	token, err := secret.Get(dir, AgentCredentialNamespace, "installed")
	if err != nil || token == "legacy-installed" {
		t.Fatal("installed bearer was not rotated privately", err)
	}
	if id, err := s.VerifyToken(context.Background(), token); err != nil || id != "installed" {
		t.Fatal("rotated bearer is not authorized", err)
	}
	if _, err := s.VerifyToken(context.Background(), "legacy-installed"); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("previous installed bearer remained authorized")
	}
	if id, err := s.VerifyToken(context.Background(), "legacy-unconfigured"); err != nil || id != "unconfigured" {
		t.Fatal("unconfigured grant was altered", err)
	}
	if _, err := secret.Get(dir, AgentCredentialNamespace, "revoked"); !errors.Is(err, secret.ErrNotFound) {
		t.Fatal("stale metadata recreated a revoked credential")
	}
	if !reflect.DeepEqual(s.cfg.Scope, prior.Scope) || !reflect.DeepEqual(s.cfg.Sends, prior.Sends) || !s.cfg.Linked || !s.cfg.WhatsAppDisconnected || !s.cfg.Grants["installed"].SendEnabled || s.cfg.Grants["unconfigured"].SendEnabled {
		t.Fatal("migration changed archive policy, send permissions, or connection intent")
	}
	if _, err := s.unseal(oldCheckpoint, "installed", "read", 7); err == nil {
		t.Fatal("previous signing key still accepted")
	}
	checkpoint := s.seal(c)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.unseal(checkpoint, "installed", "read", 7); err != nil {
		t.Fatal("SQLite signing key did not survive restart", err)
	}
	if _, err := s.VerifyToken(context.Background(), token); err != nil {
		t.Fatal("SQLite bearer did not survive restart", err)
	}
	if s.cfg.CredentialStorageVersion != 1 || s.cfg.Grants["installed"].CredentialStorageVersion != 1 || s.cfg.Grants["unconfigured"].CredentialStorageVersion != 0 {
		t.Fatal("credential storage markers do not match migrated grants")
	}
	if b, err := os.ReadFile(archivePath); err != nil || string(b) != "retained archive fixture" {
		t.Fatal("archive changed during migration", err)
	}
}

func TestCredentialMigrationRetriesWithSameSecretsAfterSettingsFailure(t *testing.T) {
	dir := t.TempDir()
	prior := legacyCredentialConfig()
	writeLegacyCredentials(t, dir, prior)
	path := filepath.Join(dir, "settings.json")
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	s := &Service{cfg: prior, opt: Options{DataDir: dir}}
	if err := s.loadCredentials(); err == nil {
		t.Fatal("migration reported success without persisting its marker")
	}
	token, err := secret.Get(dir, AgentCredentialNamespace, "installed")
	if err != nil {
		t.Fatal("credential transaction should precede settings publication", err)
	}
	key, err := secret.Get(dir, cursorCredentialNamespace, cursorCredentialAccount)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".saved", path); err != nil {
		t.Fatal(err)
	}
	s, err = New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got, _ := secret.Get(dir, AgentCredentialNamespace, "installed"); got != token {
		t.Fatal("retry rotated the already committed bearer")
	}
	if got, _ := secret.Get(dir, cursorCredentialNamespace, cursorCredentialAccount); got != key {
		t.Fatal("retry rotated the already committed signing key")
	}
	if _, err := s.VerifyToken(context.Background(), token); err != nil {
		t.Fatal(err)
	}
}

func TestCompletedCredentialMigrationDoesNotRepairMissingOrCorruptSecrets(t *testing.T) {
	for _, problem := range []string{"missing database", "corrupt database", "missing signer", "invalid signer", "missing bearer", "mismatched bearer"} {
		t.Run(problem, func(t *testing.T) {
			dir := t.TempDir()
			writeLegacyCredentials(t, dir, legacyCredentialConfig())
			s, err := New(Options{DataDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			s.Close()
			path := filepath.Join(dir, "credentials.db")
			switch problem {
			case "missing database":
				err = os.Remove(path)
			case "corrupt database":
				err = os.WriteFile(path, []byte("invalid synthetic credential data"), 0600)
			case "missing signer":
				err = secret.Delete(dir, cursorCredentialNamespace, cursorCredentialAccount)
			case "invalid signer":
				err = secret.SetExisting(dir, cursorCredentialNamespace, cursorCredentialAccount, "invalid-key")
			case "missing bearer":
				err = secret.Delete(dir, AgentCredentialNamespace, "installed")
			case "mismatched bearer":
				err = secret.SetExisting(dir, AgentCredentialNamespace, "installed", "mismatched-token")
			}
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(dir, "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := New(Options{DataDir: dir}); !errors.Is(err, ErrCredentials) {
				t.Fatalf("damaged credentials did not fail with recoverable error: %v", err)
			}
			after, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
			if !bytes.Equal(before, after) {
				t.Fatal("failed credential startup changed settings")
			}
			if problem == "missing database" {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing credentials were silently replaced")
				}
			}
		})
	}
}

func TestPairingFailureRetainsInvitationAndDoesNotRecreateLostDatabase(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.applyScope(context.Background(), engine.Policy{Mode: "all"}); err != nil {
		t.Fatal(err)
	}
	invite, err := s.invite("Fixture", "stdio")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials.db")
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.redeem(invite["token"].(string)); !errors.Is(err, ErrCredentials) {
		t.Fatal("pairing did not fail closed for lost credential database", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("pairing recreated missing credentials")
	}
	if err := os.Rename(path+".saved", path); err != nil {
		t.Fatal(err)
	}
	grant, err := s.redeem(invite["token"].(string))
	if err != nil {
		t.Fatal("invitation consumed before credential storage recovered", err)
	}
	if token, err := secret.Get(dir, AgentCredentialNamespace, grant["id"]); err != nil || token != grant["secret"] {
		t.Fatal("pairing did not persist its credential before returning", err)
	}
}

func TestPairingSettingsFailureDoesNotConsumeInvitationOrPublishGrant(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.applyScope(context.Background(), engine.Policy{Mode: "all"}); err != nil {
		t.Fatal(err)
	}
	invite, err := s.invite("Fixture", "stdio")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.redeem(invite["token"].(string)); err == nil {
		t.Fatal("pairing succeeded without durable grant")
	}
	if len(s.cfg.Grants) != 0 || len(s.cfg.Invites) != 1 {
		t.Fatal("failed settings save changed effective pairing state")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".saved", path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.redeem(invite["token"].(string)); err != nil {
		t.Fatal("pairing could not retry after settings recovery", err)
	}
}

func TestMigrationDoesNotPublishPartialGrantsForInvalidConnectionMetadata(t *testing.T) {
	dir := t.TempDir()
	writeLegacyCredentials(t, dir, legacyCredentialConfig())
	settingsPath := filepath.Join(dir, "settings.json")
	before, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(dir, "connections", "installed.json")
	if err := os.WriteFile(metadataPath, []byte(`{"id":"wrong","client":"stdio"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Options{DataDir: dir}); !errors.Is(err, ErrCredentials) {
		t.Fatal("mismatched connection metadata should require repair", err)
	}
	after, _ := os.ReadFile(settingsPath)
	if !bytes.Equal(before, after) {
		t.Fatal("failed migration published changed grants or marker")
	}
	if err := AtomicJSON(metadataPath, map[string]string{"id": "installed", "client": "stdio"}); err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{DataDir: dir})
	if err != nil {
		t.Fatal("migration did not recover after metadata repair", err)
	}
	s.Close()
}

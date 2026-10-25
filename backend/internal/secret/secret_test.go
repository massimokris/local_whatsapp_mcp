package secret

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPersistenceAndNamespaces(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Application Support", "LocalWhatsAppMCP")
	for _, credential := range []struct{ namespace, account, value string }{
		{"internal", "archive", "first-key"},
		{"agent", "archive", "separate-token"},
		{"agent", "other", "other-token"},
	} {
		if _, err := GetOrCreate(dir, credential.namespace, credential.account, func() string { return credential.value }); err != nil {
			t.Fatal(err)
		}
		got, err := Get(dir, credential.namespace, credential.account)
		if err != nil || got != credential.value {
			t.Fatalf("credential did not persist: %q, %v", got, err)
		}
	}
	if err := SetExisting(dir, "internal", "archive", "replacement-key"); err != nil {
		t.Fatal(err)
	}
	if err := SetExisting(dir, "agent", "new-agent", "new-agent-token"); err != nil {
		t.Fatal(err)
	}
	if got, err := Get(dir, "agent", "new-agent"); err != nil || got != "new-agent-token" {
		t.Fatalf("new credential in existing database did not persist: %q, %v", got, err)
	}
	if got, err := Get(dir, "internal", "archive"); err != nil || got != "replacement-key" {
		t.Fatalf("replacement did not persist: %q, %v", got, err)
	}
	if err := Delete(dir, "agent", "archive"); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(dir, "agent", "archive"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted credential should be missing: %v", err)
	}
	if got, err := Get(dir, "internal", "archive"); err != nil || got != "replacement-key" {
		t.Fatalf("deletion crossed namespace: %q, %v", got, err)
	}
	if err := Delete(dir, "agent", "archive"); err != nil {
		t.Fatalf("deletion is not idempotent: %v", err)
	}
}

func TestMissingStoreIsNotCreatedByReadsOrDeletion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := Get(dir, "internal", "archive"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing store: %v", err)
	}
	if err := Delete(dir, "internal", "archive"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("reading/deleting created a directory: %v", err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Get(dir, "internal", "archive"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing database: %v", err)
	}
	if err := SetExisting(dir, "agent", "new-agent", "new-agent-token"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("writing in existing database did not fail on missing database: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, databaseName)); !os.IsNotExist(err) {
		t.Fatalf("reading created a database: %v", err)
	}
}

func TestConcurrentGetOrCreateUsesOneDurableValue(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "credentials")
	const workers = 24
	var generated atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	values := make(chan string, workers)
	failures := make(chan error, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			value, err := GetOrCreate(dir, "internal", "archive", func() string {
				return fmt.Sprintf("generated-%d", generated.Add(1))
			})
			if err != nil {
				failures <- err
				return
			}
			values <- value
		}()
	}
	close(start)
	wg.Wait()
	close(values)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	for value := range values {
		if value != "generated-1" {
			t.Errorf("concurrent caller got %q", value)
		}
	}
	if generated.Load() != 1 {
		t.Fatalf("generated %d values", generated.Load())
	}
	if got, err := Get(dir, "internal", "archive"); err != nil || got != "generated-1" {
		t.Fatalf("value did not survive closing all connections: %q, %v", got, err)
	}
}

func TestPrivatePermissionsIncludeJournal(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := GetOrCreate(dir, "internal", "archive", func() string { return "key" }); err != nil {
		t.Fatal(err)
	}
	assertMode(t, dir, 0700)
	path := filepath.Join(dir, databaseName)
	assertMode(t, path, 0600)
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	db, err := openStore(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertMode(t, path, 0600)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE credentials SET value = ? WHERE namespace = ? AND account = ?`, "new-key", "internal", "archive"); err != nil {
		t.Fatal(err)
	}
	assertMode(t, path+"-journal", 0600)
}

func TestInvalidDatabaseDoesNotRegenerateOrLeakValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, databaseName)
	corrupt := []byte("not a database: existing-private-value")
	if err := os.WriteFile(path, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	var generated bool
	_, err := GetOrCreate(dir, "internal", "archive", func() string {
		generated = true
		return "replacement-private-value"
	})
	if err == nil || errors.Is(err, ErrNotFound) || generated {
		t.Fatalf("invalid store was treated as missing: error %v, generated %v", err, generated)
	}
	if strings.Contains(err.Error(), "private-value") {
		t.Fatalf("error exposed credential data: %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != string(corrupt) {
		t.Fatalf("invalid database was replaced: %q, %v", got, readErr)
	}
	if _, err := Get(dir, "internal", "archive"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid database read did not fail: %v", err)
	}
}

func TestRejectsSymlinksAndNonRegularFiles(t *testing.T) {
	for _, kind := range []string{"database-symlink", "database-directory", "sidecar-symlink", "data-directory-symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "data")
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(root, "outside")
			if err := os.WriteFile(outside, []byte("untouched"), 0644); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "database-symlink":
				if err := os.Symlink(outside, filepath.Join(dir, databaseName)); err != nil {
					t.Fatal(err)
				}
			case "database-directory":
				if err := os.Mkdir(filepath.Join(dir, databaseName), 0700); err != nil {
					t.Fatal(err)
				}
			case "sidecar-symlink":
				if err := os.Symlink(outside, filepath.Join(dir, databaseName+"-journal")); err != nil {
					t.Fatal(err)
				}
			case "data-directory-symlink":
				link := filepath.Join(root, "link")
				if err := os.Symlink(dir, link); err != nil {
					t.Fatal(err)
				}
				dir = link
			}
			if _, err := GetOrCreate(dir, "internal", "archive", func() string { return "private-value" }); err == nil {
				t.Fatal("accepted a symlink or non-regular database path")
			}
			got, err := os.ReadFile(outside)
			if err != nil || string(got) != "untouched" {
				t.Fatalf("changed the symlink target: %q, %v", got, err)
			}
			assertMode(t, outside, 0644)
		})
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s has permissions %o, want %o", filepath.Base(path), got, want)
	}
}

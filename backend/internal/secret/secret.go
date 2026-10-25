// Package secret stores app-owned credentials in the private application directory.
package secret

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
)

var ErrNotFound = errors.New("credential was not found in the local credential database")

const databaseName = "credentials.db"

// Get reads an existing credential without creating a database or a credential.
func Get(dataDir, namespace, account string) (string, error) {
	if err := validateKey(namespace, account); err != nil {
		return "", err
	}
	db, err := openStore(dataDir, false)
	if err != nil {
		return "", err
	}
	defer db.Close()
	return readCredential(db, namespace, account)
}

// Set inserts or replaces a credential. Credentials never appear in SQL text.
func Set(dataDir, namespace, account, value string) error {
	return setCredential(dataDir, namespace, account, value, true)
}

// SetExisting inserts or replaces a credential only in an existing database.
// It cannot silently recreate a database that was lost after application setup.
func SetExisting(dataDir, namespace, account, value string) error {
	return setCredential(dataDir, namespace, account, value, false)
}

func setCredential(dataDir, namespace, account, value string, create bool) error {
	if err := validateKey(namespace, account); err != nil {
		return err
	}
	if value == "" {
		return errors.New("a local credential cannot be empty")
	}
	db, err := openStore(dataDir, create)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO credentials(namespace, account, value) VALUES (?, ?, ?)
		ON CONFLICT(namespace, account) DO UPDATE SET value = excluded.value`, namespace, account, value)
	if err != nil {
		return storeError("save the credential")
	}
	return nil
}

// Delete is idempotent and does not create a missing database.
func Delete(dataDir, namespace, account string) error {
	if err := validateKey(namespace, account); err != nil {
		return err
	}
	db, err := openStore(dataDir, false)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM credentials WHERE namespace = ? AND account = ?`, namespace, account); err != nil {
		return storeError("delete the credential")
	}
	return nil
}

// GetOrCreate generates a credential only after an immediate SQLite transaction
// has checked for it. Concurrent helpers therefore share one durable value.
func GetOrCreate(dataDir, namespace, account string, generate func() string) (string, error) {
	if err := validateKey(namespace, account); err != nil {
		return "", err
	}
	if generate == nil {
		return "", errors.New("a local credential generator is required")
	}
	db, err := openStore(dataDir, true)
	if err != nil {
		return "", err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return "", storeError("lock the credential database")
	}
	defer tx.Rollback()
	value, err := readCredential(tx, namespace, account)
	if err == nil {
		return value, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return "", err
	}
	value = generate()
	if value == "" {
		return "", errors.New("a generated local credential cannot be empty")
	}
	if _, err := tx.Exec(`INSERT INTO credentials(namespace, account, value) VALUES (?, ?, ?)`, namespace, account, value); err != nil {
		return "", storeError("save the credential")
	}
	if err := tx.Commit(); err != nil {
		return "", storeError("save the credential")
	}
	return value, nil
}

type rowReader interface {
	QueryRow(string, ...any) *sql.Row
}

func readCredential(db rowReader, namespace, account string) (string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM credentials WHERE namespace = ? AND account = ?`, namespace, account).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil || value == "" {
		return "", storeError("read the credential")
	}
	return value, nil
}

func validateKey(namespace, account string) error {
	if strings.TrimSpace(namespace) == "" || strings.TrimSpace(account) == "" {
		return errors.New("a credential namespace and account are required")
	}
	return nil
}

func openStore(dataDir string, create bool) (*sql.DB, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, errors.New("a private application data directory is required")
	}
	dir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, storeError("open the application data directory")
	}
	if create {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, storeError("create the application data directory")
		}
	}
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, storeError("open the private application data directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, storeError("protect the application data directory")
	}
	path := filepath.Join(dir, databaseName)
	if create {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
		if err == nil {
			err = f.Close()
		}
		if err != nil && !os.IsExist(err) {
			return nil, storeError("create the credential database")
		}
	}
	if err := protectFile(path); err != nil {
		return nil, err
	}
	// SQLite gives new journals the database's 0600 mode. Validate any leftover
	// sidecars before opening, including journals from an interrupted write.
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		if err := protectFile(path + suffix); err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	location := url.URL{Scheme: "file", Path: path}
	options := url.Values{
		"mode":           {"rw"},
		"_busy_timeout":  {"10000"},
		"_txlock":        {"immediate"},
		"_journal_mode":  {"DELETE"},
		"_secure_delete": {"on"},
	}
	location.RawQuery = options.Encode()
	db, err := sql.Open("sqlite3", location.String())
	if err != nil {
		return nil, storeError("open the credential database")
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, storeError("open the credential database")
	}
	if create {
		_, err = db.Exec(`CREATE TABLE IF NOT EXISTS credentials (
			namespace TEXT NOT NULL,
			account TEXT NOT NULL,
			value TEXT NOT NULL CHECK(length(value) > 0),
			PRIMARY KEY (namespace, account)
		) WITHOUT ROWID`)
		if err != nil {
			db.Close()
			return nil, storeError("initialize the credential database")
		}
	}
	return db, nil
}

func protectFile(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return ErrNotFound
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return storeError("open a regular credential database file")
	}
	if err := os.Chmod(path, 0600); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return storeError("protect the credential database")
	}
	return nil
}

func storeError(action string) error {
	return fmt.Errorf("local credential storage could not %s; check the application data folder and retry", action)
}

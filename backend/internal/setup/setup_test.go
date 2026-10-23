package setup_test

import (
	"github.com/massi/localwhatsappmcp/internal/setup"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallPreservesOtherServersAndNeverWritesSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	original := `model = "some-model"
[mcp_servers.existing]
command = "existing-server"
`
	os.WriteFile(path, []byte(original), 0600)
	if err := setup.InstallConfig("codex", path, "/Applications/Local WhatsApp.app/Contents/Helpers/helper", "/private/app data", "connection-1", "http://127.0.0.1:10000"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "existing-server") || !strings.Contains(string(data), "http_headers_helper") || !strings.Contains(string(data), "connection-1") {
		t.Fatal(string(data))
	}
	if err := setup.InstallConfig("codex", path, "/Applications/Local WhatsApp.app/Contents/Helpers/helper", "/private/app data", "connection-1", "http://127.0.0.1:10000"); err != nil {
		t.Fatal(err)
	}
}

package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

func TestAppDeviceLabelOverridesParentForAllEngineCommands(t *testing.T) {
	t.Setenv("WACLI_DEVICE_LABEL", "conflicting parent device name")
	t.Setenv("LOCALWHATSAPPMCP_TEST_INHERITED_ENV", "inherited fixture value")

	for _, scenario := range []struct {
		name    string
		command string
		link    bool
	}{
		{name: "link", command: "auth", link: true},
		{name: "sync", command: "sync"},
		{name: "control call", command: "call"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "wacli")
			// Capture the environment in a real child for each adapter entry point.
			// The fixture never opens an account store or contacts WhatsApp.
			script := `#!/bin/sh
set -eu
root="$(dirname "$0")"
case "$*" in
  *"--events auth --follow --qr-format text") command=auth ;;
  *"--events sync --follow --refresh-contacts --refresh-groups") command=sync ;;
  *"--read-only auth status") command=call ;;
  *) exit 81 ;;
esac
printf '%s\n' "$command" "${WACLI_DEVICE_LABEL-}" "${LOCALWHATSAPPMCP_TEST_INHERITED_ENV-}" > "$root/environment"
if [ "$command" = call ]; then
  printf '%s\n' '{"success":true,"data":{"authenticated":false}}'
else
  printf '%s\n' '{"event":"policy_loaded","data":{"revision":2}}' >&2
  exec /bin/cat >/dev/null
fi
`
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			e := engine.New(bin, dir)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if scenario.command == "call" {
				if _, err := e.Call(ctx, false, "auth", "status"); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := e.Start(ctx, engine.Policy{Version: 1, Revision: 2, Mode: "initial"}, scenario.link); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := e.Stop(ctx); err != nil {
						t.Error(err)
					}
				}()
			}
			data, err := os.ReadFile(filepath.Join(dir, "environment"))
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			want := []string{scenario.command, "LocalWhatsAppMCP", "inherited fixture value"}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("child environment: got %q, want %q", got, want)
			}
		})
	}
}

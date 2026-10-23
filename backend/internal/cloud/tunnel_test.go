package cloud

import (
	"strings"
	"testing"
)

func TestProviderLaunchKeepsSecretsOutOfArgumentsAndTargetsOnlyPublicListener(t *testing.T) {
	cfg := TunnelConfig{Enabled: true, Provider: "ngrok", Hostname: "my-bridge.ngrok.app", Executable: "/opt/homebrew/bin/ngrok", CredentialRef: "provider-1"}
	spec, err := BuildTunnelCommand(cfg, "http://127.0.0.1:18789", t.TempDir(), "fixture-secret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(spec.Args, " "), "fixture-secret") || !contains(spec.Env, "NGROK_AUTHTOKEN=fixture-secret") {
		t.Fatal("credential delivery is not private")
	}
	if !contains(spec.Args, "http://127.0.0.1:18789") || !contains(spec.Args, "--inspect=false") {
		t.Fatal(spec.Args)
	}
	cfg.Provider = "cloudflared"
	cfg.TunnelID = "9dd39b09-ef0b-4cb6-981f-0e57bc6c3696"
	cfg.Executable = "/opt/homebrew/bin/cloudflared"
	cred := `{"AccountTag":"fixture","TunnelSecret":"secret","TunnelID":"9dd39b09-ef0b-4cb6-981f-0e57bc6c3696"}`
	spec, err = BuildTunnelCommand(cfg, "http://127.0.0.1:18789", t.TempDir(), cred)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(spec.Args, " "), "secret") || !contains(spec.Env, "TUNNEL_CRED_CONTENTS="+cred) {
		t.Fatal("cloudflare credential leaked")
	}
	for _, host := range []string{"https://evil.example/path", "evil.example\nfoo: bar", "localhost", "127.0.0.1"} {
		cfg.Hostname = host
		if _, err = BuildTunnelCommand(cfg, "http://127.0.0.1:18789", t.TempDir(), cred); err == nil {
			t.Fatalf("accepted hostname %q", host)
		}
	}
	cfg.Hostname = "bridge.example"
	if _, err = BuildTunnelCommand(cfg, "http://0.0.0.0:18789", t.TempDir(), cred); err == nil {
		t.Fatal("accepted public upstream")
	}
}

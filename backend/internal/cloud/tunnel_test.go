package cloud

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestTunnelShutdownWaitsUntilOwnedProcessExits(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "provider.pid")
	supervisor := &tunnelSupervisor{}
	supervisor.Start(CommandSpec{Executable: "/bin/sh", Args: []string{"-c", "echo $$ > " + shellQuote(pidfile) + "; exec /bin/sleep 60"}}, "invalid.example", tunnelTestHelper(t))
	defer supervisor.Stop()
	var pid int
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if b, e := os.ReadFile(pidfile); e == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("provider did not start")
	}
	supervisor.Stop()
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatal("provider remains alive after Quit")
	}
}

func TestTunnelOwnerFixture(t *testing.T) {
	if os.Getenv("LWMCP_TUNNEL_OWNER_FIXTURE") != "1" {
		return
	}
	dir := os.Getenv("LWMCP_TUNNEL_FIXTURE_DIR")
	supervisor := &tunnelSupervisor{}
	supervisor.Start(CommandSpec{Executable: "/bin/sh", Args: []string{"-c", "echo $$ > " + shellQuote(filepath.Join(dir, "provider.pid")) + "; sleep 60 & echo $! > " + shellQuote(filepath.Join(dir, "descendant.pid")) + "; wait"}, Env: []string{"NGROK_AUTHTOKEN=private-fixture"}}, "invalid.example", tunnelTestHelper(t))
	time.Sleep(60 * time.Second)
	supervisor.Stop()
}

func TestTunnelProviderAndDescendantStopAfterAbruptServiceDeath(t *testing.T) {
	dir := t.TempDir()
	owner := exec.Command(os.Args[0], "-test.run=^TestTunnelOwnerFixture$")
	owner.Env = append(os.Environ(), "LWMCP_TUNNEL_OWNER_FIXTURE=1", "LWMCP_TUNNEL_FIXTURE_DIR="+dir)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { owner.Process.Kill(); owner.Wait() }()
	pids := []int{}
	defer func() {
		for _, pid := range pids {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}()
	for _, name := range []string{"provider.pid", "descendant.pid"} {
		var pid int
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if b, e := os.ReadFile(filepath.Join(dir, name)); e == nil {
				pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
				if pid > 0 {
					break
				}
			}
		}
		if pid == 0 {
			t.Fatal("provider did not start")
		}
		pids = append(pids, pid)
	}
	owner.Process.Kill()
	owner.Wait()
	for _, pid := range pids {
		for deadline := time.Now().Add(7 * time.Second); time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil; time.Sleep(10 * time.Millisecond) {
		}
		if syscall.Kill(pid, 0) == nil {
			t.Fatalf("provider process %d survived service death", pid)
		}
	}
}

func tunnelTestHelper(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tunnel-worker")
	script := "#!/bin/sh\nexec " + shellQuote(os.Args[0]) + " -test.run=^TestTunnelWorkerFixture$ -- tunnel-worker\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestTunnelWorkerFixture(t *testing.T) {
	if os.Args[len(os.Args)-1] != "tunnel-worker" {
		return
	}
	if err := RunTunnelWorker(os.Stdin); err != nil {
		t.Fatal(err)
	}
}

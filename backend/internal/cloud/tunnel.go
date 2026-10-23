package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

type TunnelConfig struct {
	Enabled       bool   `json:"enabled"`
	Provider      string `json:"provider"`
	Hostname      string `json:"hostname"`
	Executable    string `json:"executable"`
	TunnelID      string `json:"tunnel_id,omitempty"`
	CredentialRef string `json:"credential_ref"`
}
type CommandSpec struct {
	Executable string
	Args, Env  []string
}

var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)
var uuidPattern = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)

func validateTunnel(c TunnelConfig) error {
	if c.Provider != "ngrok" && c.Provider != "cloudflared" {
		return errors.New("choose ngrok or cloudflared")
	}
	if !hostnamePattern.MatchString(c.Hostname) || len(c.Hostname) > 253 || net.ParseIP(c.Hostname) != nil {
		return errors.New("a stable public hostname is required")
	}
	if !filepath.IsAbs(c.Executable) {
		return errors.New("provider executable must be an absolute path")
	}
	if c.CredentialRef == "" {
		return errors.New("provider credentials have not been saved")
	}
	if c.Provider == "cloudflared" && !uuidPattern.MatchString(c.TunnelID) {
		return errors.New("a named Cloudflare tunnel ID is required")
	}
	return nil
}
func BuildTunnelCommand(c TunnelConfig, upstream, dataDir, credential string) (CommandSpec, error) {
	if err := validateTunnel(c); err != nil {
		return CommandSpec{}, err
	}
	u, e := url.Parse(upstream)
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Path != "" || u.RawQuery != "" || u.User != nil {
		return CommandSpec{}, errors.New("tunnel upstream must be the loopback MCP listener")
	}
	if credential == "" {
		return CommandSpec{}, errors.New("provider login is required")
	}
	path := filepath.Join(dataDir, "tunnel-config.json")
	spec := CommandSpec{Executable: c.Executable}
	// Both providers accept JSON as their YAML configuration. No credential appears
	// in the configuration file, process arguments, or status output.
	if c.Provider == "ngrok" {
		if strings.ContainsAny(credential, "\r\n\x00") {
			return CommandSpec{}, errors.New("invalid ngrok credential")
		}
		if e = atomicJSON(path, map[string]any{"version": "3", "agent": map[string]any{"web_addr": false}}); e != nil {
			return spec, e
		}
		spec.Args = []string{"http", upstream, "--url=https://" + c.Hostname, "--config=" + path, "--inspect=false", "--log=false"}
		spec.Env = []string{"NGROK_AUTHTOKEN=" + credential}
	} else {
		var cred struct{ AccountTag, TunnelSecret, TunnelID string }
		if json.Unmarshal([]byte(credential), &cred) != nil || cred.TunnelID != c.TunnelID || cred.AccountTag == "" || cred.TunnelSecret == "" {
			return spec, errors.New("Cloudflare credentials do not match this named tunnel")
		}
		config := map[string]any{"tunnel": c.TunnelID, "ingress": []map[string]string{{"hostname": c.Hostname, "service": upstream}, {"service": "http_status:404"}}, "metrics": "127.0.0.1:0", "loglevel": "error"}
		if e = atomicJSON(path, config); e != nil {
			return spec, e
		}
		spec.Args = []string{"tunnel", "--no-autoupdate", "--config", path, "run", c.TunnelID}
		spec.Env = []string{"TUNNEL_CRED_CONTENTS=" + credential}
	}
	return spec, nil
}

type tunnelStatus struct{ Status, Detail string }
type tunnelSupervisor struct {
	mu     sync.Mutex
	status tunnelStatus
	cancel context.CancelFunc
	done   chan struct{}
}

func (t *tunnelSupervisor) State() tunnelStatus { t.mu.Lock(); defer t.mu.Unlock(); return t.status }
func (t *tunnelSupervisor) set(status, detail string) {
	t.mu.Lock()
	t.status = tunnelStatus{status, detail}
	t.mu.Unlock()
}
func (t *tunnelSupervisor) Start(spec CommandSpec, hostname, helper string) {
	t.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	t.mu.Lock()
	t.cancel = cancel
	t.done = done
	t.mu.Unlock()
	go func() {
		defer close(done)
		delay := time.Second
		for ctx.Err() == nil {
			t.set("connecting", "Starting the saved cloud connection…")
			cmd := exec.Command(helper, "tunnel-worker")
			cmd.Env = cleanProviderEnvironment(os.Environ())
			lifetime, err := cmd.StdinPipe()
			if err != nil {
				t.set("attention", "The private tunnel ownership channel could not open.")
				return
			}
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := cmd.Start(); err != nil {
				lifetime.Close()
				t.set("attention", "The tunnel executable could not start. Run cloud setup to repair it.")
				return
			}
			if err := json.NewEncoder(lifetime).Encode(spec); err != nil {
				lifetime.Close()
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.set("attention", "The private tunnel command could not be delivered.")
				return
			}
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			ticker := time.NewTicker(5 * time.Second)
			running := true
			for running {
				select {
				case <-ctx.Done():
					_ = lifetime.Close()
					select {
					case <-exited:
					case <-time.After(7 * time.Second):
						_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
						<-exited
					}
					ticker.Stop()
					t.set("disabled", "Cloud access is off.")
					return
				case <-exited:
					lifetime.Close()
					ticker.Stop()
					running = false
					t.set("attention", "Cloud connection stopped. Check provider login or network; retrying automatically.")
				case <-ticker.C:
					if probePublic(ctx, "https://"+hostname) == nil {
						t.set("ready", "Public endpoint is reachable. Your Mac must remain awake and online.")
						delay = time.Second
					} else {
						t.set("connecting", "Waiting for the public endpoint. Check your provider hostname and network.")
					}
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if delay < time.Minute {
				delay *= 2
			}
		}
	}()
}
func (t *tunnelSupervisor) Stop() {
	t.mu.Lock()
	cancel, done := t.cancel, t.done
	t.cancel = nil
	t.done = nil
	t.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}
func cleanProviderEnvironment(env []string) []string {
	result := []string{}
	for _, entry := range env {
		if !strings.HasPrefix(entry, "NGROK_") && !strings.HasPrefix(entry, "TUNNEL_") {
			result = append(result, entry)
		}
	}
	return result
}
func probePublic(ctx context.Context, base string) error {
	client := &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("unexpected public redirect") }}
	req, e := http.NewRequestWithContext(ctx, "GET", base+"/.well-known/oauth-protected-resource", nil)
	if e != nil {
		return e
	}
	req.Header.Set("ngrok-skip-browser-warning", "LocalWhatsAppMCP")
	res, e := client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	var data struct{ Resource string }
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 16384)).Decode(&data) != nil || data.Resource != base+"/mcp" {
		return errors.New("public metadata is unavailable")
	}
	req, e = http.NewRequestWithContext(ctx, "POST", base+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"LocalWhatsAppMCP connectivity check","version":"1"}}}`))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("ngrok-skip-browser-warning", "LocalWhatsAppMCP")
	res, e = client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 401 {
		return fmt.Errorf("unauthenticated MCP should return 401, got %d", res.StatusCode)
	}
	return nil
}

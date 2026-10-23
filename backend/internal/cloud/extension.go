package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Host is the app-owned policy service; cloud credentials never bypass its grants.
type Host interface {
	DataDir() string
	LocalURL() string
	HelperPath() string
	State() map[string]any
	SetPublicURL(string)
	CreateCloudGrant(name, client string) (string, error)
	VerifyCloudGrant(id string) bool
	SetExternalVerifier(func(context.Context, string) (string, error))
}
type Secrets interface {
	Set(service, account, value string) error
	Get(service, account string) (string, error)
	Delete(service, account string) error
}

const CredentialService = "LocalWhatsAppMCP.providers"

type setupInvitation struct {
	Name, Provider string
	Expires        time.Time
}
type Extension struct {
	mu          sync.Mutex
	operations  sync.Mutex
	host        Host
	secrets     Secrets
	cfg         TunnelConfig
	oauth       *OAuth
	invites     map[string]setupInvitation
	supervisor  tunnelSupervisor
	cancel      context.CancelFunc
	done        chan struct{}
	configError string
}

func New(host Host, secrets Secrets) (*Extension, error) {
	if secrets == nil {
		return nil, errors.New("protected provider credential storage is required")
	}
	e := &Extension{host: host, secrets: secrets, invites: map[string]setupInvitation{}, done: make(chan struct{})}
	b, err := os.ReadFile(filepath.Join(host.DataDir(), "cloud.json"))
	if err == nil {
		if json.Unmarshal(b, &e.cfg) != nil {
			e.configError = "Saved cloud configuration is invalid. Run cloud setup to repair it."
			e.cfg = TunnelConfig{}
		} else if e.cfg.Enabled {
			if err = validateTunnel(e.cfg); err != nil {
				e.configError = err.Error()
				e.cfg.Enabled = false
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		e.configError = "Saved cloud configuration could not be read."
	}
	host.SetExternalVerifier(e.Verify)
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	go func() {
		defer close(e.done)
		for host.LocalURL() == "" {
			select {
			case <-ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
		e.operations.Lock()
		defer e.operations.Unlock()
		if ctx.Err() == nil {
			e.mu.Lock()
			enabled := e.cfg.Enabled && e.oauth == nil
			e.mu.Unlock()
			if enabled {
				if err := e.start(); err != nil {
					e.mu.Lock()
					e.configError = err.Error()
					e.mu.Unlock()
				}
			}
		}
	}()
	return e, nil
}
func (e *Extension) Verify(ctx context.Context, token string) (string, error) {
	e.mu.Lock()
	o, enabled := e.oauth, e.cfg.Enabled
	e.mu.Unlock()
	if !enabled || o == nil {
		return "", errors.New("cloud access is disabled")
	}
	return o.Verify(ctx, token)
}
func (e *Extension) State() any {
	e.mu.Lock()
	c, detail := e.cfg, e.configError
	e.mu.Unlock()
	s := e.supervisor.State()
	if !c.Enabled {
		s = tunnelStatus{"disabled", "Cloud access is off."}
	}
	if detail != "" {
		s = tunnelStatus{"attention", detail}
	}
	return map[string]any{"enabled": c.Enabled, "provider": c.Provider, "hostname": c.Hostname, "status": s.Status, "detail": s.Detail}
}
func (e *Extension) Close() error {
	e.cancel()
	<-e.done
	e.operations.Lock()
	defer e.operations.Unlock()
	e.supervisor.Stop()
	return nil
}
func (e *Extension) PublicHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		o, enabled := e.oauth, e.cfg.Enabled
		e.mu.Unlock()
		if !enabled || o == nil {
			http.NotFound(w, r)
			return
		}
		o.ServeHTTP(w, r)
	})
}
func (e *Extension) ControlHandler() http.Handler { return http.HandlerFunc(e.control) }
func (e *Extension) start() error {
	e.mu.Lock()
	c := e.cfg
	e.mu.Unlock()
	credential, err := e.secrets.Get(CredentialService, c.CredentialRef)
	if err != nil {
		return errors.New("Provider login is required. Run cloud setup to reconnect.")
	}
	spec, err := BuildTunnelCommand(c, e.host.LocalURL(), e.host.DataDir(), credential)
	if err != nil {
		return err
	}
	o, err := NewOAuth(OAuthOptions{Issuer: "https://" + c.Hostname, Path: filepath.Join(e.host.DataDir(), "oauth-"+fingerprint(c.Hostname)[:16]+".json"), GrantActive: e.host.VerifyCloudGrant})
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.oauth = o
	e.configError = ""
	e.mu.Unlock()
	e.host.SetPublicURL("https://" + c.Hostname)
	e.supervisor.Start(spec, c.Hostname)
	return nil
}
func (e *Extension) control(w http.ResponseWriter, r *http.Request) {
	e.operations.Lock()
	defer e.operations.Unlock()
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	switch {
	case r.Method == "GET" && r.URL.Path == "/cloud/config":
		e.mu.Lock()
		cfg := e.cfg
		e.mu.Unlock()
		writeJSON(w, 200, cfg)
	case r.Method == "POST" && r.URL.Path == "/cloud/instructions":
		var in struct{ Provider, Name string }
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.Provider != "ngrok" && in.Provider != "cloudflared" {
			http.Error(w, "Choose ngrok or cloudflared.", 400)
			return
		}
		if in.Name == "" {
			in.Name = "Cloud agent"
		}
		token := opaque()
		e.mu.Lock()
		for k, v := range e.invites {
			if time.Now().After(v.Expires) {
				delete(e.invites, k)
			}
		}
		e.invites[fingerprint(token)] = setupInvitation{in.Name, in.Provider, time.Now().Add(10 * time.Minute)}
		e.mu.Unlock()
		text := fmt.Sprintf("Set up my LocalWhatsAppMCP cloud connection using %s. Read the bundled helper's `cloud --help`. Install %s if missing using its official installer or Homebrew. Reuse existing provider login. For ngrok, the helper imports its existing configuration privately or opens a native secure credential dialog after browser login; never ask me to paste credentials into this conversation. For Cloudflare, use browser `cloudflared tunnel login`, create a named tunnel and DNS route for a stable hostname in my account, and pass the credentials-file path to the helper. Never print that file.\n\nRun %s cloud setup --data-dir %s --provider %s --client chatgpt --hostname <stable-public-hostname> --token-stdin (add --tunnel-id and --credentials-file for Cloudflare). Supply this single-use invitation on stdin, not as a command argument:\n%s\n\nThe helper stores provider secrets in Keychain, lets the app own the tunnel, verifies public access, and opens a private browser setup session. Connect ChatGPT in that same browser using the resulting MCP URL and its normal OAuth connection screen. For another client, inspect its documented client ID and exact redirect URI, then pass --client-id and --redirect-uri to the helper. Keep all long-lived credentials out of this conversation. Mac sleep or quitting the app makes cloud access unavailable.", in.Provider, in.Provider, shellQuote(e.host.HelperPath()), shellQuote(e.host.DataDir()), in.Provider, token)
		writeJSON(w, 200, map[string]string{"instructions": text, "expires_at": time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)})
	case r.Method == "POST" && r.URL.Path == "/cloud/setup":
		e.setup(w, r)
	case r.Method == "POST" && r.URL.Path == "/cloud/retry":
		e.mu.Lock()
		enabled := e.cfg.Enabled
		e.mu.Unlock()
		if enabled {
			if err := e.start(); err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
		}
		writeJSON(w, 200, e.host.State())
	case r.Method == "GET" && r.URL.Path == "/cloud/verify":
		e.mu.Lock()
		host := e.cfg.Hostname
		e.mu.Unlock()
		if host == "" {
			writeJSON(w, 400, map[string]string{"error": "Cloud setup is incomplete."})
			return
		}
		if err := probePublic(r.Context(), "https://"+host); err != nil {
			writeJSON(w, 503, map[string]string{"error": "Public endpoint is not ready or does not reject unauthenticated MCP access."})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "verified", "mcp_url": "https://" + host + "/mcp"})
	case r.Method == "DELETE" && r.URL.Path == "/cloud":
		e.mu.Lock()
		next := e.cfg
		next.Enabled = false
		e.mu.Unlock()
		if err := atomicJSON(filepath.Join(e.host.DataDir(), "cloud.json"), next); err != nil {
			writeJSON(w, 500, map[string]string{"error": "Could not save cloud settings."})
			return
		}
		e.mu.Lock()
		e.cfg = next
		e.oauth = nil
		e.mu.Unlock()
		e.supervisor.Stop()
		e.host.SetPublicURL("")
		writeJSON(w, 200, e.host.State())
	default:
		http.NotFound(w, r)
	}
}
func (e *Extension) setup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token      string   `json:"token"`
		Provider   string   `json:"provider"`
		Hostname   string   `json:"hostname"`
		Executable string   `json:"executable"`
		TunnelID   string   `json:"tunnel_id"`
		Credential string   `json:"credential"`
		ClientID   string   `json:"client_id"`
		Redirects  []string `json:"redirect_uris"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		writeJSON(w, 400, map[string]string{"error": "Invalid cloud setup request."})
		return
	}
	e.mu.Lock()
	inv, ok := e.invites[fingerprint(in.Token)]
	old := e.cfg
	e.mu.Unlock()
	if !ok || time.Now().After(inv.Expires) || inv.Provider != in.Provider {
		writeJSON(w, 403, map[string]string{"error": "Cloud invitation expired or was already used. Generate new instructions in the app."})
		return
	}
	if len(in.Redirects) == 0 {
		writeJSON(w, 400, map[string]string{"error": "The cloud client's exact redirect URI is required."})
		return
	}
	for _, u := range in.Redirects {
		if !validHTTPS(u) {
			writeJSON(w, 400, map[string]string{"error": "Cloud redirect must use HTTPS."})
			return
		}
	}
	cfg := TunnelConfig{Enabled: true, Provider: in.Provider, Hostname: in.Hostname, Executable: in.Executable, TunnelID: in.TunnelID, CredentialRef: old.CredentialRef}
	if in.Credential != "" {
		cfg.CredentialRef = "tunnel-" + opaque()
	}
	if err := validateTunnel(cfg); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if in.Credential == "" {
		var err error
		in.Credential, err = e.secrets.Get(CredentialService, cfg.CredentialRef)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "Provider credentials are missing. Run setup again."})
			return
		}
	}
	if _, err := BuildTunnelCommand(cfg, e.host.LocalURL(), e.host.DataDir(), in.Credential); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := e.secrets.Set(CredentialService, cfg.CredentialRef, in.Credential); err != nil {
		writeJSON(w, 500, map[string]string{"error": "Could not save provider credentials to Keychain."})
		return
	}
	if err := atomicJSON(filepath.Join(e.host.DataDir(), "cloud.json"), cfg); err != nil {
		if cfg.CredentialRef != old.CredentialRef {
			_ = e.secrets.Delete(CredentialService, cfg.CredentialRef)
		}
		writeJSON(w, 500, map[string]string{"error": "Could not save cloud configuration."})
		return
	}
	e.mu.Lock()
	e.cfg = cfg
	e.mu.Unlock()
	if err := e.start(); err != nil {
		e.mu.Lock()
		e.configError = err.Error()
		e.mu.Unlock()
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	grant, err := e.host.CreateCloudGrant(inv.Name, "cloud")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Confirm chat scope before pairing a cloud agent."})
		return
	}
	e.mu.Lock()
	o := e.oauth
	delete(e.invites, fingerprint(in.Token))
	e.mu.Unlock()
	handoff, err := o.Prepare(grant, in.ClientID, in.Redirects)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "Cloud browser setup could not be prepared."})
		return
	}
	if old.CredentialRef != "" && old.CredentialRef != cfg.CredentialRef {
		_ = e.secrets.Delete(CredentialService, old.CredentialRef)
	}
	writeJSON(w, 200, map[string]string{"grant_id": grant, "mcp_url": "https://" + cfg.Hostname + "/mcp", "action": "https://" + cfg.Hostname + "/oauth/bootstrap", "handoff": handoff})
}
func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'" }

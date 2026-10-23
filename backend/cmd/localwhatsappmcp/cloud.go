package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/massi/localwhatsappmcp/internal/cloud"
	"github.com/massi/localwhatsappmcp/internal/secret"
	"github.com/massi/localwhatsappmcp/internal/service"
	"github.com/massi/localwhatsappmcp/internal/setup"
	"gopkg.in/yaml.v3"
)

type protectedSecrets struct{}

func (protectedSecrets) Set(s, a, v string) error        { return secret.Set(s, a, v) }
func (protectedSecrets) Get(s, a string) (string, error) { return secret.Get(s, a) }
func (protectedSecrets) Delete(s, a string) error        { return secret.Delete(s, a) }
func init() {
	extensionFactory = func(s *service.Service) (service.Extension, error) { return cloud.New(s, protectedSecrets{}) }
	extraCommand = runCloud
}
func runCloud(args []string) error {
	if len(args) > 0 && args[0] == "tunnel-worker" {
		return cloud.RunTunnelWorker(os.Stdin)
	}
	if len(args) > 0 && args[0] == "cloud-authorize" {
		return errors.New("This browser handoff is not valid. Use freshly generated cloud setup instructions in the app, then connect your cloud agent in the browser opened by the helper.")
	}
	if len(args) > 0 && args[0] == "cloud" {
		args = args[1:]
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Println(`Cloud setup keeps provider credentials out of chat and lets the app own the tunnel.

cloud setup --provider ngrok --hostname <stable-hostname> --token-stdin [--client chatgpt]
cloud setup --provider cloudflared --hostname <stable-hostname> --tunnel-id <UUID> --credentials-file <path> --token-stdin
cloud verify [--data-dir <path>]

Options: --data-dir, --executable, --client-id, --redirect-uri, --ngrok-config.
The invitation is read from stdin. Provider secrets are imported privately into Keychain.
Existing ngrok login is reused; otherwise a native secure dialog guides token import.
Cloudflare requires a named tunnel and domain in your Cloudflare account. Create/login
with cloudflared first; the helper imports its credentials file without printing it.
--client chatgpt sets the documented CIMD and stable redirect; other clients need their
exact documented client ID (optional for DCR) and HTTPS redirect URI.
The final browser Connect step must use the browser opened by the helper.`)
		return nil
	}
	command := args[0]
	fs := flag.NewFlagSet("cloud "+command, flag.ContinueOnError)
	home, _ := os.UserHomeDir()
	dir := fs.String("data-dir", filepath.Join(home, "Library", "Application Support", "LocalWhatsAppMCP"), "application data directory")
	provider := fs.String("provider", "ngrok", "ngrok or cloudflared")
	hostname := fs.String("hostname", "", "stable provider hostname")
	executable := fs.String("executable", "", "absolute provider executable")
	tunnelID := fs.String("tunnel-id", "", "named Cloudflare tunnel UUID")
	credentialFile := fs.String("credentials-file", "", "Cloudflare credentials file")
	ngrokConfig := fs.String("ngrok-config", "", "existing ngrok config path")
	client := fs.String("client", "chatgpt", "cloud client profile")
	clientID := fs.String("client-id", "", "exact OAuth client ID")
	redirect := fs.String("redirect-uri", "", "exact HTTPS client callback")
	tokenStdin := fs.Bool("token-stdin", false, "read invitation from stdin")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	ctx := context.Background()
	if command == "verify" {
		_, err := setup.Control(ctx, *dir, "GET", "/cloud/verify", nil)
		if err == nil {
			fmt.Println("Public metadata verified; unauthenticated MCP access rejected.")
		}
		return err
	}
	if command != "setup" {
		return errors.New("unknown cloud command; use cloud --help")
	}
	if !*tokenStdin {
		return errors.New("cloud setup requires --token-stdin")
	}
	var existing cloud.TunnelConfig
	if previous, readErr := setup.Control(ctx, *dir, "GET", "/cloud/config", nil); readErr == nil {
		_ = json.Unmarshal(previous, &existing)
	}
	reuse := existing.Provider == *provider && existing.CredentialRef != "" && (*hostname == "" || *hostname == existing.Hostname)
	if reuse {
		if *hostname == "" {
			*hostname = existing.Hostname
		}
		if *executable == "" {
			*executable = existing.Executable
		}
		if *tunnelID == "" {
			*tunnelID = existing.TunnelID
		}
	}
	if *hostname == "" {
		return errors.New("first cloud setup requires the account's stable --hostname")
	}
	if *provider != "ngrok" && *provider != "cloudflared" {
		return errors.New("choose ngrok or cloudflared")
	}
	b, err := io.ReadAll(io.LimitReader(os.Stdin, 4097))
	if err != nil || len(b) > 4096 {
		return errors.New("could not read pairing invitation")
	}
	invitation := strings.TrimSpace(string(b))
	if invitation == "" {
		return errors.New("pairing invitation is empty")
	}
	if *client == "chatgpt" {
		if *clientID == "" {
			*clientID = "https://chatgpt.com/oauth/client.json"
		}
		if *redirect == "" {
			*redirect = "https://chatgpt.com/connector_platform_oauth_redirect"
		}
	}
	if *redirect == "" {
		return errors.New("specify the client's exact documented --redirect-uri")
	}
	if *executable == "" {
		*executable, err = providerExecutable(*provider)
		if err != nil {
			return err
		}
	}
	var credential string
	if reuse && *ngrokConfig == "" && *credentialFile == "" {
		// Reuse the app-owned Keychain reference without exposing it to this helper output.
	} else if *provider == "ngrok" {
		credential, err = importNgrokCredential(*ngrokConfig)
	} else {
		if *credentialFile == "" {
			return errors.New("pass the named tunnel --credentials-file path after cloudflared browser login and tunnel creation")
		}
		raw, e := os.ReadFile(*credentialFile)
		if e != nil {
			return errors.New("Cloudflare credential file could not be read")
		}
		if len(raw) > 65536 {
			return errors.New("invalid Cloudflare credential file")
		}
		credential = string(raw)
	}
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"token": invitation, "provider": *provider, "hostname": *hostname, "executable": *executable, "tunnel_id": *tunnelID, "credential": credential, "client_id": *clientID, "redirect_uris": []string{*redirect}})
	response, err := setup.Control(ctx, *dir, "POST", "/cloud/setup", body)
	if err != nil {
		return err
	}
	var out struct {
		GrantID string `json:"grant_id"`
		URL     string `json:"mcp_url"`
		Action  string `json:"action"`
		Handoff string `json:"handoff"`
	}
	if json.Unmarshal(response, &out) != nil || out.Handoff == "" {
		return errors.New("cloud setup returned an invalid private handoff")
	}
	fmt.Println("Provider configuration saved in the app. Waiting for the public endpoint…")
	deadline := time.Now().Add(90 * time.Second)
	verified := false
	for time.Now().Before(deadline) {
		if _, err = setup.Control(ctx, *dir, "GET", "/cloud/verify", nil); err == nil {
			verified = true
			break
		}
		time.Sleep(2 * time.Second)
	}
	if !verified {
		return errors.New("tunnel configuration is saved, but the public endpoint did not become ready; check cloud status, run cloud verify, then generate a new pairing invitation")
	}
	if err = openPrivateHandoff(out.Action, out.Handoff); err != nil {
		return err
	}
	fmt.Printf("Cloud connection prepared. MCP URL: %s\nFinish the cloud client's normal OAuth connection in the same browser. Sending is disabled.\n", out.URL)
	return nil
}
func providerExecutable(name string) (string, error) {
	if p, e := exec.LookPath(name); e == nil {
		return filepath.Abs(p)
	}
	for _, p := range []string{filepath.Join("/opt/homebrew/bin", name), filepath.Join("/usr/local/bin", name)} {
		if info, e := os.Stat(p); e == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s is missing; install it using the official provider installer or Homebrew, then retry", name)
}
func importNgrokCredential(path string) (string, error) {
	home, _ := os.UserHomeDir()
	paths := []string{path}
	if path == "" {
		paths = []string{filepath.Join(home, "Library/Application Support/ngrok/ngrok.yml"), filepath.Join(home, ".config/ngrok/ngrok.yml"), filepath.Join(home, ".ngrok2/ngrok.yml")}
	}
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		if len(b) > 1<<20 {
			return "", errors.New("ngrok configuration is too large")
		}
		var config struct {
			Authtoken string `yaml:"authtoken"`
			Agent     struct {
				Authtoken string `yaml:"authtoken"`
			} `yaml:"agent"`
		}
		if yaml.Unmarshal(b, &config) != nil {
			return "", errors.New("existing ngrok configuration is invalid")
		}
		if config.Agent.Authtoken != "" {
			return config.Agent.Authtoken, nil
		}
		if config.Authtoken != "" {
			return config.Authtoken, nil
		}
	}
	// This explicit setup action opens provider login. Recovery never does so.
	_ = exec.Command("/usr/bin/open", "https://dashboard.ngrok.com/get-started/your-authtoken").Run()
	script := `set answer to display dialog "Sign in to ngrok in your browser, copy your personal authtoken, and paste it here. It will be stored directly in Keychain and never shown to your agent." default answer "" with hidden answer buttons {"Cancel", "Save to Keychain"} default button "Save to Keychain" with title "LocalWhatsAppMCP · ngrok"\nreturn text returned of answer`
	output, e := exec.Command("/usr/bin/osascript", "-e", strings.ReplaceAll(script, `\n`, "\n")).Output()
	if e != nil {
		return "", errors.New("ngrok credential import was cancelled; no credential was saved")
	}
	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", errors.New("ngrok credential is empty")
	}
	return token, nil
}

var handoffTemplate = template.Must(template.New("handoff").Parse(`<!doctype html><html><head><meta name="referrer" content="no-referrer"><title>Connect LocalWhatsAppMCP</title></head><body><p>Opening your private connection session…</p><form id="handoff" method="post" action="{{.Action}}"><input type="hidden" name="handoff" value="{{.Token}}"><button type="submit">Continue</button></form><script nonce="{{.Nonce}}">document.getElementById('handoff').submit()</script></body></html>`))

func openPrivateHandoff(action, token string) error {
	u, err := url.Parse(action)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.Path != "/oauth/bootstrap" || u.User != nil {
		return errors.New("invalid browser handoff destination")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return errors.New("could not open private browser handoff")
	}
	defer listener.Close()
	// This random local route is only a delivery nonce; the authorization secret
	// is sent in the HTML POST body and never placed in a URL or browser history.
	nonceBytes := make([]byte, 32)
	if _, err = rand.Read(nonceBytes); err != nil {
		return err
	}
	nonce := fmt.Sprintf("%x", nonceBytes)
	route := "/launch/" + nonce
	done := make(chan struct{}, 1)
	var consumed atomic.Bool
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != route || r.Host != listener.Addr().String() {
			http.NotFound(w, r)
			return
		}
		if consumed.Swap(true) {
			http.Error(w, "handoff already used", 410)
			return
		}
		done <- struct{}{}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"'; form-action "+u.Scheme+"://"+u.Host+"; base-uri 'none'; frame-ancestors 'none'")
		_ = handoffTemplate.Execute(w, map[string]string{"Action": action, "Token": token, "Nonce": nonce})
	})
	go server.Serve(listener)
	defer server.Close()
	if err = exec.Command("/usr/bin/open", "http://"+listener.Addr().String()+route).Run(); err != nil {
		return errors.New("could not open your browser; generate a new cloud invitation and retry")
	}
	select {
	case <-done:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		return nil
	case <-time.After(2 * time.Minute):
		return errors.New("private browser handoff expired; generate a new cloud invitation")
	}
}

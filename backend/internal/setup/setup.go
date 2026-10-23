// Package setup installs non-secret client configuration and keeps credentials in Keychain.
package setup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/massi/localwhatsappmcp/internal/secret"
	"github.com/massi/localwhatsappmcp/internal/service"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pelletier/go-toml/v2"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const KeychainService = "com.localwhatsappmcp.agent"

func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func Control(ctx context.Context, dataDir, method, path string, body []byte) ([]byte, error) {
	if !strings.HasPrefix(path, "/") {
		return nil, errors.New("private control path must start with /")
	}
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", filepath.Join(dataDir, "control.sock"))
	}}, Timeout: 120 * time.Second}
	req, err := http.NewRequestWithContext(ctx, method, "http://local"+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("LocalWhatsAppMCP is not running; open the app and retry")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 400 {
		var out struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(b, &out) == nil && out.Error != "" {
			return nil, errors.New(out.Error)
		}
		return nil, errors.New("private setup request failed")
	}
	return b, nil
}
func InstallConfig(client, path, helper, dataDir, id, url string) error {
	if client == "stdio" {
		return nil
	}
	if client != "codex" && client != "claude" {
		return errors.New("unsupported client")
	}
	if len(id) < 1 {
		return errors.New("connection identifier is missing")
	}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("client configuration could not be read")
	}
	doc := map[string]any{}
	if len(b) > 0 {
		if client == "codex" {
			err = toml.Unmarshal(b, &doc)
		} else {
			err = json.Unmarshal(b, &doc)
		}
		if err != nil {
			return errors.New("client configuration is invalid; repair it before installing")
		}
	}
	command := Quote(helper) + " headers --data-dir " + Quote(dataDir) + " --connection " + Quote(id)
	rootKey := "mcp_servers"
	if client == "claude" {
		rootKey = "mcpServers"
	}
	servers, ok := doc[rootKey].(map[string]any)
	if !ok {
		if doc[rootKey] != nil {
			return errors.New("client MCP configuration has an unexpected shape")
		}
		servers = map[string]any{}
		doc[rootKey] = servers
	}
	name := "localwhatsapp_" + id[:min(len(id), 12)]
	entry := map[string]any{"url": strings.TrimSuffix(url, "/") + "/mcp"}
	if client == "codex" {
		entry["http_headers_helper"] = command
		entry["startup_timeout_sec"] = 30
		entry["tool_timeout_sec"] = 120
	} else {
		entry["type"] = "http"
		entry["headersHelper"] = command
	}
	servers[name] = entry
	if client == "codex" {
		b, err = toml.Marshal(doc)
	} else {
		b, err = json.MarshalIndent(doc, "", "  ")
	}
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return service.AtomicWrite(path, append(b, '\n'), 0600)
}

type Connection struct {
	ID         string `json:"id"`
	Client     string `json:"client"`
	URL        string `json:"url"`
	ConfigPath string `json:"config_path"`
}

func connectionPath(dir, id string) string { return filepath.Join(dir, "connections", id+".json") }
func ConfigPath(client string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch client {
	case "codex":
		root := os.Getenv("CODEX_HOME")
		if root == "" {
			root = filepath.Join(home, ".codex")
		}
		return filepath.Join(root, "config.toml"), nil
	case "claude":
		return filepath.Join(home, ".claude.json"), nil
	case "stdio":
		return "", nil
	}
	return "", errors.New("choose codex, claude, or stdio")
}
func Install(ctx context.Context, dir, helper, client, token, resume string) (Connection, error) {
	var c Connection
	if resume != "" {
		var err error
		c, err = Load(dir, resume)
		if err != nil {
			return c, err
		}
		client = c.Client
	} else {
		b, _ := json.Marshal(map[string]string{"token": strings.TrimSpace(token)})
		raw, err := Control(ctx, dir, "POST", "/pairing/redeem", b)
		if err != nil {
			return c, err
		}
		var grant struct{ ID, Secret, Client, URL string }
		if json.Unmarshal(raw, &grant) != nil || grant.ID == "" || grant.Secret == "" {
			return c, errors.New("invalid pairing response")
		}
		if grant.Client != client {
			_, _ = Control(ctx, dir, "DELETE", "/agents/"+grant.ID, nil)
			return c, errors.New("invitation was created for another client; create the correct invitation")
		}
		if err = secret.Set(KeychainService, grant.ID, grant.Secret); err != nil {
			_, _ = Control(ctx, dir, "DELETE", "/agents/"+grant.ID, nil)
			return c, err
		}
		path, err := ConfigPath(client)
		if err != nil {
			return c, err
		}
		c = Connection{ID: grant.ID, Client: client, URL: grant.URL, ConfigPath: path}
		if err = os.MkdirAll(filepath.Join(dir, "connections"), 0700); err != nil {
			return c, err
		}
		if err = service.AtomicJSON(connectionPath(dir, c.ID), c); err != nil {
			return c, errors.New("credential saved in Keychain but connection metadata could not be saved; remove this connection in the app and pair again")
		}
	}
	if err := InstallConfig(client, c.ConfigPath, helper, dir, c.ID, c.URL); err != nil {
		return c, fmt.Errorf("credential saved; repair client configuration then run setup --resume %s: %w", c.ID, err)
	}
	if err := Doctor(ctx, dir, c.ID); err != nil {
		return c, fmt.Errorf("connection installed; verification failed. Run doctor --connection %s after recovery: %w", c.ID, err)
	}
	return c, nil
}
func Load(dir, id string) (Connection, error) {
	var c Connection
	if len(id) > 100 || strings.ContainsAny(id, "/\\.") {
		return c, errors.New("invalid connection identifier")
	}
	b, err := os.ReadFile(connectionPath(dir, id))
	if err != nil || json.Unmarshal(b, &c) != nil || c.ID != id {
		return c, errors.New("connection metadata is unavailable; pair again in the app")
	}
	return c, nil
}
func Headers(dir, id string) (map[string]string, error) {
	if _, err := Load(dir, id); err != nil {
		return nil, err
	}
	value, err := secret.Get(KeychainService, id)
	if err != nil {
		return nil, err
	}
	return map[string]string{"Authorization": "Bearer " + value}, nil
}

type bearerTransport struct{ secret string }

func (t bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.secret)
	return http.DefaultTransport.RoundTrip(r)
}
func Connect(ctx context.Context, dir, id string) (*mcp.ClientSession, error) {
	c, err := Load(dir, id)
	if err != nil {
		return nil, err
	}
	value, err := secret.Get(KeychainService, id)
	if err != nil {
		return nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "LocalWhatsAppMCP helper", Version: "0.1.0"}, nil)
	return client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: strings.TrimSuffix(c.URL, "/") + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{value}, Timeout: 120 * time.Second}, DisableStandaloneSSE: true}, nil)
}
func Doctor(ctx context.Context, dir, id string) error {
	session, err := Connect(ctx, dir, id)
	if err != nil {
		return errors.New("authenticated MCP connection could not initialize")
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) < 9 {
		return errors.New("MCP tool discovery failed")
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "check_status", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		return errors.New("MCP status verification failed; confirm chat scope and connection in the app")
	}
	return nil
}
func Stdio(ctx context.Context, dir, id string) error {
	remote, err := Connect(ctx, dir, id)
	if err != nil {
		return errors.New("authenticated local MCP service is unavailable")
	}
	defer remote.Close()
	listed, err := remote.ListTools(ctx, nil)
	if err != nil {
		return errors.New("MCP tool discovery failed")
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "LocalWhatsAppMCP stdio adapter", Version: "0.1.0"}, nil)
	for _, tool := range listed.Tools {
		definition := *tool
		server.AddTool(&definition, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return remote.CallTool(ctx, &mcp.CallToolParams{Name: req.Params.Name, Arguments: json.RawMessage(req.Params.Arguments)})
		})
	}
	return server.Run(ctx, &mcp.StdioTransport{})
}

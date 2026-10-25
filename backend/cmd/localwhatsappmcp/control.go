package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// controlRequest carries native app operations over its private Unix socket.
func controlRequest(ctx context.Context, dataDir, method, path string, body []byte) ([]byte, error) {
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
		return nil, errors.New("private app control request failed")
	}
	return b, nil
}

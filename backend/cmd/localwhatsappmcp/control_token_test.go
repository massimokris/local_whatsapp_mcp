package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/service"
)

// Synthetic session tokens. They only exist in tests.
var (
	testControlToken  = base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	otherControlToken = base64.RawURLEncoding.EncodeToString([]byte("fedcba9876543210fedcba9876543210"))
)

func TestReadControlTokenAcceptsOneLine(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n"} {
		reader := bufio.NewReaderSize(strings.NewReader(testControlToken+ending+"rest"), 128)
		token, err := readControlToken(reader, time.Second)
		if err != nil || token != testControlToken {
			t.Fatalf("token line %q not accepted: %v", ending, err)
		}
		rest, _ := io.ReadAll(reader)
		if string(rest) != "rest" {
			t.Fatalf("bytes after the token line were lost: %q", rest)
		}
	}
}

func TestReadControlTokenRejectsMalformed(t *testing.T) {
	for _, input := range []string{"", testControlToken, strings.Repeat("A", 65) + "\n", strings.Repeat("A", 300) + "\n",
		"!" + testControlToken[1:] + "\n", testControlToken + "=\n", "\r" + testControlToken + "\n", testControlToken[:42] + "\r\n"} {
		if _, err := readControlToken(bufio.NewReaderSize(strings.NewReader(input), 128), time.Second); err == nil {
			t.Errorf("malformed token line accepted: %q", input)
		}
	}
}

func TestReadControlTokenTimesOut(t *testing.T) {
	r, w := io.Pipe()
	defer w.Close()
	start := time.Now()
	if _, err := readControlToken(bufio.NewReaderSize(r, 128), 50*time.Millisecond); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("missing token did not time out: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout took too long")
	}
}

func TestServeRefusesControlSocketWithoutToken(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "lwmcp-ctl-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	err = serve(context.Background(), service.Options{DataDir: dir, CursorKey: []byte("01234567890123456789012345678901")}, "/usr/bin/true", "127.0.0.1:0", "")
	if err == nil || !strings.Contains(err.Error(), "control socket") {
		t.Fatalf("serve accepted a missing session token: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "control.sock")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("control socket was created without a session token")
	}
	if code := startupErrorCode(err); code != "control_unavailable" {
		t.Fatalf("unexpected startup code %q", code)
	}
}

func TestControlSocketRequiresSessionToken(t *testing.T) {
	// sun_path on macOS is 104 bytes: keep the directory short.
	dir, err := os.MkdirTemp("/tmp", "lwmcp-ctl-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() {
		served <- serve(ctx, service.Options{DataDir: dir, CursorKey: []byte("01234567890123456789012345678901")}, "/usr/bin/true", "127.0.0.1:0", testControlToken)
	}()
	defer func() {
		cancel()
		select {
		case <-served:
		case <-time.After(15 * time.Second):
			t.Error("serve did not stop")
		}
	}()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(dir, "control.sock")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("control socket did not appear")
		}
	}
	for _, token := range []string{"", otherControlToken} {
		if _, err := controlRequest(context.Background(), dir, "GET", "/state", token, nil); err == nil || !strings.Contains(err.Error(), "current app session") {
			t.Fatalf("control accepted token %q: %v", token, err)
		}
	}
	out, err := controlRequest(context.Background(), dir, "GET", "/state", testControlToken, nil)
	var state map[string]any
	if err != nil || json.Unmarshal(out, &state) != nil || state["scope"] == nil {
		t.Fatalf("session token did not reach /state: %v %s", err, out)
	}
}

func TestControlCommandFixture(t *testing.T) {
	dir := os.Getenv("LWMCP_CONTROL_COMMAND_FIXTURE")
	if dir == "" {
		return
	}
	err := run([]string{"control", "--data-dir", dir, "--method", "GET", "--path", "/state"})
	if err == nil {
		os.Stdout.WriteString("unexpected success")
		return
	}
	os.Stdout.WriteString(err.Error())
}

func TestControlCommandReadsTokenFromStdin(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestControlCommandFixture$")
	cmd.Env = append(os.Environ(), "LWMCP_CONTROL_COMMAND_FIXTURE="+t.TempDir())
	cmd.Stdin = strings.NewReader("")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "requires the current app session token") || strings.Contains(string(out), "not running") {
		t.Fatalf("control did not require a stdin token before connecting: %s", out)
	}
}

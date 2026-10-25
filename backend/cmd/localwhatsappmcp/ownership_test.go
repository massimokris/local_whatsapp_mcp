package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/service"
)

func TestServiceOwnerFixture(t *testing.T) {
	role := os.Getenv("LWMCP_SERVICE_OWNER_FIXTURE")
	if role == "" {
		return
	}
	dir := os.Getenv("LWMCP_SERVICE_FIXTURE_DIR")
	if role == "owner" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestServiceOwnerFixture$")
		cmd.Env = append(os.Environ(), "LWMCP_SERVICE_OWNER_FIXTURE=service")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		pipe, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		defer pipe.Close()
		if err = cmd.Start(); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, "service.pid"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600)
		cmd.Wait()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { io.Copy(io.Discard, os.Stdin); cancel() }()
	opt := service.Options{DataDir: dir, CursorKey: []byte("01234567890123456789012345678901")}
	if err := serve(ctx, opt, filepath.Join(dir, "wacli"), "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
}

func TestServiceAndWriterStopAfterAbruptOwnerDeath(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "lwmcp-owner-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	script := fmt.Sprintf("#!/bin/sh\necho $$ > '%s'\nprintf '%%s\\n' '{\"event\":\"policy_loaded\",\"data\":{\"revision\":1},\"ts\":1}' >&2\nexec /bin/cat >/dev/null\n", filepath.Join(dir, "engine.pid"))
	if err := os.WriteFile(filepath.Join(dir, "wacli"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	owner := exec.Command(os.Args[0], "-test.run=^TestServiceOwnerFixture$")
	owner.Stdout, owner.Stderr = os.Stdout, os.Stderr
	owner.Env = append(os.Environ(), "LWMCP_SERVICE_OWNER_FIXTURE=owner", "LWMCP_SERVICE_FIXTURE_DIR="+dir)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { owner.Process.Kill(); owner.Wait() }()
	readPID := func(name string) int {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
				if pid, e := strconv.Atoi(strings.TrimSpace(string(b))); e == nil {
					return pid
				}
			}
		}
		t.Fatal("process did not start: " + name)
		return 0
	}
	servicePID := readPID("service.pid")
	defer syscall.Kill(servicePID, syscall.SIGKILL)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		_, err := controlRequest(context.Background(), dir, "POST", "/whatsapp/connect", []byte("{}"))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("private service did not start", err)
		}
	}
	enginePID := readPID("engine.pid")
	defer syscall.Kill(enginePID, syscall.SIGKILL)
	owner.Process.Kill()
	owner.Wait()
	for _, pid := range []int{servicePID, enginePID} {
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && syscall.Kill(pid, 0) == nil; time.Sleep(10 * time.Millisecond) {
		}
		if syscall.Kill(pid, 0) == nil {
			t.Fatalf("owned process %d survived owner death", pid)
		}
	}
}

func TestStartupDiagnosticFixture(t *testing.T) {
	dir := os.Getenv("LWMCP_DIAGNOSTIC_FIXTURE")
	if dir == "" {
		return
	}
	os.Args = []string{"helper", "serve", "--wacli", "/usr/bin/true", "--data-dir", dir, "--listen", "127.0.0.1:0"}
	main()
}
func TestStartupReportsSafeStructuredRecovery(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("private-fixture-token-invalid-json"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartupDiagnosticFixture$")
	cmd.Env = append(os.Environ(), "LWMCP_DIAGNOSTIC_FIXTURE="+dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("invalid settings accepted")
	}
	if !strings.Contains(string(out), `"startup_error":"configuration_invalid"`) || strings.Contains(string(out), "private-fixture") {
		t.Fatalf("unsafe or missing recovery diagnostic: %s", out)
	}
}

func TestCredentialFailureReportsSafeStructuredRecovery(t *testing.T) {
	dir := t.TempDir()
	s, err := service.New(service.Options{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := os.WriteFile(filepath.Join(dir, "credentials.db"), []byte("private-fixture-token-invalid-database"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartupDiagnosticFixture$")
	cmd.Env = append(os.Environ(), "LWMCP_DIAGNOSTIC_FIXTURE="+dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("invalid credential database accepted")
	}
	if !strings.Contains(string(out), `"startup_error":"credentials_unavailable"`) || strings.Contains(string(out), "private-fixture") {
		t.Fatalf("unsafe or missing credential recovery diagnostic: %s", out)
	}
}

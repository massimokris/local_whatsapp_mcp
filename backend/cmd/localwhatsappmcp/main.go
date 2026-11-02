package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/massi/localwhatsappmcp/internal/engine"
	"github.com/massi/localwhatsappmcp/internal/service"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if len(os.Args) > 1 && os.Args[1] == "control" {
			json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
		} else if len(os.Args) > 1 && os.Args[1] == "serve" {
			_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"startup_error": startupErrorCode(err)})
		} else {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(1)
	}
}
func defaultDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Application Support", "LocalWhatsAppMCP")
}
func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: localwhatsappmcp serve|control")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("data-dir", defaultDir(), "private application data directory")
	switch args[0] {
	case "serve":
		binary := fs.String("wacli", "", "bundled WhatsApp engine")
		listen := fs.String("listen", "127.0.0.1:32191", "loopback MCP address")
		ownerStdin := fs.Bool("owner-stdin", false, "stop when the owning app closes stdin")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *binary == "" {
			return errors.New("--wacli is required")
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var controlToken string
		if *ownerStdin {
			// The first line on the ownership pipe is the app session token.
			// EOF detection must reuse this reader so buffered bytes are not lost.
			reader := bufio.NewReaderSize(os.Stdin, 128)
			token, err := readControlToken(reader, 10*time.Second)
			if err != nil {
				return fmt.Errorf("private app control socket could not be created: %w", err)
			}
			controlToken = token
			go func() { _, _ = io.Copy(io.Discard, reader); cancel() }()
		}
		return serve(ctx, service.Options{DataDir: *dir}, *binary, *listen, controlToken)
	case "control":
		method := fs.String("method", "GET", "")
		path := fs.String("path", "/state", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		// stdin: one session token line, then the optional JSON body.
		reader := bufio.NewReaderSize(os.Stdin, 128)
		token, err := readControlToken(reader, 10*time.Second)
		if err != nil {
			return errors.New("private control requires the current app session token")
		}
		var b []byte
		if *method != "GET" && *method != "DELETE" {
			b, err = io.ReadAll(io.LimitReader(reader, 1<<20))
			if err != nil {
				return err
			}
			if len(b) == 0 {
				b = []byte("{}")
			}
		}
		out, err := controlRequest(context.Background(), *dir, *method, *path, token, b)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(append(out, '\n'))
		return err
	default:
		return errors.New("unknown service command")
	}
}
func serve(parent context.Context, options service.Options, binary, address, controlToken string) error {
	dir := options.DataDir
	host, _, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return errors.New("MCP listener must use an explicit loopback IP address")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dir, "service.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return errors.New("LocalWhatsAppMCP service is already running")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	tcp, err := net.Listen("tcp", address)
	if err != nil {
		return errors.New("the local MCP port is in use; quit the other LocalWhatsAppMCP service and retry")
	}
	defer tcp.Close()
	baseURL := "http://" + tcp.Addr().String()
	release, err := waitForWriter(dir)
	if err != nil {
		return err
	}
	options.Engine, options.LocalURL = engine.New(binary, dir), baseURL
	s, err := service.New(options)
	release()
	if err != nil {
		return err
	}
	defer s.Close()
	// Checked after service.New so configuration and credential failures keep their own startup codes.
	if !service.ValidControlToken(controlToken) {
		return errors.New("private app control socket could not be created")
	}
	socket := filepath.Join(dir, "control.sock")
	if err = os.Remove(socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	unix, err := net.Listen("unix", socket)
	if err != nil {
		return errors.New("private app control socket could not be created")
	}
	defer unix.Close()
	defer os.Remove(socket)
	if err = os.Chmod(socket, 0600); err != nil {
		return err
	}
	local := &http.Server{Handler: s.LocalHandler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	private := &http.Server{Handler: service.RequireControlToken(controlToken, s.ControlHandler()), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(parent, syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	failures := make(chan error, 2)
	go func() { failures <- local.Serve(tcp) }()
	go func() { failures <- private.Serve(unix) }()
	go func() { _ = s.ResumeSavedSession(ctx) }()
	select {
	case <-ctx.Done():
	case err = <-failures:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	// Cancel engine work before waiting for in-flight HTTP calls.
	_ = s.Close()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	local.Shutdown(shutdown)
	private.Shutdown(shutdown)
	return err
}

// readControlToken reads exactly one newline-terminated token line from r.
// It returns an error on EOF before newline, a line longer than 64 bytes,
// an invalid token, or when timeout elapses first.
func readControlToken(r *bufio.Reader, timeout time.Duration) (string, error) {
	type result struct {
		token string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		line, err := r.ReadSlice('\n')
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			done <- result{err: errors.New("control session token line is too long")}
			return
		case err != nil:
			done <- result{err: errors.New("control session token is missing")}
			return
		}
		value := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
		if len(value) > 64 || !service.ValidControlToken(value) {
			done <- result{err: errors.New("control session token is invalid")}
			return
		}
		done <- result{token: value}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case res := <-done:
		return res.token, res.err
	case <-timer.C:
		return "", errors.New("timed out waiting for the control session token")
	}
}

// Wait for the previous app-owned writer to finish after its lifetime pipe closes.
// The upstream lock file is only locked; its diagnostic contents are never changed.
func waitForWriter(dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, "store", "LOCK"), os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return func() {}, nil
	}
	if err != nil {
		return nil, errors.New("cannot inspect the WhatsApp writer lock")
	}
	deadline := time.Now().Add(25 * time.Second)
	for {
		if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
			return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, errors.New("previous WhatsApp writer is still stopping; retry after it exits")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Startup output is an allowlisted protocol, never arbitrary error/log content.
func startupErrorCode(err error) string {
	detail := err.Error()
	switch {
	case strings.Contains(detail, "saved configuration is missing"):
		return "configuration_missing"
	case strings.Contains(detail, "saved configuration is invalid"), strings.Contains(detail, "saved chat scope is invalid"):
		return "configuration_invalid"
	case errors.Is(err, service.ErrCredentials):
		return "credentials_unavailable"
	case strings.Contains(detail, "port is in use"):
		return "port_in_use"
	case strings.Contains(detail, "service is already running"):
		return "already_running"
	case strings.Contains(detail, "previous WhatsApp writer"):
		return "writer_stopping"
	case strings.Contains(detail, "control socket"):
		return "control_unavailable"
	}
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return "storage_unavailable"
	}
	return "startup_failed"
}

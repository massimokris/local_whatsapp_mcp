package cloud

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// RunTunnelWorker isolates provider lifetime from an abruptly terminated service.
// The first stdin line is a bounded private command; EOF on that same pipe owns
// the provider's lifetime. Credentials never become worker arguments or files.
func RunTunnelWorker(input io.Reader) error {
	reader := bufio.NewReaderSize(input, 64<<10)
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return errors.New("tunnel worker did not receive a valid private command")
	}
	var spec CommandSpec
	if json.Unmarshal(line, &spec) != nil || !filepath.IsAbs(spec.Executable) {
		return errors.New("tunnel worker command is invalid")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _, _ = io.Copy(io.Discard, reader); cancel() }()
	if ctx.Err() != nil {
		return nil
	}
	cmd := exec.Command(spec.Executable, spec.Args...)
	cmd.Env = append(cleanProviderEnvironment(os.Environ()), spec.Env...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if cmd.Start() != nil {
		return errors.New("the tunnel provider could not start")
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	// Kill remaining descendants even if the provider itself exits first.
	defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	select {
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-exited
		}
		return nil
	case err := <-exited:
		if err != nil {
			return errors.New("the tunnel provider stopped")
		}
		return nil
	}
}

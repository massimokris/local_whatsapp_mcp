package engine

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Engine struct {
	mu          sync.Mutex
	binary, dir string
	status      Status
	cancel      context.CancelFunc
	done        chan struct{}
}

func New(binary, dir string) *Engine {
	return &Engine{binary: binary, dir: dir, status: Status{Status: "disconnected", Detail: "Connect WhatsApp to import available history."}}
}

// RestoreSession reads upstream-owned authentication before the service publishes
// its first state. Link intent alone does not prove that a QR was ever scanned.
// It performs no WhatsApp network work and is called before starting the writer.
func (e *Engine) RestoreSession(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		return errors.New("stop WhatsApp sync before checking the saved session")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	authenticated, err := e.sessionAuthenticated(ctx)
	if err != nil {
		return errors.New("the saved WhatsApp session could not be checked; check its file permissions and reopen the app")
	}
	e.status.Linked = authenticated
	e.status.QR = ""
	if authenticated {
		e.status.Status = "connecting"
		e.status.Detail = "Reconnecting to WhatsApp using the saved session."
	}
	return nil
}

func (e *Engine) sessionAuthenticated(ctx context.Context) (bool, error) {
	raw, err := e.Call(ctx, false, "auth", "status")
	var status struct {
		Authenticated *bool `json:"authenticated"`
	}
	if err != nil || json.Unmarshal(raw, &status) != nil || status.Authenticated == nil {
		return false, errors.New("the saved WhatsApp session could not be checked")
	}
	return *status.Authenticated, nil
}
func (e *Engine) Status() Status {
	e.mu.Lock()
	status := e.status
	e.mu.Unlock()
	status.LastSyncedAt = e.lastSuccessfulSync()
	return status
}

// Read the engine's persisted successful-sync marker rather than treating a
// connection, history-start event, or process heartbeat as a completed sync.
func (e *Engine) lastSuccessfulSync() string {
	u := url.URL{Scheme: "file", Path: filepath.Join(e.dir, "store", "wacli.db")}
	db, err := sql.Open("sqlite3", u.String()+"?mode=ro&_query_only=true&_busy_timeout=50")
	if err != nil {
		return ""
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var timestamp sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT last_sync_at FROM retrieval_state WHERE id=1`).Scan(&timestamp); err != nil || !timestamp.Valid || timestamp.Int64 <= 0 {
		return ""
	}
	return time.Unix(timestamp.Int64, 0).UTC().Format(time.RFC3339)
}

func (e *Engine) args(args ...string) []string {
	return append([]string{"--store", filepath.Join(e.dir, "store"), "--content-policy", filepath.Join(e.dir, "content-policy.json")}, args...)
}
func (e *Engine) Start(ctx context.Context, p Policy, link bool) error {
	e.mu.Lock()
	if e.cancel != nil {
		e.mu.Unlock()
		return errors.New("sync writer is already running")
	}
	if err := os.MkdirAll(filepath.Join(e.dir, "store"), 0700); err != nil {
		e.mu.Unlock()
		return errors.New("cannot create the private WhatsApp store")
	}
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	e.cancel = cancel
	e.done = done
	e.status.Status = "connecting"
	e.status.Detail = "Starting WhatsApp and applying saved chat scope."
	e.status.QR = ""
	e.mu.Unlock()
	ack := make(chan error, 1)
	go e.run(runCtx, done, p, link, ack)
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	select {
	case err := <-ack:
		if err != nil {
			cancel()
			<-done
		}
		return err
	case <-ctx.Done():
		cancel()
		<-done
		return ctx.Err()
	case <-timer.C:
		cancel()
		<-done
		return errors.New("WhatsApp engine did not acknowledge the saved chat policy")
	}
}
func (e *Engine) Stop(ctx context.Context) error {
	e.mu.Lock()
	cancel, done := e.cancel, e.done
	e.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		e.mu.Lock()
		if e.done == done {
			e.cancel = nil
			e.done = nil
		}
		e.mu.Unlock()
		return nil
	case <-ctx.Done():
		return errors.New("waiting for the WhatsApp writer to stop; chat scope remains blocked")
	}
}

// Logout invalidates the linked-device session without deleting the archive.
// The service stops and awaits the owned writer before entering this method.
func (e *Engine) Logout(ctx context.Context) error {
	e.mu.Lock()
	running := e.cancel != nil
	e.mu.Unlock()
	if running {
		return errors.New("stop WhatsApp sync before disconnecting")
	}
	// Upstream logout requires an authenticated session. A read-only status check
	// makes retry safe after a successful logout whose response was interrupted.
	authenticated, err := e.sessionAuthenticated(ctx)
	if err != nil {
		return errors.New("could not check the WhatsApp session; retry Disconnect")
	}
	if authenticated {
		raw, err := e.Call(ctx, true, "auth", "logout")
		var result struct {
			LoggedOut bool `json:"logged_out"`
		}
		if err != nil || json.Unmarshal(raw, &result) != nil || !result.LoggedOut {
			return errors.New("WhatsApp could not sign out this linked device. Check your connection and retry Disconnect")
		}
	}
	e.mu.Lock()
	e.status = Status{Status: "disconnected", Detail: "WhatsApp is disconnected. Your archive stays on this Mac."}
	e.mu.Unlock()
	return nil
}
func (e *Engine) run(ctx context.Context, done chan struct{}, p Policy, link bool, ack chan error) {
	defer close(done)
	defer func() {
		e.mu.Lock()
		e.status.Status = "disconnected"
		e.status.Detail = "WhatsApp sync is stopped."
		e.status.QR = ""
		e.mu.Unlock()
	}()
	first := true
	backoff := time.Second
	for ctx.Err() == nil {
		args := []string{"--owner-stdin", "--events", "sync", "--follow", "--refresh-contacts", "--refresh-groups"}
		if link {
			args = []string{"--owner-stdin", "--events", "auth", "--follow", "--qr-format", "text"}
		}
		cmd := exec.Command(e.binary, e.args(args...)...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Stdout = io.Discard
		lifetime, lifetimeErr := cmd.StdinPipe()
		if lifetimeErr != nil {
			if first {
				ack <- errors.New("cannot create WhatsApp lifetime pipe")
			}
			return
		}
		stderr, err := cmd.StderrPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err != nil {
			lifetime.Close()
			if first {
				ack <- errors.New("cannot start bundled WhatsApp engine; reinstall the app or check its executable permissions")
				return
			}
			e.setAttention("WhatsApp engine could not restart. Retry connection in the app.")
		} else {
			stopped := make(chan struct{})
			go func() {
				select {
				case <-ctx.Done():
					lifetime.Close()
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
					select {
					case <-stopped:
					case <-time.After(15 * time.Second):
						_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					}
				case <-stopped:
				}
			}()
			scanner := bufio.NewScanner(stderr)
			scanner.Buffer(make([]byte, 4096), 1<<20)
			policyLoaded := false
			for scanner.Scan() {
				var event struct {
					Event string `json:"event"`
					Data  struct {
						Revision int64  `json:"revision"`
						Code     string `json:"code"`
					} `json:"data"`
					TS int64 `json:"ts"`
				}
				if json.Unmarshal(scanner.Bytes(), &event) != nil {
					continue
				}
				if event.Event == "policy_loaded" {
					if event.Data.Revision != p.Revision {
						_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
						if first {
							ack <- errors.New("WhatsApp engine acknowledged the wrong policy revision")
							first = false
						}
						continue
					}
					policyLoaded = true
					if first {
						ack <- nil
						first = false
					}
				}
				e.mu.Lock()
				e.status.LastEventAt = time.Now().UTC().Format(time.RFC3339)
				switch event.Event {
				case "qr_code":
					e.status.Status = "linking"
					e.status.QR = event.Data.Code
					e.status.Linked = false
					e.status.Detail = "Scan this code using WhatsApp Linked Devices."
				case "connected":
					e.status.Status = "ready"
					e.status.QR = ""
					e.status.Detail = "Connected. Available history may still arrive."
					e.status.Linked = true
				case "history_sync":
					e.status.Status = "syncing"
					e.status.Detail = "Receiving available history from WhatsApp. History coverage may be partial."
				case "progress":
					e.status.Status = "syncing"
					e.status.Detail = "Receiving WhatsApp updates. Available history may still arrive."
				case "disconnected":
					e.status.Status = "connecting"
					e.status.QR = ""
					e.status.Detail = "Reconnecting to WhatsApp."
				case "logged_out":
					e.status.Status = "attention"
					e.status.QR = ""
					e.status.Linked = false
					e.status.Detail = "WhatsApp disconnected this linked device. Connect WhatsApp again."
				}
				e.mu.Unlock()
			}
			_ = cmd.Wait()
			lifetime.Close()
			close(stopped)
			if first && !policyLoaded {
				ack <- errors.New("WhatsApp engine exited before loading the saved chat policy")
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		e.setAttention("WhatsApp sync stopped; retrying connection. History coverage remains partial.")
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}
func (e *Engine) setAttention(detail string) {
	e.mu.Lock()
	e.status.Status = "attention"
	e.status.Detail = detail
	e.status.QR = ""
	e.mu.Unlock()
}

type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, errors.New("structured engine response exceeded its limit")
	}
	return b.Buffer.Write(p)
}
func (e *Engine) Call(ctx context.Context, write bool, args ...string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	global := []string{"--json", "--timeout", "80s"}
	if !write {
		global = append(global, "--read-only")
	}
	cmd := exec.CommandContext(ctx, e.binary, e.args(append(global, args...)...)...)
	out := &limitedBuffer{limit: 24 << 20}
	stderr := &limitedBuffer{limit: 1 << 20}
	cmd.Stdout = out
	cmd.Stderr = stderr
	runErr := cmd.Run()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   string          `json:"error"`
	}
	parseErr := json.Unmarshal(out.Bytes(), &env)
	if runErr != nil || parseErr != nil || !env.Success {
		detail := strings.ToLower(env.Error + stderr.String())
		switch {
		case strings.Contains(detail, "media_not_cached"):
			return nil, errors.New("attachment_not_cached: WhatsApp is disconnected and this attachment is not cached; connect WhatsApp to download it")
		case strings.Contains(detail, "checkpoint") && (strings.Contains(detail, "expir") || strings.Contains(detail, "retention")):
			return nil, errors.New("checkpoint_expired: capture a new checkpoint, rebuild from the retained archive, then replay changes; the expired gap cannot be recovered")
		case strings.Contains(detail, "cursor"):
			return nil, errors.New("cursor_expired: the archive changed; restart this paginated read")
		case strings.Contains(detail, "phone") && (strings.Contains(detail, "timeout") || strings.Contains(detail, "no_response") || strings.Contains(detail, "did not")):
			return nil, errors.New("phone_not_responding: keep the phone online and retry attachment recovery")
		case strings.Contains(detail, "unavailable") || strings.Contains(detail, "expired") || strings.Contains(detail, "404") || strings.Contains(detail, "410"):
			return nil, errors.New("media_unavailable: the recorded attachment is no longer available from WhatsApp or the phone")
		case strings.Contains(detail, "not authenticated") || strings.Contains(detail, "logged out"):
			return nil, errors.New("WhatsApp needs to be connected again in the app")
		default:
			return nil, errors.New("WhatsApp operation failed; check connection status and retry reads. For sends, reuse the same request identifier and check for an uncertain result.")
		}
	}
	return env.Data, nil
}
func (e *Engine) Resolve(ctx context.Context, id string) ([]string, error) {
	return e.ResolveMany(ctx, []string{id})
}
func (e *Engine) ResolveMany(ctx context.Context, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return []string{}, nil
	}
	args := []string{"policy", "resolve"}
	for _, id := range ids {
		args = append(args, "--chat", id)
	}
	b, err := e.Call(ctx, false, args...)
	if err != nil {
		return nil, err
	}
	var result struct {
		ChatIDs []string `json:"chat_ids"`
	}
	if json.Unmarshal(b, &result) != nil || len(result.ChatIDs) == 0 {
		return nil, errors.New("verified conversation identities could not be resolved")
	}
	return result.ChatIDs, nil
}

func (e *Engine) Catalog(ctx context.Context) ([]Chat, error) {
	path := filepath.Join(e.dir, "store", "wacli.db")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return []Chat{}, nil
	}
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite3", u.String()+"?mode=ro&_query_only=true&_busy_timeout=5000")
	if err != nil {
		return nil, errors.New("local chat catalog is unavailable")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	rows, err := db.QueryContext(ctx, `SELECT c.jid, COALESCE(NULLIF(c.name,''),NULLIF(a.alias,''),NULLIF(p.system_name,''),NULLIF(p.full_name,''),NULLIF(p.push_name,''),NULLIF(p.phone,''),c.jid), COALESCE(p.phone,''),c.kind,COALESCE(c.last_message_ts,0),COALESCE(NULLIF(a.alias,''),NULLIF(p.system_name,''),NULLIF(p.full_name,''),p.push_name,'') FROM chats c LEFT JOIN contacts p ON p.jid=c.jid LEFT JOIN contact_aliases a ON a.jid=c.jid WHERE c.kind IN ('dm','group','unknown') AND (c.jid LIKE '%@s.whatsapp.net' OR c.jid LIKE '%@lid' OR c.jid LIKE '%@g.us') ORDER BY COALESCE(c.last_message_ts,0) DESC,c.jid`)
	if err != nil {
		return nil, errors.New("local chat catalog is not ready; wait for WhatsApp to initialize")
	}
	defer rows.Close()
	out := []Chat{}
	for rows.Next() {
		var c Chat
		var ts int64
		if err = rows.Scan(&c.ID, &c.Name, &c.Phone, &c.Kind, &ts, &c.ContactName); err != nil {
			return nil, errors.New("local chat catalog has an unexpected format")
		}
		if c.Kind != "group" {
			c.Kind = "direct"
		}
		if c.Phone == "" && strings.HasSuffix(c.ID, "@s.whatsapp.net") {
			c.Phone = strings.TrimSuffix(c.ID, "@s.whatsapp.net")
		}
		if ts > 0 {
			c.LastActivity = time.Unix(ts, 0).UTC().Format(time.RFC3339)
		}
		out = append(out, c)
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("local chat catalog could not finish reading")
	}
	rows.Close()
	// Resolve in bounded batches through wacli; names and phone formatting never
	// establish equivalence, and the companion does not read session keys.
	for start := 0; start < len(out); start += 256 {
		end := min(start+256, len(out))
		args := []string{"policy", "resolve"}
		for _, chat := range out[start:end] {
			args = append(args, "--chat", chat.ID)
		}
		raw, err := e.Call(ctx, false, args...)
		if err != nil {
			return nil, err
		}
		var resolved struct {
			Groups map[string][]string `json:"groups"`
		}
		if json.Unmarshal(raw, &resolved) != nil || resolved.Groups == nil {
			return nil, errors.New("verified chat identities are unavailable; reinstall the complete app")
		}
		for i := start; i < end; i++ {
			aliases, ok := resolved.Groups[out[i].ID]
			if !ok || len(aliases) == 0 {
				return nil, errors.New("verified chat identities are incomplete; retry after sync")
			}
			out[i].Aliases = aliases
		}
	}
	return out, nil
}

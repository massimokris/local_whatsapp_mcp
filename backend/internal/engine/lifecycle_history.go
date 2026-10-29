package engine

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/massi/localwhatsappmcp/internal/persistence"
)

const lifecycleHistoryLimit = 128
const lifecycleHistoryByteLimit = 64 << 10

// lifecycleEntry is deliberately separate from engineEvent. Only this closed
// vocabulary can reach disk; incoming strings and event bodies are never copied.
type lifecycleEntry struct {
	Sequence   uint64 `json:"sequence"`
	At         string `json:"at"`
	Event      string `json:"event"`
	Mode       string `json:"mode,omitempty"`
	LogoutCode int    `json:"logout_code,omitempty"`
	OnConnect  *bool  `json:"on_connect,omitempty"`
	Exit       string `json:"exit,omitempty"`
}

// lifecycleHistory owns projection, ordering, retention and private persistence.
// Enqueue never performs I/O. One lazy writer drains a bounded queue and exits;
// disk failures cannot change engine state or delay a connection transition.
type lifecycleHistory struct {
	path    string
	write   func(string, []byte, os.FileMode) error
	mu      sync.Mutex
	pending []lifecycleEntry
	writing bool
	idle    chan struct{}
	// Only the single writer accesses the retained state below.
	loaded   bool
	entries  []lifecycleEntry
	sequence uint64
}

func newLifecycleHistory(dir string) *lifecycleHistory {
	return &lifecycleHistory{path: filepath.Join(dir, "lifecycle-history.json"), write: persistence.Write}
}

func safeLifecycleEntry(entry lifecycleEntry) (lifecycleEntry, bool) {
	safe := lifecycleEntry{Sequence: entry.Sequence, At: entry.At, Event: entry.Event}
	switch entry.Event {
	case "process_start":
		if entry.Mode == "auth" || entry.Mode == "sync" {
			safe.Mode = entry.Mode
		}
	case "terminal_logout":
		switch entry.LogoutCode {
		case 401, 403, 406:
			safe.LogoutCode = entry.LogoutCode
		}
		if entry.OnConnect != nil {
			value := *entry.OnConnect
			safe.OnConnect = &value
		}
	case "process_exit":
		switch entry.Exit {
		case "clean", "error", "canceled", "start_failed":
			safe.Exit = entry.Exit
		}
	case "qr_available", "paired", "login_confirmed", "disconnected", "stop_requested":
	default:
		return lifecycleEntry{}, false
	}
	return safe, true
}

func (h *lifecycleHistory) record(entry lifecycleEntry) {
	if h == nil {
		return
	}
	safe, ok := safeLifecycleEntry(entry)
	if !ok {
		return
	}
	safe.At = time.Now().UTC().Format(time.RFC3339Nano)
	safe.Sequence = 0
	h.mu.Lock()
	if len(h.pending) == lifecycleHistoryLimit {
		copy(h.pending, h.pending[1:])
		h.pending = h.pending[:lifecycleHistoryLimit-1]
	}
	h.pending = append(h.pending, safe)
	if !h.writing {
		h.writing = true
		h.idle = make(chan struct{})
		go h.drain()
	}
	h.mu.Unlock()
}

func (h *lifecycleHistory) recordEvent(event engineEvent) {
	entry := lifecycleEntry{}
	switch event.Event {
	case "qr_code":
		entry.Event = "qr_available"
	case "paired":
		entry.Event = "paired"
	case "connected":
		entry.Event = "login_confirmed"
	case "disconnected":
		entry.Event = "disconnected"
	case "logged_out":
		entry = lifecycleEntry{Event: "terminal_logout", LogoutCode: event.Data.ReasonCode, OnConnect: event.Data.OnConnect}
	case "stopping":
		if event.Data.Reason == "logged_out" {
			entry.Event = "terminal_logout"
		}
	}
	h.record(entry)
}

func (h *lifecycleHistory) drain() {
	if !h.loaded {
		h.load()
		h.loaded = true
	}
	for {
		h.mu.Lock()
		batch := h.pending
		h.pending = nil
		if len(batch) == 0 {
			h.writing = false
			close(h.idle)
			h.mu.Unlock()
			return
		}
		h.mu.Unlock()
		for _, entry := range batch {
			h.sequence++
			entry.Sequence = h.sequence
			h.entries = append(h.entries, entry)
		}
		if len(h.entries) > lifecycleHistoryLimit {
			h.entries = append([]lifecycleEntry(nil), h.entries[len(h.entries)-lifecycleHistoryLimit:]...)
		}
		body, err := json.Marshal(h.entries)
		if err == nil {
			_ = h.write(h.path, body, 0600)
		}
	}
}

func (h *lifecycleHistory) load() {
	// Refuse symlinks, FIFOs and public files. Reading prior diagnostics is
	// optional and bounded; a corrupt artifact cannot delay engine startup.
	fd, err := syscall.Open(h.path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return
	}
	file := os.NewFile(uintptr(fd), h.path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > lifecycleHistoryByteLimit {
		return
	}
	body, err := io.ReadAll(io.LimitReader(file, lifecycleHistoryByteLimit+1))
	if err != nil || len(body) > lifecycleHistoryByteLimit {
		return
	}
	var entries []lifecycleEntry
	if json.Unmarshal(body, &entries) != nil {
		return
	}
	for _, entry := range entries {
		safe, ok := safeLifecycleEntry(entry)
		at, err := time.Parse(time.RFC3339Nano, entry.At)
		if !ok || err != nil || entry.Sequence <= h.sequence || entry.Sequence > 1<<60 {
			continue
		}
		safe.At = at.UTC().Format(time.RFC3339Nano)
		h.sequence = safe.Sequence
		h.entries = append(h.entries, safe)
	}
	if len(h.entries) > lifecycleHistoryLimit {
		h.entries = append([]lifecycleEntry(nil), h.entries[len(h.entries)-lifecycleHistoryLimit:]...)
	}
}

// Stop may briefly await the final snapshot after all process callbacks end.
// Diagnostic I/O failure or stalls never become connection failures.
func (h *lifecycleHistory) drainBounded() {
	if h == nil {
		return
	}
	h.mu.Lock()
	idle, writing := h.idle, h.writing
	h.mu.Unlock()
	if !writing {
		return
	}
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-idle:
	case <-timer.C:
	}
}

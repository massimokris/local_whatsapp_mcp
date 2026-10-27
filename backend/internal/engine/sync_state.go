package engine

import "time"

// The engine sends a snapshot every five seconds, including on quiet accounts.
// Four missed snapshots invalidate success and force a fresh delivery boundary.
const syncWatchdogTimeout = 20 * time.Second

type engineEvent struct {
	Event string `json:"event"`
	Data  struct {
		Revision     int64  `json:"revision"`
		Code         string `json:"code"`
		RunID        string `json:"run_id"`
		ConnectionID uint64 `json:"connection_id"`
		State        string `json:"state"`
	} `json:"data"`
}

type syncEvidence struct {
	runID              string
	revision           int64
	connectionID       uint64
	closedConnectionID uint64
	lastSnapshot       time.Time
	connected          bool
	expired            bool
	stopping           bool
}

// The upstream tracker owns recovery evidence. Local lifecycle changes may
// remove success, but must not erase a known unresolved persistence failure.
func (e *Engine) invalidateSyncLocked() {
	if e.status.SyncState != SyncError {
		e.status.SyncState = SyncUnknown
	}
	e.sync.connected = false
}

func (e *Engine) expireSyncLocked(now time.Time) bool {
	if e.sync.stopping || e.sync.runID == "" || e.sync.lastSnapshot.IsZero() {
		return false
	}
	if e.sync.expired || now.Sub(e.sync.lastSnapshot) >= syncWatchdogTimeout {
		e.sync.expired = true
		e.invalidateSyncLocked()
		e.status.Status = "attention"
		e.status.Detail = "WhatsApp sync is not responding; reconnecting."
		e.status.QR = ""
		return true
	}
	return false
}

func (e *Engine) observeEvent(event engineEvent, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.sync.expired || e.sync.stopping {
		return
	}
	if event.Event == "sync_state" || event.Event == "sync_snapshot" {
		d := event.Data
		if d.RunID != e.sync.runID || d.Revision != e.sync.revision || d.ConnectionID < e.sync.connectionID {
			return
		}
		if d.ConnectionID > e.sync.connectionID {
			e.sync.connectionID = d.ConnectionID
			e.invalidateSyncLocked()
		}
		if NormalizeSyncState(d.State) != d.State {
			if e.status.SyncState != SyncError {
				e.status.SyncState = SyncUnknown
			}
			return
		}
		// No connection has supplied a delivery boundary before its matching
		// Connected event. Startup snapshots still establish process liveness.
		if d.State == SyncCaughtUp && (d.ConnectionID == 0 || d.ConnectionID <= e.sync.closedConnectionID || !e.sync.connected) {
			return
		}
		e.sync.lastSnapshot = now
		e.status.LastEventAt = now.UTC().Format(time.RFC3339)
		if event.Event == "sync_snapshot" && d.State == SyncCaughtUp {
			return // Liveness is not completion evidence.
		}
		// Only completion of the affected work can clear a known failure.
		// Startup, reconnect and liveness evidence cannot establish recovery.
		if e.status.SyncState == SyncError && d.State != SyncCaughtUp {
			return
		}
		e.status.SyncState = d.State
		switch d.State {
		case SyncCaughtUp:
			e.status.Status = "ready"
			e.status.Detail = "Selected chats are caught up through the current WhatsApp delivery boundary. History coverage may be partial."
		case Syncing:
			if e.sync.connected {
				e.status.Status = "syncing"
				e.status.Detail = "Receiving and saving WhatsApp messages. Available history may still arrive."
			}
		case SyncError:
			e.status.Status = "attention"
			e.status.Detail = "WhatsApp messages could not be saved. Sync will recover when the affected updates are saved."
		}
		return
	}
	// New engines attach identity to lifecycle events as well as sync events.
	// Untagged events from an older engine never establish completion.
	if event.Data.RunID != "" {
		d := event.Data
		if d.RunID != e.sync.runID || d.Revision != e.sync.revision || d.ConnectionID < e.sync.connectionID {
			return
		}
		if d.ConnectionID > e.sync.connectionID {
			e.sync.connectionID = d.ConnectionID
			e.invalidateSyncLocked()
		}
	}
	e.status.LastEventAt = now.UTC().Format(time.RFC3339)
	switch event.Event {
	case "qr_code":
		e.invalidateSyncLocked()
		e.status.Status = "linking"
		e.status.QR = event.Data.Code
		e.status.Linked = false
		e.status.Detail = "Scan this code using WhatsApp Linked Devices."
	case "connected":
		if event.Data.RunID != "" && event.Data.ConnectionID <= e.sync.closedConnectionID {
			return
		}
		e.invalidateSyncLocked()
		e.sync.connected = true
		if e.status.SyncState != SyncError {
			e.status.Status = "ready"
			e.status.Detail = "Connected. Available history may still arrive."
		} else {
			e.status.Status = "attention"
			e.status.Detail = "WhatsApp messages could not be saved. Sync will recover when the affected updates are saved."
		}
		e.status.QR = ""
		e.status.Linked = true
	case "history_sync", "progress":
		if e.status.SyncState == SyncError {
			return
		}
		e.status.SyncState = Syncing
		e.status.Status = "syncing"
		e.status.Detail = "Receiving WhatsApp updates. Available history may still arrive."
	case "disconnected", "connection_check_failed":
		e.sync.closedConnectionID = e.sync.connectionID
		e.invalidateSyncLocked()
		e.status.Status = "connecting"
		e.status.QR = ""
		e.status.Detail = "Reconnecting to WhatsApp."
	case "logged_out":
		e.sync.closedConnectionID = e.sync.connectionID
		e.invalidateSyncLocked()
		e.status.Status = "attention"
		e.status.QR = ""
		e.status.Linked = false
		e.status.Detail = "WhatsApp disconnected this linked device. Connect WhatsApp again."
	}
}

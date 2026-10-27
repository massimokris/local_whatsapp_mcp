package engine

import (
	"testing"
	"time"
)

func syncFixture() (*Engine, time.Time) {
	now := time.Unix(1700000000, 0)
	return &Engine{status: Status{Status: "connecting", SyncState: SyncUnknown}, sync: syncEvidence{runID: "run-1", revision: 7, lastSnapshot: now}}, now
}

func syncEvent(name, state string, connection uint64) engineEvent {
	e := engineEvent{Event: name}
	e.Data.RunID = "run-1"
	e.Data.Revision = 7
	e.Data.ConnectionID = connection
	e.Data.State = state
	return e
}

func TestSyncRequiresExplicitCurrentBoundaryAndRemainsCaughtUpWhileQuiet(t *testing.T) {
	e, now := syncFixture()
	e.observeEvent(syncEvent("connected", "", 1), now)
	e.observeEvent(syncEvent("sync_snapshot", SyncCaughtUp, 1), now)
	if e.status.SyncState == SyncCaughtUp {
		t.Fatal("snapshot independently established completion")
	}
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 1), now)
	if e.status.SyncState != SyncCaughtUp {
		t.Fatal("explicit boundary did not establish completion")
	}
	// A quiet account stays caught up indefinitely, with liveness snapshots.
	for i := 1; i <= 100; i++ {
		current := now.Add(time.Duration(i) * 5 * time.Second)
		e.observeEvent(syncEvent("sync_snapshot", SyncCaughtUp, 1), current)
		if e.expireSyncLocked(current) || e.status.SyncState != SyncCaughtUp {
			t.Fatal("quiet healthy account expired")
		}
	}
	e.observeEvent(syncEvent("sync_state", Syncing, 1), now.Add(501*time.Second))
	if e.status.SyncState != Syncing {
		t.Fatal("new live/history work retained success")
	}
	e.observeEvent(syncEvent("sync_snapshot", SyncCaughtUp, 1), now.Add(502*time.Second))
	if e.status.SyncState != Syncing {
		t.Fatal("snapshot hid pending work")
	}
}

func TestSyncRejectsDelayedConnectionProcessAndScopeEvidence(t *testing.T) {
	e, now := syncFixture()
	e.observeEvent(syncEvent("connected", "", 1), now)
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 1), now)
	e.observeEvent(syncEvent("disconnected", "", 1), now)
	e.observeEvent(syncEvent("connected", "", 3), now) // generations may skip
	for _, name := range []string{"sync_state", "sync_snapshot", "connected", "disconnected"} {
		e.observeEvent(syncEvent(name, SyncCaughtUp, 1), now)
	}
	if e.status.SyncState == SyncCaughtUp || !e.sync.connected {
		t.Fatal("old connection changed current connection")
	}
	for _, change := range []func(*engineEvent){
		func(e *engineEvent) { e.Data.RunID = "old-run" },
		func(e *engineEvent) { e.Data.Revision = 6 },
	} {
		event := syncEvent("sync_state", SyncCaughtUp, 3)
		change(&event)
		e.observeEvent(event, now)
		if e.status.SyncState == SyncCaughtUp {
			t.Fatal("stale process/scope restored success")
		}
	}
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 3), now)
	if e.status.SyncState != SyncCaughtUp {
		t.Fatal("current connection could not complete")
	}
}

func TestSyncPersistenceFailureRequiresRecoveryBeyondReconnect(t *testing.T) {
	e, now := syncFixture()
	e.observeEvent(syncEvent("connected", "", 1), now)
	e.observeEvent(syncEvent("sync_state", SyncError, 1), now)
	e.observeEvent(syncEvent("disconnected", "", 1), now)
	e.observeEvent(syncEvent("connected", "", 2), now)
	for _, name := range []string{"sync_state", "sync_snapshot"} {
		for _, state := range []string{SyncUnknown, Syncing, SyncError} {
			e.observeEvent(syncEvent(name, state, 2), now)
			if e.status.SyncState != SyncError {
				t.Fatal("reconnection/liveness erased failed work")
			}
		}
	}
	e.observeEvent(syncEvent("sync_snapshot", SyncCaughtUp, 2), now)
	if e.status.SyncState != SyncError {
		t.Fatal("snapshot erased failed work")
	}
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 2), now)
	if e.status.SyncState != SyncCaughtUp {
		t.Fatal("successful affected work could not recover")
	}
}

func TestSyncWatchdogAndFailedConnectionChecksInvalidateSuccess(t *testing.T) {
	for _, failure := range []string{"watchdog", "connection_check_failed", "disconnected", "logged_out"} {
		t.Run(failure, func(t *testing.T) {
			e, now := syncFixture()
			e.observeEvent(syncEvent("connected", "", 1), now)
			e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 1), now)
			if failure == "watchdog" {
				// Other log activity is not a valid state/liveness snapshot.
				e.observeEvent(engineEvent{Event: "unrelated_log"}, now.Add(19*time.Second))
				if !e.expireSyncLocked(now.Add(syncWatchdogTimeout)) {
					t.Fatal("unresponsive engine was not detected")
				}
				e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 1), now.Add(21*time.Second))
			} else {
				e.observeEvent(syncEvent(failure, "", 1), now)
			}
			if e.status.SyncState == SyncCaughtUp {
				t.Fatal("unavailable connection retained success")
			}
		})
	}
}

func TestSyncUnknownAndZeroConnectionCannotEstablishCompletion(t *testing.T) {
	e, now := syncFixture()
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 0), now)
	if e.status.SyncState == SyncCaughtUp {
		t.Fatal("unconnected process established completion")
	}
	e.observeEvent(syncEvent("connected", "", 1), now)
	e.observeEvent(syncEvent("sync_state", "future_state", 1), now)
	if e.status.SyncState != SyncUnknown {
		t.Fatal("unknown state was trusted")
	}
}

func TestNewConnectionNeedsItsOwnConnectedEvidence(t *testing.T) {
	e, now := syncFixture()
	e.observeEvent(syncEvent("connected", "", 1), now)
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 1), now)
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 2), now)
	if e.status.SyncState == SyncCaughtUp {
		t.Fatal("new boundary reused the old connection's transport evidence")
	}
	e.observeEvent(syncEvent("connected", "", 2), now)
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 2), now)
	if e.status.SyncState != SyncCaughtUp {
		t.Fatal("matching new connection could not complete")
	}
}

func TestUnfamiliarCurrentSyncStateRemovesExistingSuccess(t *testing.T) {
	for _, value := range []string{"future_state", ""} {
		e, now := syncFixture()
		e.observeEvent(syncEvent("connected", "", 1), now)
		e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 1), now)
		e.observeEvent(syncEvent("sync_state", value, 1), now)
		if e.status.SyncState != SyncUnknown {
			t.Fatal("unrecognized current evidence retained success")
		}
	}
}

func TestDelayedConnectedCannotReviveEndedConnection(t *testing.T) {
	e, now := syncFixture()
	e.observeEvent(syncEvent("connected", "", 1), now)
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 1), now)
	e.observeEvent(syncEvent("disconnected", "", 1), now)
	e.observeEvent(syncEvent("connected", "", 1), now)
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 1), now)
	if e.status.SyncState == SyncCaughtUp || e.sync.connected {
		t.Fatal("delayed Connected revived an ended connection")
	}
}

func TestRecoveredConnectionShowsUnrecoveredMessageFailure(t *testing.T) {
	e, now := syncFixture()
	e.observeEvent(syncEvent("connected", "", 1), now)
	e.observeEvent(syncEvent("sync_state", SyncError, 1), now)
	e.observeEvent(syncEvent("disconnected", "", 1), now)
	e.observeEvent(syncEvent("connected", "", 2), now)
	e.observeEvent(syncEvent("sync_state", SyncError, 2), now)
	if e.status.SyncState != SyncError || e.status.Status != "attention" {
		t.Fatalf("reconnection hid failure: %+v", e.status)
	}
}

func TestStoppingProcessCannotRestoreSuccessWhileAwaitingExit(t *testing.T) {
	e, now := syncFixture()
	e.observeEvent(syncEvent("connected", "", 1), now)
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 1), now)
	e.sync.stopping = true
	e.invalidateSyncLocked()
	e.observeEvent(syncEvent("connected", "", 2), now)
	e.observeEvent(syncEvent("sync_state", SyncCaughtUp, 2), now)
	if e.status.SyncState == SyncCaughtUp {
		t.Fatal("events restored success while stopping")
	}
}

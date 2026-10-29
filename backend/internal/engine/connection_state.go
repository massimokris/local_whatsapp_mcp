package engine

import "time"

type engineEvent struct {
	Event string `json:"event"`
	Data  struct {
		Revision   int64  `json:"revision"`
		Code       string `json:"code"`
		Reason     string `json:"reason"`
		ReasonCode int    `json:"reason_code"`
		OnConnect  *bool  `json:"on_connect"`
	} `json:"data"`
}

// The owned process pipe supplies upstream connection events. Message progress
// and old completion events do not alter connection status.
func (e *Engine) observeEvent(event engineEvent, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopping || e.loggedOut {
		return
	}
	switch event.Event {
	case "qr_code":
		e.status.Status = "linking"
		e.status.QR = event.Data.Code
		e.status.Linked = false
		e.status.Detail = "Scan this code using WhatsApp Linked Devices."
	case "connected":
		e.status.Status = "ready"
		e.status.Detail = "Connected to WhatsApp."
		e.status.QR = ""
		e.status.Linked = true
	case "disconnected":
		e.status.Status = "connecting"
		e.status.QR = ""
		e.status.Detail = "Reconnecting to WhatsApp."
	case "logged_out", "stopping":
		if event.Event == "stopping" && event.Data.Reason != "logged_out" {
			return
		}
		e.loggedOut = true
		e.status.Status = "attention"
		e.status.QR = ""
		e.status.Linked = false
		e.status.Detail = "WhatsApp disconnected this linked device. Connect WhatsApp again."
	default:
		return
	}
	e.status.LastEventAt = now.UTC().Format(time.RFC3339)
}

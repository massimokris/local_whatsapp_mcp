// Package engine is the fixed-store, structured boundary to the bundled wacli.
package engine

import (
	"context"
	"encoding/json"
)

type Policy struct {
	Version  int      `json:"version"`
	Revision int64    `json:"revision"`
	Mode     string   `json:"mode"`
	ChatIDs  []string `json:"chat_ids"`
}
type Chat struct {
	Aliases      []string `json:"aliases,omitempty"`
	ContactName  string   `json:"contact_name,omitempty"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Phone        string   `json:"phone"`
	Kind         string   `json:"kind"`
	Archived     bool     `json:"archived"`
	LastActivity string   `json:"last_activity"`
}
type Status struct {
	DisconnectPending bool   `json:"disconnect_pending,omitempty"`
	Linked            bool   `json:"linked"`
	Status            string `json:"status"`
	Detail            string `json:"detail"`
	QR                string `json:"qr,omitempty"`
	LastEventAt       string `json:"last_event_at,omitempty"`
	LastSyncedAt      string `json:"last_synced_at,omitempty"`
}
type Boundary interface {
	RestoreSession(context.Context) error
	Start(context.Context, Policy, bool) error
	Stop(context.Context) error
	Logout(context.Context) error
	Status() Status
	Catalog(context.Context) ([]Chat, error)
	ResolveMany(context.Context, []string) ([]string, error)
	Call(context.Context, bool, ...string) (json.RawMessage, error)
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

// The service fixture never invokes wacli or accesses the user's app directory.
type sendOutcomeEngine struct {
	manualFixtureEngine
	response json.RawMessage
	err      error
	calls    [][]string
	check    func([]string) error
}

func (e *sendOutcomeEngine) Call(_ context.Context, write bool, args ...string) (json.RawMessage, error) {
	if !write {
		return nil, errors.New("unexpected archive command in send fixture")
	}
	e.calls = append(e.calls, append([]string(nil), args...))
	if e.check != nil {
		if err := e.check(args); err != nil {
			return nil, err
		}
	}
	return e.response, e.err
}

func sendOutcomeFixture(t *testing.T, e *sendOutcomeEngine) (*Service, Options, context.Context, string) {
	t.Helper()
	s, options := httpPairingFixture(t)
	s.opt.Engine = e
	options.Engine = e
	connection, err := s.createManualConnection("Synthetic send test")
	if err != nil {
		s.Close()
		t.Fatal(err)
	}
	grant := s.cfg.Grants[connection.ID]
	grant.SendEnabled = true
	s.cfg.Grants[connection.ID] = grant
	if err := s.saveLocked(); err != nil {
		s.Close()
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), identityKey{}, connection.ID)
	return s, options, ctx, connection.ID
}

func sendOutcomeResult(t *testing.T, s *Service, ctx context.Context, request Send, file bool) map[string]any {
	t.Helper()
	result, err := s.send(ctx, request, file)
	if err != nil {
		t.Fatal(err)
	}
	out, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("unexpected send result: %T", result)
	}
	return out
}

func sendFlagValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func hasSendFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func TestSelfChatTextUsesSupportedEngineOptIn(t *testing.T) {
	const self = "123@s.whatsapp.net"
	const message = "Exact self-chat text: hi\nsecond line — test"
	e := &sendOutcomeEngine{response: json.RawMessage(`{"id":"self-text-accepted"}`)}
	// Model the upstream self-recipient guard: an intentional self send must
	// opt in through the supported CLI flag, rather than bypassing the engine.
	e.check = func(args []string) error {
		if !hasSendFlag(args, "--allow-self") {
			return &engine.SendError{Detail: "self text requires explicit opt-in", Uncertain: false}
		}
		return nil
	}
	s, _, ctx, _ := sendOutcomeFixture(t, e)
	defer s.Close()
	out := sendOutcomeResult(t, s, ctx, Send{ChatID: self, RequestID: "self-text", Text: message}, false)
	if out["status"] != "accepted" || out["message_id"] != "self-text-accepted" {
		t.Fatalf("intentional self text was rejected: %v", out)
	}
	if len(e.calls) != 1 {
		t.Fatalf("engine calls = %d, want 1", len(e.calls))
	}
	args := e.calls[0]
	if len(args) < 2 || args[0] != "send" || args[1] != "text" || sendFlagValue(args, "--to") != self || sendFlagValue(args, "--message") != message || !hasSendFlag(args, "--no-preview") {
		t.Fatalf("self-text command changed recipient or content: %q", args)
	}
}

func TestAttachmentSendKeepsFileCommandAndCleansStaging(t *testing.T) {
	e := &sendOutcomeEngine{response: json.RawMessage(`{"id":"file-accepted"}`)}
	s, _, ctx, _ := sendOutcomeFixture(t, e)
	defer s.Close()
	request := Send{ChatID: "123@s.whatsapp.net", RequestID: "file-test", Data: "Zml4dHVyZQ==", Filename: "note.txt", MIME: "text/plain", Caption: "Exact caption"}
	out := sendOutcomeResult(t, s, ctx, request, true)
	if out["status"] != "accepted" || len(e.calls) != 1 {
		t.Fatalf("attachment outcome: %v, calls: %d", out, len(e.calls))
	}
	args := e.calls[0]
	if len(args) < 2 || args[0] != "send" || args[1] != "file" || hasSendFlag(args, "--allow-self") || sendFlagValue(args, "--to") != request.ChatID || sendFlagValue(args, "--filename") != request.Filename || sendFlagValue(args, "--mime") != request.MIME || sendFlagValue(args, "--caption") != request.Caption {
		t.Fatalf("attachment command changed: %q", args)
	}
	staged := sendFlagValue(args, "--file")
	if staged == "" {
		t.Fatal("attachment was not staged for the engine")
	}
	if _, err := os.Stat(staged); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attachment staging was retained: %v", err)
	}
}

func TestSendOutcomesRemainIdempotentAcrossRestart(t *testing.T) {
	const rejection = "request deadline passed before dispatch; it was not sent"
	const ambiguous = "WhatsApp send outcome is uncertain; check the conversation before sending again"
	for _, test := range []struct {
		name       string
		response   json.RawMessage
		err        error
		wantStatus string
		wantID     string
		wantError  string
	}{
		{name: "accepted", response: json.RawMessage(`{"id":"accepted-id"}`), wantStatus: "accepted", wantID: "accepted-id"},
		{name: "definite rejection", err: &engine.SendError{Detail: rejection}, wantStatus: "failed", wantError: rejection},
		{name: "wrapped definite rejection", err: fmt.Errorf("engine call: %w", &engine.SendError{Detail: rejection}), wantStatus: "failed", wantError: rejection},
		{name: "ambiguous transmission", err: &engine.SendError{Detail: ambiguous, Uncertain: true}, wantStatus: "uncertain", wantError: ambiguous},
		{name: "untyped transport error", err: errors.New("private unclassified upstream error"), wantStatus: "uncertain"},
		{name: "cancelled call", err: context.Canceled, wantStatus: "uncertain"},
		{name: "malformed response", response: json.RawMessage(`{"id":`), wantStatus: "uncertain"},
		{name: "missing message id", response: json.RawMessage(`{"sent":true}`), wantStatus: "uncertain"},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := &sendOutcomeEngine{response: test.response, err: test.err}
			s, options, ctx, _ := sendOutcomeFixture(t, e)
			defer func() {
				if s != nil {
					s.Close()
				}
			}()
			request := Send{ChatID: "123@s.whatsapp.net", RequestID: "same-request", Text: "hello"}
			first := sendOutcomeResult(t, s, ctx, request, false)
			if first["status"] != test.wantStatus || first["message_id"] != test.wantID {
				t.Fatalf("wrong send outcome: %v", first)
			}
			if test.wantError != "" && first["error"] != test.wantError {
				t.Fatalf("send reason was lost: %v", first)
			}
			if strings.Contains(fmt.Sprint(first), "private unclassified upstream error") {
				t.Fatal("unclassified upstream detail leaked into the tool result")
			}
			assertReplay := func() {
				t.Helper()
				replayed := sendOutcomeResult(t, s, ctx, request, false)
				for _, key := range []string{"status", "message_id", "error", "request_id"} {
					if replayed[key] != first[key] {
						t.Fatalf("saved %s changed: first=%v replay=%v", key, first, replayed)
					}
				}
				if len(e.calls) != 1 {
					t.Fatalf("request was dispatched %d times", len(e.calls))
				}
			}
			assertReplay()
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			s, err = New(options)
			if err != nil {
				t.Fatal(err)
			}
			assertReplay()
			changed := request
			changed.Text = "different content"
			if _, err := s.send(ctx, changed, false); err == nil || !strings.Contains(err.Error(), "different content") {
				t.Fatalf("request id accepted changed content: %v", err)
			}
			if len(e.calls) != 1 {
				t.Fatal("changed-content request reached the engine")
			}
		})
	}
}

func TestLegacyUncertainSendIsNeverRedispatched(t *testing.T) {
	e := &sendOutcomeEngine{response: json.RawMessage(`{"id":"must-not-be-used"}`)}
	s, options, ctx, id := sendOutcomeFixture(t, e)
	defer func() {
		if s != nil {
			s.Close()
		}
	}()
	request := Send{ChatID: "123@s.whatsapp.net", RequestID: "legacy-text", Text: "hello"}
	// Existing on-disk records have no error field. This is the pre-upgrade
	// request hash fixture also covered by send_retry_test.go.
	s.cfg.Sends[id+":"+request.RequestID] = sendRecord{Hash: "452ed5f4bf7b2d601bc6027bba917aded4b84c07328d5c819f8ba321b750fa50", Status: "uncertain"}
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	s, err = New(options)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		out := sendOutcomeResult(t, s, ctx, request, false)
		if out["status"] != "uncertain" || out["message_id"] != "" {
			t.Fatalf("legacy uncertainty changed: %v", out)
		}
	}
	if len(e.calls) != 0 {
		t.Fatal("legacy uncertain send was retried")
	}
}

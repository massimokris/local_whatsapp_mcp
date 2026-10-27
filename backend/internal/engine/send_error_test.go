package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

const selfTextRejection = "send text to the linked account itself is not supported: WhatsApp can acknowledge self-messages without delivering them; use the official Message Yourself chat"

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

func sendErrorJSON(detail string) string {
	b, _ := json.Marshal(map[string]any{"success": false, "data": nil, "error": detail})
	return string(b)
}

func fakeSendEngine(t *testing.T, stdout, stderr, ending string) *engine.Engine {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n"
	if stdout != "" {
		script += "printf '%s\\n' " + shellQuote(stdout) + "\n"
	}
	if stderr != "" {
		script += "printf '%s\\n' " + shellQuote(stderr) + " >&2\n"
	}
	script += ending + "\n"
	binary := filepath.Join(dir, "fake-wacli")
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return engine.New(binary, dir)
}

func TestSendCallDistinguishesPreSendRejectionsFromAmbiguousFailures(t *testing.T) {
	const queueRejection = "request timed out in the send queue before dispatch; it was not sent"
	const scopedRejection = "content excluded by chat scope"
	for _, test := range []struct {
		name, stdout, stderr, ending, wantDetail string
		uncertain                                bool
	}{
		{name: "self rejection", stderr: sendErrorJSON(selfTextRejection), wantDetail: selfTextRejection},
		{name: "queue refusal", stderr: sendErrorJSON(queueRejection), wantDetail: queueRejection},
		{name: "policy rejection on stdout", stdout: sendErrorJSON(scopedRejection), wantDetail: scopedRejection},
		{name: "warning then root rejection", stderr: "synthetic warning\n" + sendErrorJSON(scopedRejection), wantDetail: scopedRejection},
		{name: "older engine rejects self flag", stderr: sendErrorJSON("unknown flag: --allow-self"), wantDetail: "unknown flag: --allow-self"},
		{name: "authentication rejection", stderr: sendErrorJSON("not authenticated; run `wacli auth`"), wantDetail: "not authenticated; run `wacli auth`"},
		{name: "root error with no success field", stderr: `{"error":"` + scopedRejection + `"}`, uncertain: true},
		{name: "root success with error field", stderr: `{"success":true,"error":"` + scopedRejection + `"}`, uncertain: true},
		{name: "failure envelope with result data", stderr: `{"success":false,"error":"` + scopedRejection + `","data":{"id":"possibly-sent"}}`, uncertain: true},
		{name: "unstructured rejection", stderr: selfTextRejection, uncertain: true},
		{name: "unrecognized detail is redacted", stderr: sendErrorJSON("private-unclassified-fixture-token"), uncertain: true},
		{name: "transport eof", stderr: sendErrorJSON("EOF"), uncertain: true},
		{name: "timeout wraps known rejection", stderr: sendErrorJSON("delegated text failed after dispatch and may still have gone through; check before retrying: " + selfTextRejection), uncertain: true},
		{name: "earlier rejection followed by unknown error", stderr: sendErrorJSON(selfTextRejection) + "\n" + sendErrorJSON("EOF"), uncertain: true},
		{name: "earlier rejection followed by malformed output", stderr: sendErrorJSON(selfTextRejection) + "\n{", uncertain: true},
		{name: "contradictory success and rejection", stdout: `{"success":true,"data":{"id":"possibly-sent"}}`, stderr: sendErrorJSON(selfTextRejection), uncertain: true},
		{name: "conflicting error envelopes", stdout: sendErrorJSON(scopedRejection), stderr: sendErrorJSON(selfTextRejection), uncertain: true},
		{name: "empty output", uncertain: true},
		{name: "signal despite rejection output", stderr: sendErrorJSON(selfTextRejection), ending: "kill -TERM $$", uncertain: true},
		{name: "unexpected exit code", stderr: sendErrorJSON(selfTextRejection), ending: "exit 2", uncertain: true},
		{name: "zero exit with rejection output", stderr: sendErrorJSON(selfTextRejection), ending: "exit 0", uncertain: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ending := test.ending
			if ending == "" {
				ending = "exit 1"
			}
			e := fakeSendEngine(t, test.stdout, test.stderr, ending)
			_, err := e.Call(context.Background(), true, "send", "text", "--to", "123@s.whatsapp.net", "--message", "synthetic fixture", "--allow-self")
			var sendErr *engine.SendError
			if !errors.As(err, &sendErr) || sendErr.Uncertain != test.uncertain {
				t.Fatalf("send failure classification = %#v (%v), want uncertain=%v", sendErr, err, test.uncertain)
			}
			if test.wantDetail != "" && sendErr.Detail != test.wantDetail {
				t.Fatalf("known rejection detail = %q, want %q", sendErr.Detail, test.wantDetail)
			}
			if test.uncertain && (sendErr.Detail == "" || strings.Contains(sendErr.Detail, "private-unclassified-fixture-token")) {
				t.Fatal("unknown error lost its safe diagnostic or exposed raw upstream content")
			}
		})
	}
}

func TestSendCallReportsFailureToStartAsDefinitelyNotSent(t *testing.T) {
	dir := t.TempDir()
	e := engine.New(filepath.Join(dir, "missing-engine"), dir)
	_, err := e.Call(context.Background(), true, "send", "text", "--to", "123@s.whatsapp.net", "--message", "synthetic fixture")
	var sendErr *engine.SendError
	if !errors.As(err, &sendErr) || sendErr.Uncertain || !strings.Contains(sendErr.Detail, "could not start") {
		t.Fatalf("start failure = %#v (%v), want definite failure", sendErr, err)
	}
}

func TestInterruptedSendStaysUncertainDespiteKnownErrorOutput(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started")
	e := fakeSendEngine(t, "", sendErrorJSON(selfTextRejection), "touch "+shellQuote(marker)+"\nexec sleep 30")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := e.Call(ctx, true, "send", "text", "--to", "123@s.whatsapp.net", "--message", "synthetic fixture")
		done <- err
	}()
	for deadline := time.Now().Add(3 * time.Second); ; {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("synthetic engine did not start")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		var sendErr *engine.SendError
		if !errors.As(err, &sendErr) || !sendErr.Uncertain {
			t.Fatalf("interrupted send = %#v (%v), want uncertain", sendErr, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("interrupted send did not stop")
	}
}

func TestTruncatedSendDiagnosticsStayUncertain(t *testing.T) {
	// Keep a valid rejection in the retained buffer, followed only by spaces.
	// The child still exits 1 after its diagnostic pipe fills: without explicit
	// truncation tracking, the retained prefix would look like a definite error.
	e := fakeSendEngine(t, "", sendErrorJSON(selfTextRejection), "head -c 2097152 /dev/zero | tr '\\000' ' ' >&2\nexit 1")
	_, err := e.Call(context.Background(), true, "send", "file", "--to", "123@s.whatsapp.net", "--file", "synthetic fixture")
	var sendErr *engine.SendError
	if !errors.As(err, &sendErr) || !sendErr.Uncertain {
		t.Fatalf("truncated send diagnostics = %#v (%v), want uncertain", sendErr, err)
	}
}

func TestReadErrorsDoNotBecomeSendFailures(t *testing.T) {
	e := fakeSendEngine(t, "", sendErrorJSON("not authenticated; run `wacli auth`"), "exit 1")
	_, err := e.Call(context.Background(), false, "messages", "list", "--chat", "123@s.whatsapp.net")
	var sendErr *engine.SendError
	if err == nil || errors.As(err, &sendErr) || !strings.Contains(err.Error(), "connected again") {
		t.Fatalf("read error changed: %v", err)
	}
}

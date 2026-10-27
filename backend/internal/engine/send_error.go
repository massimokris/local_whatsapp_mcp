package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"os/exec"
)

// SendError reports a safe diagnostic. Uncertain is false only when the engine
// is known to have stopped before submitting a WhatsApp message. Unknown errors
// must remain uncertain: an unsuccessful process can still have sent a message.
type SendError struct {
	Detail    string
	Uncertain bool
}

func (e *SendError) Error() string { return e.Detail }

func uncertainSendError() *SendError {
	return &SendError{
		Detail:    "WhatsApp send failed without a confirmed outcome; check the conversation before sending again",
		Uncertain: true,
	}
}

func sendCallError(stdout, stderr *limitedBuffer, runErr, contextErr error) *SendError {
	uncertain := uncertainSendError()
	if contextErr != nil || stdout.truncated || stderr.truncated {
		return uncertain
	}
	// wacli emits its terminal JSON rejection and exits 1. Signals, missing or
	// malformed output, and other exit statuses do not prove no submission.
	var exited *exec.ExitError
	if !errors.As(runErr, &exited) || exited.ExitCode() != 1 {
		return uncertain
	}
	var detail string
	if terminal := bytes.TrimSpace(stderr.Bytes()); len(terminal) > 0 {
		if i := bytes.LastIndexByte(terminal, '\n'); i >= 0 {
			terminal = terminal[i+1:]
		}
		var ok bool
		detail, ok = sendErrorEnvelope(terminal)
		if !ok {
			return uncertain
		}
	}
	if output := bytes.TrimSpace(stdout.Bytes()); len(output) > 0 {
		other, ok := sendErrorEnvelope(output)
		if !ok || (detail != "" && detail != other) {
			return uncertain
		}
		detail = other
	}
	// Match complete pinned-engine errors only. In particular, a timeout may
	// wrap a validation error with "may still have gone through"; that wrapped
	// error must not become a definite rejection via a substring match.
	if definiteSendRejection(detail) {
		return &SendError{Detail: detail}
	}
	return uncertain
}

func sendErrorEnvelope(raw []byte) (string, bool) {
	var envelope struct {
		Success *bool           `json:"success"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Success == nil || *envelope.Success || envelope.Error == "" {
		return "", false
	}
	if data := bytes.TrimSpace(envelope.Data); len(data) > 0 && !bytes.Equal(data, []byte("null")) {
		return "", false
	}
	return envelope.Error, true
}

func definiteSendRejection(detail string) bool {
	switch detail {
	case "send text to the linked account itself is not supported: WhatsApp can acknowledge self-messages without delivering them; use the official Message Yourself chat",
		"request deadline passed before dispatch; it was not sent",
		"request timed out in the send queue before dispatch; it was not sent",
		"send spacing exceeded request timeout before dispatch; it was not sent",
		"content excluded by chat scope",
		"not authenticated; run `wacli auth`",
		"recipient is required",
		"--to and --message are required",
		"read-only mode: command would intentionally modify WhatsApp or the local store",
		"unknown flag: --allow-self",
		"content policy cannot be opened",
		"content policy is invalid",
		"content policy has trailing data",
		"content policy version or revision is invalid",
		"content policy mode is invalid",
		"content policy contains an invalid chat identity":
		return true
	default:
		return false
	}
}

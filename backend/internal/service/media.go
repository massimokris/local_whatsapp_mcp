package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

func (s *Service) send(ctx context.Context, v Send, file bool) (any, error) {
	p, err := s.access(ctx, true)
	if err != nil {
		return nil, err
	}
	if !s.allowed(ctx, p, v.ChatID) {
		return nil, ErrScope
	}
	if err = s.requireWhatsAppConnection(); err != nil {
		return nil, err
	}
	if len(v.RequestID) < 1 || len(v.RequestID) > 128 {
		return nil, errors.New("request_id must contain 1 to 128 characters")
	}
	if s.opt.Engine == nil {
		return nil, errors.New("WhatsApp is not connected")
	}
	if !file && (len(v.Text) == 0 || len(v.Text) > 64<<10) {
		return nil, errors.New("text must contain 1 to 65536 bytes")
	}
	if len(v.Caption) > 64<<10 {
		return nil, errors.New("caption is too long")
	}
	key := AgentID(ctx) + ":" + v.RequestID
	b, _ := json.Marshal(v)
	// Keep these bytes stable: saved request hashes survive app upgrades.
	sendKind := byte(0)
	if file {
		sendKind = 1
	}
	hash := digest(string(append(b, sendKind)))
	var args []string
	var path string
	if file {
		if len(v.Data) > 28<<20 {
			return nil, errors.New("attachment exceeds 20 MiB")
		}
		data, e := base64.StdEncoding.DecodeString(v.Data)
		if e != nil || len(data) == 0 || len(data) > 20<<20 {
			return nil, errors.New("attachment must contain valid base64 bytes, at most 20 MiB")
		}
		if len(v.Filename) > 255 || strings.ContainsAny(v.Filename, "/\\\r\n\x00") {
			return nil, errors.New("invalid attachment filename")
		}
		dir := filepath.Join(s.opt.DataDir, "staging")
		if e = os.MkdirAll(dir, 0700); e != nil {
			return nil, errors.New("cannot create private attachment staging")
		}
		f, e := os.CreateTemp(dir, "send-*")
		if e != nil {
			return nil, e
		}
		path = f.Name()
		defer os.Remove(path)
		if e = f.Chmod(0600); e == nil {
			_, e = f.Write(data)
		}
		f.Close()
		if e != nil {
			return nil, errors.New("cannot stage attachment")
		}
		args = []string{"send", "file", "--to", v.ChatID, "--file", path}
		if v.Filename != "" {
			args = append(args, "--filename", v.Filename)
		}
		if v.MIME != "" {
			if _, _, e = mime.ParseMediaType(v.MIME); e != nil {
				return nil, errors.New("invalid MIME type")
			}
			args = append(args, "--mime", v.MIME)
		}
		if v.Caption != "" {
			args = append(args, "--caption", v.Caption)
		}
	} else {
		args = []string{"send", "text", "--to", v.ChatID, "--message", v.Text, "--no-preview", "--allow-self"}
	}
	s.mu.Lock()
	if old, ok := s.cfg.Sends[key]; ok {
		s.mu.Unlock()
		if old.Hash != hash {
			return nil, errors.New("request_id was already used with different content")
		}
		return sendResult(old, v.RequestID), nil
	}
	s.cfg.Sends[key] = sendRecord{Hash: hash, Status: "uncertain"}
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return nil, errors.New("could not persist request identifier; nothing was dispatched")
	}
	// Revalidate at the dispatch boundary after staging and durable idempotency recording.
	if _, err = s.access(ctx, true); err != nil {
		return nil, err
	}
	if !s.allowed(ctx, p, v.ChatID) {
		return nil, ErrScope
	}
	if err = s.finish(ctx, p); err != nil {
		return nil, err
	}
	raw, callErr := s.opt.Engine.Call(ctx, true, args...)
	rec := sendRecord{Hash: hash, Status: "uncertain"}
	if callErr != nil {
		rec.Error = "WhatsApp send failed without a confirmed outcome; check the conversation before sending again"
		var sendErr *engine.SendError
		if errors.As(callErr, &sendErr) {
			rec.Error = sendErr.Detail
			if !sendErr.Uncertain {
				rec.Status = "failed"
			}
		}
	} else {
		var result struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &result) == nil && result.ID != "" {
			rec.Status = "accepted"
			rec.MessageID = result.ID
		} else {
			rec.Error = "WhatsApp send returned no valid message ID; check the conversation before sending again"
		}
	}
	s.mu.Lock()
	s.cfg.Sends[key] = rec
	saveErr := s.saveLocked()
	s.mu.Unlock()
	if saveErr != nil {
		return nil, errors.New("send outcome could not be saved; reuse this request_id and check the conversation before sending again")
	}
	return sendResult(rec, v.RequestID), nil
}

func sendResult(rec sendRecord, requestID string) map[string]any {
	note := "The send outcome is uncertain. Check the selected conversation before deciding whether to send again; this request identifier will never dispatch again."
	switch rec.Status {
	case "accepted", "sent":
		note = "Accepted means WhatsApp accepted the send, not observed delivery or read. This request identifier will never dispatch again."
	case "failed":
		note = "No WhatsApp message was sent. This request identifier will never dispatch again."
	}
	out := map[string]any{"status": rec.Status, "message_id": rec.MessageID, "request_id": requestID, "note": note}
	if rec.Error != "" {
		out["error"] = rec.Error
	}
	return out
}

type attachment struct {
	File string `json:"file"`
	Chat string `json:"chat"`
	MIME string `json:"mime"`
}

type AttachmentRequest struct {
	ChatID    string `json:"chat_id"`
	MessageID string `json:"message_id"`
	Offset    int64  `json:"offset,omitempty"`
	MaxBytes  int    `json:"max_bytes,omitempty"`
}

func (s *Service) getAttachment(ctx context.Context, t AttachmentRequest) (any, error) {
	p, err := s.access(ctx, false)
	if err != nil {
		return nil, err
	}
	if !s.allowed(ctx, p, t.ChatID) {
		return nil, ErrScope
	}
	if len(t.MessageID) == 0 || len(t.MessageID) > 256 {
		return nil, errors.New("invalid message identifier")
	}
	if s.opt.Engine == nil {
		return nil, errors.New("archive is not connected")
	}
	// Exact media lookup does not expand identity aliases; resolve the retained
	// message through the read command, then authorize its canonical identity.
	shown, err := s.opt.Engine.Call(ctx, false, "messages", "show", "--chat", t.ChatID, "--id", t.MessageID)
	if err != nil {
		return nil, err
	}
	var parent archiveMessage
	if json.Unmarshal(shown, &parent) != nil || parent.MsgID != t.MessageID || !s.allowed(ctx, p, parent.ChatJID) {
		return nil, ErrScope
	}
	t.ChatID = parent.ChatJID
	dir := filepath.Join(s.opt.DataDir, "attachments")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New("cannot create private attachment cache")
	}
	name := digest(t.ChatID + ":" + t.MessageID)
	path := filepath.Join(dir, name)
	connected := s.requireWhatsAppConnection() == nil
	args := []string{"media", "download", "--chat", t.ChatID, "--id", t.MessageID, "--output", path}
	if connected {
		args = append(args, "--retry-on-phone")
	} else {
		// The engine verifies cached bytes against retained media metadata. A
		// cache miss must stop before CDN access, authentication, or phone retry.
		args = append(args, "--cache-only")
	}
	raw, err := s.opt.Engine.Call(ctx, connected, args...)
	if err != nil {
		return nil, err
	}
	var result struct {
		Downloaded bool   `json:"downloaded"`
		Source     string `json:"source"`
		MIME       string `json:"mime_type"`
	}
	if json.Unmarshal(raw, &result) != nil || !result.Downloaded {
		return nil, errors.New("attachment was not downloaded")
	}
	if !connected && result.Source != "cache" {
		return nil, errors.New("the offline attachment cache could not be verified")
	}
	if err = s.finish(ctx, p); err != nil {
		return nil, err
	}
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		return nil, errors.New("attachment cache file is unavailable")
	}
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	a := attachment{File: name, Chat: t.ChatID, MIME: result.MIME}
	b, _ := json.Marshal(a)
	token := s.seal(cursor{Agent: AgentID(ctx), Revision: p.Revision, Query: "attachment", Upstream: string(b), Expires: time.Now().Add(time.Hour).Unix()})
	out := map[string]any{"status": "available", "source": result.Source, "bytes": st.Size(), "mime_type": result.MIME, "url": s.requestBaseURL(ctx) + "/attachments/" + token, "authorization": "Use the same agent bearer authentication as MCP. URL expires after one hour and becomes invalid on a scope change."}
	if t.Offset < 0 || t.Offset > st.Size() {
		return nil, errors.New("attachment offset is outside the file")
	}
	chunkSize := t.MaxBytes
	if chunkSize <= 0 {
		chunkSize = 1 << 20
	}
	if chunkSize > 1<<20 {
		chunkSize = 1 << 20
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, errors.New("attachment cache is unavailable")
	}
	defer f.Close()
	if _, e = f.Seek(t.Offset, io.SeekStart); e != nil {
		return nil, errors.New("attachment offset is unavailable")
	}
	data, e := io.ReadAll(io.LimitReader(f, int64(chunkSize)))
	if e != nil {
		return nil, errors.New("attachment chunk could not be read")
	}
	out["data_base64"] = base64.StdEncoding.EncodeToString(data)
	out["offset"] = t.Offset
	out["next_offset"] = t.Offset + int64(len(data))
	out["has_more"] = t.Offset+int64(len(data)) < st.Size()
	out["chunk_instructions"] = "Decode data_base64 and concatenate chunks in offset order. Request next_offset while has_more is true; scope is rechecked for every chunk."
	return out, s.finish(ctx, p)
}
func (s *Service) attachmentHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	p, err := s.access(r.Context(), false)
	if err != nil {
		apiError(w, err)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/attachments/")
	c, err := s.unseal(token, AgentID(r.Context()), "attachment", p.Revision)
	if err != nil {
		apiError(w, ErrUnauthorized)
		return
	}
	var a attachment
	if json.Unmarshal([]byte(c.Upstream), &a) != nil || filepath.Base(a.File) != a.File || !s.allowed(r.Context(), p, a.Chat) {
		apiError(w, ErrScope)
		return
	}
	path := filepath.Join(s.opt.DataDir, "attachments", a.File)
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	if err = s.finish(r.Context(), p); err != nil {
		apiError(w, err)
		return
	}
	w.Header().Set("Content-Type", a.MIME)
	w.Header().Set("Content-Disposition", `attachment; filename="attachment"`)
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "HEAD" {
		return
	}
	buf := make([]byte, 32<<10)
	for {
		if r.Context().Err() != nil {
			return
		}
		n, e := f.Read(buf)
		if n > 0 {
			if _, err = w.Write(buf[:n]); err != nil {
				return
			}
		}
		if e != nil {
			return
		}
	}
}

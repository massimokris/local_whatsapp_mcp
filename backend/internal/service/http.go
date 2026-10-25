package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/massi/localwhatsappmcp/internal/engine"
	"github.com/massi/localwhatsappmcp/internal/secret"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

type identityKey struct{}
type operationContextKey struct{}

func AgentID(ctx context.Context) string { id, _ := ctx.Value(identityKey{}).(string); return id }
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
func apiError(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	if errors.Is(err, ErrUnauthorized) {
		code = 401
	}
	if errors.Is(err, ErrScope) {
		code = 403
	}
	if errors.Is(err, ErrSetup) || errors.Is(err, ErrTransition) {
		code = 409
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}
func decode(r *http.Request, out any) error {
	d := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return errors.New("request must contain valid JSON")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("request has trailing data")
	}
	return nil
}
func (s *Service) ControlHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/state":
			writeJSON(w, 200, s.State())
		case r.Method == "POST" && r.URL.Path == "/whatsapp/disconnect":
			if err := s.Disconnect(r.Context()); err != nil {
				apiError(w, err)
				return
			}
			writeJSON(w, 200, s.State())
		case r.Method == "POST" && (r.URL.Path == "/whatsapp/connect" || r.URL.Path == "/lifecycle/wake"):
			if err := s.Resume(r.Context(), r.URL.Path == "/whatsapp/connect"); err != nil {
				apiError(w, err)
				return
			}
			writeJSON(w, 200, s.State())
		case r.Method == "GET" && r.URL.Path == "/catalog":
			chats := []engine.Chat{}
			var err error
			if s.opt.Engine != nil {
				chats, err = s.opt.Engine.Catalog(r.Context())
			}
			if err != nil {
				apiError(w, err)
				return
			}
			writeJSON(w, 200, map[string]any{"chats": chats})
		case r.Method == "PUT" && r.URL.Path == "/scope":
			var p struct {
				Mode    string   `json:"mode"`
				ChatIDs []string `json:"chat_ids"`
			}
			if err := decode(r, &p); err != nil {
				apiError(w, err)
				return
			}
			if err := s.applyScope(r.Context(), engine.Policy{Mode: p.Mode, ChatIDs: p.ChatIDs}); err != nil {
				apiError(w, err)
				return
			}
			writeJSON(w, 200, s.State())
		case r.Method == "POST" && r.URL.Path == "/pairing":
			var p struct {
				Name string `json:"name"`
				// Accepted for older callers; all invitations use generic pairing.
				Client string `json:"client"`
			}
			if err := decode(r, &p); err != nil {
				apiError(w, err)
				return
			}
			v, err := s.invite(p.Name)
			if err != nil {
				apiError(w, err)
				return
			}
			writeJSON(w, 200, v)
		case r.Method == "POST" && r.URL.Path == "/agents/manual":
			var p struct {
				Name string `json:"name"`
			}
			if err := decode(r, &p); err != nil {
				apiError(w, err)
				return
			}
			v, err := s.createManualConnection(p.Name)
			if err != nil {
				apiError(w, err)
				return
			}
			writeJSON(w, 200, v)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/agents/") && strings.HasSuffix(r.URL.Path, "/credentials"):
			w.Header().Set("Cache-Control", "no-store")
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/agents/"), "/credentials")
			v, err := s.credentialsForConnection(id)
			if errors.Is(err, errConnectionNotFound) {
				http.NotFound(w, r)
				return
			}
			if err != nil {
				apiError(w, err)
				return
			}
			writeJSON(w, 200, v)
		case r.Method == "POST" && r.URL.Path == "/pairing/redeem":
			var p struct {
				Token string `json:"token"`
			}
			if err := decode(r, &p); err != nil {
				apiError(w, err)
				return
			}
			v, err := s.redeem(p.Token)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				apiError(w, err)
				return
			}
			writeJSON(w, 200, v)
		case strings.HasPrefix(r.URL.Path, "/agents/") && (r.Method == "PATCH" || r.Method == "DELETE"):
			id := strings.TrimPrefix(r.URL.Path, "/agents/")
			var enabled struct {
				SendEnabled bool `json:"send_enabled"`
			}
			if r.Method == "PATCH" {
				if err := decode(r, &enabled); err != nil {
					apiError(w, err)
					return
				}
			}
			s.operations.Lock()
			s.mu.Lock()
			g, ok := s.cfg.Grants[id]
			if !ok {
				s.mu.Unlock()
				s.operations.Unlock()
				http.NotFound(w, r)
				return
			}
			candidate := s.cfg
			candidate.Grants = maps.Clone(s.cfg.Grants)
			if r.Method == "DELETE" {
				delete(candidate.Grants, id)
			} else {
				g.SendEnabled = enabled.SendEnabled
				candidate.Grants[id] = g
			}
			err := AtomicJSON(filepath.Join(s.opt.DataDir, "settings.json"), candidate)
			if err == nil {
				s.cfg = candidate
			}
			s.mu.Unlock()
			s.operations.Unlock()
			if err != nil {
				apiError(w, err)
				return
			}
			if r.Method == "DELETE" {
				// Revocation is already durable. Cleanup failure must never restore
				// access; any leftover bearer has no matching grant.
				_ = secret.Delete(s.opt.DataDir, AgentCredentialNamespace, id)
			}
			writeJSON(w, 200, s.State())
		default:
			http.NotFound(w, r)
		}
	})
}
func (s *Service) AuthHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := r.Header.Get("Authorization")
		if !strings.HasPrefix(value, "Bearer ") {
			w.Header().Set("WWW-Authenticate", `Bearer realm="LocalWhatsAppMCP"`)
			apiError(w, ErrUnauthorized)
			return
		}
		id, err := s.VerifyToken(r.Context(), strings.TrimPrefix(value, "Bearer "))
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="LocalWhatsAppMCP", error="invalid_token"`)
			apiError(w, err)
			return
		}
		s.operations.RLock()
		defer s.operations.RUnlock()
		s.mu.Lock()
		generation := s.generation
		s.mu.Unlock()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		stop := context.AfterFunc(generation, cancel)
		defer stop()
		ctx = context.WithValue(ctx, authorizationKey{}, &requestAuthorization{})
		// Some MCP protocol versions intentionally detach HTTP cancellation. Keep
		// this operation's lifetime available to the actual tool handler as well.
		ctx = context.WithValue(ctx, operationContextKey{}, ctx)
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, identityKey{}, id)))
	})
}
func (s *Service) LocalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.AuthHandler(s.mcpHandler()))
	mux.Handle("/attachments/", s.AuthHandler(http.HandlerFunc(s.attachmentHTTP)))
	mux.HandleFunc("/pairing/redeem", s.pairingHTTP)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		local, _ := url.Parse(s.LocalURL())
		if local != nil && local.Host != "" {
			if r.Host != local.Host {
				http.Error(w, "unrecognized host", 403)
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && origin != s.LocalURL() {
				http.Error(w, "unrecognized origin", 403)
				return
			}
		}
		if len(r.Header.Get("Origin")) > 0 {
			w.Header().Add("Vary", "Origin")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		r.Body = http.MaxBytesReader(w, r.Body, 28<<20)
		mux.ServeHTTP(w, r)
	})
}

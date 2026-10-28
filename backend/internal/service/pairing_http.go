package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

const pairingBodyLimit = 4096

// pairingHTTP exchanges only an existing invitation. Invitation creation and
// grant administration remain on the private control socket.
func (s *Service) pairingHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "use POST to exchange a pairing code"})
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "Content-Type must be application/json"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, pairingBodyLimit))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "pairing request is too large"})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pairing request could not be read"})
		}
		return
	}
	token, err := pairingRequestToken(body)
	if err != nil {
		apiError(w, err)
		return
	}
	grant, err := s.redeem(token)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnauthorized), errors.Is(err, ErrSetup), errors.Is(err, ErrTransition):
			apiError(w, err)
		case errors.Is(err, ErrCredentials):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		default:
			// Filesystem diagnostics can contain private paths. A failed save
			// leaves the invitation available for a retry after recovery.
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "connection could not be saved; retry after checking the app"})
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"id":           grant["id"],
		"url":          s.requestBaseURL(r.Context()) + "/mcp",
		"access_token": grant["secret"],
		"token_type":   "Bearer",
	})
}

// Require exactly one string field, rejecting duplicate keys, null values,
// unknown fields, and trailing JSON instead of accepting ambiguous requests.
func pairingRequestToken(body []byte) (string, error) {
	invalid := errors.New("request must be a JSON object containing only a non-empty token string")
	d := json.NewDecoder(bytes.NewReader(body))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return "", invalid
	}
	var token string
	seen := false
	for d.More() {
		key, err := d.Token()
		if err != nil || key != "token" || seen {
			return "", invalid
		}
		value, err := d.Token()
		if err != nil {
			return "", invalid
		}
		var ok bool
		token, ok = value.(string)
		if !ok || strings.TrimSpace(token) == "" {
			return "", invalid
		}
		seen = true
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') || !seen {
		return "", invalid
	}
	if _, err := d.Token(); err != io.EOF {
		return "", invalid
	}
	return token, nil
}

package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

// ControlTokenHeader carries the app session token on the private control socket.
const ControlTokenHeader = "X-LWMCP-Control-Token"

const controlTokenRejected = "private control requires the current app session; reopen LocalWhatsAppMCP"

// ValidControlToken reports whether value is a 32-byte base64url (no padding) token.
// The length check also rejects CR/LF, which the base64 decoder would otherwise skip.
func ValidControlToken(value string) bool {
	if len(value) != 43 {
		return false
	}
	b, err := base64.RawURLEncoding.Strict().DecodeString(value)
	return err == nil && len(b) == 32
}

// RequireControlToken rejects every request whose header does not match token.
// An invalid configured token fails closed. Digests keep the comparison
// constant-time regardless of the received length, and the received value is
// never echoed.
func RequireControlToken(token string, next http.Handler) http.Handler {
	configured := ValidControlToken(token)
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(r.Header.Get(ControlTokenHeader)))
		if !configured || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": controlTokenRejected})
			return
		}
		next.ServeHTTP(w, r)
	})
}

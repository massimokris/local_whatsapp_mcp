package service_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/massi/localwhatsappmcp/internal/service"
)

// Synthetic session tokens. They only exist in tests.
var (
	fixtureControlToken = base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	otherControlToken   = base64.RawURLEncoding.EncodeToString([]byte("fedcba9876543210fedcba9876543210"))
)

type spyHandler struct{ calls int }

func (h *spyHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.calls++
	w.WriteHeader(http.StatusTeapot)
	w.Write([]byte("inner"))
}

func controlCall(h http.Handler, token *string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "/state", nil)
	if token != nil {
		r.Header.Set(service.ControlTokenHeader, *token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestValidControlTokenFormat(t *testing.T) {
	if len(fixtureControlToken) != 43 || !service.ValidControlToken(fixtureControlToken) {
		t.Fatal("canonical 32-byte token rejected")
	}
	for _, bad := range []string{"", fixtureControlToken[:42], fixtureControlToken + "A", fixtureControlToken[:42] + "=",
		"+" + fixtureControlToken[1:], "/" + fixtureControlToken[1:], " " + fixtureControlToken[1:],
		fixtureControlToken[:42] + "\r", fixtureControlToken[:21] + "\n" + fixtureControlToken[22:]} {
		if service.ValidControlToken(bad) {
			t.Errorf("malformed token accepted: %q", bad)
		}
	}
}

func TestControlTokenMissingIsRejected(t *testing.T) {
	inner := &spyHandler{}
	w := controlCall(service.RequireControlToken(fixtureControlToken, inner), nil)
	if w.Code != http.StatusUnauthorized || inner.calls != 0 {
		t.Fatalf("missing token reached handler: %d calls=%d", w.Code, inner.calls)
	}
	if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Body.String(), "current app session") {
		t.Fatalf("unexpected rejection: %v %s", w.Header(), w.Body.String())
	}
}

func TestControlTokenWrongIsRejected(t *testing.T) {
	inner := &spyHandler{}
	h := service.RequireControlToken(fixtureControlToken, inner)
	for _, value := range []string{otherControlToken, "short", fixtureControlToken + fixtureControlToken} {
		if w := controlCall(h, &value); w.Code != http.StatusUnauthorized {
			t.Errorf("wrong token %q returned %d", value, w.Code)
		}
	}
	if inner.calls != 0 {
		t.Fatalf("wrong tokens reached handler %d times", inner.calls)
	}
}

func TestControlTokenCorrectReachesHandler(t *testing.T) {
	inner := &spyHandler{}
	w := controlCall(service.RequireControlToken(fixtureControlToken, inner), &fixtureControlToken)
	if inner.calls != 1 || w.Code != http.StatusTeapot || w.Body.String() != "inner" {
		t.Fatalf("correct token not forwarded: calls=%d code=%d body=%q", inner.calls, w.Code, w.Body.String())
	}
}

func TestInvalidConfiguredTokenFailsClosed(t *testing.T) {
	for _, configured := range []string{"", "not-a-token", fixtureControlToken[:42] + "="} {
		inner := &spyHandler{}
		h := service.RequireControlToken(configured, inner)
		for _, value := range []string{"", configured, fixtureControlToken} {
			if w := controlCall(h, &value); w.Code != http.StatusUnauthorized {
				t.Errorf("configured %q accepted %q: %d", configured, value, w.Code)
			}
		}
		if w := controlCall(h, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("configured %q accepted a missing header", configured)
		}
		if inner.calls != 0 {
			t.Errorf("configured %q reached handler", configured)
		}
	}
}

func TestControlRejectionNeverEchoesValue(t *testing.T) {
	h := service.RequireControlToken(fixtureControlToken, &spyHandler{})
	if w := controlCall(h, &otherControlToken); strings.Contains(w.Body.String(), otherControlToken) {
		t.Fatal("rejection echoed the received token")
	}
}

func TestEveryControlRouteRequiresToken(t *testing.T) {
	s, err := service.New(service.Options{DataDir: t.TempDir(), CursorKey: []byte("01234567890123456789012345678901")})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := service.RequireControlToken(fixtureControlToken, s.ControlHandler())
	routes := []struct{ method, path string }{{"GET", "/state"}, {"GET", "/catalog"}, {"PUT", "/scope"}, {"POST", "/pairing"},
		{"POST", "/agents/manual"}, {"GET", "/agents/x/credentials"}, {"PATCH", "/agents/x"}, {"DELETE", "/agents/x"},
		{"POST", "/whatsapp/connect"}, {"POST", "/whatsapp/disconnect"}, {"POST", "/lifecycle/wake"}, {"POST", "/pairing/redeem"}}
	for _, route := range routes {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(route.method, route.path, strings.NewReader(`{"mode":"all","chat_ids":[],"send_enabled":true}`)))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token returned %d", route.method, route.path, w.Code)
		}
	}
	raw, _ := json.Marshal(s.State())
	if strings.Contains(string(raw), `"mode":"all"`) {
		t.Fatal("a rejected request changed chat scope")
	}
}

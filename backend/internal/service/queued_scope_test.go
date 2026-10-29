package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/massi/localwhatsappmcp/internal/engine"
)

type queuedScopeEngine struct {
	manualFixtureEngine
	entered chan struct{}
	release chan struct{}
	stops   atomic.Int32
	starts  atomic.Int32
	failure string
}

func (e *queuedScopeEngine) Stop(ctx context.Context) error {
	switch e.stops.Add(1) {
	case 1:
		close(e.entered)
		select {
		case <-e.release:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	case 2:
		if e.failure == "stop" {
			return errors.New("synthetic stop failure while applying new scope")
		}
	}
	return nil
}

func (e *queuedScopeEngine) Start(context.Context, engine.Policy, bool) error {
	if e.starts.Add(1) == 2 && e.failure == "ack" {
		return errors.New("synthetic missing policy acknowledgement")
	}
	return nil
}

func (e *queuedScopeEngine) Status() engine.Status {
	return engine.Status{Status: "ready", Linked: true}
}

func waitForQueuedScope(t *testing.T, s *Service, unblock func()) {
	t.Helper()
	for deadline := time.Now().Add(time.Second); ; {
		if s.State()["mcp"].(map[string]any)["status"] == "applying_scope" {
			return
		}
		if time.Now().After(deadline) {
			unblock()
			t.Fatal("scope never queued")
		}
		time.Sleep(time.Millisecond)
	}
}

func assertScopeBlocksMCP(t *testing.T, s *Service, token string) {
	t.Helper()
	response := ngrokRequest(t, s, publicFixtureURL, http.MethodPost, "/mcp", token, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"find_conversations","arguments":{}}}`)
	if !strings.Contains(response.Body.String(), "chat scope is being applied") {
		t.Errorf("failed queued scope reopened access before policy acknowledgement: %s", response.Body.String())
	}
	if status := s.State()["mcp"].(map[string]any)["status"]; status != "applying_scope" {
		t.Errorf("failed queued scope reported mcp status %v", status)
	}
}

func TestQueuedFailedScopeRemainsBlocked(t *testing.T) {
	for _, direction := range []string{"widen", "narrow"} {
		for _, failure := range []string{"stop", "ack"} {
			t.Run(direction+"/"+failure, func(t *testing.T) {
				s, _ := httpPairingFixture(t)
				defer s.Close()
				body := `{"mode":"all","chat_ids":[]}`
				if direction == "narrow" {
					if response := manualControl(t, s, http.MethodPut, "/scope", map[string]any{"mode": "all", "chat_ids": []string{}}); response.Code != http.StatusOK {
						t.Fatal("could not seed all-chat scope")
					}
					body = `{"mode":"selected","chat_ids":["123@s.whatsapp.net"]}`
				}
				credentials := createManualFixture(t, s, "Queued scope fixture")
				e := &queuedScopeEngine{entered: make(chan struct{}), release: make(chan struct{}), failure: failure}
				s.opt.Engine = e
				first := make(chan error, 1)
				go func() { first <- s.Resume(t.Context(), true) }()
				<-e.entered
				second := make(chan int, 1)
				go func() {
					w := httptest.NewRecorder()
					s.ControlHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/scope", strings.NewReader(body)))
					second <- w.Code
				}()
				waitForQueuedScope(t, s, func() { close(e.release) })
				close(e.release)
				if err := <-first; err != nil {
					t.Fatal(err)
				}
				if code := <-second; code == http.StatusOK {
					t.Fatal("fixture scope did not fail")
				}
				assertScopeBlocksMCP(t, s, credentials.AccessToken)

				// A later acknowledged attempt, not the earlier Connect, can
				// reopen access under the chosen policy.
				retried := httptest.NewRecorder()
				s.ControlHandler().ServeHTTP(retried, httptest.NewRequest(http.MethodPut, "/scope", strings.NewReader(body)))
				if retried.Code != http.StatusOK || s.State()["mcp"].(map[string]any)["status"] != "ready" {
					t.Fatal("acknowledged scope retry did not reopen access")
				}
				response := ngrokRequest(t, s, publicFixtureURL, http.MethodPost, "/mcp", credentials.AccessToken, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"find_conversations","arguments":{}}}`)
				if strings.Contains(response.Body.String(), "999@s.whatsapp.net") != (direction == "widen") {
					t.Fatal("acknowledged scope did not enforce the chosen chat selection")
				}
			})
		}
	}
}

func TestQueuedScopeKeepsAccessBlockedBetweenSerializedChanges(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	credentials := createManualFixture(t, s, "Queued scope admission fixture")
	invitation := httpPairingInvite(t, s)
	e := &queuedScopeEngine{entered: make(chan struct{}), release: make(chan struct{}), failure: "ack"}
	s.opt.Engine = e
	first := make(chan error, 1)
	go func() { first <- s.Resume(t.Context(), true) }()
	<-e.entered
	second := make(chan int, 1)
	go func() {
		w := httptest.NewRecorder()
		s.ControlHandler().ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/scope", strings.NewReader(`{"mode":"all","chat_ids":[]}`)))
		second <- w.Code
	}()
	waitForQueuedScope(t, s, func() { close(e.release) })

	// Queue real authenticated requests behind the preceding lifecycle action.
	// Their synthetic handlers hold admission open until state is inspected;
	// the queued scope must remain visible before it can acquire exclusivity.
	var requests sync.WaitGroup
	queued := make(chan struct{}, 16)
	entered := make(chan struct{}, 16)
	releaseRequests := make(chan struct{})
	handler := s.AuthHandler(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		entered <- struct{}{}
		<-releaseRequests
	}))
	for range cap(queued) {
		requests.Go(func() {
			request := httptest.NewRequest(http.MethodGet, "/mcp", nil)
			request.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
			queued <- struct{}{}
			handler.ServeHTTP(httptest.NewRecorder(), request)
		})
	}
	for range cap(queued) {
		<-queued
	}
	close(e.release)
	if err := <-first; err != nil {
		t.Error(err)
	}
	<-entered
	if status := s.State()["mcp"].(map[string]any)["status"]; status != "applying_scope" {
		t.Errorf("earlier completion hid the queued scope: %v", status)
	}
	for _, request := range []struct{ path, body string }{
		{"/agents/manual", `{"name":"Should stay blocked"}`},
		{"/pairing", `{"name":"Should stay blocked"}`},
	} {
		response := httptest.NewRecorder()
		s.ControlHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body)))
		if response.Code != http.StatusConflict {
			t.Errorf("queued scope permitted %s: HTTP %d", request.path, response.Code)
		}
	}
	body, _ := json.Marshal(map[string]string{"token": invitation})
	if response := httpPairingRequest(s, http.MethodPost, string(body), "application/json"); response.Code != http.StatusConflict {
		t.Errorf("queued scope permitted invitation redemption: HTTP %d", response.Code)
	}
	close(releaseRequests)
	requests.Wait()
	if status := <-second; status == http.StatusOK {
		t.Fatal("fixture did not fail policy acknowledgement")
	}
	assertScopeBlocksMCP(t, s, credentials.AccessToken)
}

func TestSuccessfulScopeCannotClearQueuedFailedScope(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	credentials := createManualFixture(t, s, "Multiple scope fixture")
	if err := s.Resume(t.Context(), true); err != nil {
		t.Fatal(err)
	}
	e := &queuedScopeEngine{entered: make(chan struct{}), release: make(chan struct{}), failure: "ack"}
	s.opt.Engine = e
	apply := func(body string, done chan<- int) {
		response := httptest.NewRecorder()
		s.ControlHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/scope", strings.NewReader(body)))
		done <- response.Code
	}
	first := make(chan int, 1)
	go apply(`{"mode":"all","chat_ids":[]}`, first)
	<-e.entered
	second := make(chan int, 1)
	queued := make(chan struct{})
	go func() {
		close(queued)
		apply(`{"mode":"selected","chat_ids":["123@s.whatsapp.net"]}`, second)
	}()
	<-queued
	close(e.release)
	if status := <-first; status != http.StatusOK {
		t.Fatalf("first scope returned %d", status)
	}
	if status := <-second; status == http.StatusOK {
		t.Fatal("second scope did not fail acknowledgement")
	}
	assertScopeBlocksMCP(t, s, credentials.AccessToken)
	if response := manualControl(t, s, http.MethodPut, "/scope", map[string]any{"mode": "selected", "chat_ids": []string{"123@s.whatsapp.net"}}); response.Code != http.StatusOK {
		t.Fatal("later valid scope did not recover")
	}
	if status := s.State()["mcp"].(map[string]any)["status"]; status != "ready" {
		t.Fatalf("completed scope retained another request's queued ownership: %v", status)
	}
}

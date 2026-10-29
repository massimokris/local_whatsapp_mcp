package service

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type pausedLifecycleEngine struct {
	manualFixtureEngine
	stopEntered chan struct{}
	stopRelease chan struct{}
	once        sync.Once
}

func (e *pausedLifecycleEngine) Stop(ctx context.Context) error {
	var err error
	e.once.Do(func() {
		close(e.stopEntered)
		select {
		case <-e.stopRelease:
		case <-ctx.Done():
			err = ctx.Err()
		}
	})
	return err
}

// An authenticated client can pause while sending a body or consuming a stream.
// App-owned reconnect must cancel that request before awaiting exclusive access,
// just as scope changes and disconnect do; the client cannot own the app's wake.
func TestResumeInterruptsIncompleteAuthenticatedHTTPBody(t *testing.T) {
	for _, path := range []string{"/lifecycle/wake", "/whatsapp/connect"} {
		t.Run(path, func(t *testing.T) {
			s, _ := httpPairingFixture(t)
			defer s.Close()
			if err := s.Resume(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			credentials := createManualFixture(t, s, "Paused synthetic client")
			started := make(chan struct{})
			handler := s.LocalHandler()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Body = &observedNetworkBody{ReadCloser: r.Body, started: started}
				handler.ServeHTTP(w, r)
			}))
			defer server.Close()
			conn, err := net.Dial("tcp", server.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_, err = fmt.Fprintf(conn, "POST /mcp HTTP/1.1\r\nHost: fixture.ngrok-free.app\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nAccept: application/json, text/event-stream\r\nContent-Length: 128\r\n\r\n{", credentials.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("synthetic request did not enter its body read")
			}
			completed := make(chan int, 1)
			go func() {
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
				response := httptest.NewRecorder()
				s.ControlHandler().ServeHTTP(response, request)
				completed <- response.Code
			}()
			select {
			case code := <-completed:
				if code != http.StatusOK {
					t.Fatalf("resume status %d", code)
				}
			case <-time.After(2 * time.Second):
				conn.Close()
				<-completed
				t.Fatal("resume blocked on an unfinished authenticated HTTP request")
			}
			response := ngrokRequest(t, s, publicFixtureURL, http.MethodPost, "/mcp", credentials.AccessToken, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"check_status","arguments":{}}}`)
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"structuredContent"`) {
				t.Fatal("resume did not restore authorized requests after cancellation")
			}
		})
	}
}

func TestOverlappingLifecycleChangesDrainRequestsFromEachLifetime(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	credentials := createManualFixture(t, s, "Concurrent lifecycle fixture")
	engine := &pausedLifecycleEngine{stopEntered: make(chan struct{}), stopRelease: make(chan struct{})}
	s.opt.Engine = engine
	first := make(chan error, 1)
	go func() { first <- s.Resume(context.Background(), true) }()
	<-engine.stopEntered
	second := make(chan int, 1)
	go func() {
		response := httptest.NewRecorder()
		s.ControlHandler().ServeHTTP(response, httptest.NewRequest(http.MethodPut, "/scope", strings.NewReader(`{"mode":"all","chat_ids":[]}`)))
		second <- response.Code
	}()
	// The private state proves the second lifecycle action has begun. The
	// first one still owns the engine, so requests below queue behind it.
	for deadline := time.Now().Add(2 * time.Second); ; {
		if s.State()["mcp"].(map[string]any)["status"] == "applying_scope" {
			break
		}
		if time.Now().After(deadline) {
			close(engine.stopRelease)
			t.Fatal("scope change never began")
		}
		time.Sleep(time.Millisecond)
	}
	var requests sync.WaitGroup
	queued := make(chan struct{}, 16)
	releaseRequests := make(chan struct{})
	handler := s.AuthHandler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-releaseRequests:
		}
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
	close(engine.stopRelease)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	select {
	case status := <-second:
		close(releaseRequests)
		requests.Wait()
		if status != http.StatusOK {
			t.Fatalf("overlapping scope change returned %d", status)
		}
	case <-time.After(2 * time.Second):
		close(releaseRequests)
		requests.Wait()
		<-second
		t.Fatal("second lifecycle change failed to cancel the renewed request lifetime")
	}
}

func TestSupersededStartupDoesNotCancelOrWaitForCurrentRequest(t *testing.T) {
	s, _ := httpPairingFixture(t)
	defer s.Close()
	credentials := createManualFixture(t, s, "Startup fixture")
	if err := s.Resume(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	requestDone := make(chan struct{})
	handler := s.AuthHandler(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		entered <- r.Context()
		<-release
	}))
	go func() {
		defer close(requestDone)
		request := httptest.NewRequest(http.MethodGet, "/mcp", nil)
		request.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}()
	requestContext := <-entered
	startupDone := make(chan error, 1)
	go func() { startupDone <- s.ResumeSavedSession(context.Background()) }()
	select {
	case err := <-startupDone:
		if err != nil {
			t.Error(err)
		}
		if requestContext.Err() != nil {
			t.Error("superseded startup canceled the current request")
		}
	case <-time.After(2 * time.Second):
		t.Error("superseded startup waited for unrelated active request")
	}
	close(release)
	<-requestDone
}

func TestShutdownCannotBeReopenedByScopeOrQueuedLifecycleChange(t *testing.T) {
	s, _ := httpPairingFixture(t)
	credentials := createManualFixture(t, s, "Shutdown fixture")
	engine := &pausedLifecycleEngine{stopEntered: make(chan struct{}), stopRelease: make(chan struct{})}
	s.opt.Engine = engine
	resumed := make(chan error, 1)
	go func() { resumed <- s.Resume(context.Background(), true) }()
	<-engine.stopEntered
	closed := make(chan error, 1)
	go func() { closed <- s.Close() }()
	for deadline := time.Now().Add(2 * time.Second); ; {
		if _, err := s.VerifyToken(context.Background(), credentials.AccessToken); err == ErrUnauthorized {
			break
		}
		if time.Now().After(deadline) {
			close(engine.stopRelease)
			t.Fatal("shutdown did not close authorization")
		}
		time.Sleep(time.Millisecond)
	}
	close(engine.stopRelease)
	<-resumed
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if response := manualControl(t, s, http.MethodPut, "/scope", map[string]any{"mode": "all", "chat_ids": []string{}}); response.Code != http.StatusUnauthorized {
		t.Fatalf("scope change after shutdown returned %d", response.Code)
	}
	if err := s.Resume(context.Background(), true); err != ErrUnauthorized {
		t.Fatalf("connect after shutdown: %v", err)
	}
	if _, err := s.VerifyToken(context.Background(), credentials.AccessToken); err != ErrUnauthorized {
		t.Fatal("shutdown restored authorization")
	}
}

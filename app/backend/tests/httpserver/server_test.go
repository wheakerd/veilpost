package httpserver_test

import (
	"context"
	"errors"
	"github.com/wheakerd/veilpost/app/backend/internal/httpserver"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func listenerForTest(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return listener
}

func waitFor(t *testing.T, event <-chan struct{}) {
	t.Helper()
	select {
	case <-event:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server event")
	}
}

func TestShutdownDrainsInFlightRequest(t *testing.T) {
	entered, finish, stopping := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httpserver.New("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-finish:
			_, _ = io.WriteString(w, "completed")
		case <-r.Context().Done():
		}
	}))
	server.RegisterOnShutdown(func() { close(stopping) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); _ = server.Close() })
	listener := listenerForTest(t)
	served := make(chan error, 1)
	go func() { served <- httpserver.Serve(ctx, server, listener, 3*time.Second) }()
	client := &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	received := make(chan error, 1)
	go func() {
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			defer response.Body.Close()
			var body []byte
			body, err = io.ReadAll(response.Body)
			if err == nil && (response.StatusCode != 200 || string(body) != "completed") {
				err = errors.New("in-flight response was not completed")
			}
		}
		received <- err
	}()
	waitFor(t, entered)
	cancel()
	waitFor(t, stopping)
	select {
	case err := <-served:
		t.Fatalf("shutdown returned before the request completed: %v", err)
	default:
	}
	close(finish)
	select {
	case err := <-received:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not complete")
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not complete")
	}
}

func TestShutdownDeadlineClosesStalledRequest(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	server := httpserver.New("", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(stopped)
	}))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel(); _ = server.Close() })
	listener := listenerForTest(t)
	served := make(chan error, 1)
	go func() { served <- httpserver.Serve(ctx, server, listener, 25*time.Millisecond) }()
	client := &http.Client{Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	go func() {
		response, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			response.Body.Close()
		}
	}()
	waitFor(t, entered)
	cancel()
	select {
	case err := <-served:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected shutdown deadline, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown deadline was not enforced")
	}
	waitFor(t, stopped)
}

func TestCancelledStartupClosesListener(t *testing.T) {
	listener := listenerForTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := httpserver.Serve(ctx, httpserver.New("", http.NotFoundHandler()), listener, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err == nil {
		connection.Close()
		t.Fatal("listener remained open after cancelled startup")
	}
}

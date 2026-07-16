package relayclient

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/andrewedunn/agent-reading-relay/internal/relay"
)

func TestPublishUsesUnixSocketAndDecodesResponse(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "relay.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/articles" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var request relay.PublishRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Agent != "research-agent" || !request.Send {
			t.Fatalf("unexpected publish request: %#v", request)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(relay.PublishResponse{ID: "abc", Status: "delivered", BookmarkID: "123", Created: true})
	})}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	client := New(socket)
	got, err := client.Publish(context.Background(), relay.PublishRequest{
		Title: "Brief", Markdown: "Body", Agent: "research-agent", Send: true,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got.ID != "abc" || got.BookmarkID != "123" {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestPublishReturnsRelayError(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "relay.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "agent is not allowlisted"})
	})}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())

	_, err = New(socket).Publish(context.Background(), relay.PublishRequest{Title: "Brief", Markdown: "Body", Agent: "unknown"})
	if err == nil || err.Error() != "relay: agent is not allowlisted" {
		t.Fatalf("unexpected error: %v", err)
	}
}

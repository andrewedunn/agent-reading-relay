package relay

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andrewedunn/agent-reading-relay/internal/instapaper"
	"github.com/andrewedunn/agent-reading-relay/internal/store"
)

type fakeInstapaper struct {
	calls []instapaper.AddBookmarkRequest
	err   error
}

func (f *fakeInstapaper) AddBookmark(_ context.Context, request instapaper.AddBookmarkRequest) (instapaper.Bookmark, error) {
	f.calls = append(f.calls, request)
	return instapaper.Bookmark{ID: "bookmark-123"}, f.err
}

func newTestService(t *testing.T, client Instapaper) *Service {
	t.Helper()
	articles, err := store.Open(filepath.Join(t.TempDir(), "relay.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { articles.Close() })
	return &Service{
		Store: articles, Instapaper: client,
		PublicBaseURL: "https://reader.example", OwnerEmail: "owner@example.com",
		AllowedAgents: map[string]bool{"research-agent": true, "family-agent": true},
		Now:           func() time.Time { return time.Unix(100, 0).UTC() },
	}
}

func TestPublishGeneratedDraftArchivesWithoutExternalWrite(t *testing.T) {
	client := &fakeInstapaper{}
	service := newTestService(t, client)

	got, err := service.Publish(context.Background(), PublishRequest{
		Title: "Weekly Brief", Markdown: "# Weekly Brief\n\nHello **the user**.",
		Agent: "research-agent", Send: false,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got.Status != store.StatusDraft || got.ID == "" {
		t.Fatalf("unexpected response: %#v", got)
	}
	if got.CanonicalURL != "https://reader.example/articles/"+got.ID {
		t.Fatalf("canonical URL = %q", got.CanonicalURL)
	}
	if len(client.calls) != 0 {
		t.Fatalf("draft caused %d Instapaper calls", len(client.calls))
	}
	article, err := service.Store.Get(context.Background(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(article.HTML, "<strong>the user</strong>") {
		t.Fatalf("saved HTML was not rendered: %s", article.HTML)
	}
}

func TestPublishGeneratedSendSuppliesCanonicalURLAndHTML(t *testing.T) {
	client := &fakeInstapaper{}
	service := newTestService(t, client)

	got, err := service.Publish(context.Background(), PublishRequest{
		Title: "Research Summary", Description: "Three findings",
		Markdown: "# Research Summary\n\nUseful findings.", Agent: "research-agent", Send: true,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got.Status != store.StatusDelivered || got.BookmarkID != "bookmark-123" {
		t.Fatalf("unexpected response: %#v", got)
	}
	if len(client.calls) != 1 {
		t.Fatalf("Instapaper calls = %d, want 1", len(client.calls))
	}
	call := client.calls[0]
	if call.URL != got.CanonicalURL || call.Title != "Research Summary" || call.HTML == "" {
		t.Fatalf("unexpected Instapaper request: %#v", call)
	}
}

func TestPublishURLDoesNotInventArticleContent(t *testing.T) {
	client := &fakeInstapaper{}
	service := newTestService(t, client)

	got, err := service.Publish(context.Background(), PublishRequest{
		Title: "Existing Article", URL: "https://example.com/read-me", Agent: "family-agent", Send: true,
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if got.CanonicalURL != "https://example.com/read-me" {
		t.Fatalf("canonical URL = %q", got.CanonicalURL)
	}
	if len(client.calls) != 1 || client.calls[0].HTML != "" {
		t.Fatalf("unexpected Instapaper calls: %#v", client.calls)
	}
}

func TestPublishIsIdempotentAfterDelivery(t *testing.T) {
	client := &fakeInstapaper{}
	service := newTestService(t, client)
	request := PublishRequest{Title: "Same Brief", Markdown: "Same body", Agent: "research-agent", Send: true}

	first, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Publish(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(client.calls) != 1 {
		t.Fatalf("idempotency failed: first=%#v second=%#v calls=%d", first, second, len(client.calls))
	}
}

func TestPublishRejectsUnsafeGeneratedSourceURL(t *testing.T) {
	service := newTestService(t, &fakeInstapaper{})
	_, err := service.Publish(context.Background(), PublishRequest{
		Title: "Brief", Markdown: "Body", SourceURL: "javascript:alert(1)", Agent: "research-agent",
	})
	if err == nil || !strings.Contains(err.Error(), "source_url") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPublishRejectsUnallowlistedAgent(t *testing.T) {
	service := newTestService(t, &fakeInstapaper{})
	_, err := service.Publish(context.Background(), PublishRequest{
		Title: "Nope", Markdown: "Nope", Agent: "unknown", Send: false,
	})
	if err == nil || !strings.Contains(err.Error(), "not allowlisted") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConcurrentPublishSendHasSingleDeliveryWinner(t *testing.T) {
	client := &blockingInstapaper{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	service := newTestService(t, client)
	request := PublishRequest{Title: "Concurrent Brief", Markdown: "Body", Agent: "research-agent", Send: true}

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.Publish(context.Background(), request)
		firstDone <- err
	}()
	<-client.started

	secondDone := make(chan error, 1)
	go func() {
		_, err := service.Publish(context.Background(), request)
		secondDone <- err
	}()
	if err := <-secondDone; err != nil {
		t.Fatalf("second Publish: %v", err)
	}
	close(client.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Publish: %v", err)
	}
	if got := client.callCount(); got != 1 {
		t.Fatalf("Instapaper calls = %d, want 1", got)
	}
}

type blockingInstapaper struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (b *blockingInstapaper) AddBookmark(_ context.Context, _ instapaper.AddBookmarkRequest) (instapaper.Bookmark, error) {
	b.mu.Lock()
	b.calls++
	callNumber := b.calls
	if callNumber == 1 {
		close(b.started)
	}
	b.mu.Unlock()
	if callNumber == 1 {
		<-b.release
	}
	return instapaper.Bookmark{ID: "bookmark-123"}, nil
}

func (b *blockingInstapaper) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

func TestPublishValidatesInputShape(t *testing.T) {
	service := newTestService(t, &fakeInstapaper{})
	tests := []struct {
		name    string
		request PublishRequest
		want    string
	}{
		{name: "both content modes", request: PublishRequest{Title: "Brief", Markdown: "Body", URL: "https://example.com", Agent: "research-agent"}, want: "exactly one"},
		{name: "neither content mode", request: PublishRequest{Title: "Brief", Agent: "research-agent"}, want: "exactly one"},
		{name: "blank title", request: PublishRequest{Title: "  ", Markdown: "Body", Agent: "research-agent"}, want: "title is required"},
		{name: "blank agent", request: PublishRequest{Title: "Brief", Markdown: "Body", Agent: "  "}, want: "agent is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.Publish(context.Background(), test.request)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestURLDraftCanCorrectMetadataBeforeSend(t *testing.T) {
	client := &fakeInstapaper{}
	service := newTestService(t, client)
	url := "https://example.com/correct-me"
	if _, err := service.Publish(context.Background(), PublishRequest{
		Title: "Old title", Description: "Old description", URL: url, Agent: "research-agent",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Publish(context.Background(), PublishRequest{
		Title: "Correct title", Description: "Correct description", URL: url, Agent: "research-agent", Send: true,
	}); err != nil {
		t.Fatal(err)
	}
	if len(client.calls) != 1 || client.calls[0].Title != "Correct title" || client.calls[0].Description != "Correct description" {
		t.Fatalf("unexpected Instapaper calls: %#v", client.calls)
	}
}

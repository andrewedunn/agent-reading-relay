package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveIsIdempotentByArticleID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	input := Article{
		ID: "abc123", Kind: "generated", Title: "Agent Brief",
		CanonicalURL: "https://reader.example/articles/abc123",
		Markdown:     "# Agent Brief", HTML: "<h1>Agent Brief</h1>",
		AuthorAgent: "build-agent", ContentHash: "hash", Status: StatusDraft,
		CreatedAt: time.Unix(100, 0).UTC(),
	}
	first, created, err := store.Save(context.Background(), input)
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if !created {
		t.Fatal("first Save should create article")
	}
	second, created, err := store.Save(context.Background(), input)
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}
	if created {
		t.Fatal("second Save should return existing article")
	}
	if first.ID != second.ID || second.Title != input.Title {
		t.Fatalf("unexpected saved articles: first=%#v second=%#v", first, second)
	}
}

func TestSaveUpdatesMetadataBeforeDelivery(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	base := Article{
		ID: "url123", Kind: "url", Title: "Old title", Description: "Old description",
		CanonicalURL: "https://example.com/article", SourceURL: "https://example.com/article",
		AuthorAgent: "research-agent", ContentHash: "hash", Status: StatusDraft,
		CreatedAt: time.Unix(100, 0).UTC(),
	}
	if _, _, err := store.Save(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	base.Title = "Corrected title"
	base.Description = "Corrected description"
	base.AuthorAgent = "family-agent"
	got, created, err := store.Save(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("metadata update should not create a second article")
	}
	if got.Title != "Corrected title" || got.Description != "Corrected description" || got.AuthorAgent != "family-agent" {
		t.Fatalf("metadata was not updated: %#v", got)
	}
}

func TestClaimDeliveryHasSingleWinner(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _, err = store.Save(context.Background(), Article{
		ID: "abc123", Kind: "url", Title: "Article", CanonicalURL: "https://example.com/a",
		AuthorAgent: "research-agent", ContentHash: "hash", Status: StatusDraft, CreatedAt: time.Unix(100, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	won, err := store.ClaimDelivery(context.Background(), "abc123")
	if err != nil || !won {
		t.Fatalf("first ClaimDelivery = %v, %v; want true, nil", won, err)
	}
	won, err = store.ClaimDelivery(context.Background(), "abc123")
	if err != nil || won {
		t.Fatalf("second ClaimDelivery = %v, %v; want false, nil", won, err)
	}
	got, err := store.Get(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusSending {
		t.Fatalf("status = %q, want %q", got.Status, StatusSending)
	}
}

func TestMarkDeliveredPersistsBookmarkID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, _, err = store.Save(context.Background(), Article{
		ID: "abc123", Kind: "url", Title: "An article", CanonicalURL: "https://example.com/article",
		AuthorAgent: "research-agent", ContentHash: "hash", Status: StatusDraft, CreatedAt: time.Unix(100, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	deliveredAt := time.Unix(200, 0).UTC()
	if err := store.MarkDelivered(context.Background(), "abc123", "999", deliveredAt); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	got, err := store.Get(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusDelivered || got.BookmarkID != "999" || !got.DeliveredAt.Equal(deliveredAt) {
		t.Fatalf("unexpected delivery state: %#v", got)
	}
}

func TestMarkFailedStoresOnlySafeError(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "relay.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, _, _ = store.Save(context.Background(), Article{
		ID: "abc123", Kind: "url", Title: "An article", CanonicalURL: "https://example.com/article",
		AuthorAgent: "research-agent", ContentHash: "hash", Status: StatusDraft, CreatedAt: time.Unix(100, 0).UTC(),
	})
	if err := store.MarkFailed(context.Background(), "abc123", "Instapaper returned HTTP 500"); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed || got.DeliveryError != "Instapaper returned HTTP 500" {
		t.Fatalf("unexpected failed state: %#v", got)
	}
}

func TestOpenRecoversInterruptedDeliveries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.sqlite3")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = first.Save(context.Background(), Article{
		ID: "abc123", Kind: "url", Title: "Article", CanonicalURL: "https://example.com/retry",
		AuthorAgent: "research-agent", ContentHash: "hash", Status: StatusDraft, CreatedAt: time.Unix(100, 0).UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if won, err := first.ClaimDelivery(context.Background(), "abc123"); err != nil || !won {
		t.Fatalf("ClaimDelivery = %v, %v", won, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Get(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusFailed || got.DeliveryError == "" {
		t.Fatalf("interrupted delivery was not recovered: %#v", got)
	}
}

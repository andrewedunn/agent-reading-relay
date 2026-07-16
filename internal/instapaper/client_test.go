package instapaper

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func fixedSigner() Signer {
	return Signer{
		Nonce: func() (string, error) { return "nonce", nil },
		Now:   func() time.Time { return time.Unix(100, 0) },
	}
}

func TestClientAddsGeneratedArticleWithHTMLContent(t *testing.T) {
	var received url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/bookmarks/add" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "OAuth ") {
			t.Fatalf("missing OAuth header: %q", r.Header.Get("Authorization"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		received = r.Form
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"type":"bookmark","bookmark_id":12345}]`)
	}))
	defer server.Close()

	client := Client{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
		Credentials: Credentials{
			ConsumerKey: "consumer", ConsumerSecret: "consumer-secret",
			AccessToken: "token", AccessTokenSecret: "token-secret",
		},
		Signer: fixedSigner(),
	}
	bookmark, err := client.AddBookmark(context.Background(), AddBookmarkRequest{
		URL:         "https://reader.example/articles/abc",
		Title:       "Agent Brief",
		Description: "A concise summary",
		HTML:        "<h1>Agent Brief</h1><p>Hello</p>",
	})
	if err != nil {
		t.Fatalf("AddBookmark: %v", err)
	}
	if bookmark.ID != "12345" {
		t.Fatalf("bookmark ID = %q, want 12345", bookmark.ID)
	}
	for key, want := range map[string]string{
		"url": "https://reader.example/articles/abc", "title": "Agent Brief",
		"description": "A concise summary", "content": "<h1>Agent Brief</h1><p>Hello</p>",
		"resolve_final_url": "0",
	} {
		if received.Get(key) != want {
			t.Errorf("form %s = %q, want %q", key, received.Get(key), want)
		}
	}
}

func TestExchangeCredentialsUsesXAuthAndReturnsTokens(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("x_auth_username"); got != "reader@example.com" {
			t.Errorf("username = %q", got)
		}
		if got := r.Form.Get("x_auth_password"); got != "temporary-password" {
			t.Errorf("password = %q", got)
		}
		_, _ = io.WriteString(w, "oauth_token=access123&oauth_token_secret=secret456")
	}))
	defer server.Close()

	got, err := ExchangeCredentials(context.Background(), server.Client(), server.URL, fixedSigner(), Credentials{
		ConsumerKey: "consumer", ConsumerSecret: "consumer-secret",
	}, "reader@example.com", "temporary-password")
	if err != nil {
		t.Fatalf("ExchangeCredentials: %v", err)
	}
	if got.AccessToken != "access123" || got.AccessTokenSecret != "secret456" {
		t.Fatalf("unexpected credentials: %#v", got)
	}
	if strings.Contains(authorization, "oauth_token=") {
		t.Fatalf("xAuth OAuth header includes access token: %s", authorization)
	}
}

func TestClientReportsInstapaperErrorWithoutEchoingRequestContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `[{"type":"error","error_code":1242,"message":"bad request"}]`, http.StatusBadRequest)
	}))
	defer server.Close()

	client := Client{
		BaseURL: server.URL, HTTPClient: server.Client(),
		Credentials: Credentials{ConsumerKey: "consumer", ConsumerSecret: "consumer-secret", AccessToken: "token", AccessTokenSecret: "token-secret"},
		Signer:      fixedSigner(),
	}
	_, err := client.AddBookmark(context.Background(), AddBookmarkRequest{URL: "https://example.test", Title: "private-title", HTML: "private-body"})
	if err == nil {
		t.Fatal("expected error")
	}
	encoded, _ := json.Marshal(err.Error())
	if strings.Contains(string(encoded), "private-title") || strings.Contains(string(encoded), "private-body") {
		t.Fatalf("error leaked request content: %v", err)
	}
}

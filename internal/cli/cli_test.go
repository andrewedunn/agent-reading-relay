package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrewedunn/agent-reading-relay/internal/instapaper"
	"github.com/andrewedunn/agent-reading-relay/internal/relay"
)

type fakePublisher struct {
	request relay.PublishRequest
	calls   int
}

func (f *fakePublisher) Publish(_ context.Context, request relay.PublishRequest) (relay.PublishResponse, error) {
	f.calls++
	f.request = request
	return relay.PublishResponse{ID: "abc", Status: "delivered", BookmarkID: "123"}, nil
}

func TestRunPublishReadsMarkdownAndRequiresExplicitSendFlag(t *testing.T) {
	file := filepath.Join(t.TempDir(), "brief.md")
	if err := os.WriteFile(file, []byte("# Brief\n\nHello"), 0o600); err != nil {
		t.Fatal(err)
	}
	publisher := &fakePublisher{}
	var stdout bytes.Buffer
	deps := Dependencies{NewPublisher: func(string) Publisher { return publisher }}

	err := Run(context.Background(), []string{
		"publish", "--title", "Daily Brief", "--file", file,
		"--agent", "research-agent", "--send",
	}, strings.NewReader(""), &stdout, io.Discard, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if publisher.request.Markdown != "# Brief\n\nHello" || !publisher.request.Send {
		t.Fatalf("unexpected request: %#v", publisher.request)
	}
	if !strings.Contains(stdout.String(), `"status": "delivered"`) {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
}

func TestRunSaveURLDefaultsToDraft(t *testing.T) {
	publisher := &fakePublisher{}
	deps := Dependencies{NewPublisher: func(string) Publisher { return publisher }}
	if err := Run(context.Background(), []string{
		"save-url", "--title", "Article", "--url", "https://example.com/a", "--agent", "family-agent",
	}, strings.NewReader(""), io.Discard, io.Discard, deps); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if publisher.request.URL != "https://example.com/a" || publisher.request.Send {
		t.Fatalf("unexpected request: %#v", publisher.request)
	}
}

func TestRunConfigureInstapaperDoesNotPrintSecrets(t *testing.T) {
	answers := []string{"consumer-key", "consumer-secret", "reader@example.com", "account-password"}
	promptIndex := 0
	var written instapaper.Credentials
	deps := Dependencies{
		Prompt: func(string, bool) (string, error) {
			answer := answers[promptIndex]
			promptIndex++
			return answer, nil
		},
		Exchange: func(_ context.Context, consumer instapaper.Credentials, username, password string) (instapaper.Credentials, error) {
			if username != "reader@example.com" || password != "account-password" {
				t.Fatalf("unexpected xAuth input")
			}
			consumer.AccessToken = "access-token"
			consumer.AccessTokenSecret = "access-secret"
			return consumer, nil
		},
		WriteCredentials: func(_ string, credentials instapaper.Credentials) error {
			written = credentials
			return nil
		},
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"configure-instapaper", "--credentials-dir", t.TempDir()}, strings.NewReader(""), &stdout, io.Discard, deps); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if written.AccessToken != "access-token" || written.ConsumerKey != "consumer-key" {
		t.Fatalf("unexpected credentials: %#v", written)
	}
	for _, secret := range answers {
		if strings.Contains(stdout.String(), secret) {
			t.Fatalf("stdout leaked secret %q: %s", secret, stdout.String())
		}
	}
}

func TestRunRejectsWhitespaceRequiredFlagsBeforeCallingRelay(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "publish title", args: []string{"publish", "--title", "  ", "--agent", "research-agent"}},
		{name: "publish agent", args: []string{"publish", "--title", "Brief", "--agent", "  "}},
		{name: "save URL title", args: []string{"save-url", "--title", "  ", "--url", "https://example.com", "--agent", "research-agent"}},
		{name: "save URL agent", args: []string{"save-url", "--title", "Article", "--url", "https://example.com", "--agent", "  "}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publisher := &fakePublisher{}
			deps := Dependencies{NewPublisher: func(string) Publisher { return publisher }}
			err := Run(context.Background(), test.args, strings.NewReader("Body"), io.Discard, io.Discard, deps)
			if err == nil {
				t.Fatal("expected validation error")
			}
			if publisher.calls != 0 {
				t.Fatalf("relay was called %d times", publisher.calls)
			}
		})
	}
}

func TestRunPublishRejectsOversizedMarkdownFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "too-large.md")
	if err := os.WriteFile(file, bytes.Repeat([]byte("x"), (4<<20)+1), 0o600); err != nil {
		t.Fatal(err)
	}
	publisher := &fakePublisher{}
	deps := Dependencies{NewPublisher: func(string) Publisher { return publisher }}
	err := Run(context.Background(), []string{
		"publish", "--title", "Large brief", "--file", file, "--agent", "research-agent",
	}, strings.NewReader(""), io.Discard, io.Discard, deps)
	if err == nil || !strings.Contains(err.Error(), "exceeds 4 MiB") {
		t.Fatalf("unexpected error: %v", err)
	}
	if publisher.calls != 0 {
		t.Fatalf("relay was called %d times", publisher.calls)
	}
}

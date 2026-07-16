package article

import (
	"strings"
	"testing"
)

func TestRenderMarkdownProducesSafeReaderHTML(t *testing.T) {
	got, err := RenderMarkdown("# Weekly Brief\n\nHello **the user**.\n\n<script>alert('no')</script>")
	if err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}

	for _, want := range []string{"<h1>Weekly Brief</h1>", "<strong>the user</strong>"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered HTML missing %q: %s", want, got)
		}
	}
	if strings.Contains(strings.ToLower(got), "<script") {
		t.Errorf("rendered HTML retained script: %s", got)
	}
}

func TestRenderMarkdownRejectsEmptyDocuments(t *testing.T) {
	if _, err := RenderMarkdown(" \n\t"); err == nil {
		t.Fatal("expected empty Markdown to be rejected")
	}
}

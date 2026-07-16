package article

import (
	"bytes"
	"errors"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

// RenderMarkdown converts Markdown into sanitized HTML suitable for an
// e-reader article body.
func RenderMarkdown(markdown string) (string, error) {
	if strings.TrimSpace(markdown) == "" {
		return "", errors.New("markdown is empty")
	}

	var rendered bytes.Buffer
	if err := goldmark.Convert([]byte(markdown), &rendered); err != nil {
		return "", err
	}

	return string(bluemonday.UGCPolicy().SanitizeBytes(rendered.Bytes())), nil
}

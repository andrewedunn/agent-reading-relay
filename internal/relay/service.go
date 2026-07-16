package relay

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/andrewedunn/agent-reading-relay/internal/article"
	"github.com/andrewedunn/agent-reading-relay/internal/instapaper"
	"github.com/andrewedunn/agent-reading-relay/internal/store"
)

type Instapaper interface {
	AddBookmark(context.Context, instapaper.AddBookmarkRequest) (instapaper.Bookmark, error)
}

type Service struct {
	Store         *store.Store
	Instapaper    Instapaper
	PublicBaseURL string
	OwnerEmail    string
	AllowedAgents map[string]bool
	Now           func() time.Time
}

type PublishRequest struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Markdown    string `json:"markdown,omitempty"`
	URL         string `json:"url,omitempty"`
	SourceURL   string `json:"source_url,omitempty"`
	Agent       string `json:"agent"`
	Send        bool   `json:"send"`
}

type PublishResponse struct {
	ID           string `json:"id"`
	CanonicalURL string `json:"canonical_url"`
	Status       string `json:"status"`
	BookmarkID   string `json:"bookmark_id,omitempty"`
	Created      bool   `json:"created"`
}

func (s *Service) Publish(ctx context.Context, request PublishRequest) (PublishResponse, error) {
	request.Title = strings.TrimSpace(request.Title)
	request.Agent = strings.ToLower(strings.TrimSpace(request.Agent))
	request.Markdown = strings.TrimSpace(request.Markdown)
	request.URL = strings.TrimSpace(request.URL)
	request.SourceURL = strings.TrimSpace(request.SourceURL)
	if request.Title == "" {
		return PublishResponse{}, fmt.Errorf("title is required")
	}
	if request.Agent == "" {
		return PublishResponse{}, fmt.Errorf("agent is required")
	}
	if !s.AllowedAgents[request.Agent] {
		return PublishResponse{}, fmt.Errorf("agent %q is not allowlisted", request.Agent)
	}
	if (request.Markdown == "") == (request.URL == "") {
		return PublishResponse{}, fmt.Errorf("provide exactly one of markdown or url")
	}
	if s.Store == nil {
		return PublishResponse{}, fmt.Errorf("article store is not configured")
	}

	stored, err := s.prepareArticle(request)
	if err != nil {
		return PublishResponse{}, err
	}
	saved, created, err := s.Store.Save(ctx, stored)
	if err != nil {
		return PublishResponse{}, err
	}
	response := responseFor(saved, created)
	if !request.Send {
		return response, nil
	}
	claimed, err := s.Store.ClaimDelivery(ctx, saved.ID)
	if err != nil {
		return PublishResponse{}, err
	}
	if !claimed {
		current, err := s.Store.Get(ctx, saved.ID)
		if err != nil {
			return PublishResponse{}, err
		}
		return responseFor(current, created), nil
	}
	if s.Instapaper == nil {
		err := fmt.Errorf("Instapaper delivery is not configured")
		_ = s.Store.MarkFailed(ctx, saved.ID, err.Error())
		return PublishResponse{}, err
	}

	bookmark, err := s.Instapaper.AddBookmark(ctx, instapaper.AddBookmarkRequest{
		URL:         saved.CanonicalURL,
		Title:       saved.Title,
		Description: saved.Description,
		HTML:        saved.HTML,
	})
	if err != nil {
		safeError := safeDeliveryError(err)
		_ = s.Store.MarkFailed(ctx, saved.ID, safeError)
		return PublishResponse{}, fmt.Errorf("deliver article to Instapaper: %s", safeError)
	}
	if err := s.Store.MarkDelivered(ctx, saved.ID, bookmark.ID, s.now()); err != nil {
		return PublishResponse{}, err
	}
	delivered, err := s.Store.Get(ctx, saved.ID)
	if err != nil {
		return PublishResponse{}, err
	}
	return responseFor(delivered, created), nil
}

func (s *Service) prepareArticle(request PublishRequest) (store.Article, error) {
	now := s.now()
	if request.SourceURL != "" {
		if err := validateHTTPURL("source_url", request.SourceURL); err != nil {
			return store.Article{}, err
		}
	}
	if request.URL != "" {
		if err := validateHTTPURL("url", request.URL); err != nil {
			return store.Article{}, err
		}
		digest := sha256.Sum256([]byte(request.URL))
		return store.Article{
			ID: shortID(digest[:]), Kind: "url", Title: request.Title,
			Description: request.Description, SourceURL: request.URL, CanonicalURL: request.URL,
			AuthorAgent: request.Agent, ContentHash: hex.EncodeToString(digest[:]),
			Status: store.StatusDraft, CreatedAt: now,
		}, nil
	}

	if strings.TrimSpace(s.PublicBaseURL) == "" {
		return store.Article{}, fmt.Errorf("public base URL is required for generated articles")
	}
	html, err := article.RenderMarkdown(request.Markdown)
	if err != nil {
		return store.Article{}, fmt.Errorf("render article: %w", err)
	}
	identity := strings.Join([]string{request.Title, request.Description, request.SourceURL, request.Markdown}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	id := shortID(digest[:])
	return store.Article{
		ID: id, Kind: "generated", Title: request.Title, Description: request.Description,
		SourceURL:    request.SourceURL,
		CanonicalURL: strings.TrimRight(s.PublicBaseURL, "/") + "/articles/" + id,
		Markdown:     request.Markdown, HTML: html, AuthorAgent: request.Agent,
		ContentHash: hex.EncodeToString(digest[:]), Status: store.StatusDraft, CreatedAt: now,
	}, nil
}

func validateHTTPURL(field, value string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%s must be an absolute HTTP or HTTPS URL", field)
	}
	return nil
}

func responseFor(article store.Article, created bool) PublishResponse {
	return PublishResponse{
		ID: article.ID, CanonicalURL: article.CanonicalURL, Status: article.Status,
		BookmarkID: article.BookmarkID, Created: created,
	}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func shortID(digest []byte) string {
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:12]))
}

func safeDeliveryError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

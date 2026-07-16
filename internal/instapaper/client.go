package instapaper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const defaultBaseURL = "https://www.instapaper.com"

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	BaseURL     string
	HTTPClient  HTTPDoer
	Credentials Credentials
	Signer      Signer
}

type AddBookmarkRequest struct {
	URL         string
	Title       string
	Description string
	HTML        string
}

type Bookmark struct {
	ID string
}

func (c Client) AddBookmark(ctx context.Context, input AddBookmarkRequest) (Bookmark, error) {
	if strings.TrimSpace(input.URL) == "" {
		return Bookmark{}, fmt.Errorf("bookmark URL is required")
	}

	form := url.Values{"url": {input.URL}}
	if input.Title != "" {
		form.Set("title", input.Title)
	}
	if input.Description != "" {
		form.Set("description", input.Description)
	}
	if input.HTML != "" {
		form.Set("content", input.HTML)
		form.Set("resolve_final_url", "0")
	}

	endpoint := strings.TrimRight(c.baseURL(), "/") + "/api/1/bookmarks/add"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Bookmark{}, fmt.Errorf("create Instapaper request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := c.Signer.Sign(req, c.Credentials); err != nil {
		return Bookmark{}, fmt.Errorf("sign Instapaper request: %w", err)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Bookmark{}, fmt.Errorf("call Instapaper: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Bookmark{}, fmt.Errorf("read Instapaper response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Bookmark{}, parseAPIError(resp.StatusCode, body)
	}

	var objects []struct {
		Type       string          `json:"type"`
		BookmarkID json.RawMessage `json:"bookmark_id"`
	}
	if err := json.Unmarshal(body, &objects); err != nil {
		return Bookmark{}, fmt.Errorf("decode Instapaper response: %w", err)
	}
	for _, object := range objects {
		if len(object.BookmarkID) == 0 {
			continue
		}
		var idString string
		if err := json.Unmarshal(object.BookmarkID, &idString); err == nil {
			return Bookmark{ID: idString}, nil
		}
		var idNumber json.Number
		if err := json.Unmarshal(object.BookmarkID, &idNumber); err == nil {
			return Bookmark{ID: idNumber.String()}, nil
		}
	}
	return Bookmark{}, fmt.Errorf("Instapaper response did not include a bookmark ID")
}

// ExchangeCredentials exchanges a username and password for an xAuth token.
// Callers must discard the password immediately after this function returns.
func ExchangeCredentials(ctx context.Context, httpClient HTTPDoer, endpoint string, signer Signer, consumer Credentials, username, password string) (Credentials, error) {
	if username == "" || password == "" {
		return Credentials{}, fmt.Errorf("Instapaper username and password are required")
	}
	form := url.Values{
		"x_auth_username": {username},
		"x_auth_password": {password},
		"x_auth_mode":     {"client_auth"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Credentials{}, fmt.Errorf("create xAuth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := signer.Sign(req, consumer); err != nil {
		return Credentials{}, fmt.Errorf("sign xAuth request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Credentials{}, fmt.Errorf("call Instapaper xAuth: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return Credentials{}, fmt.Errorf("read Instapaper xAuth response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Credentials{}, parseAPIError(resp.StatusCode, body)
	}
	values, err := url.ParseQuery(strings.TrimSpace(string(body)))
	if err != nil {
		return Credentials{}, fmt.Errorf("decode Instapaper xAuth response: %w", err)
	}
	token, secret := values.Get("oauth_token"), values.Get("oauth_token_secret")
	if token == "" || secret == "" {
		return Credentials{}, fmt.Errorf("Instapaper xAuth response did not include both token fields")
	}
	consumer.AccessToken = token
	consumer.AccessTokenSecret = secret
	return consumer, nil
}

func (c Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return defaultBaseURL
}

func (c Client) httpClient() HTTPDoer {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func parseAPIError(status int, body []byte) error {
	var objects []struct {
		ErrorCode json.RawMessage `json:"error_code"`
		Message   string          `json:"message"`
	}
	if json.Unmarshal(body, &objects) == nil {
		for _, object := range objects {
			if object.Message == "" {
				continue
			}
			code := strings.Trim(string(object.ErrorCode), `"`)
			if code != "" && code != "null" {
				return fmt.Errorf("Instapaper API error %s (HTTP %d): %s", code, status, object.Message)
			}
			return fmt.Errorf("Instapaper API error (HTTP %d): %s", status, object.Message)
		}
	}
	return fmt.Errorf("Instapaper API returned HTTP %s", strconv.Itoa(status))
}

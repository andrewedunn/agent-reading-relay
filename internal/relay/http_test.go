package relay

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIHandlerPublishesDraft(t *testing.T) {
	service := newTestService(t, &fakeInstapaper{})
	handler := APIHandler(service)
	body := `{"title":"Daily Brief","markdown":"# Daily Brief\n\nHello","agent":"research-agent","send":false}`
	req := httptest.NewRequest(http.MethodPost, "/v1/articles", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var response PublishResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID == "" || response.Status != "draft" {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestAPIHandlerRejectsUnknownFields(t *testing.T) {
	service := newTestService(t, &fakeInstapaper{})
	handler := APIHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/v1/articles", bytes.NewBufferString(`{"title":"Brief","markdown":"Body","agent":"research-agent","surprise":true}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestAPIHandlerRejectsTrailingJSON(t *testing.T) {
	service := newTestService(t, &fakeInstapaper{})
	handler := APIHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/v1/articles", bytes.NewBufferString(`{"title":"Brief","markdown":"Body","agent":"research-agent"} {"second":true}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestPublicHandlerRequiresOwnerAuthentication(t *testing.T) {
	service := newTestService(t, &fakeInstapaper{})
	published, err := service.Publish(t.Context(), PublishRequest{
		Title: "Private Brief", Markdown: "# Private Brief\n\nSecret-ish body.", Agent: "research-agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := PublicHandler(service)

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/articles/"+published.ID, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	spoofed := httptest.NewRequest(http.MethodGet, "/articles/"+published.ID, nil)
	spoofed.Header.Set("X-ExeDev-Email", "owner@example.com")
	spoofedResponse := httptest.NewRecorder()
	handler.ServeHTTP(spoofedResponse, spoofed)
	if spoofedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("email-only status = %d", spoofedResponse.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/articles/"+published.ID, nil)
	req.Header.Set("X-ExeDev-UserID", "owner-user-id")
	req.Header.Set("X-ExeDev-Email", "owner@example.com")
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, req)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, body=%s", authorized.Code, authorized.Body.String())
	}
	for _, want := range []string{"Private Brief", "Secret-ish body.", "research-agent"} {
		if !strings.Contains(authorized.Body.String(), want) {
			t.Errorf("article page missing %q: %s", want, authorized.Body.String())
		}
	}
}

func TestHealthHandlerDoesNotExposeConfiguration(t *testing.T) {
	service := newTestService(t, &fakeInstapaper{})
	handler := PublicHandler(service)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != "ok" {
		t.Fatalf("unexpected health response: status=%d body=%q", w.Code, w.Body.String())
	}
}

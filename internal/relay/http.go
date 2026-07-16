package relay

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

func APIHandler(service *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/articles", func(w http.ResponseWriter, r *http.Request) {
		body := http.MaxBytesReader(w, r.Body, 4<<20)
		decoder := json.NewDecoder(body)
		decoder.DisallowUnknownFields()
		var request PublishRequest
		if err := decoder.Decode(&request); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request")
			return
		}
		if err := ensureJSONEOF(decoder); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request")
			return
		}

		response, err := service.Publish(r.Context(), request)
		if err != nil {
			slog.Warn("publish article", "agent", request.Agent, "send", request.Send, "error", err)
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		status := http.StatusOK
		if response.Created {
			status = http.StatusCreated
		}
		writeJSON(w, status, response)
	})
	return mux
}

func PublicHandler(service *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("GET /articles/{id}", func(w http.ResponseWriter, r *http.Request) {
		userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
		userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
		if service.OwnerEmail == "" || userID == "" || !strings.EqualFold(userEmail, service.OwnerEmail) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		article, err := service.Store.Get(r.Context(), r.PathValue("id"))
		if err != nil || article.Kind != "generated" {
			http.NotFound(w, r)
			return
		}
		data := articlePageData{
			Title: article.Title, Description: article.Description, SourceURL: article.SourceURL,
			AuthorAgent: article.AuthorAgent, HTML: template.HTML(article.HTML), // sanitized by article.RenderMarkdown
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if err := articlePage.Execute(w, data); err != nil {
			slog.Warn("render article", "article_id", article.ID, "error", err)
		}
	})
	return mux
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("request contains multiple JSON values")
	}
	return err
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

type articlePageData struct {
	Title       string
	Description string
	SourceURL   string
	AuthorAgent string
	HTML        template.HTML
}

var articlePage = template.Must(template.New("article").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root { color-scheme: light dark; font-family: Georgia, serif; line-height: 1.65; }
body { max-width: 44rem; margin: 3rem auto; padding: 0 1.25rem 5rem; }
header { border-bottom: 1px solid #8885; margin-bottom: 2rem; }
h1 { line-height: 1.15; }
.meta { font-family: system-ui, sans-serif; opacity: .72; font-size: .9rem; }
a { color: inherit; }
img { max-width: 100%; height: auto; }
</style>
</head>
<body>
<header>
<h1>{{.Title}}</h1>
{{if .Description}}<p>{{.Description}}</p>{{end}}
<p class="meta">Prepared by {{.AuthorAgent}}{{if .SourceURL}} · <a href="{{.SourceURL}}" rel="noreferrer">Source</a>{{end}}</p>
</header>
<article>{{.HTML}}</article>
</body>
</html>`))

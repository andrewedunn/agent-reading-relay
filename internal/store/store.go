package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StatusDraft     = "draft"
	StatusSending   = "sending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

type Article struct {
	ID            string
	Kind          string
	Title         string
	Description   string
	SourceURL     string
	CanonicalURL  string
	Markdown      string
	HTML          string
	AuthorAgent   string
	ContentHash   string
	Status        string
	BookmarkID    string
	DeliveryError string
	CreatedAt     time.Time
	DeliveredAt   time.Time
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	for _, pragma := range []string{"PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=1000"} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("configure SQLite database: %w", err)
		}
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize SQLite database: %w", err)
	}
	if _, err := db.Exec(`
		UPDATE articles
		SET status = ?, delivery_error = ?
		WHERE status = ?`, StatusFailed, "delivery interrupted by service restart", StatusSending); err != nil {
		db.Close()
		return nil, fmt.Errorf("recover interrupted deliveries: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Save(ctx context.Context, article Article) (Article, bool, error) {
	result, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO articles (
			id, kind, title, description, source_url, canonical_url, markdown, html,
			author_agent, content_hash, status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		article.ID, article.Kind, article.Title, article.Description, article.SourceURL,
		article.CanonicalURL, article.Markdown, article.HTML, article.AuthorAgent,
		article.ContentHash, article.Status, formatTime(article.CreatedAt),
	)
	if err != nil {
		return Article{}, false, fmt.Errorf("save article: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return Article{}, false, fmt.Errorf("read saved article result: %w", err)
	}
	created := rows == 1
	if !created {
		if _, err := s.db.ExecContext(ctx, `
			UPDATE articles
			SET title = ?, description = ?, source_url = ?, markdown = ?, html = ?,
			    author_agent = ?, content_hash = ?
			WHERE id = ? AND status IN (?, ?)`,
			article.Title, article.Description, article.SourceURL, article.Markdown, article.HTML,
			article.AuthorAgent, article.ContentHash, article.ID, StatusDraft, StatusFailed,
		); err != nil {
			return Article{}, false, fmt.Errorf("update pending article metadata: %w", err)
		}
	}
	saved, err := s.Get(ctx, article.ID)
	if err != nil {
		return Article{}, false, err
	}
	return saved, created, nil
}

func (s *Store) ClaimDelivery(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE articles
		SET status = ?, delivery_error = ''
		WHERE id = ? AND status IN (?, ?)`, StatusSending, id, StatusDraft, StatusFailed)
	if err != nil {
		return false, fmt.Errorf("claim article delivery: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("claim article delivery: %w", err)
	}
	return rows == 1, nil
}

func (s *Store) Get(ctx context.Context, id string) (Article, error) {
	var article Article
	var createdAt string
	var deliveredAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id, kind, title, description, source_url, canonical_url, markdown, html,
		       author_agent, content_hash, status, bookmark_id, delivery_error,
		       created_at, delivered_at
		FROM articles WHERE id = ?`, id).Scan(
		&article.ID, &article.Kind, &article.Title, &article.Description,
		&article.SourceURL, &article.CanonicalURL, &article.Markdown, &article.HTML,
		&article.AuthorAgent, &article.ContentHash, &article.Status,
		&article.BookmarkID, &article.DeliveryError, &createdAt, &deliveredAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Article{}, fmt.Errorf("article %q not found", id)
	}
	if err != nil {
		return Article{}, fmt.Errorf("get article: %w", err)
	}
	article.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return Article{}, fmt.Errorf("parse article creation time: %w", err)
	}
	if deliveredAt.Valid {
		article.DeliveredAt, err = time.Parse(time.RFC3339Nano, deliveredAt.String)
		if err != nil {
			return Article{}, fmt.Errorf("parse article delivery time: %w", err)
		}
	}
	return article, nil
}

func (s *Store) MarkDelivered(ctx context.Context, id, bookmarkID string, deliveredAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE articles
		SET status = ?, bookmark_id = ?, delivery_error = '', delivered_at = ?
		WHERE id = ?`, StatusDelivered, bookmarkID, formatTime(deliveredAt), id)
	return updateResult("mark article delivered", result, err)
}

func (s *Store) MarkFailed(ctx context.Context, id, safeError string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE articles SET status = ?, delivery_error = ? WHERE id = ?`,
		StatusFailed, safeError, id)
	return updateResult("mark article failed", result, err)
}

func updateResult(action string, result sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	if rows != 1 {
		return fmt.Errorf("%s: article not found", action)
	}
	return nil
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

const schema = `
CREATE TABLE IF NOT EXISTS articles (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('generated', 'url')),
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    source_url TEXT NOT NULL DEFAULT '',
    canonical_url TEXT NOT NULL UNIQUE,
    markdown TEXT NOT NULL DEFAULT '',
    html TEXT NOT NULL DEFAULT '',
    author_agent TEXT NOT NULL,
    content_hash TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('draft', 'sending', 'delivered', 'failed')),
    bookmark_id TEXT NOT NULL DEFAULT '',
    delivery_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    delivered_at TEXT
);
CREATE INDEX IF NOT EXISTS articles_created_at_idx ON articles(created_at DESC);
`

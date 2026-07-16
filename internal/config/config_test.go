package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsSystemdCredentialFiles(t *testing.T) {
	dir := t.TempDir()
	credentials := map[string]string{
		"instapaper_consumer_key":        "consumer-key\n",
		"instapaper_consumer_secret":     "consumer-secret\n",
		"instapaper_access_token":        "access-token\n",
		"instapaper_access_token_secret": "access-secret\n",
	}
	for name, value := range credentials {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CREDENTIALS_DIRECTORY", dir)
	t.Setenv("READING_RELAY_PUBLIC_BASE_URL", "https://reader.example")
	t.Setenv("READING_RELAY_OWNER_EMAIL", "owner@example.com")
	t.Setenv("READING_RELAY_ALLOWED_AGENTS", "research-agent, family-agent")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Instapaper.ConsumerKey != "consumer-key" || got.Instapaper.AccessTokenSecret != "access-secret" {
		t.Fatalf("unexpected credentials: %#v", got.Instapaper)
	}
	if !got.AllowedAgents["research-agent"] || !got.AllowedAgents["family-agent"] {
		t.Fatalf("unexpected allowlist: %#v", got.AllowedAgents)
	}
}

func TestLoadAllowsDraftOnlyModeWhenNoCredentialsExist(t *testing.T) {
	t.Setenv("CREDENTIALS_DIRECTORY", t.TempDir())
	t.Setenv("READING_RELAY_PUBLIC_BASE_URL", "https://reader.example")
	t.Setenv("READING_RELAY_OWNER_EMAIL", "owner@example.com")
	t.Setenv("READING_RELAY_ALLOWED_AGENTS", "research-agent")
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.PublicAddr != "127.0.0.1:8484" {
		t.Fatalf("default public address = %q", got.PublicAddr)
	}
	if got.Instapaper.Configured() {
		t.Fatalf("expected draft-only configuration: %#v", got.Instapaper)
	}
}

func TestLoadRejectsPartialInstapaperCredentials(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "instapaper_consumer_key"), []byte("only-one"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CREDENTIALS_DIRECTORY", dir)
	t.Setenv("READING_RELAY_PUBLIC_BASE_URL", "https://reader.example")
	t.Setenv("READING_RELAY_OWNER_EMAIL", "owner@example.com")
	t.Setenv("READING_RELAY_ALLOWED_AGENTS", "research-agent")
	if _, err := Load(); err == nil {
		t.Fatal("expected partial credential configuration to fail")
	}
}

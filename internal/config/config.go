package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andrewedunn/agent-reading-relay/internal/instapaper"
)

type InstapaperConfig struct {
	instapaper.Credentials
}

func (c InstapaperConfig) Configured() bool {
	return c.ConsumerKey != "" && c.ConsumerSecret != "" && c.AccessToken != "" && c.AccessTokenSecret != ""
}

type Config struct {
	DBPath        string
	PublicAddr    string
	SocketPath    string
	PublicBaseURL string
	OwnerEmail    string
	AllowedAgents map[string]bool
	CredentialDir string
	Instapaper    InstapaperConfig
}

func Load() (Config, error) {
	config := Config{
		DBPath:        envOrDefault("READING_RELAY_DB_PATH", "/var/lib/reading-relay/relay.sqlite3"),
		PublicAddr:    envOrDefault("READING_RELAY_PUBLIC_ADDR", "127.0.0.1:8484"),
		SocketPath:    envOrDefault("READING_RELAY_SOCKET_PATH", "/run/reading-relay/relay.sock"),
		PublicBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("READING_RELAY_PUBLIC_BASE_URL")), "/"),
		OwnerEmail:    strings.TrimSpace(os.Getenv("READING_RELAY_OWNER_EMAIL")),
		AllowedAgents: parseAllowlist(os.Getenv("READING_RELAY_ALLOWED_AGENTS")),
		CredentialDir: credentialDirectory(),
	}
	if config.PublicBaseURL == "" {
		return Config{}, fmt.Errorf("READING_RELAY_PUBLIC_BASE_URL is required")
	}
	if config.OwnerEmail == "" {
		return Config{}, fmt.Errorf("READING_RELAY_OWNER_EMAIL is required")
	}
	if len(config.AllowedAgents) == 0 {
		return Config{}, fmt.Errorf("READING_RELAY_ALLOWED_AGENTS must contain at least one agent")
	}

	values := make(map[string]string)
	for _, name := range []string{
		"instapaper_consumer_key", "instapaper_consumer_secret",
		"instapaper_access_token", "instapaper_access_token_secret",
	} {
		value, err := readOptionalCredential(config.CredentialDir, name)
		if err != nil {
			return Config{}, err
		}
		values[name] = value
	}
	config.Instapaper = InstapaperConfig{Credentials: instapaper.Credentials{
		ConsumerKey: values["instapaper_consumer_key"], ConsumerSecret: values["instapaper_consumer_secret"],
		AccessToken: values["instapaper_access_token"], AccessTokenSecret: values["instapaper_access_token_secret"],
	}}
	configuredCount := 0
	for _, value := range values {
		if value != "" {
			configuredCount++
		}
	}
	if configuredCount != 0 && configuredCount != len(values) {
		return Config{}, fmt.Errorf("Instapaper credential set is incomplete")
	}
	return config, nil
}

func credentialDirectory() string {
	if value := strings.TrimSpace(os.Getenv("READING_RELAY_CREDENTIALS_DIR")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("CREDENTIALS_DIRECTORY")); value != "" {
		return value
	}
	return "/etc/reading-relay/credentials"
}

func readOptionalCredential(dir, name string) (string, error) {
	value, err := os.ReadFile(filepath.Join(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read credential %s: %w", name, err)
	}
	return strings.TrimSpace(string(value)), nil
}

func parseAllowlist(value string) map[string]bool {
	result := make(map[string]bool)
	for _, entry := range strings.Split(value, ",") {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry != "" {
			result[entry] = true
		}
	}
	return result
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

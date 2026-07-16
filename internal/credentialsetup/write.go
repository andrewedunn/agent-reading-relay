package credentialsetup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andrewedunn/agent-reading-relay/internal/instapaper"
)

func WriteCredentialSet(dir string, credentials instapaper.Credentials) error {
	values := map[string]string{
		"instapaper_consumer_key":        credentials.ConsumerKey,
		"instapaper_consumer_secret":     credentials.ConsumerSecret,
		"instapaper_access_token":        credentials.AccessToken,
		"instapaper_access_token_secret": credentials.AccessTokenSecret,
	}
	for name, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("credential %s is empty", name)
		}
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("credential %s contains a newline", name)
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure credential directory: %w", err)
	}

	temps := make(map[string]string, len(values))
	cleanup := func() {
		for _, path := range temps {
			_ = os.Remove(path)
		}
	}
	defer cleanup()
	for name, value := range values {
		file, err := os.CreateTemp(dir, "."+name+"-*")
		if err != nil {
			return fmt.Errorf("create temporary credential %s: %w", name, err)
		}
		path := file.Name()
		temps[name] = path
		if err := file.Chmod(0o600); err != nil {
			file.Close()
			return fmt.Errorf("secure temporary credential %s: %w", name, err)
		}
		if _, err := file.WriteString(value + "\n"); err != nil {
			file.Close()
			return fmt.Errorf("write credential %s: %w", name, err)
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return fmt.Errorf("sync credential %s: %w", name, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close credential %s: %w", name, err)
		}
	}
	for name, temporary := range temps {
		if err := os.Rename(temporary, filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("install credential %s: %w", name, err)
		}
		delete(temps, name)
	}
	return nil
}

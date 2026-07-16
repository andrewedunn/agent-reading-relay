package credentialsetup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/andrewedunn/agent-reading-relay/internal/instapaper"
)

func TestWriteCredentialSetCreatesPrivateFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "credentials")
	credentials := instapaper.Credentials{
		ConsumerKey: "consumer", ConsumerSecret: "consumer-secret",
		AccessToken: "access", AccessTokenSecret: "access-secret",
	}
	if err := WriteCredentialSet(dir, credentials); err != nil {
		t.Fatalf("WriteCredentialSet: %v", err)
	}
	for name, want := range map[string]string{
		"instapaper_consumer_key":        "consumer",
		"instapaper_consumer_secret":     "consumer-secret",
		"instapaper_access_token":        "access",
		"instapaper_access_token_secret": "access-secret",
	} {
		path := filepath.Join(dir, name)
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(contents) != want+"\n" {
			t.Errorf("%s contents = %q", name, contents)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s permissions = %o, want 600", name, info.Mode().Perm())
		}
	}
}

func TestWriteCredentialSetRejectsIncompleteValues(t *testing.T) {
	err := WriteCredentialSet(t.TempDir(), instapaper.Credentials{ConsumerKey: "consumer"})
	if err == nil {
		t.Fatal("expected incomplete credential set to fail")
	}
}

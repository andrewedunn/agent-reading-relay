package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/andrewedunn/agent-reading-relay/internal/credentialsetup"
	"github.com/andrewedunn/agent-reading-relay/internal/instapaper"
	"github.com/andrewedunn/agent-reading-relay/internal/relay"
	"github.com/andrewedunn/agent-reading-relay/internal/relayclient"
)

const (
	defaultSocketPath    = "/run/reading-relay/relay.sock"
	defaultCredentialDir = "/etc/reading-relay/credentials"
	xAuthEndpoint        = "https://www.instapaper.com/api/1/oauth/access_token"
	maxMarkdownBytes     = 4 << 20
)

type Publisher interface {
	Publish(context.Context, relay.PublishRequest) (relay.PublishResponse, error)
}

type Dependencies struct {
	NewPublisher     func(socketPath string) Publisher
	Prompt           func(label string, secret bool) (string, error)
	Exchange         func(context.Context, instapaper.Credentials, string, string) (instapaper.Credentials, error)
	WriteCredentials func(string, instapaper.Credentials) error
}

func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, dependencies Dependencies) error {
	dependencies = dependencies.withDefaults()
	if len(args) == 0 {
		return errors.New("usage: readerctl <publish|save-url|configure-instapaper> [options]")
	}
	switch args[0] {
	case "publish":
		return runPublish(ctx, args[1:], stdin, stdout, stderr, dependencies)
	case "save-url":
		return runSaveURL(ctx, args[1:], stdout, stderr, dependencies)
	case "configure-instapaper":
		return runConfigure(ctx, args[1:], stdout, stderr, dependencies)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runPublish(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, dependencies Dependencies) error {
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	flags.SetOutput(stderr)
	title := flags.String("title", "", "article title")
	file := flags.String("file", "-", "Markdown file, or - for stdin")
	description := flags.String("description", "", "brief plaintext description")
	sourceURL := flags.String("source-url", "", "optional source URL")
	agent := flags.String("agent", "", "calling agent identity")
	send := flags.Bool("send", false, "write to Instapaper (requires explicit user instruction)")
	socket := flags.String("socket", socketPath(), "reading relay Unix socket")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" || strings.TrimSpace(*agent) == "" {
		return errors.New("--title and --agent are required")
	}
	var markdown []byte
	var err error
	if *file == "-" {
		markdown, err = readMarkdown(stdin)
	} else {
		input, openErr := os.Open(*file)
		if openErr != nil {
			return fmt.Errorf("read Markdown: %w", openErr)
		}
		markdown, err = readMarkdown(input)
		closeErr := input.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}
	if err != nil {
		return fmt.Errorf("read Markdown: %w", err)
	}
	response, err := dependencies.NewPublisher(*socket).Publish(ctx, relay.PublishRequest{
		Title: *title, Description: *description, Markdown: string(markdown),
		SourceURL: *sourceURL, Agent: *agent, Send: *send,
	})
	if err != nil {
		return err
	}
	return writeOutput(stdout, response)
}

func runSaveURL(ctx context.Context, args []string, stdout, stderr io.Writer, dependencies Dependencies) error {
	flags := flag.NewFlagSet("save-url", flag.ContinueOnError)
	flags.SetOutput(stderr)
	title := flags.String("title", "", "article title")
	articleURL := flags.String("url", "", "article URL")
	description := flags.String("description", "", "brief plaintext description")
	agent := flags.String("agent", "", "calling agent identity")
	send := flags.Bool("send", false, "write to Instapaper (requires explicit user instruction)")
	socket := flags.String("socket", socketPath(), "reading relay Unix socket")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*title) == "" || strings.TrimSpace(*articleURL) == "" || strings.TrimSpace(*agent) == "" {
		return errors.New("--title, --url, and --agent are required")
	}
	response, err := dependencies.NewPublisher(*socket).Publish(ctx, relay.PublishRequest{
		Title: *title, Description: *description, URL: *articleURL, Agent: *agent, Send: *send,
	})
	if err != nil {
		return err
	}
	return writeOutput(stdout, response)
}

func runConfigure(ctx context.Context, args []string, stdout, stderr io.Writer, dependencies Dependencies) error {
	flags := flag.NewFlagSet("configure-instapaper", flag.ContinueOnError)
	flags.SetOutput(stderr)
	credentialDir := flags.String("credentials-dir", defaultCredentialDir, "root-owned credential directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if dependencies.Prompt == nil {
		return errors.New("interactive prompt is unavailable")
	}
	consumerKey, err := dependencies.Prompt("Instapaper consumer key", true)
	if err != nil {
		return err
	}
	consumerSecret, err := dependencies.Prompt("Instapaper consumer secret", true)
	if err != nil {
		return err
	}
	username, err := dependencies.Prompt("Instapaper username/email", false)
	if err != nil {
		return err
	}
	password, err := dependencies.Prompt("Instapaper password (used once, never stored)", true)
	if err != nil {
		return err
	}
	credentials, err := dependencies.Exchange(ctx, instapaper.Credentials{
		ConsumerKey: strings.TrimSpace(consumerKey), ConsumerSecret: strings.TrimSpace(consumerSecret),
	}, strings.TrimSpace(username), password)
	password = ""
	if err != nil {
		return fmt.Errorf("exchange Instapaper credentials: %w", err)
	}
	if err := dependencies.WriteCredentials(*credentialDir, credentials); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Instapaper OAuth credentials installed in %s\n", *credentialDir)
	return err
}

func readMarkdown(input io.Reader) ([]byte, error) {
	contents, err := io.ReadAll(io.LimitReader(input, maxMarkdownBytes+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxMarkdownBytes {
		return nil, fmt.Errorf("Markdown exceeds 4 MiB limit")
	}
	return contents, nil
}

func writeOutput(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func socketPath() string {
	if value := strings.TrimSpace(os.Getenv("READING_RELAY_SOCKET_PATH")); value != "" {
		return value
	}
	return defaultSocketPath
}

func (d Dependencies) withDefaults() Dependencies {
	if d.NewPublisher == nil {
		d.NewPublisher = func(socketPath string) Publisher { return relayclient.New(socketPath) }
	}
	if d.Exchange == nil {
		d.Exchange = func(ctx context.Context, consumer instapaper.Credentials, username, password string) (instapaper.Credentials, error) {
			return instapaper.ExchangeCredentials(ctx, http.DefaultClient, xAuthEndpoint, instapaper.Signer{}, consumer, username, password)
		}
	}
	if d.WriteCredentials == nil {
		d.WriteCredentials = credentialsetup.WriteCredentialSet
	}
	return d
}

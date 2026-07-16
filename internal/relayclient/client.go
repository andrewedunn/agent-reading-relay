package relayclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/andrewedunn/agent-reading-relay/internal/relay"
)

type Client struct {
	httpClient *http.Client
}

func New(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{httpClient: &http.Client{Transport: transport, Timeout: 90 * time.Second}}
}

func (c *Client) Publish(ctx context.Context, input relay.PublishRequest) (relay.PublishResponse, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return relay.PublishResponse{}, fmt.Errorf("encode relay request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://reading-relay/v1/articles", bytes.NewReader(body))
	if err != nil {
		return relay.PublishResponse{}, fmt.Errorf("create relay request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return relay.PublishResponse{}, fmt.Errorf("call reading relay: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return relay.PublishResponse{}, fmt.Errorf("read relay response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiError struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(responseBody, &apiError) == nil && apiError.Error != "" {
			return relay.PublishResponse{}, fmt.Errorf("relay: %s", apiError.Error)
		}
		return relay.PublishResponse{}, fmt.Errorf("relay returned HTTP %d", resp.StatusCode)
	}
	var output relay.PublishResponse
	if err := json.Unmarshal(responseBody, &output); err != nil {
		return relay.PublishResponse{}, fmt.Errorf("decode relay response: %w", err)
	}
	return output, nil
}

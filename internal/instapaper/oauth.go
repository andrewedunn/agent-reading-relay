package instapaper

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Credentials contains the Instapaper OAuth consumer and access-token
// credentials. AccessToken fields are intentionally optional for xAuth.
type Credentials struct {
	ConsumerKey       string
	ConsumerSecret    string
	AccessToken       string
	AccessTokenSecret string
}

// Signer signs OAuth 1.0a requests using HMAC-SHA1, the only signature method
// accepted by Instapaper.
type Signer struct {
	Nonce func() (string, error)
	Now   func() time.Time
}

func (s Signer) Sign(req *http.Request, credentials Credentials) error {
	if credentials.ConsumerKey == "" || credentials.ConsumerSecret == "" {
		return fmt.Errorf("consumer credentials are required")
	}

	nonceFn := s.Nonce
	if nonceFn == nil {
		nonceFn = randomNonce
	}
	nonce, err := nonceFn()
	if err != nil {
		return fmt.Errorf("generate OAuth nonce: %w", err)
	}
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	oauth := url.Values{
		"oauth_consumer_key":     {credentials.ConsumerKey},
		"oauth_nonce":            {nonce},
		"oauth_signature_method": {"HMAC-SHA1"},
		"oauth_timestamp":        {strconv.FormatInt(nowFn().Unix(), 10)},
		"oauth_version":          {"1.0"},
	}
	if credentials.AccessToken != "" {
		oauth.Set("oauth_token", credentials.AccessToken)
	}

	params, err := requestParameters(req, oauth)
	if err != nil {
		return err
	}
	base := strings.Join([]string{
		strings.ToUpper(req.Method),
		percentEncode(baseURI(req.URL)),
		percentEncode(normalizedParameters(params)),
	}, "&")
	key := percentEncode(credentials.ConsumerSecret) + "&" + percentEncode(credentials.AccessTokenSecret)
	mac := hmac.New(sha1.New, []byte(key))
	_, _ = mac.Write([]byte(base))
	oauth.Set("oauth_signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))

	req.Header.Set("Authorization", authorizationHeader(oauth))
	return nil
}

type parameter struct {
	key   string
	value string
}

func requestParameters(req *http.Request, oauth url.Values) ([]parameter, error) {
	var params []parameter
	add := func(values url.Values) {
		for key, values := range values {
			for _, value := range values {
				params = append(params, parameter{key: key, value: value})
			}
		}
	}
	add(req.URL.Query())
	add(oauth)

	contentType := strings.Split(req.Header.Get("Content-Type"), ";")[0]
	if req.Body != nil && contentType == "application/x-www-form-urlencoded" {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read form body for OAuth signature: %w", err)
		}
		req.Body.Close()
		req.Body = io.NopCloser(strings.NewReader(string(body)))
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, fmt.Errorf("parse form body for OAuth signature: %w", err)
		}
		add(values)
	}
	return params, nil
}

func normalizedParameters(params []parameter) string {
	encoded := make([]parameter, 0, len(params))
	for _, param := range params {
		if param.key == "oauth_signature" {
			continue
		}
		encoded = append(encoded, parameter{key: percentEncode(param.key), value: percentEncode(param.value)})
	}
	sort.Slice(encoded, func(i, j int) bool {
		if encoded[i].key == encoded[j].key {
			return encoded[i].value < encoded[j].value
		}
		return encoded[i].key < encoded[j].key
	})
	parts := make([]string, len(encoded))
	for i, param := range encoded {
		parts[i] = param.key + "=" + param.value
	}
	return strings.Join(parts, "&")
}

func authorizationHeader(values url.Values) string {
	params := make([]parameter, 0, len(values))
	for key, entries := range values {
		if !strings.HasPrefix(key, "oauth_") {
			continue
		}
		for _, value := range entries {
			params = append(params, parameter{key: percentEncode(key), value: percentEncode(value)})
		}
	}
	sort.Slice(params, func(i, j int) bool { return params[i].key < params[j].key })
	parts := make([]string, len(params))
	for i, param := range params {
		parts[i] = fmt.Sprintf(`%s="%s"`, param.key, param.value)
	}
	return "OAuth " + strings.Join(parts, ", ")
}

func baseURI(u *url.URL) string {
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port != "" && !((scheme == "http" && port == "80") || (scheme == "https" && port == "443")) {
		host += ":" + port
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	return scheme + "://" + host + path
}

func percentEncode(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func randomNonce() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

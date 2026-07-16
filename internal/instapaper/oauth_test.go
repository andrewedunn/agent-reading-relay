package instapaper

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSignerMatchesRFC5849Example(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://photos.example.net/photos?file=vacation.jpg&size=original", nil)
	if err != nil {
		t.Fatal(err)
	}

	signer := Signer{
		Nonce: func() (string, error) { return "kllo9940pd9333jh", nil },
		Now:   func() time.Time { return time.Unix(1191242096, 0) },
	}
	err = signer.Sign(req, Credentials{
		ConsumerKey:       "dpf43f3p2l4k3l03",
		ConsumerSecret:    "kd94hf93k423kf44",
		AccessToken:       "nnch734d00sl2jdk",
		AccessTokenSecret: "pfkkdhi9sl3r4s00",
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	header := req.Header.Get("Authorization")
	if !strings.Contains(header, `oauth_signature="tR3%2BTy81lMeYAr%2FFid0kMTYa%2FWM%3D"`) {
		t.Fatalf("unexpected Authorization header: %s", header)
	}
}

func TestSignerCanOmitAccessTokenForXAuth(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://www.instapaper.com/api/1/oauth/access_token", strings.NewReader("x_auth_mode=client_auth&x_auth_username=reader%40example.com&x_auth_password=secret"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	signer := Signer{
		Nonce: func() (string, error) { return "nonce", nil },
		Now:   func() time.Time { return time.Unix(100, 0) },
	}
	if err := signer.Sign(req, Credentials{ConsumerKey: "key", ConsumerSecret: "secret"}); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if strings.Contains(req.Header.Get("Authorization"), "oauth_token") {
		t.Fatalf("xAuth request must omit oauth_token: %s", req.Header.Get("Authorization"))
	}
}

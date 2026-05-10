package scimconnector

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/cerberauth/scimply/protocol"
)

type ClientConfig struct {
	BaseURL    string
	Version    protocol.Version
	HTTPClient *http.Client
	AuthHeader string
	Timeout    time.Duration
}

type Option func(*ClientConfig)

func WithBaseURL(url string) Option {
	return func(c *ClientConfig) {
		c.BaseURL = url
	}
}

func WithBearerToken(token string) Option {
	return func(c *ClientConfig) {
		c.AuthHeader = "Bearer " + token
	}
}

func WithBasicAuth(username, password string) Option {
	return func(c *ClientConfig) {
		encoded := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))
		c.AuthHeader = "Basic " + encoded
	}
}

func WithTimeout(d time.Duration) Option {
	return func(c *ClientConfig) {
		c.Timeout = d
	}
}

func WithVersion(v protocol.Version) Option {
	return func(c *ClientConfig) {
		c.Version = v
	}
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *ClientConfig) {
		c.HTTPClient = hc
	}
}

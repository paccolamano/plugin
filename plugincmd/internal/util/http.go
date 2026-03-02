package util

import (
	"context"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var JSONContent = http.Header{"Content-Type": []string{"application/json"}}

type Response struct {
	*http.Response
}

type HTTPClientOption func(*HTTPClient)

type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
	token      string
	headers    http.Header
}

func NewHTTPClient(opts ...HTTPClientOption) *HTTPClient {
	c := &HTTPClient{
		httpClient: &http.Client{},
		headers:    make(http.Header),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func WithBaseURL(u string) HTTPClientOption {
	return func(c *HTTPClient) {
		c.baseURL = strings.TrimSuffix(u, "/")
	}
}

func WithToken(token string) HTTPClientOption {
	return func(c *HTTPClient) {
		c.token = token
	}
}

func WithTimeout(d time.Duration) HTTPClientOption {
	return func(c *HTTPClient) {
		c.httpClient.Timeout = d
	}
}

func WithHTTPClient(hc *http.Client) HTTPClientOption {
	return func(c *HTTPClient) {
		c.httpClient = hc
	}
}

func WithHeader(key, value string) HTTPClientOption {
	return func(c *HTTPClient) {
		c.headers.Set(key, value)
	}
}

func (c *HTTPClient) DoAbsoluteRequest(ctx context.Context, method, rawURL string, query url.Values, contentLength int64, header http.Header, ibody io.Reader) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), ibody)
	if err != nil {
		return nil, err
	}

	maps.Copy(req.Header, c.headers)
	maps.Copy(req.Header, header)

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	if contentLength >= 0 {
		req.ContentLength = contentLength
	}

	return c.httpClient.Do(req)
}

func (c *HTTPClient) DoRequest(ctx context.Context, method, path string, query url.Values, contentLength int64, header http.Header, ibody io.Reader) (*http.Response, error) {
	return c.DoAbsoluteRequest(ctx, method, c.baseURL+path, query, contentLength, header, ibody)
}

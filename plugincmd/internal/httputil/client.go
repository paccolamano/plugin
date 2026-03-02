package httputil

import (
	"context"
	"encoding/json"
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

type ClientOption func(*Client)

type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
	headers    http.Header
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		httpClient: &http.Client{},
		headers:    make(http.Header),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func WithBaseURL(u string) ClientOption {
	return func(c *Client) {
		c.baseURL = strings.TrimSuffix(u, "/")
	}
}

func WithToken(token string) ClientOption {
	return func(c *Client) {
		c.token = token
	}
}

func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

func WithHTTPClient(hc *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = hc
	}
}

func WithHeader(key, value string) ClientOption {
	return func(c *Client) {
		c.headers.Set(key, value)
	}
}

func (c *Client) DoRequest(ctx context.Context, method, path string, query url.Values, contentLength int64, header http.Header, ibody io.Reader) (*http.Response, error) {
	return c.DoAbsolute(ctx, method, c.baseURL+path, query, contentLength, header, ibody)
}

func (c *Client) DoAbsolute(ctx context.Context, method, rawURL string, query url.Values, contentLength int64, header http.Header, ibody io.Reader) (*http.Response, error) {
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

func (c *Client) GetResponse(ctx context.Context, method, path string, query url.Values, contentLength int64, header http.Header, ibody io.Reader) (*Response, error) {
	cresp, err := c.DoRequest(ctx, method, path, query, contentLength, header, ibody)
	if err != nil {
		return nil, err
	}
	return &Response{Response: cresp}, nil
}

func (c *Client) GetParsedResponse(ctx context.Context, method, path string, query url.Values, header http.Header, ibody io.Reader, obj any) (*Response, error) {
	resp, err := c.GetResponse(ctx, method, path, query, -1, header, ibody)
	if err != nil {
		return resp, err
	}
	defer resp.Body.Close()

	return resp, json.NewDecoder(resp.Body).Decode(obj)
}

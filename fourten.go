package fourten

import (
	"context"
	"net/http"
	"net/url"
)

type Client struct {
	Request *http.Request

	httpClient *http.Client
}

type Option func(*Client)

func New(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{},
		Request: &http.Request{
			URL:    &url.URL{},
			Header: make(http.Header),
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Refine(opts ...Option) *Client {
	new := &Client{
		httpClient: c.httpClient,
		Request: &http.Request{
			URL:    c.Request.URL.ResolveReference(&url.URL{}),
			Header: c.Request.Header.Clone(),
		},
	}
	for _, opt := range opts {
		opt(new)
	}
	return new
}

func BaseURL(base string) Option {
	u, err := url.Parse(base)
	if err != nil {
		panic(err)
	}
	return func(c *Client) {
		c.Request.URL = u
	}
}

func SetHeader(header, value string) Option {
	return func(c *Client) {
		c.Request.Header.Set(header, value)
	}
}
func Bearer(token string) Option {
	return SetHeader("Authorization", "Bearer "+token)
}

func (c *Client) GET(ctx context.Context, target string) (*http.Response, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	req := &http.Request{
		Method: "GET",
		URL:    c.Request.URL.ResolveReference(targetURL),
		Header: c.Request.Header.Clone(),
	}
	req = req.WithContext(ctx)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

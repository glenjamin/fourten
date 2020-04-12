package fourten

import (
	"context"
	"net/http"
	"net/url"
)

type Client struct {
	httpClient *http.Client
	baseURL    url.URL
}

type Option func(*Client)

func New(opts ...Option) Client {
	c := &Client{
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return *c
}

func BaseURL(base string) Option {
	u, err := url.Parse(base)
	if err != nil {
		panic(err)
	}
	return func(c *Client) {
		c.baseURL = *u
	}
}

func (c *Client) GET(ctx context.Context, path string) (*http.Response, error) {
	target := c.baseURL.ResolveReference(&url.URL{Path: path}).String()

	// TODO: we can probably eliminate this error condition by making the request ourselves
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	return res, nil
}

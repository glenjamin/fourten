package fourten

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	Request *http.Request

	decoder    Decoder
	httpClient *http.Client
}

type Option func(*Client)

// Decoder is used to populate target from the reader
type Decoder func(contentType string, r io.Reader, target interface{}) error

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

func DecodeJSON(c *Client) {
	SetHeader("Accept", "application/json")(c)
	c.decoder = jsonDecoder
}
func jsonDecoder(contentType string, r io.Reader, target interface{}) error {
	if !strings.HasPrefix(contentType, "application/json") {
		return errors.New("expected JSON content-type, got " + contentType)
	}
	if err := json.NewDecoder(r).Decode(target); err != nil {
		return fmt.Errorf("failed to decode: %w", err)
	}
	return nil
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

func (c *Client) GETDecoded(ctx context.Context, target string, output interface{}) (*http.Response, error) {
	if c.decoder == nil {
		return nil, errors.New("output requested but no decoder configured")
	}

	res, err := c.GET(ctx, target)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	err = c.decoder(res.Header.Get("content-type"), res.Body, output)
	if err != nil {
		return nil, err
	}
	return res, nil
}

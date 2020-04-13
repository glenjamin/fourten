package fourten

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	Request *http.Request

	encoder Encoder
	decoder Decoder

	httpClient *http.Client
}

type Option func(*Client)

// Encoder is used to populate requests from input, the return value is compatible with http.Request.GetBody
type Encoder func(input interface{}) func() (io.ReadCloser, error)

// Decoder is used to populate target from the reader
type Decoder func(contentType string, r io.Reader, target interface{}) error

func New(opts ...Option) *Client {
	c := &Client{
		Request: &http.Request{
			URL:    &url.URL{},
			Header: make(http.Header),
		},
		httpClient: &http.Client{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Refine(opts ...Option) *Client {
	new := &Client{
		Request: &http.Request{
			URL:    c.Request.URL.ResolveReference(&url.URL{}),
			Header: c.Request.Header.Clone(),
		},

		encoder:    c.encoder,
		decoder:    c.decoder,
		httpClient: c.httpClient,
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

func EncodeJSON(c *Client) {
	SetHeader("Content-Type", "application/json; charset=utf-8")(c)
	c.encoder = jsonEncoder
}
func jsonEncoder(input interface{}) func() (io.ReadCloser, error) {
	// A little sleight of hand to ensure we only encode once, regardless of how many readers are needed
	b := &bytes.Buffer{}
	err := json.NewEncoder(b).Encode(input)
	return func() (io.ReadCloser, error) {
		if err != nil {
			return nil, fmt.Errorf("failed to encode: %w", err)
		}
		return ioutil.NopCloser(bytes.NewReader(b.Bytes())), nil
	}
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

// GET makes an HTTP request to the supplied target.
// It is the responsibility of the caller to close the response body
func (c *Client) GET(ctx context.Context, target string) (*http.Response, error) {
	req, err := c.buildRequest(ctx, "GET", target)
	if err != nil {
		return nil, err
	}

	return c.httpClient.Do(req)
}

func (c *Client) POST(ctx context.Context, target string, input interface{}) (*http.Response, error) {
	if c.encoder == nil {
		return nil, errors.New("input requested but no encoder configured")
	}

	req, err := c.buildRequest(ctx, "POST", target)
	if err != nil {
		return nil, err
	}

	req.GetBody = c.encoder(input)
	req.Body, err = req.GetBody()
	if err != nil {
		return nil, err
	}

	return c.httpClient.Do(req)
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

func (c *Client) POSTDecoded(ctx context.Context, target string, input, output interface{}) (*http.Response, error) {
	if c.decoder == nil {
		return nil, errors.New("output requested but no decoder configured")
	}

	res, err := c.POST(ctx, target, input)
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

func (c *Client) buildRequest(ctx context.Context, method, target string) (*http.Request, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	req := &http.Request{
		Method: method,
		URL:    c.Request.URL.ResolveReference(targetURL),
		Header: c.Request.Header.Clone(),
	}

	return req.WithContext(ctx), nil
}

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
	"time"
)

type Client struct {
	Request *http.Request

	timeout time.Duration
	encoder Encoder
	decoder Decoder

	httpClient *http.Client
}

type Option func(*Client)

// Encoder is used to populate requests from input, the return value is compatible with http.Request.GetBody
type Encoder func(input interface{}) (length int64, getBody func() (io.ReadCloser, error))

// Decoder is used to populate target from the reader
type Decoder func(contentType string, r io.Reader, target interface{}) error

func New(opts ...Option) *Client {
	c := &Client{
		Request: &http.Request{
			URL:    &url.URL{},
			Header: make(http.Header),
		},

		timeout:    time.Second,
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

		timeout:    c.timeout,
		encoder:    c.encoder,
		decoder:    c.decoder,
		httpClient: c.httpClient,
	}
	for _, opt := range opts {
		opt(new)
	}
	return new
}

func RequestTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.timeout = d
	}
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
func jsonEncoder(input interface{}) (int64, func() (io.ReadCloser, error)) {
	// A little sleight of hand to ensure we only encode once, regardless of how many readers are needed
	b := &bytes.Buffer{}
	err := json.NewEncoder(b).Encode(input)
	return int64(b.Len()), func() (io.ReadCloser, error) {
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
	return c.SendDecoded(ctx, "GET", target, nil, nil)
}
func (c *Client) HEAD(ctx context.Context, target string) (*http.Response, error) {
	return c.SendDecoded(ctx, "HEAD", target, nil, nil)
}
func (c *Client) OPTIONS(ctx context.Context, target string) (*http.Response, error) {
	return c.SendDecoded(ctx, "OPTIONS", target, nil, nil)
}
func (c *Client) POST(ctx context.Context, target string, input interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, "POST", target, input, nil)
}
func (c *Client) PUT(ctx context.Context, target string, input interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, "PUT", target, input, nil)
}
func (c *Client) PATCH(ctx context.Context, target string, input interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, "PATCH", target, input, nil)
}
func (c *Client) DELETE(ctx context.Context, target string, input interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, "DELETE", target, input, nil)
}
func (c *Client) GETDecoded(ctx context.Context, target string, output interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, "GET", target, nil, output)
}
func (c *Client) OPTIONSDecoded(ctx context.Context, target string, output interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, "OPTIONS", target, nil, output)
}
func (c *Client) POSTDecoded(ctx context.Context, target string, input, output interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, "POST", target, input, output)
}
func (c *Client) PUTDecoded(ctx context.Context, target string, input, output interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, "PUT", target, input, output)
}
func (c *Client) PATCHDecoded(ctx context.Context, target string, input, output interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, "PATCH", target, input, output)
}
func (c *Client) DELETEDecoded(ctx context.Context, target string, input, output interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, "DELETE", target, input, output)
}

func (c *Client) Send(ctx context.Context, method, target string, input interface{}) (*http.Response, error) {
	return c.SendDecoded(ctx, method, target, input, nil)
}
func (c *Client) SendDecoded(ctx context.Context, method, target string, input, output interface{}) (*http.Response, error) {
	req, err := c.buildRequest(method, target)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req = req.WithContext(ctx)

	err = c.setupEncoding(req, input)
	if err != nil {
		return nil, err
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	httpErr := coerceHTTPError(res, err)

	// non-nil output means we try output decoding
	if output != nil {
		// when we handle output, we close body - otherwise it's up to the caller
		defer res.Body.Close()
		if c.decoder == nil {
			return nil, errors.New("output requested but no decoder configured")
		}
		err = c.decoder(res.Header.Get("content-type"), res.Body, output)
		if err != nil {
			return nil, err
		}
	}

	return res, httpErr
}

func (c *Client) buildRequest(method, target string) (*http.Request, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	req := &http.Request{
		Method: method,
		URL:    c.Request.URL.ResolveReference(targetURL),
		Header: c.Request.Header.Clone(),
	}

	return req, nil
}

func (c *Client) setupEncoding(req *http.Request, input interface{}) error {
	// non-nil input means we try input encoding
	if input != nil {
		if c.encoder == nil {
			return errors.New("input requested but no encoder configured")
		}
		req.ContentLength, req.GetBody = c.encoder(input)
	} else {
		req.ContentLength = 0
		req.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
	}
	var err error
	req.Body, err = req.GetBody()
	return err
}

func coerceHTTPError(res *http.Response, err error) error {
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		return &HTTPError{res}
	}
	return nil
}

var ErrHTTP = fmt.Errorf("base HTTP error")

type HTTPError struct {
	Response *http.Response
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP Status %d", e.Response.StatusCode)
}

// Is allows HTTPError to match errors.Is(fourten.ErrHTTP), potentially saving you a type cast
func (e *HTTPError) Is(err error) bool {
	return err == ErrHTTP
}

package fourten

import (
	"bytes"
	"compress/gzip"
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

// Client represents a usable HTTP client, it should be initialised with New()
type Client struct {
	Request *http.Request

	timeout time.Duration
	encoder Encoder
	decoder Decoder

	httpClient *http.Client
}

// Option is used to apply changes to a Client in a neat manner
type Option func(*Client)

// Encoder is used to populate requests from input
type Encoder func(input interface{}) (RequestEncoding, error)

// RequestEncoding is used to populate the outgoing http.Request
type RequestEncoding struct {
	// GetBody will be used to populate Request.Body and Request.GetBody
	GetBody func() (io.ReadCloser, error)
	// ContentLength should be set, or set to -1 if unknown
	ContentLength int64
	// Header can be used to overwrite any headers already in the Request
	Header http.Header
}

// Decoder is used to populate target from the reader
type Decoder func(contentType string, r io.Reader, target interface{}) error

const defaultUserAgent = "fourten (Go HTTP Client)"

// New constructs a Client, applying the specified options
func New(opts ...Option) *Client {
	c := &Client{
		Request: &http.Request{
			URL:    &url.URL{},
			Header: make(http.Header),
		},

		timeout:    time.Second,
		httpClient: &http.Client{},
	}
	c.Request.Header.Set("User-Agent", defaultUserAgent)
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Derive copies the current client, applying additional options as specified
func (c *Client) Derive(opts ...Option) *Client {
	httpClient := *c.httpClient

	derived := &Client{
		Request: &http.Request{
			URL:    c.Request.URL.ResolveReference(&url.URL{}),
			Header: c.Request.Header.Clone(),
		},

		timeout:    c.timeout,
		encoder:    c.encoder,
		decoder:    c.decoder,
		httpClient: &httpClient,
	}
	for _, opt := range opts {
		opt(derived)
	}
	return derived
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

type URLModifier func(u *url.URL) error

func Query(values url.Values) URLModifier {
	return func(u *url.URL) error {
		if u.RawQuery != "" {
			return errors.New("refusing to overwrite querystring with Query Values")
		}
		u.RawQuery = values.Encode()
		return nil
	}
}

func QueryMap(m map[string]string) URLModifier {
	return func(u *url.URL) error {
		if u.RawQuery != "" {
			return errors.New("refusing to overwrite querystring with QueryMap")
		}
		values := url.Values{}
		for k, v := range m {
			values.Set(k, v)
		}
		u.RawQuery = values.Encode()
		return nil
	}
}

func Param(k, v string) URLModifier {
	return func(u *url.URL) error {
		before := u.Path
		u.Path = strings.ReplaceAll(u.Path, ":"+k, url.PathEscape(v))
		if before == u.Path {
			return fmt.Errorf("failed to find parameter %v", k)
		}
		return nil
	}
}
func IntParam(k string, v int) URLModifier {
	return func(u *url.URL) error {
		before := u.Path
		u.Path = strings.ReplaceAll(u.Path, ":"+k, url.PathEscape(fmt.Sprintf("%d", v)))
		if before == u.Path {
			return fmt.Errorf("failed to find parameter %v", k)
		}
		return nil
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

func DontFollowRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}
func NoFollow(c *Client) {
	c.httpClient.CheckRedirect = DontFollowRedirect
}

func EncodeJSON(c *Client) {
	c.encoder = jsonEncoder
}
func jsonEncoder(input interface{}) (RequestEncoding, error) {
	// A little sleight of hand to ensure we only encode once, regardless of how many readers are needed
	b := &bytes.Buffer{}
	err := json.NewEncoder(b).Encode(input)
	if err != nil {
		return RequestEncoding{}, err
	}
	header := http.Header{}
	header.Set("Content-Type", "application/json; charset=utf-8")
	return RequestEncoding{
		ContentLength: int64(b.Len()),
		GetBody: func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader(b.Bytes())), nil
		},
		Header: header,
	}, nil
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

func DontDecode(c *Client) {
	c.Request.Header.Del("Accept")
	c.decoder = nil
}

func GzipRequests(c *Client) {
	encoder := c.encoder
	c.encoder = func(input interface{}) (RequestEncoding, error) {
		enc, err := encoder(input)
		if err != nil {
			return RequestEncoding{}, err
		}
		// No point gzipping really small bodies
		if enc.ContentLength < 1024 {
			return enc, nil
		}
		r, err := enc.GetBody()
		if err != nil {
			return RequestEncoding{}, err
		}
		var buf bytes.Buffer
		gzw := gzip.NewWriter(&buf)
		if _, err = io.Copy(gzw, r); err != nil {
			return RequestEncoding{}, err
		}
		if err = gzw.Close(); err != nil {
			return RequestEncoding{}, err
		}

		enc.GetBody = func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewReader(buf.Bytes())), nil
		}
		enc.ContentLength = int64(buf.Len())
		enc.Header.Set("Content-Encoding", "gzip")
		return enc, nil
	}
}

// GET makes an HTTP request to the supplied target.
// It is the responsibility of the caller to close the response body if output is nil
func (c *Client) GET(ctx context.Context, target string, output interface{}, ums ...URLModifier) (*http.Response, error) {
	return c.Call(ctx, "GET", target, nil, output, ums...)
}
func (c *Client) HEAD(ctx context.Context, target string, ums ...URLModifier) (*http.Response, error) {
	return c.Call(ctx, "HEAD", target, nil, nil, ums...)
}
func (c *Client) OPTIONS(ctx context.Context, target string, output interface{}, ums ...URLModifier) (*http.Response, error) {
	return c.Call(ctx, "OPTIONS", target, nil, output, ums...)
}
func (c *Client) POST(ctx context.Context, target string, input, output interface{}, ums ...URLModifier) (*http.Response, error) {
	return c.Call(ctx, "POST", target, input, output, ums...)
}
func (c *Client) PUT(ctx context.Context, target string, input, output interface{}, ums ...URLModifier) (*http.Response, error) {
	return c.Call(ctx, "PUT", target, input, output, ums...)
}
func (c *Client) PATCH(ctx context.Context, target string, input, output interface{}, ums ...URLModifier) (*http.Response, error) {
	return c.Call(ctx, "PATCH", target, input, output, ums...)
}
func (c *Client) DELETE(ctx context.Context, target string, input, output interface{}, ums ...URLModifier) (*http.Response, error) {
	return c.Call(ctx, "DELETE", target, input, output, ums...)
}

func (c *Client) Call(ctx context.Context, method, target string, input, output interface{}, ums ...URLModifier) (*http.Response, error) {
	if output != nil && c.decoder == nil {
		return nil, errors.New("output requested but no decoder configured")
	}

	req, err := c.buildRequest(method, target, ums)
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

	httpErr := coerceHTTPError(res)

	// non-nil decoder means we are responsible for output decoding
	if c.decoder != nil {
		// when we handle output, we close body - otherwise it's up to the caller
		defer res.Body.Close()

		// if we have an http error don't decode to output, it's unlikely to match
		// instead, we'll read from res to free the connection up, but store the data for later use
		if httpErr != nil {
			if err := httpErr.populateBody(c.decoder); err != nil {
				return nil, fmt.Errorf("failed to read error body: %w", err)
			}
		} else {
			if err := handleDecoding(res, c.decoder, output); err != nil {
				return nil, err
			}
		}
	}

	if httpErr != nil {
		return res, httpErr
	}
	return res, nil
}

func (c *Client) buildRequest(method, target string, ums []URLModifier) (*http.Request, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	req := &http.Request{
		Method: method,
		URL:    c.Request.URL.ResolveReference(targetURL),
		Header: c.Request.Header.Clone(),
	}

	for _, um := range ums {
		if err := um(req.URL); err != nil {
			return nil, err
		}
	}

	return req, nil
}

func (c *Client) setupEncoding(req *http.Request, input interface{}) error {
	// non-nil input means we try input encoding
	if input != nil {
		if c.encoder == nil {
			return errors.New("input requested but no encoder configured")
		}
		encoding, err := c.encoder(input)
		if err != nil {
			return fmt.Errorf("failed to encode %v: %w", input, err)
		}
		req.ContentLength = encoding.ContentLength
		req.GetBody = encoding.GetBody
		copyHeaders(req.Header, encoding.Header)
		if req.Body, err = encoding.GetBody(); err != nil {
			return fmt.Errorf("failed to read body from encoding of %v: %w", input, err)
		}
	} else {
		req.ContentLength = 0
		req.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
	}
	var err error
	req.Body, err = req.GetBody()
	return err
}

func copyHeaders(base http.Header, merge http.Header) {
	for header, values := range merge {
		base[header] = values
	}
}

func handleDecoding(res *http.Response, decoder Decoder, output interface{}) error {
	switch {
	// expected response but didn't get one
	case res.Body == http.NoBody && output != nil:
		return errors.New("unexpected empty response")
	// didn't expect a response and didn't get one
	case res.Body == http.NoBody && output == nil:
		return nil
	// got a response but don't care
	case output == nil:
		_, err := io.Copy(ioutil.Discard, res.Body)
		return err
	}

	// Hand off to the decoder if we got this far
	return decoder(res.Header.Get("content-type"), res.Body, output)
}

func coerceHTTPError(res *http.Response) *HTTPError {
	if res.StatusCode >= 300 {
		return &HTTPError{Response: res}
	}
	return nil
}

func AsHTTPError(err error) *HTTPError {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		return httpErr
	}
	return nil
}

var ErrHTTP = fmt.Errorf("base HTTP error")

type HTTPError struct {
	Response *http.Response

	body    *bytes.Buffer
	decoder Decoder
}

func (e *HTTPError) populateBody(decoder Decoder) error {
	e.decoder = decoder
	b := make([]byte, 0, e.Response.ContentLength)
	e.body = bytes.NewBuffer(b)
	_, err := io.Copy(e.body, e.Response.Body)
	return err
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP Status %d", e.Response.StatusCode)
}

// Is allows HTTPError to match errors.Is(fourten.ErrHTTP), potentially saving you a type cast
func (e *HTTPError) Is(err error) bool {
	return err == ErrHTTP
}

// Decode will use the configured decoder to populate output from the response body
func (e *HTTPError) Decode(output interface{}) error {
	resp := *e.Response
	resp.Body = ioutil.NopCloser(bytes.NewReader(e.body.Bytes()))
	return handleDecoding(&resp, e.decoder, output)
}

func (e *HTTPError) Body() string {
	return e.body.String()
}

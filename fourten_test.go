package fourten_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/NYTimes/gziphandler"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
	"gotest.tools/v3/assert/opt"

	"github.com/glenjamin/fourten"
)

var ctx context.Context
var server *RecordingServer

func init() {
	ctx = context.Background()
	server = NewServer(StubResponse{
		Status: 200,
		Body:   "PONG",
	})
}

var contentTypeJSON = Headers{"content-type": []string{"application/json; charset=utf-8"}}

func TestURLResolution(t *testing.T) {
	t.Run("Can request absolute URLs", func(t *testing.T) {
		client := fourten.New()
		res, err := client.GET(ctx, server.URL+"/ping", nil)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Method, "GET"))
		assert.Check(t, cmp.Equal(server.Request.URL.Path, "/ping"))
		assertResponse(t, res, server.Response)
	})

	t.Run("Can request URLs relative to a base URL", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL))
		res, err := client.GET(ctx, "/ping", nil)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Method, "GET"))
		assert.Check(t, cmp.Equal(server.Request.URL.Path, "/ping"))
		assertResponse(t, res, server.Response)
	})

	t.Run("errors on invalid URL", func(t *testing.T) {
		_, err := fourten.New().GET(ctx, ":::", nil)
		assert.ErrorContains(t, err, "parse")
	})

	t.Run("panics on invalid base URL", func(t *testing.T) {
		// panic because fitting an error into the Option signature would be a pain
		assert.Assert(t, cmp.Panics(func() {
			fourten.New(fourten.BaseURL(":/:/:"))
		}))
	})
}

func TestParameters(t *testing.T) {
	client := fourten.New(fourten.BaseURL(server.URL))
	t.Run("Can add querystring params via url.Values", func(t *testing.T) {
		v := url.Values{"foo": {"bar"}, "wibble": {"wobble", "wubble"}}
		_, err := client.GET(ctx, "/ping", nil, fourten.Query(v))
		assert.NilError(t, err)

		assert.DeepEqual(t, server.Request.URL.Query(), v)
	})
	t.Run("Can add querystring params via a map[string]string", func(t *testing.T) {
		_, err := client.GET(ctx, "/ping", nil,
			fourten.QueryMap(map[string]string{"foo": "bar", "wibble": "wobble"}))
		assert.NilError(t, err)

		assert.DeepEqual(t, server.Request.URL.Query(), url.Values{"foo": {"bar"}, "wibble": {"wobble"}})
	})
	t.Run("Adding a query when the querystring is non-empty is an error", func(t *testing.T) {
		_, err := client.GET(ctx, "/ping?a=123", nil, fourten.QueryMap(map[string]string{"foo": "bar"}))
		assert.ErrorContains(t, err, "querystring")

		_, err = client.GET(ctx, "/ping?a=123", nil, fourten.Query(url.Values{"foo": {"bar"}}))
		assert.ErrorContains(t, err, "querystring")
	})
	t.Run("Can template URLs with parameters", func(t *testing.T) {
		_, err := client.GET(ctx, "/user/:user-id/profile", nil, fourten.Param("user-id", "glenjamin"))
		assert.NilError(t, err)

		assert.Equal(t, server.Request.URL.Path, "/user/glenjamin/profile")
	})
	t.Run("Can template URLs with multiple parameters", func(t *testing.T) {
		_, err := client.GET(ctx, "/blog/:slug/comment/:comment-id", nil,
			fourten.Param("slug", "cycling-is-great"), fourten.Param("comment-id", "sdklfhewkf"))
		assert.NilError(t, err)

		assert.Equal(t, server.Request.URL.Path, "/blog/cycling-is-great/comment/sdklfhewkf")
	})
	t.Run("Can template URLs with numeric parameters", func(t *testing.T) {
		_, err := client.GET(ctx, "/user/:user-id", nil, fourten.IntParam("user-id", 6723))
		assert.NilError(t, err)

		assert.Equal(t, server.Request.URL.Path, "/user/6723")
	})
	t.Run("Can template URLs with auto-escaping of parameters", func(t *testing.T) {
		_, err := client.GET(ctx, "/user/:username", nil, fourten.Param("username", "a!/b c"))
		assert.NilError(t, err)

		assert.Equal(t, server.Request.URL.Path, "/user/a%21%2Fb%20c")
	})
	t.Run("Can template URLs and pass querystring at the same time", func(t *testing.T) {
		_, err := client.GET(ctx, "/user/:user-id/profile", nil,
			fourten.Param("user-id", "glenjamin"),
			fourten.QueryMap(map[string]string{"foo": "bar", "wibble": "wobble"}))
		assert.NilError(t, err)

		assert.Equal(t, server.Request.URL.Path, "/user/glenjamin/profile")
		assert.DeepEqual(t, server.Request.URL.Query(), url.Values{"foo": {"bar"}, "wibble": {"wobble"}})
	})
	t.Run("Attempting to use a parameter which doesn't exist is an error", func(t *testing.T) {
		_, err := client.GET(ctx, "/user/blah", nil, fourten.Param("user-id", "glenjamin"))
		assert.ErrorContains(t, err, "parameter")
		assert.ErrorContains(t, err, "user-id")

		_, err = client.GET(ctx, "/user/blah", nil, fourten.IntParam("count", 123))
		assert.ErrorContains(t, err, "parameter")
		assert.ErrorContains(t, err, "count")
	})
}

func TestHeaders(t *testing.T) {
	t.Run("Default user agent", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL))

		_, err := client.GET(ctx, "/ping", nil)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("User-Agent"), "fourten (Go HTTP Client)"))
	})

	t.Run("Can set headers", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.SetHeader("Wibble", "Wobble"))

		_, err := client.GET(ctx, "/ping", nil)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Wibble"), "Wobble"))
	})

	t.Run("Can set bearer tokens", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.Bearer("ipromisetopaythebearer"))

		_, err := client.GET(ctx, "/ping", nil)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Authorization"), "Bearer ipromisetopaythebearer"))
	})
}

func TestDecoding(t *testing.T) {
	t.Run("Refuses to decode unless configured to", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL))

		body := make(map[string]interface{})
		_, err := client.GET(ctx, "/data", &body)
		assert.ErrorContains(t, err, "no decoder")
	})

	t.Run("Requests and Decodes JSON into provided map", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Headers = contentTypeJSON
		server.Response.Body = `{"json": "made easy"}`

		body := make(map[string]interface{})
		_, err := client.GET(ctx, "/data", &body)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Accept"), "application/json"))
		assert.Check(t, cmp.DeepEqual(body, map[string]interface{}{"json": "made easy"}))
	})

	t.Run("Requests and Decodes JSON into provided struct", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Headers = contentTypeJSON
		server.Response.Body = `{"json": "made easy"}`

		type resp struct {
			Json string
		}
		body := resp{}
		_, err := client.GET(ctx, "/data", &body)
		assert.NilError(t, err)

		assert.Check(t, cmp.DeepEqual(body, resp{"made easy"}))
	})

	t.Run("decoder + nil output param means discard body", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Headers = contentTypeJSON
		server.Response.Body = `{"json": "made easy"}`

		res, err := client.GET(ctx, "/data", nil)
		assert.NilError(t, err)

		b := make([]byte, 10)
		n, err := res.Body.Read(b)
		assert.Check(t, err != nil)
		assert.Check(t, cmp.Equal(n, 0))
	})

	t.Run("empty response body with an out param is an error", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Body = ``

		var out interface{}
		res, err := client.GET(ctx, "/data", &out)
		assert.ErrorContains(t, err, "empty response")
		assert.Assert(t, res == nil)
	})

	t.Run("empty response body with a nil out is fine", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON, fourten.RequestTimeout(time.Minute))

		server.Response.Body = ``

		res, err := client.GET(ctx, "/data", nil)
		assert.NilError(t, err)

		// And the body is still closed
		b := make([]byte, 10)
		n, err := res.Body.Read(b)
		assert.Check(t, err != nil)
		assert.Check(t, cmp.Equal(n, 0))
	})

	t.Run("pass an unbound map pointer to ignore without disabling decoding", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Headers = contentTypeJSON
		server.Response.Body = `{"json": "made easy"}`

		res, err := client.GET(ctx, "/data", &map[string]interface{}{})
		assert.NilError(t, err)

		b := make([]byte, 10)
		n, err := res.Body.Read(b)
		assert.Check(t, err != nil)
		assert.Check(t, cmp.Equal(n, 0))
	})

	t.Run("decoding can be opted out of", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Body = `abcdef`

		res, err := client.Derive(fourten.DontDecode).GET(ctx, "/data", nil)
		assert.NilError(t, err)

		body, err := ioutil.ReadAll(res.Body)
		assert.NilError(t, err)
		assert.Equal(t, string(body), "abcdef")
	})

	t.Run("Handles failed struct decodes correctly", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Headers = contentTypeJSON
		server.Response.Body = `{"json": 123, "and": "more"}`

		type resp struct {
			Json string
		}
		body := resp{}
		_, err := client.GET(ctx, "/data", &body)
		assert.Assert(t, cmp.ErrorContains(err, "json: cannot unmarshal"))
	})

	t.Run("Won't decode JSON without a content type", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Body = `{"json": {"made": "easy"}}`

		body := make(map[string]interface{})
		_, err := client.GET(ctx, "/data", &body)
		assert.ErrorContains(t, err, "expected JSON content-type")
	})

	t.Run("Handles attempts to decode invalid data cleanly", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Headers = contentTypeJSON
		server.Response.Body = `{"json": {"made"`

		body := make(map[string]interface{})
		_, err := client.GET(ctx, "/data", &body)
		assert.ErrorContains(t, err, "unexpected EOF")
	})
}

func TestEncoding(t *testing.T) {
	t.Run("Refuses to encode unless configured to", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL))

		input := map[string]interface{}{}
		_, err := client.POST(ctx, "/data", &input, nil)
		assert.ErrorContains(t, err, "no encoder")
	})

	t.Run("Can POST nil even if not configured", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL))

		_, err := client.POST(ctx, "/data", nil, nil)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Content-Length"), "0"))

		requestBody, err := ioutil.ReadAll(server.Request.Body)
		assert.NilError(t, err)
		assert.DeepEqual(t, requestBody, []byte{})
	})

	t.Run("Can POST nil when configured", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL), fourten.EncodeJSON)

		_, err := client.POST(ctx, "/data", nil, nil)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Content-Type"), ""))
		assert.Check(t, cmp.Equal(server.Request.Header.Get("Content-Length"), "0"))

		requestBody, err := ioutil.ReadAll(server.Request.Body)
		assert.NilError(t, err)
		assert.DeepEqual(t, requestBody, []byte{})
	})

	t.Run("Can POST encoded JSON", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.EncodeJSON)

		input := map[string]interface{}{
			"request": "params",
			"of_json": true,
		}
		_, err := client.POST(ctx, "/data", &input, nil)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Content-Type"), "application/json; charset=utf-8"))
		assert.Check(t, cmp.Equal(server.Request.Header.Get("Content-Length"), "36"))

		requestBody, err := ioutil.ReadAll(server.Request.Body)
		assert.NilError(t, err)
		requestData := make(map[string]interface{})
		assert.NilError(t, json.Unmarshal(requestBody, &requestData))
		assert.DeepEqual(t, requestData, input)
	})

	t.Run("Handles attempts to encode invalid data cleanly", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.EncodeJSON)

		input := math.Inf(1)
		_, err := client.POST(ctx, "/data", &input, nil)
		assert.ErrorContains(t, err, "unsupported value")
	})
}

func TestEncodingAndDecoding(t *testing.T) {
	t.Run("Can POST encoded JSON and decode the response", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.EncodeJSON, fourten.DecodeJSON)

		server.Response.Headers = contentTypeJSON
		server.Response.Body = `{"easy_as": 123}`

		input := map[string]interface{}{
			"request": "params",
			"of_json": true,
		}
		output := make(map[string]interface{})
		_, err := client.POST(ctx, "/data", &input, &output)
		assert.NilError(t, err)

		requestBody, err := ioutil.ReadAll(server.Request.Body)
		assert.NilError(t, err)
		requestData := make(map[string]interface{})
		assert.NilError(t, json.Unmarshal(requestBody, &requestData))
		assert.Check(t, cmp.DeepEqual(requestData, input))

		assert.Check(t, cmp.DeepEqual(output, map[string]interface{}{"easy_as": 123.0}))
	})
}

func TestMethods(t *testing.T) {
	client := fourten.New(fourten.BaseURL(server.URL), fourten.EncodeJSON, fourten.DecodeJSON)

	t.Run("HEAD", func(t *testing.T) {
		res, err := client.HEAD(ctx, "/method/test")
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Method, "HEAD"))
		assert.Check(t, cmp.Equal(server.Request.URL.Path, "/method/test"))
		assert.Check(t, cmp.Equal(res.StatusCode, 200))
	})

	t.Run("without bodies", func(t *testing.T) {
		tests := []struct {
			method string
			fn     func(context.Context, string, interface{}, ...fourten.URLModifier) (*http.Response, error)
		}{
			{"GET", client.GET},
			{"OPTIONS", client.OPTIONS},
		}
		for _, test := range tests {
			t.Run(test.method, func(t *testing.T) {
				res, err := test.fn(ctx, "/method/test", nil)
				assert.NilError(t, err)

				assert.Check(t, cmp.Equal(server.Request.Method, test.method))
				assert.Check(t, cmp.Equal(server.Request.URL.Path, "/method/test"))
				assert.Check(t, cmp.Equal(res.StatusCode, 200))
			})
		}
	})

	t.Run("without bodies but with decoding", func(t *testing.T) {
		tests := []struct {
			method string
			fn     func(context.Context, string, interface{}, ...fourten.URLModifier) (*http.Response, error)
		}{
			{"GET", client.GET},
			{"OPTIONS", client.OPTIONS},
		}
		for _, test := range tests {
			t.Run(test.method, func(t *testing.T) {
				server.Response.Headers = contentTypeJSON
				server.Response.Body = `{"some": "json"}`

				output := make(map[string]interface{})
				res, err := test.fn(ctx, "/method/test", &output)
				assert.NilError(t, err)

				assert.Check(t, cmp.Equal(server.Request.Method, test.method))
				assert.Check(t, cmp.Equal(server.Request.URL.Path, "/method/test"))
				assert.Check(t, cmp.Equal(res.StatusCode, 200))
				assert.Check(t, cmp.DeepEqual(output, map[string]interface{}{"some": "json"}))
			})
		}
	})

	t.Run("with bodies", func(t *testing.T) {
		tests := []struct {
			method string
			fn     func(context.Context, string, interface{}, interface{}, ...fourten.URLModifier) (*http.Response, error)
		}{
			{"POST", client.POST},
			{"PUT", client.PUT},
			{"PATCH", client.PATCH},
			{"DELETE", client.DELETE},
			{"ANYTHING", func(ctx context.Context, s string, i, o interface{}, ums ...fourten.URLModifier) (*http.Response, error) {
				return client.Call(ctx, "ANYTHING", s, i, nil, ums...)
			}},
		}
		for _, test := range tests {
			t.Run(test.method, func(t *testing.T) {
				server.Response.Headers = contentTypeJSON
				server.Response.Body = `{"some": "json"}`

				input := map[string]interface{}{"input": "json"}
				res, err := test.fn(ctx, "/method/test", input, nil)
				assert.NilError(t, err)

				assert.Check(t, cmp.Equal(server.Request.Method, test.method))
				assert.Check(t, cmp.Equal(server.Request.URL.Path, "/method/test"))
				requestBody, err := ioutil.ReadAll(server.Request.Body)
				assert.NilError(t, err)
				assert.Check(t, cmp.Equal(string(requestBody), `{"input":"json"}`+"\n"))
				assert.Check(t, cmp.Equal(res.StatusCode, 200))
			})
		}
	})

	t.Run("with bodies and decoding", func(t *testing.T) {
		tests := []struct {
			method string
			fn     func(context.Context, string, interface{}, interface{}, ...fourten.URLModifier) (*http.Response, error)
		}{
			{"POST", client.POST},
			{"PUT", client.PUT},
			{"PATCH", client.PATCH},
			{"DELETE", client.DELETE},
			{"ANYTHING", func(ctx context.Context, s string, i, o interface{}, ums ...fourten.URLModifier) (*http.Response, error) {
				return client.Call(ctx, "ANYTHING", s, i, o, ums...)
			}},
		}
		for _, test := range tests {
			t.Run(test.method, func(t *testing.T) {
				server.Response.Headers = contentTypeJSON
				server.Response.Body = `{"some": "json"}`

				input := map[string]interface{}{"input": "json"}
				output := make(map[string]interface{})
				res, err := test.fn(ctx, "/method/test", input, &output)
				assert.NilError(t, err)

				assert.Check(t, cmp.Equal(server.Request.Method, test.method))
				assert.Check(t, cmp.Equal(server.Request.URL.Path, "/method/test"))
				requestBody, err := ioutil.ReadAll(server.Request.Body)
				assert.NilError(t, err)
				assert.Check(t, cmp.Equal(string(requestBody), `{"input":"json"}`+"\n"))
				assert.Check(t, cmp.Equal(res.StatusCode, 200))
				assert.Check(t, cmp.DeepEqual(output, map[string]interface{}{"some": "json"}))
			})
		}
	})
}

func TestStatusCodes(t *testing.T) {
	client := fourten.New(fourten.BaseURL(server.URL), fourten.DecodeJSON)

	// TODO: 1xx codes? (probably safe to ignore for now)

	okCodes := []int{200, 201, 202, 203, 205, 206, 207, 208, 226}
	for _, code := range okCodes {
		t.Run("HTTP Status "+strconv.Itoa(code)+" is handled as non-error", func(t *testing.T) {
			stubResponse := StubResponse{
				Status:  code,
				Headers: contentTypeJSON,
				Body:    `{"valid": "json"}`,
			}
			server.Response = stubResponse

			var output map[string]interface{}
			res, err := client.GET(ctx, "/ok", &output)
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(res.StatusCode, code))
			assert.DeepEqual(t, output, map[string]interface{}{"valid": "json"})
		})
	}

	t.Run("HTTP Status 204 is handled as non-error with no body", func(t *testing.T) {
		server.Response = StubResponse{Status: 204}

		res, err := client.GET(ctx, "/ok", nil)
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(res.StatusCode, 204))
	})

	redirectErrorCodes := []int{301, 302, 303, 307, 308}
	for _, code := range redirectErrorCodes {
		t.Run("HTTP Status "+strconv.Itoa(code)+" follows Redirects", func(t *testing.T) {
			serverResponse := StubResponse{
				Status:  code,
				Headers: Headers{"location": []string{"/redirected"}},
			}
			server.Response = serverResponse

			res, err := client.Derive(fourten.DontDecode).GET(ctx, "/redirect", nil)
			assert.NilError(t, err)

			assert.Check(t, cmp.Equal(server.Request.Method, "GET"))
			assert.Check(t, cmp.Equal(server.Request.URL.Path, "/redirected"))
			assertResponse(t, res, StubResponse{Status: 200, Body: "PONG"})
		})
		t.Run("HTTP Status "+strconv.Itoa(code)+" follows Redirect loops until exhaustion", func(t *testing.T) {
			serverResponse := StubResponse{
				Status:  code,
				Headers: Headers{"location": []string{"/loop"}},
			}
			server.Response = serverResponse
			server.Sticky = true
			defer server.Reset()

			res, err := client.Derive(fourten.DontDecode).GET(ctx, "/loop", nil)
			assert.Check(t, res == nil)
			assert.ErrorContains(t, err, "stopped after 10 redirects")
		})
		t.Run("HTTP Status "+strconv.Itoa(code)+" can be told not to follow redirects", func(t *testing.T) {
			serverResponse := StubResponse{
				Status:  code,
				Headers: Headers{"location": []string{"/redirected"}},
				Body:    "redirect body",
			}
			server.Response = serverResponse

			res, err := client.Derive(fourten.NoFollow, fourten.DontDecode).GET(ctx, "/redirect", nil)

			// We get an error value
			assert.Check(t, cmp.ErrorContains(err, fmt.Sprintf("HTTP Status %d", code)))
			// But the normal response is still populated, including a readable body
			assertResponse(t, res, serverResponse)
			// The error can be compared against a sentinel
			assert.Check(t, errors.Is(err, fourten.ErrHTTP))
			// Or as a custom error type
			httpErr := fourten.AsHTTPError(err)
			assert.Check(t, httpErr != nil)
			assert.Check(t, cmp.Equal(httpErr.Response, res), "expected response to match error field")
		})
	}

	postRedirectErrorCodes := []int{307, 308}
	for _, code := range postRedirectErrorCodes {
		t.Run("HTTP Status "+strconv.Itoa(code)+" follows Redirects, repeating request body", func(t *testing.T) {
			serverResponse := StubResponse{
				Status:  code,
				Headers: Headers{"location": []string{"/redirected"}},
			}
			server.Response = serverResponse

			input := map[string]interface{}{"input": "json"}
			res, err := client.Derive(fourten.EncodeJSON, fourten.DontDecode).POST(ctx, "/redirect", input, nil)
			assert.NilError(t, err)

			assert.Check(t, cmp.Equal(server.Request.Method, "POST"))
			assert.Check(t, cmp.Equal(server.Request.URL.Path, "/redirected"))
			requestBody, err := ioutil.ReadAll(server.Request.Body)
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(string(requestBody), `{"input":"json"}`+"\n"))
			assertResponse(t, res, StubResponse{Status: 200, Body: "PONG"})
		})
	}

	standardErrorCodes := []int{
		300, 305, 306,
		400, 401, 402, 403, 404, 405, 406, 407, 408, 409, 410,
		411, 412, 413, 414, 415, 416, 417, 418, 421, 422, 423,
		424, 426, 428, 429, 431, 451,
		500, 501, 502, 503, 504, 505, 506, 507, 508, 510, 511,
	}

	for _, code := range standardErrorCodes {
		t.Run("HTTP Status "+strconv.Itoa(code)+" without decoding", func(t *testing.T) {
			serverResponse := StubResponse{Status: code, Body: "WHOOPS"}
			server.Response = serverResponse

			res, err := client.Derive(fourten.DontDecode).GET(ctx, "/error", nil)

			// We get an error value
			assert.Check(t, cmp.ErrorContains(err, fmt.Sprintf("HTTP Status %d", code)))
			// But the normal response is still populated, including a readable body
			assertResponse(t, res, serverResponse)
			// The error can be compared against a sentinel
			assert.Check(t, errors.Is(err, fourten.ErrHTTP))
			// Or cast into the custom type
			var httpErr *fourten.HTTPError
			assert.Check(t, errors.As(err, &httpErr))
			assert.Equal(t, httpErr.Response, res, "expected response to match error field")
			// Or into custom type via helper
			httpErrSugar := fourten.AsHTTPError(err)
			assert.Check(t, cmp.Equal(httpErr, httpErrSugar))
		})

		t.Run("HTTP Status "+strconv.Itoa(code)+" with error decoding", func(t *testing.T) {
			stubResponse := StubResponse{
				Status:  code,
				Headers: contentTypeJSON,
				Body:    `{"error": "aaarrggh"}`,
			}
			server.Response = stubResponse

			var output interface{}
			res, err := client.GET(ctx, "/error", &output)

			// We get an error value
			assert.Check(t, cmp.ErrorContains(err, fmt.Sprintf("HTTP Status %d", code)))
			// The normal response is still populated
			assert.Check(t, cmp.Equal(res.StatusCode, code))
			// But the body has been consumed
			assert.Check(t, bodyConsumed(res.Body))
			// And output remains nil
			assert.Check(t, output == nil)

			// The error can be compared against a sentinel
			assert.Check(t, errors.Is(err, fourten.ErrHTTP))
			// Or cast into the custom type
			httpErr := fourten.AsHTTPError(err)
			assert.Check(t, httpErr != nil)
			assert.Check(t, cmp.Equal(httpErr.Response, res), "expected response to match error field")
			// And the type allows for decoding
			var errOut map[string]interface{}
			assert.Check(t, cmp.Nil(httpErr.Decode(&errOut)))
			assert.Check(t, cmp.DeepEqual(errOut, map[string]interface{}{"error": "aaarrggh"}))
			// Or simply reading bytes
			assert.Check(t, cmp.DeepEqual(httpErr.Body(), `{"error": "aaarrggh"}`))
		})

		t.Run("failed to read body during error response", func(t *testing.T) {
			server.Response = StubResponse{
				Status:  code,
				Headers: map[string][]string{"Content-Length": {"1"}},
				Body:    "more than 1",
			}

			var output map[string]interface{}
			_, err := client.GET(ctx, "/error", &output)
			assert.Check(t, cmp.ErrorContains(err, "unexpected EOF"))
		})

		t.Run("when error decoding fails", func(t *testing.T) {
			stubResponse := StubResponse{
				Status:  code,
				Headers: contentTypeJSON,
				Body:    `{"error": }`,
			}
			server.Response = stubResponse

			var output map[string]interface{}
			res, err := client.GET(ctx, "/error", &output)

			// We get an error value
			assert.Check(t, cmp.ErrorContains(err, fmt.Sprintf("HTTP Status %d", code)))
			// The normal response is still populated
			assert.Check(t, cmp.Equal(res.StatusCode, code))
			// But the body has been consumed
			assert.Check(t, bodyConsumed(res.Body))
			// And output remains nil
			assert.Check(t, cmp.DeepEqual(output, map[string]interface{}(nil)))

			var httpErr *fourten.HTTPError
			assert.Check(t, errors.As(err, &httpErr))
			assert.Check(t, cmp.Equal(httpErr.Response, res), "expected response to match error field")
			// And the type allows for decoding
			var errOut map[string]interface{}
			assert.Check(t, cmp.ErrorContains(httpErr.Decode(&errOut), "failed to decode"))

			// when decoding fails, can still fall back to reading strings
			assert.Check(t, cmp.Equal(httpErr.Body(), `{"error": }`))
		})
	}

	t.Run("HTTP Status 304 without decoding", func(t *testing.T) {
		serverResponse := StubResponse{Status: 304, Body: ""}
		server.Response = serverResponse

		res, err := client.GET(ctx, "/error", nil)

		// We get an error value
		assert.Check(t, cmp.ErrorContains(err, fmt.Sprintf("HTTP Status %d", 304)))
		// But the normal response is still populated, including a readable body
		assertResponse(t, res, serverResponse)
		// The error can be compared against a sentinel
		assert.Check(t, errors.Is(err, fourten.ErrHTTP))
		// Or cast into the custom type
		var httpErr *fourten.HTTPError
		assert.Check(t, errors.As(err, &httpErr))
		assert.Equal(t, httpErr.Response, res, "expected response to match error field")
	})
}

func TestRefine(t *testing.T) {
	clientA := fourten.New(fourten.BaseURL(server.URL + "/server-a/"))
	clientB := clientA.Derive(fourten.BaseURL(server.URL + "/server-b/"))

	_, err := clientA.GET(ctx, "ping", nil)
	assert.NilError(t, err, "%#v", err)
	assert.Check(t, cmp.Equal(server.Request.URL.Path, "/server-a/ping"))

	_, err = clientB.GET(ctx, "ping", nil)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(server.Request.URL.Path, "/server-b/ping"))
}

func TestTimeouts(t *testing.T) {
	client := fourten.New(fourten.BaseURL(server.URL),
		fourten.RequestTimeout(time.Nanosecond))

	server.Delay = time.Millisecond

	_, err := client.GET(ctx, "/request", nil)
	assert.ErrorContains(t, err, "deadline exceeded")
}

func TestAsHTTPError(t *testing.T) {
	t.Run("returns a type-cast HTTPError if passed one", func(t *testing.T) {
		var err error = &fourten.HTTPError{}
		httpErr := fourten.AsHTTPError(err)
		assert.Check(t, cmp.Equal(httpErr, err.(*fourten.HTTPError)))
	})
	t.Run("returns nil if not passed an HTTPError", func(t *testing.T) {
		var err = errors.New("not an http error")
		httpErr := fourten.AsHTTPError(err)
		assert.Check(t, httpErr == nil)
	})
}

func TestChunkedResponses(t *testing.T) {
	client := fourten.New(fourten.BaseURL(server.URL), fourten.DecodeJSON)
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "{")
		_, _ = fmt.Fprint(w, `"json":true`)
		for i := 0; i < 512; i++ {
			// Pad out the response to trigger automatic response chunking
			_, _ = fmt.Fprint(w, `    `)
		}
		_, _ = fmt.Fprint(w, "}")
	})
	var out map[string]bool
	res, err := client.GET(ctx, "/chunked", &out)
	assert.NilError(t, err)
	assert.Check(t, cmp.DeepEqual(res.TransferEncoding, []string{"chunked"}))
	assert.Check(t, cmp.DeepEqual(out, map[string]bool{"json": true}))
}

func TestGzippedResponses(t *testing.T) {
	client := fourten.New(fourten.BaseURL(server.URL), fourten.DecodeJSON)
	gzipWrapper, err := gziphandler.NewGzipLevelAndMinSize(gzip.BestSpeed, 1)
	assert.NilError(t, err)
	server.Handler = gzipWrapper(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"hello":"decompressed world"}`)
	}))

	var out map[string]string
	res, err := client.GET(ctx, "/gzipped", &out)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(res.Uncompressed, true))
	assert.Check(t, cmp.DeepEqual(out, map[string]string{"hello": "decompressed world"}))
}

func TestGzippedRequests(t *testing.T) {
	client := fourten.New(fourten.BaseURL(server.URL), fourten.EncodeJSON, fourten.GzipRequests)
	in := make([]string, 300)
	for i := 0; i < len(in); i++ {
		in[i] = "abc"
	}
	_, err := client.POST(ctx, "/zippy", in, nil)
	assert.NilError(t, err)

	assert.Check(t, server.Request.ContentLength < 100)
	assert.Check(t, cmp.Equal(server.Request.Header.Get("Content-Encoding"), "gzip"))
	gr, err := gzip.NewReader(server.Request.Body)
	assert.NilError(t, err)
	var body []string
	err = json.NewDecoder(gr).Decode(&body)
	assert.NilError(t, err)
	assert.Check(t, cmp.DeepEqual(in, body))
}

func TestRetries(t *testing.T) {
	server.Sticky = true
	defer server.Reset()

	handlers := map[string]http.Handler{}
	requestTimes := map[string][]time.Time{}
	mu := &sync.Mutex{}
	requests := func(path string) []time.Time {
		mu.Lock()
		defer mu.Unlock()
		return requestTimes[path]
	}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestTimes[r.URL.RequestURI()] = append(requestTimes[r.URL.RequestURI()], time.Now())
		if handler, ok := handlers[r.URL.Path]; ok {
			mu.Unlock()
			handler.ServeHTTP(w, r)
		} else {
			mu.Unlock()
			w.WriteHeader(500)
		}
	})

	client := fourten.New(fourten.BaseURL(server.URL))

	t.Run("retry behaviour", func(t *testing.T) {
		t.Run("does not retry failed requests by default", func(t *testing.T) {
			t.Parallel()
			_, _ = client.GET(ctx, "/off", nil)
			assert.Assert(t, cmp.Len(requests("/off"), 1))
		})
		t.Run("enabling retries defaults to a reasonable policy", func(t *testing.T) {
			t.Parallel()
			_, _ = client.Derive(
				fourten.RetryOnError, fourten.RetrySpeedupFactor(10),
			).GET(ctx, "/default", nil)

			reqs := requests("/default")
			assert.Assert(t, cmp.Len(reqs, 3))

			wait1 := reqs[1].Sub(reqs[0])
			wait2 := reqs[2].Sub(reqs[1])
			assert.Check(t, wait2 > wait1, "gap between requests increases: %q %q", wait1, wait2)
		})
		t.Run("tuning max number of requests", func(t *testing.T) {
			t.Parallel()
			_, _ = client.Derive(
				fourten.RetryOnError,
				fourten.RetryMaxAttempts(5),
				fourten.RetrySpeedupFactor(10),
			).GET(ctx, "/max-num", nil)

			reqs := requests("/max-num")
			assert.Assert(t, cmp.Len(reqs, 5))

			wait1 := reqs[1].Sub(reqs[0])
			wait2 := reqs[2].Sub(reqs[1])
			wait3 := reqs[3].Sub(reqs[2])
			wait4 := reqs[4].Sub(reqs[3])
			assert.Check(t, wait2 > wait1, "gap between requests increases")
			assert.Check(t, wait3 > wait2, "gap between requests increases")
			assert.Check(t, wait4 > wait3, "gap between requests increases")
		})
		t.Run("speedup factor helps with testing", func(t *testing.T) {
			t.Parallel()
			_, _ = client.Derive(
				fourten.RetryOnError, fourten.RetryMaxAttempts(2),
				fourten.RetrySpeedupFactor(10),
			).GET(ctx, "/speedup", nil)
			reqs := requests("/speedup")
			assert.Assert(t, cmp.Len(reqs, 2))
			wait := reqs[1].Sub(reqs[0])

			assert.Check(t, wait < 10*time.Millisecond, "expected %q < 2ms", wait)
		})
		t.Run("tuning max duration of retrying", func(t *testing.T) {
			t.Parallel()
			_, _ = client.Derive(
				fourten.RetryOnError,
				fourten.RetryMaxDuration(90*time.Millisecond),
			).GET(ctx, "/max-duration", nil)

			assert.Assert(t, cmp.Len(requests("/max-duration"), 2))
		})
		t.Run("tuning the exponential backoff", func(t *testing.T) {
			t.Parallel()
			_, _ = client.Derive(
				fourten.RetryOnError,
				fourten.RetryMaxAttempts(4),
				fourten.RetryBackoff(10*time.Millisecond, 30*time.Millisecond, 3, 0.1),
			).GET(ctx, "/backoff", nil)

			reqs := requests("/backoff")
			assert.Assert(t, cmp.Len(reqs, 4))

			assert.Check(t, cmp.DeepEqual([]time.Duration{
				reqs[1].Sub(reqs[0]),
				reqs[2].Sub(reqs[1]),
				reqs[3].Sub(reqs[2]),
			}, []time.Duration{
				10 * time.Millisecond,
				30 * time.Millisecond,
				30 * time.Millisecond,
			}, opt.DurationWithThreshold(10*time.Millisecond)))
		})
		t.Run("using a fixed backoff", func(t *testing.T) {
			t.Parallel()
			_, _ = client.Derive(
				fourten.RetryOnError,
				fourten.RetryDelay(30*time.Millisecond),
			).GET(ctx, "/fixed", nil)

			reqs := requests("/fixed")
			assert.Assert(t, cmp.Len(reqs, 3))

			assert.Check(t, cmp.DeepEqual([]time.Duration{
				reqs[1].Sub(reqs[0]),
				reqs[2].Sub(reqs[1]),
			}, []time.Duration{
				30 * time.Millisecond,
				30 * time.Millisecond,
			}, opt.DurationWithThreshold(10*time.Millisecond)))
		})
		t.Run("if retries are not enabled, tuning fails", func(t *testing.T) {
			t.Parallel()
			assert.Check(t, cmp.Panics(func() {
				_ = client.Derive(fourten.RetryMaxAttempts(1))
			}))
			assert.Check(t, cmp.Panics(func() {
				_ = client.Derive(fourten.RetryMaxDuration(0))
			}))
			assert.Check(t, cmp.Panics(func() {
				_ = client.Derive(fourten.RetryBackoff(0, 0, 0, 0))
			}))
			assert.Check(t, cmp.Panics(func() {
				_ = client.Derive(fourten.RetryDelay(0))
			}))
		})
		t.Run("deriving different retries from one root", func(t *testing.T) {
			t.Parallel()
			retrying := client.Derive(fourten.RetryOnError, fourten.RetryDelay(time.Nanosecond))
			maxTwo := retrying.Derive(fourten.RetryMaxAttempts(2))
			maxFour := retrying.Derive(fourten.RetryMaxAttempts(4))

			_, _ = retrying.GET(ctx, "/retrying", nil)
			_, _ = maxTwo.GET(ctx, "/max-two", nil)
			_, _ = maxFour.GET(ctx, "/max-four", nil)

			assert.Assert(t, cmp.Len(requests("/retrying"), 3))
			assert.Assert(t, cmp.Len(requests("/max-two"), 2))
			assert.Assert(t, cmp.Len(requests("/max-four"), 4))
		})
		t.Run("multiple requests use distinct backoffs", func(t *testing.T) {
			t.Parallel()
			retrying := client.Derive(fourten.RetryOnError,
				fourten.RetryMaxAttempts(2),
				fourten.RetryDelay(100*time.Millisecond))

			_, _ = retrying.GET(ctx, "/multi-1", nil)
			_, _ = retrying.GET(ctx, "/multi-2", nil)

			assert.Assert(t, cmp.Len(requests("/multi-1"), 2))
			assert.Assert(t, cmp.Len(requests("/multi-2"), 2))
		})
		t.Run("request timeout applies to each individual retry", func(t *testing.T) {
			t.Parallel()
			retrying := client.Derive(fourten.RetryOnError,
				fourten.RetryMaxAttempts(2),
				fourten.RetryDelay(0),
				fourten.RequestTimeout(40*time.Millisecond))

			handlers["/timeout"] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(100 * time.Millisecond)
				w.WriteHeader(200)
			})

			_, _ = retrying.GET(ctx, "/timeout", nil)

			reqs := requests("/timeout")
			assert.Assert(t, cmp.Len(reqs, 2))
			assert.Check(t, cmp.DeepEqual(reqs[1].Sub(reqs[0]), 40*time.Millisecond,
				opt.DurationWithThreshold(10*time.Millisecond)))
		})
		t.Run("retry re-sends request body", func(t *testing.T) {
			t.Fatalf("TODO")
		})
	})
	t.Run("what gets retried", func(t *testing.T) {
		retrier := client.Derive(fourten.RetryOnError,
			fourten.NoFollow,
			fourten.RetryMaxAttempts(2),
			fourten.RetryDelay(0))
		handlers["/status"] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			status, err := strconv.Atoi(r.URL.RawQuery)
			if err != nil {
				panic(err)
			}
			w.Header().Set("Location", "somewhere")
			w.WriteHeader(status)
		})
		countReqs := func(path string) int {
			_, _ = retrier.GET(ctx, path, nil)
			return len(requests(path))
		}
		doesRetry := func(path string) cmp.Comparison {
			return func() cmp.Result {
				if countReqs(path) > 1 {
					return cmp.ResultSuccess
				}
				return cmp.ResultFailure(fmt.Sprintf("expected %s to retry", path))
			}
		}
		doesNotRetry := func(path string) cmp.Comparison {
			return func() cmp.Result {
				n := countReqs(path)
				if n == 1 {
					return cmp.ResultSuccess
				}
				return cmp.ResultFailure(fmt.Sprintf("expected %s to not retry, got %d requests", path, n))
			}
		}
		t.Run("doesn't retry 2xx", func(t *testing.T) {
			for _, status := range []int{200, 201} {
				assert.Check(t, doesNotRetry(fmt.Sprintf("/status?%d", status)))
			}
		})
		t.Run("doesn't retry 3xx", func(t *testing.T) {
			for _, status := range []int{301, 302} {
				assert.Check(t, doesNotRetry(fmt.Sprintf("/status?%d", status)))
			}
		})
		t.Run("doesn't retry 4xx", func(t *testing.T) {
			for _, status := range []int{400, 401, 403, 404} {
				assert.Check(t, doesNotRetry(fmt.Sprintf("/status?%d", status)))
			}
		})
		t.Run("does retry 5xx", func(t *testing.T) {
			for _, status := range []int{500} {
				assert.Check(t, doesRetry(fmt.Sprintf("/status?%d", status)))
			}
		})
		t.Run("does retry connection errors", func(t *testing.T) {

		})
		t.Run("doesn't retry programmer errors", func(t *testing.T) {

		})
	})
	t.Run("custom retry behaviour", func(t *testing.T) {

	})
}

func assertResponse(t *testing.T, res *http.Response, want StubResponse) {
	t.Helper()
	assert.Assert(t, res != nil)
	assert.Check(t, cmp.Equal(res.StatusCode, want.Status))

	for header, values := range want.Headers {
		assert.Check(t, cmp.DeepEqual(res.Header.Values(header), values))
	}

	body, err := ioutil.ReadAll(res.Body)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(string(body), want.Body))
}

func bodyConsumed(r io.Reader) cmp.Comparison {
	return func() cmp.Result {
		_, err := r.Read(nil)
		return cmp.ErrorContains(err, "read on closed")()
	}
}

type RecordingServer struct {
	URL   string
	Delay time.Duration
	// Request is the last request we received
	Request http.Request
	// Response is the next request we will return
	Response StubResponse
	// Handler provides custom server behaviour, instead of using Response
	Handler http.Handler
	// Sticky disables resetting the Response after each request
	Sticky bool

	defaultResponse StubResponse

	mu *sync.Mutex
}
type StubResponse struct {
	Status  int
	Headers Headers
	Body    string
}
type Headers = map[string][]string

func NewServer(defaultResponse StubResponse) *RecordingServer {
	recording := &RecordingServer{
		Response:        defaultResponse,
		defaultResponse: defaultResponse,
		mu:              &sync.Mutex{},
	}
	server := httptest.NewServer(recording)
	recording.URL = server.URL
	return recording
}
func (s *RecordingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	// Copy the request, preserving the body
	s.Request = *r
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	s.Request.Body = ioutil.NopCloser(bytes.NewReader(body))

	if s.Handler != nil {
		s.mu.Unlock()
		s.Handler.ServeHTTP(w, r)
	} else {
		defer s.mu.Unlock()

		if s.Delay != 0 {
			time.Sleep(s.Delay)
		}

		h := w.Header()
		for header, values := range s.Response.Headers {
			for _, value := range values {
				h.Add(header, value)
			}
		}
		w.WriteHeader(s.Response.Status)
		_, _ = io.Copy(w, bytes.NewBufferString(s.Response.Body))
	}

	// Reset stub after each call, unless we're being sticky
	if !s.Sticky {
		s.Reset()
	}
}

func (s *RecordingServer) Reset() {
	s.Sticky = false
	s.Delay = 0
	s.Response = s.defaultResponse
	s.Handler = nil
}

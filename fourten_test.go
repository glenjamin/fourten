package fourten_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

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

func TestHeaders(t *testing.T) {
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

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Content-Type"), "application/json; charset=utf-8"))
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
			fn     func(context.Context, string, interface{}) (*http.Response, error)
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
			fn     func(context.Context, string, interface{}) (*http.Response, error)
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
			fn     func(context.Context, string, interface{}, interface{}) (*http.Response, error)
		}{
			{"POST", client.POST},
			{"PUT", client.PUT},
			{"PATCH", client.PATCH},
			{"DELETE", client.DELETE},
			{"ANYTHING", func(ctx context.Context, s string, i, o interface{}) (*http.Response, error) {
				return client.Call(ctx, "ANYTHING", s, i, nil)
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
			fn     func(context.Context, string, interface{}, interface{}) (*http.Response, error)
		}{
			{"POST", client.POST},
			{"PUT", client.PUT},
			{"PATCH", client.PATCH},
			{"DELETE", client.DELETE},
			{"ANYTHING", func(ctx context.Context, s string, i, o interface{}) (*http.Response, error) {
				return client.Call(ctx, "ANYTHING", s, i, o)
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

		var output map[string]interface{}
		res, err := client.GET(ctx, "/ok", &output)
		assert.NilError(t, err)
		assert.Check(t, cmp.Equal(res.StatusCode, 204))
		assert.Check(t, cmp.DeepEqual(output, map[string]interface{}(nil)))
	})

	redirectErrorCodes := []int{301, 302, 303, 307, 308}
	for _, code := range redirectErrorCodes {
		t.Run("HTTP Status "+strconv.Itoa(code)+" follows Redirects", func(t *testing.T) {
			serverResponse := StubResponse{
				Status:  code,
				Headers: Headers{"location": []string{"/redirected"}},
			}
			server.Response = serverResponse

			res, err := client.GET(ctx, "/redirect", nil)
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
			defer func() { server.Sticky = false }()

			res, err := client.GET(ctx, "/loop", nil)
			assert.Check(t, res == nil)
			assert.ErrorContains(t, err, "stopped after 10 redirects")
		})
		t.Run("HTTP Status "+strconv.Itoa(code)+" can be told not to follow redirects", func(t *testing.T) {
			nofollow := client.Derive(fourten.NoFollow)
			serverResponse := StubResponse{
				Status:  code,
				Headers: Headers{"location": []string{"/redirected"}},
				Body:    "redirect body",
			}
			server.Response = serverResponse

			res, err := nofollow.GET(ctx, "/redirect", nil)

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

			res, err := client.GET(ctx, "/error", nil)

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
		})

		t.Run("HTTP Status "+strconv.Itoa(code)+" with error decoding", func(t *testing.T) {
			stubResponse := StubResponse{
				Status:  code,
				Headers: contentTypeJSON,
				Body:    `{"error": "aaarrggh"}`,
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

			// The error can be compared against a sentinel
			assert.Check(t, errors.Is(err, fourten.ErrHTTP))
			// Or cast into the custom type
			var httpErr *fourten.HTTPError
			assert.Check(t, errors.As(err, &httpErr))
			assert.Equal(t, httpErr.Response, res, "expected response to match error field")
			// And the type allows for decoding
			var errOut map[string]interface{}
			assert.Check(t, cmp.Nil(httpErr.Decode(&errOut)))
			assert.Check(t, cmp.DeepEqual(errOut, map[string]interface{}{"error": "aaarrggh"}))
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
			assert.Equal(t, httpErr.Response, res, "expected response to match error field")
			// And the type allows for decoding
			var errOut map[string]interface{}
			assert.Check(t, cmp.ErrorContains(httpErr.Decode(&errOut), "failed to decode"))
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
	// Sticky disables resetting the Response after each request
	Sticky bool

	defaultResponse StubResponse
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
	}
	server := httptest.NewServer(recording)
	recording.URL = server.URL
	return recording
}
func (s *RecordingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Copy the request, preserving the body
	s.Request = *r
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	s.Request.Body = ioutil.NopCloser(bytes.NewReader(body))

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

	// Reset stub after each call, unless we're being sticky
	if !s.Sticky {
		s.Delay = 0
		s.Response = s.defaultResponse
	}
}

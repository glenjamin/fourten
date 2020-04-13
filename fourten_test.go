package fourten_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestURLResolution(t *testing.T) {
	t.Run("Can request absolute URLs", func(t *testing.T) {
		client := fourten.New()
		res, err := client.GET(ctx, server.URL+"/ping")
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Method, "GET"))
		assert.Check(t, cmp.Equal(server.Request.URL.Path, "/ping"))
		assertResponse(t, res, server.Response)
	})

	t.Run("Can request URLs relative to a base URL", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL))
		res, err := client.GET(ctx, "/ping")
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Method, "GET"))
		assert.Check(t, cmp.Equal(server.Request.URL.Path, "/ping"))
		assertResponse(t, res, server.Response)
	})

	t.Run("errors on invalid URL", func(t *testing.T) {
		_, err := fourten.New().GET(context.Background(), ":::")
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

		_, err := client.GET(ctx, "/ping")
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Wibble"), "Wobble"))
	})

	t.Run("Can set bearer tokens", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.Bearer("ipromisetopaythebearer"))

		_, err := client.GET(ctx, "/ping")
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Authorization"), "Bearer ipromisetopaythebearer"))
	})
}

func TestDecoding(t *testing.T) {
	t.Run("Refuses to decode unless configured to", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL))

		body := make(map[string]interface{})
		_, err := client.GETDecoded(ctx, "/data", &body)
		assert.ErrorContains(t, err, "no decoder")
	})

	t.Run("Requests and Decodes JSON into provided pointer", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Headers = Headers{"content-type": []string{"application/json; charset=utf-8"}}
		server.Response.Body = `{"json": "made easy"}`

		body := make(map[string]interface{})
		_, err := client.GETDecoded(ctx, "/data", &body)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Accept"), "application/json"))
		assert.Check(t, cmp.DeepEqual(body, map[string]interface{}{"json": "made easy"}))
	})

	t.Run("Won't decode JSON without a content type", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.DecodeJSON)

		server.Response.Body = `{"json": {"made": "easy"}}`

		body := make(map[string]interface{})
		_, err := client.GETDecoded(ctx, "/data", &body)
		assert.ErrorContains(t, err, "expected JSON content-type")
	})
}

func TestEncoding(t *testing.T) {
	t.Run("Refuses to encode unless configured to", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL))

		input := map[string]interface{}{}
		_, err := client.POST(ctx, "/data", &input)
		assert.ErrorContains(t, err, "no encoder")
	})

	t.Run("Can POST encoded JSON", func(t *testing.T) {
		client := fourten.New(fourten.BaseURL(server.URL),
			fourten.EncodeJSON)

		input := map[string]interface{}{
			"request": "params",
			"of_json": true,
		}
		_, err := client.POST(ctx, "/data", &input)
		assert.NilError(t, err)

		assert.Check(t, cmp.Equal(server.Request.Header.Get("Content-Type"), "application/json; charset=utf-8"))

		body, err := ioutil.ReadAll(server.Request.Body)
		assert.NilError(t, err)
		requestBody := make(map[string]interface{})
		assert.NilError(t, json.Unmarshal(body, &requestBody))
		assert.DeepEqual(t, requestBody, input)
	})
}

func TestRefine(t *testing.T) {
	clientA := fourten.New(fourten.BaseURL(server.URL + "/server-a/"))
	clientB := clientA.Refine(fourten.BaseURL(server.URL + "/server-b/"))

	_, err := clientA.GET(ctx, "ping")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(server.Request.URL.Path, "/server-a/ping"))

	_, err = clientB.GET(ctx, "ping")
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(server.Request.URL.Path, "/server-b/ping"))
}

func assertResponse(t *testing.T, res *http.Response, want StubResponse) {
	assert.Check(t, res != nil)
	assert.Check(t, cmp.Equal(res.StatusCode, want.Status))

	for header, values := range want.Headers {
		assert.Check(t, cmp.DeepEqual(res.Header.Values(header), values))
	}

	body, err := ioutil.ReadAll(res.Body)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(string(body), want.Body))
}

type RecordingServer struct {
	URL      string
	Request  http.Request
	Response StubResponse
	Close    func()

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
	recording.Close = server.Close
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

	h := w.Header()
	for header, values := range s.Response.Headers {
		for _, value := range values {
			h.Add(header, value)
		}
	}
	w.WriteHeader(s.Response.Status)
	_, _ = io.Copy(w, bytes.NewBufferString(s.Response.Body))

	// Reset stub after each call
	s.Response = s.defaultResponse
}

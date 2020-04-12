package fourten_test

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"gotest.tools/v3/assert/cmp"

	"gotest.tools/v3/assert"

	"github.com/glenjamin/fourten"
)

func TestSimpleHappyPaths(t *testing.T) {
	server := NewServer()
	defer server.Close()
	ctx := context.Background()

	server.Response = StubResponse{
		Status: 200,
		Body:   "PONG",
	}

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
}

func TestErrorHandling(t *testing.T) {
	t.Run("errors on invalid URL", func(t *testing.T) {
		_, err := fourten.New().GET(context.Background(), "nope://")
		assert.ErrorContains(t, err, "unsupported")
	})
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
}
type StubResponse struct {
	Status  int
	Headers map[string][]string
	Body    string
}

func NewServer() *RecordingServer {
	recording := &RecordingServer{}
	server := httptest.NewServer(recording)
	recording.URL = server.URL
	recording.Close = server.Close
	return recording
}
func (s *RecordingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Request = *r
	h := w.Header()
	for header, values := range s.Response.Headers {
		for _, value := range values {
			h.Add(header, value)
		}
	}
	w.WriteHeader(s.Response.Status)
	_, _ = io.Copy(w, bytes.NewBufferString(s.Response.Body))
}

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

func TestSimpleHappyPath(t *testing.T) {
	server := NewServer()
	defer server.Close()
	ctx := context.Background()

	server.Response = StubResponse{
		Status: 200,
		Body:   "PONG",
	}

	client := fourten.New(fourten.BaseURL(server.URL))
	res, err := client.GET(ctx, "/ping")
	assert.NilError(t, err)

	assert.Check(t, res != nil)
	assert.Check(t, cmp.Equal(res.StatusCode, 200))

	body, err := ioutil.ReadAll(res.Body)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(string(body), "PONG"))
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
	io.Copy(w, bytes.NewBufferString(s.Response.Body))
}

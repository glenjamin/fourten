package fourten

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

type roundTripFn func(req *http.Request) (*http.Response, error)

func (f roundTripFn) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestTimeout_DefaultsToShort(t *testing.T) {
	client := New()

	// Grab the context from the request that's passing through
	var requestCtx context.Context
	client.httpClient.Transport = roundTripFn(func(req *http.Request) (*http.Response, error) {
		requestCtx = req.Context()
		return nil, errors.New("not a real roundtrip")
	})

	_, _ = client.GET(context.Background(), "/anywhere", nil)

	assert.Assert(t, requestCtx != nil)
	deadline, ok := requestCtx.Deadline()
	assert.Assert(t, ok, "expected request context to have deadline")
	twoSecondsAhead := time.Now().Add(2 * time.Second)
	assert.Assert(t, deadline.Before(twoSecondsAhead),
		"expected deadline to be short: %v < %v", deadline, twoSecondsAhead)
}

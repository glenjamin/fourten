# Four Ten

> 410 Gone

An opinionated Go HTTP Client.

This library aims to provide a high-level interface for performing HTTP requests with minimal ceremony,
including key features for use in a production setting.

It is my intention that you shouldn't need to drop down to lower level abstractions,
but the library will allow you to if you so desire.

> Being abstract is something profoundly different from being vague …
> The purpose of abstraction is not to be vague,
> but to create a new semantic level in which one can be absolutely precise. 

*Edsger W. Dijkstra*

## Goals

- Be simple and easy to use
- Be reasonably fast
- HTTP Status errors are Go errors
- Everything requires a Context
- Short, sensible default timeouts
- Allow setting up reusable defaults across requests
- Support automatic retry of idempotent requests
- Easily handle JSON request and response bodies
- Allow consumers to add observability via metrics and tracing

## Usage

```go
// Setup a client with defaults you care about
client := fourten.New(
    fourten.BaseURL("https://reqres.in/api"),
    fourten.DecodeJSON,
    fourten.SetHeader("Authorization", "Bearer 1234567890"),
    fourten.RetryMaxAttempts(3),
    fourten.ResponseTimeout(time.Second),
    fourten.Observe(func(req fourten.RequestInfo) fourten.ResponseObserver {
        start := time.Now()
        return func(res fourten.ResponseInfo) {
            metrics.Timer("http.request", time.Since(start), map[string]string{
                "error": String(res.err != nil),
                "status": res.StatusCode,
                "route": req.Target,
            })
        }
    }),
)

ctx := context.Background()

// Make GET requests with response decoding
{
    json := make(map[string]interace{})
    res, err := client.GET(ctx, "/items", &json)
    println(err, res, json)
}

// HTTP Status codes are turned into errors
{
    res, err := client.GET(ctx, "/error", nil) // 4xx, 5xx etc
    errors.Is(err, fourten.ErrHTTP) // true

    // And can be cast into useful error types
    var httpErr *fourten.HttpError
    if errors.As(err, &httpErr) {
        json := make(map[string]interace{})
        err := httpErr.Decode(json)
        raw := httpErr.Body()
        println(err, res, json, raw)
    }
}

// Derive new clients from the existing client's defaults as needed
derived := client.Derive(
    fourten.DontRetry,
    fourten.EncodeJSON,
)

// Make POST requests with request encoding and body decoding
{
    input = map[string]interface{}{"abc": "def"}
    output := make(map[string]interace{})
    res, err := derived.POST(ctx, "/items", input, &output)
    println(err, res, output)
}

// To override options per request, derive a client inline
{
    res, err := client.Derive(fourten.DontRetry).POST(ctx, "/items/one-shot", nil, nil)
    println(err, res, json)
}

// URL parameters can be filled in via optional additional arguments
{
    res, err := client.POST(ctx, "/items/:item-id", nil, nil, fourten.Param("item-id", "123456"))
    println(err, res, json)
}

// Sending loads of data? gzip your bodies
{
	zippy := client.Derive(fourten.GzipRequests)
}

// Retries are off by default, but can be enabled and configured
retrying := client.Derive(
    fourten.RetryOnError,
    fourten.RetryMaxAttempts(3),
    fourten.RetryMaxDuration(5 * time.Second),
    // initial delay, max delay, iteration multiplier, jitter factor
    fourten.RetryBackoff(200 * time.Millisecond, time.Second, 2, 0.1)
    // to help with automated testing, you can speed things up
    fourten.RetrySpeedupFactor(10)
)

// Or you can supply completely custom retry logic
retrying := client.Derive(
    fourten.RetryStrategy(func() fourten.Retrier {
        attempts := 0
        return func(err error) time.Duration {
            if attempt += 1; attempt > 3 {
                return -1
            }
            if httpErr := fourten.AsHTTPError(err); httpErr != nil {
                if httpErr.Response.StatusCode >= 500 {
                    return 200 * time.Millisecond
                }
                return -1
            }
            return time.Second
        }
    }),
)
```

## Docs

TODO

## TODO

* ensure we handle connection errors & timeouts properly
* configure connection pooling - http://tleyden.github.io/blog/2016/11/21/tuning-the-go-http-client-library-for-load-testing/
* Retries
* O11y
* Docs

## License

MIT

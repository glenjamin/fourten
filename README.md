# Four Ten

> 410 Gone

An opinionated Go HTTP Client.

This library aims to provide a high-level interface for performing HTTP requests with minimal ceremony,
including key features for use in a production setting.

It is my intention that you shouldn't need to drop down to lower level abstractions,
but the libarary will allow you to if you so desire. 

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
    fourten.Retry(3),
    fourten.ResponseTimeout(time.Second),
    fourten.Observe(func(info *fourten.ReqInfo, req *http.Request) fourten.ResponseObserver {
        start := time.Now()
        return func(res *http.Response, err error) {
            metrics.Timer("http.request", time.Since(start), map[string]string{
                "error": String(err != nil),
                "status": res.StatusCode,
                "route": info.Target,
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
    errors.As(err, &httpErr) // true
    json := make(map[string]interace{})
    err := httpErr.Decode(json)
    raw := httpErr.Body()
    println(err, res, json, raw)
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

// URL parameters can be filled in via optional additional parameters
{
    res, err := client.POST(ctx, "/items/:item-id", nil, nil, fourten.Param("item-id", "123456"))
    println(err, res, json)
}
```

## Docs

TODO

## TODO

* handle chunked transfer encoding responses
* handle gzip server responses
* allow gzipping client requests
* ensure we handle connection errors properly
* configure connection pooling - http://tleyden.github.io/blog/2016/11/21/tuning-the-go-http-client-library-for-load-testing/
* url params
* Retries
* O11y
* fourten.DiscardBody
* Docs

## License

MIT

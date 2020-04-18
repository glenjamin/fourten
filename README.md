# Four Ten

> 410 Gone

An opinionated Go HTTP Client.

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
    res, err := client.GETDecoded(ctx, "/items", &json)
    println(err, res, json)
}

// HTTP Status codes are turned into errors
{
    res, err := client.GET(ctx, "/error") // 4xx, 5xx etc
    errors.Is(err, fourten.ErrHTTP) // true
}

// Refine the client's defaults as needed
refined := client.Refine(
    fourten.DontRetry,
    fourten.EncodeJSON,
)

// Make POST requests with request encoding and body decoding
{
    input = map[string]interface{}{"abc": "def"}
    output := make(map[string]interace{})
    res, err := refined.POSTDecoded(ctx, "/items", input, &output)
    println(err, res, output)
}

// Override options per request too if you want
{
    res, err := client.POST(ctx, "/items/one-shot", nil, fourten.DontRetry)
    println(err, res, json)
}

// URL parameters can be filled in too
{
    res, err := client.POST(ctx, "/items/:item-id", nil, fourten.Param("item-id", "123456"))
    println(err, res, json)
}
```

## Docs

TODO

## TODO

* Test all HTTP status codes
* HTTP Error response decoding
    * HTTPError.Decode() method
    * HTTPError.Body() method? (would require some read buffering if it's a decode fallback)
* per-request options
* url params
* Retries
* O11y
* Specify Redirect behaviour?
* Docs

## License

MIT

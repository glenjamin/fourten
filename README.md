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
    fourten.Observe(func(req fourten.Request) fourten.ResponseObserver {
        start := time.Now()
        return func(res http.Response, err error) {
            metrics.Timer(req.Name, time.Since(start), map[string]string{
                "error": String(err != nil),
                "status": res.StatusCode,
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

// Refine the client's defaults as needed
refined := client.Refine(
    fourten.DontRetry(),
    fourten.EncodeJSON,
    fourten.Named("refined"),
)

// Make POST requests with request encoding and body decoding
{
    input = map[string]interface{}{"abc": "def"}
    output := make(map[string]interace{})
    res, err := refined.POSTDecoded(ctx, "/items", input, &output)
    println(err, res, output)
}
```

## Docs

TODO

## TODO

* Timeouts (use context deadline)
* PUT/DELETE etc
* HTTP Errors
* Retries
* O11y
* Docs

## License

MIT

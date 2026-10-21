# Rayon Go SDK

Server-side Go client for Rayon header and form generation and account usage. Go 1.23+, standard library only. Keep your API key on your server.

[Documentation](https://docs.rayonapi.com/) · [Examples](https://github.com/Rayon-API/rayon-examples)

## Install

Unreleased local source. In your consuming Go module, point to this checkout:

```sh
go mod edit -require=github.com/Rayon-API/rayon-sdk-go@v0.0.0
go mod edit -replace=github.com/Rayon-API/rayon-sdk-go=/absolute/path/to/rayon-sdk-go
```

After repository/tag publication, install with `go get github.com/Rayon-API/rayon-sdk-go`.

## Check usage

```go
client, err := rayon.NewClient(os.Getenv("RAYON_API_KEY"))
if err != nil { log.Fatal(err) }
defer client.Close()
usage, err := client.Usage(context.Background())
if err != nil { log.Fatal(err) }
fmt.Println(usage)
```

Import the package as `rayon "github.com/Rayon-API/rayon-sdk-go"`.
Usage is read-only. Numeric usage values use `json.Number` to preserve precision.

## Generate

Prepare `shape.json` using complete raw protection JavaScript in execution order, its context URLs, a required `profile` object (`{}` uses API defaults), and the exact target request. Supply measured browser values and JavaScript-visible `document_cookie` when your target flow needs them. See [material preparation](https://docs.rayonapi.com/material) and [profiles and sessions](https://docs.rayonapi.com/profiles-and-sessions). URLs provide context; the SDK does not download scripts or send target requests.

```go
package main

import (
    "context"
    "encoding/json"
    "log"
    "os"
    "time"

    rayon "github.com/Rayon-API/rayon-sdk-go"
)

func main() {
    data, err := os.ReadFile("shape.json")
    if err != nil { log.Fatal(err) }
    var shape rayon.ShapeRequest
    if err := json.Unmarshal(data, &shape); err != nil { log.Fatal(err) }
    client, err := rayon.NewClient(os.Getenv("RAYON_API_KEY"))
    if err != nil { log.Fatal(err) }
    defer client.Close()
    ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
    defer cancel()
    result, err := client.Generate(ctx, shape)
    if err != nil { log.Fatal(err) }
    log.Println(result.RequestID)
    // Apply every result.Headers entry to the exact target request,
    // using your target client and existing session.
}
```

Supply exactly one of `scripts` or `material_ref`. All four `request` fields are required. GET/HEAD use `content_type: null` and `body: null`. In constructed Go requests, nil `ContentType` and `Body` pointers encode null; `rayon.String("")` preserves an empty body. Preserve whitespace and encoding. Validate supplied profiles offline with `rayon.ValidateProfile`.

Header results contain the six same-family `a`, `b`, `c`, `d`, `f`, `z` headers and may include `a0`. Preserve every name, case and value. Keep browser identity consistent with `ProfileUsed` and retain your session cookies. Results do not verify target acceptance or return target cookies.

For form telemetry, set `Mode: "form"`, POST with `application/x-www-form-urlencoded`, and supply the application body before telemetry insertion. `Form` optionally describes its ID/name. Submit `result.FormBody` followed by `&` and your original application body when nonempty. `FormBody` is telemetry only, already encoded once; preserve `FormFields` names and values. See [form mode](https://docs.rayonapi.com/form-payload-integration).

Reuse a client for material caching and connection pooling. See [client behavior](docs/reference.md) for limits, recovery, cancellation and error handling.

See [API reference](https://docs.rayonapi.com/api-reference) for detailed fields and limits, [SECURITY.md](SECURITY.md) for private reports and [RELEASE.md](RELEASE.md) for publishing.
